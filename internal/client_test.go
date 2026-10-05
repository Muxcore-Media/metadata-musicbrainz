package internal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc, interval time.Duration) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(ClientConfig{BaseURL: srv.URL + "/ws/2", UserAgent: "test-agent/1 ( test@example.invalid )", MinInterval: interval})
}

func TestClientSearchRequestShape(t *testing.T) {
	var gotUA, gotPath, gotQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotPath = r.Header.Get("User-Agent"), r.URL.Path
		gotQuery = r.URL.Query().Get("query") + "|" + r.URL.Query().Get("fmt") + "|" + r.URL.Query().Get("limit") + "|" + r.URL.Query().Get("offset")
		_, _ = w.Write([]byte(`{"created":"x","count":1,"offset":0,"artists":[{"id":"` + radioheadMBID + `","name":"Radiohead","sort-name":"Radiohead","score":100,"type":"Group","country":"GB","disambiguation":"English rock band"}]}`))
	}, -1)
	page, err := c.SearchArtists(context.Background(), "Radiohead", 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if gotUA != "test-agent/1 ( test@example.invalid )" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if gotPath != "/ws/2/artist" || gotQuery != "Radiohead|json|5|10" {
		t.Errorf("path=%q query=%q", gotPath, gotQuery)
	}
	if page.Count != 1 || page.Artists[0].Score != 100 || page.Artists[0].SortName != "Radiohead" {
		t.Errorf("decoded %+v", page)
	}
}

func TestClientLookupAndBrowse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/2/artist/" + radioheadMBID:
			_, _ = w.Write([]byte(`{"id":"` + radioheadMBID + `","name":"Radiohead","life-span":{"begin":"1985","ended":false}}`))
		case "/ws/2/release-group":
			if r.URL.Query().Get("artist") != radioheadMBID {
				http.Error(w, `{"error":"bad artist"}`, http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"release-group-count":1,"release-group-offset":0,"release-groups":[{"id":"rg1","title":"OK Computer","primary-type":"Album","secondary-types":[],"first-release-date":"1997-05-21"}]}`))
		default:
			http.NotFound(w, r)
		}
	}, -1)
	ctx := context.Background()
	a, err := c.Artist(ctx, radioheadMBID)
	if err != nil || a.LifeSpan.Begin != "1985" {
		t.Fatalf("artist: %+v %v", a, err)
	}
	p, err := c.ReleaseGroups(ctx, radioheadMBID, 100, 0)
	if err != nil || p.Count != 1 || p.ReleaseGroups[0].FirstReleaseDate != "1997-05-21" {
		t.Fatalf("release groups: %+v %v", p, err)
	}
}

func TestClientStatusMapping(t *testing.T) {
	cases := []struct {
		want   error
		status int
	}{
		{ErrNotFound, http.StatusNotFound},
		{ErrInvalid, http.StatusBadRequest},
		{ErrUnavailable, http.StatusServiceUnavailable},
		{ErrUnavailable, http.StatusTooManyRequests},
		{ErrUnavailable, http.StatusForbidden},
	}
	for _, tc := range cases {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
		}, -1)
		_, err := c.Artist(context.Background(), radioheadMBID)
		if !errors.Is(err, tc.want) {
			t.Errorf("HTTP %d: got %v, want %v", tc.status, err, tc.want)
		}
	}
}

func TestClientBadJSONIsUnavailable(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }, -1)
	if _, err := c.Artist(context.Background(), radioheadMBID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestClientRateLimitSpacesRequests(t *testing.T) {
	var n atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		_, _ = w.Write([]byte(`{"count":0,"artists":[]}`))
	}, 80*time.Millisecond)
	start := time.Now()
	for range 3 {
		if _, err := c.SearchArtists(context.Background(), "x", 1, 0); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 150*time.Millisecond {
		t.Fatalf("3 requests took %v, want >= 2 intervals", el)
	}
	if n.Load() != 3 {
		t.Fatalf("server saw %d requests", n.Load())
	}
}

func TestClientRateLimitHonorsContext(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }, time.Hour)
	if _, err := c.SearchArtists(context.Background(), "x", 1, 0); err != nil { // consumes the free slot
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.SearchArtists(ctx, "x", 1, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestDefaultUserAgentIdentifiesModule(t *testing.T) {
	if !strings.Contains(DefaultUserAgent, "metadata-musicbrainz/"+Version) || !strings.Contains(DefaultUserAgent, "github.com/Muxcore-Media") {
		t.Fatalf("DefaultUserAgent = %q", DefaultUserAgent)
	}
}
