package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the MusicBrainz WS/2 root.
	DefaultBaseURL = "https://musicbrainz.org/ws/2"
	// DefaultTimeout bounds each upstream request.
	DefaultTimeout = 15 * time.Second
	maxBodyBytes   = 8 << 20
)

// DefaultUserAgent identifies this module; MusicBrainz asks for a contact URL
// or address, so operators should override it via MUSICBRAINZ_USER_AGENT.
var DefaultUserAgent = "MuxCore-metadata-musicbrainz/" + Version + " ( https://github.com/Muxcore-Media/metadata-musicbrainz )"

// ClientConfig configures a live MusicBrainz client.
type ClientConfig struct {
	HTTPClient  *http.Client
	BaseURL     string
	UserAgent   string
	Timeout     time.Duration
	MinInterval time.Duration // zero means 1s; negative disables limiting (tests)
}

// Client is a MusicBrainz WS/2 JSON client. It sends a descriptive User-Agent
// and never issues more than one request per MinInterval.
type Client struct {
	hc      *http.Client
	lim     *limiter
	baseURL string
	ua      string
}

// NewClient builds a Client, filling defaults for unset fields.
func NewClient(cfg ClientConfig) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.MinInterval == 0 {
		cfg.MinInterval = time.Second
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{
		hc:      hc,
		lim:     newLimiter(cfg.MinInterval),
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		ua:      cfg.UserAgent,
	}
}

// SearchArtists implements Source.
func (c *Client) SearchArtists(ctx context.Context, query string, limit, offset int) (*SearchPage, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	var out SearchPage
	if err := c.getJSON(ctx, "/artist", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Artist implements Source.
func (c *Client) Artist(ctx context.Context, mbid string) (*Artist, error) {
	var out Artist
	if err := c.getJSON(ctx, "/artist/"+url.PathEscape(mbid), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReleaseGroups implements Source (browse request; lookups cap sub-entities at 25).
func (c *Client) ReleaseGroups(ctx context.Context, mbid string, limit, offset int) (*ReleaseGroupPage, error) {
	q := url.Values{}
	q.Set("artist", mbid)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	var out ReleaseGroupPage
	if err := c.getJSON(ctx, "/release-group", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	q.Set("fmt", "json")
	if err := c.lim.wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: read body: %w", ErrUnavailable, err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	case resp.StatusCode == http.StatusBadRequest:
		return fmt.Errorf("%w: %s", ErrInvalid, upstreamMessage(body))
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: decode %s: %w", ErrUnavailable, path, err)
	}
	return nil
}

func upstreamMessage(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return e.Error
	}
	return "bad request"
}
