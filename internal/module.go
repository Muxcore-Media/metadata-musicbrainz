package internal

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/Muxcore-Media/core/pkg/contracts"
	musicbrainzv1 "github.com/Muxcore-Media/metadata-musicbrainz/proto/musicbrainzv1"
)

const (
	// DefaultGRPCAddr is the module's reserved port in _mvp/PORTS.md.
	DefaultGRPCAddr = ":9413"

	defaultSearchLimit = 25
	maxSearchLimit     = 100
	browsePageSize     = 100
	maxReleaseGroups   = 500 // 5 browse pages; bounds latency at 1 req/s
	healthTTL          = time.Minute
)

var mbidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Config configures the module. Use ConfigFromEnv for process configuration.
type Config struct {
	Source    Source // overrides Fixture/BaseURL when set (tests)
	ID        string
	GRPCAddr  string
	BaseURL   string
	UserAgent string
	Version   string
	Timeout   time.Duration
	Fixture   bool
}

// ConfigFromEnv reads MUSICBRAINZ_FIXTURE, MUSICBRAINZ_BASE_URL,
// MUSICBRAINZ_USER_AGENT and METADATA_GRPC_ADDR.
func ConfigFromEnv() Config {
	cfg := Config{
		GRPCAddr:  os.Getenv("METADATA_GRPC_ADDR"),
		BaseURL:   os.Getenv("MUSICBRAINZ_BASE_URL"),
		UserAgent: os.Getenv("MUSICBRAINZ_USER_AGENT"),
	}
	v := strings.TrimSpace(os.Getenv("MUSICBRAINZ_FIXTURE"))
	if b, err := strconv.ParseBool(v); err == nil {
		cfg.Fixture = b
	}
	return cfg
}

// Module implements contracts.Module and the MusicBrainzService gRPC service.
type Module struct {
	musicbrainzv1.UnimplementedMusicBrainzServiceServer

	healthAt  time.Time
	healthErr error
	lis       net.Listener
	grpcSrv   *grpc.Server
	src       Source
	id        string
	grpcAddr  string
	version   string
	fixture   bool
	mu        sync.Mutex
}

// NewModule builds a module. It fails only if the embedded fixture is corrupt.
func NewModule(cfg Config) (*Module, error) {
	if cfg.ID == "" {
		cfg.ID = "metadata-musicbrainz"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = DefaultGRPCAddr
	}
	if cfg.Version == "" {
		cfg.Version = Version
	}
	src := cfg.Source
	if src == nil {
		if cfg.Fixture {
			f, err := NewFixture()
			if err != nil {
				return nil, err
			}
			src = f
		} else {
			src = NewClient(ClientConfig{BaseURL: cfg.BaseURL, UserAgent: cfg.UserAgent, Timeout: cfg.Timeout})
		}
	}
	return &Module{
		id:       cfg.ID,
		grpcAddr: cfg.GRPCAddr,
		version:  cfg.Version,
		src:      src,
		fixture:  cfg.Fixture,
	}, nil
}

// Addr returns the bound gRPC address after Init (useful when listening on :0).
func (m *Module) Addr() string {
	if m.lis == nil {
		return m.grpcAddr
	}
	return m.lis.Addr().String()
}

// Info implements contracts.Module.
func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:             m.id,
		Name:           "Metadata MusicBrainz",
		Version:        m.version,
		Roles:          []string{"metadata"},
		Description:    "MusicBrainz artist and release-group metadata provider for music libraries",
		Author:         "MuxCore",
		Capabilities:   []string{"metadata.music", "metadata.musicbrainz"},
		MinCoreVersion: "0.6.0",
		HTTPAddr:       m.grpcAddr,
	}
}

// Init implements contracts.Module.
func (m *Module) Init(ctx context.Context) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	slog.Info("metadata-musicbrainz initialized", "addr", lis.Addr().String(), "fixture", m.fixture)
	return nil
}

// Start implements contracts.Module. It serves TLS when MUXCORE_TLS_CERT and
// MUXCORE_TLS_KEY are set and plaintext otherwise (dev; see MUXCORE_INSECURE_DISABLE_TLS).
func (m *Module) Start(_ context.Context) error {
	var opts []grpc.ServerOption
	certFile, keyFile := os.Getenv("MUXCORE_TLS_CERT"), os.Getenv("MUXCORE_TLS_KEY")
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("load TLS cert/key: %w", err)
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})))
	} else {
		slog.Warn("metadata-musicbrainz serving plaintext gRPC; set MUXCORE_TLS_CERT/MUXCORE_TLS_KEY for production")
	}
	m.grpcSrv = grpc.NewServer(opts...)
	musicbrainzv1.RegisterMusicBrainzServiceServer(m.grpcSrv, m)
	srv, lis := m.grpcSrv, m.lis
	go func() {
		if err := srv.Serve(lis); err != nil {
			slog.Error("metadata-musicbrainz gRPC serve error", "error", err)
		}
	}()
	return nil
}

// Stop implements contracts.Module.
func (m *Module) Stop(_ context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("metadata-musicbrainz stopped")
	return nil
}

// Health implements contracts.Module. Fixture mode is always healthy; live mode
// probes the API at most once per minute so health checks do not consume the
// 1 req/s budget.
func (m *Module) Health(ctx context.Context) error {
	if m.fixture {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.healthAt.IsZero() && time.Since(m.healthAt) < healthTTL {
		return m.healthErr
	}
	pctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	_, err := m.src.SearchArtists(pctx, "health", 1, 0)
	if err != nil {
		err = fmt.Errorf("musicbrainz health check failed: %w", err)
	}
	m.healthAt, m.healthErr = time.Now(), err
	return err
}

// SearchArtist implements MusicBrainzServiceServer.
func (m *Module) SearchArtist(ctx context.Context, req *musicbrainzv1.SearchArtistRequest) (*musicbrainzv1.SearchArtistResponse, error) {
	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, status.Error(codes.InvalidArgument, "query required")
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	limit = min(limit, maxSearchLimit)
	offset := max(int(req.GetOffset()), 0)

	page, err := m.src.SearchArtists(ctx, query, limit, offset)
	if err != nil {
		return nil, grpcError(err)
	}
	out := &musicbrainzv1.SearchArtistResponse{
		Artists: make([]*musicbrainzv1.ArtistSearchResult, 0, len(page.Artists)),
		Total:   int32(min(page.Count, 1<<31-1)), //nolint:gosec // clamped to int32 range
	}
	for i := range page.Artists {
		a := &page.Artists[i]
		out.Artists = append(out.Artists, &musicbrainzv1.ArtistSearchResult{
			Id:             a.ID,
			Name:           a.Name,
			Score:          int32(min(max(a.Score, 0), 100)), //nolint:gosec // clamped to 0..100
			SortName:       a.SortName,
			Disambiguation: a.Disambiguation,
			Type:           a.Type,
			Country:        a.Country,
		})
	}
	return out, nil
}

// GetArtist implements MusicBrainzServiceServer.
func (m *Module) GetArtist(ctx context.Context, req *musicbrainzv1.GetArtistRequest) (*musicbrainzv1.GetArtistResponse, error) {
	mbid := strings.TrimSpace(req.GetMusicbrainzId())
	if mbid == "" {
		return nil, status.Error(codes.InvalidArgument, "musicbrainz_id required")
	}
	if !mbidRE.MatchString(mbid) {
		return nil, status.Error(codes.InvalidArgument, "musicbrainz_id must be a UUID")
	}
	a, err := m.src.Artist(ctx, mbid)
	if err != nil {
		return nil, grpcError(err)
	}
	rgs, err := m.allReleaseGroups(ctx, mbid)
	if err != nil {
		return nil, grpcError(err)
	}
	out := &musicbrainzv1.GetArtistResponse{
		Id:             a.ID,
		Name:           a.Name,
		Disambiguation: a.Disambiguation,
		SortName:       a.SortName,
		Type:           a.Type,
		Country:        a.Country,
		BeginYear:      yearOf(a.LifeSpan.Begin),
		ReleaseGroups:  make([]*musicbrainzv1.ReleaseGroup, 0, len(rgs)),
	}
	for i := range rgs {
		rg := &rgs[i]
		out.ReleaseGroups = append(out.ReleaseGroups, &musicbrainzv1.ReleaseGroup{
			Id:                   rg.ID,
			Title:                rg.Title,
			PrimaryType:          rg.PrimaryType,
			FirstReleaseDateYear: yearOf(rg.FirstReleaseDate),
			SecondaryTypes:       rg.SecondaryTypes,
			FirstReleaseDate:     rg.FirstReleaseDate,
		})
	}
	return out, nil
}

func (m *Module) allReleaseGroups(ctx context.Context, mbid string) ([]ReleaseGroup, error) {
	var all []ReleaseGroup
	for offset := 0; offset < maxReleaseGroups; {
		page, err := m.src.ReleaseGroups(ctx, mbid, browsePageSize, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, page.ReleaseGroups...)
		offset += browsePageSize
		if len(page.ReleaseGroups) == 0 || offset >= page.Count {
			break
		}
	}
	return all, nil
}

// yearOf parses the leading four-digit year of a MusicBrainz partial date.
func yearOf(date string) int32 {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil || y <= 0 {
		return 0
	}
	return int32(y) //nolint:gosec // four digits always fit int32
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
