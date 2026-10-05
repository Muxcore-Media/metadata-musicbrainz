package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	musicbrainzv1 "github.com/Muxcore-Media/metadata-musicbrainz/proto/musicbrainzv1"
)

func startModule(t *testing.T, cfg Config) musicbrainzv1.MusicBrainzServiceClient {
	t.Helper()
	cfg.GRPCAddr = "127.0.0.1:0"
	m, err := NewModule(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
	conn, err := grpc.NewClient(m.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return musicbrainzv1.NewMusicBrainzServiceClient(conn)
}

func TestInfo(t *testing.T) {
	m, err := NewModule(Config{Fixture: true})
	if err != nil {
		t.Fatal(err)
	}
	info := m.Info()
	if info.ID != "metadata-musicbrainz" || info.Version != Version || info.HTTPAddr != DefaultGRPCAddr {
		t.Fatalf("%+v", info)
	}
	if !strings.Contains(strings.Join(info.Capabilities, ","), "metadata.music") {
		t.Fatalf("capabilities %v", info.Capabilities)
	}
	if err := m.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("MUSICBRAINZ_FIXTURE", "1")
	t.Setenv("MUSICBRAINZ_BASE_URL", "http://example.invalid/ws/2")
	t.Setenv("MUSICBRAINZ_USER_AGENT", "ua/1")
	t.Setenv("METADATA_GRPC_ADDR", ":1234")
	cfg := ConfigFromEnv()
	if !cfg.Fixture || cfg.BaseURL != "http://example.invalid/ws/2" || cfg.UserAgent != "ua/1" || cfg.GRPCAddr != ":1234" {
		t.Fatalf("%+v", cfg)
	}
}

func TestGRPCFixtureEndToEnd(t *testing.T) {
	cli := startModule(t, Config{Fixture: true})
	ctx := context.Background()

	sr, err := cli.SearchArtist(ctx, &musicbrainzv1.SearchArtistRequest{Query: "Radiohead", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(sr.GetArtists()) != 1 || sr.GetArtists()[0].GetId() != radioheadMBID || sr.GetArtists()[0].GetScore() != 100 || sr.GetTotal() != 1 {
		t.Fatalf("search: %v", sr)
	}

	ar, err := cli.GetArtist(ctx, &musicbrainzv1.GetArtistRequest{MusicbrainzId: radioheadMBID})
	if err != nil {
		t.Fatal(err)
	}
	if ar.GetName() != "Radiohead" || ar.GetDisambiguation() != "English rock band" || ar.GetBeginYear() != 1985 || ar.GetCountry() != "GB" {
		t.Fatalf("artist: %v", ar)
	}
	var okc *musicbrainzv1.ReleaseGroup
	for _, rg := range ar.GetReleaseGroups() {
		if rg.GetTitle() == "OK Computer" {
			okc = rg
		}
	}
	if okc == nil || okc.GetPrimaryType() != "Album" || okc.GetFirstReleaseDateYear() != 1997 || okc.GetFirstReleaseDate() != "1997-05-21" || okc.GetId() == "" {
		t.Fatalf("OK Computer missing or wrong: %v", okc)
	}
}

func TestGRPCErrorCodes(t *testing.T) {
	cli := startModule(t, Config{Fixture: true})
	ctx := context.Background()
	_, err := cli.SearchArtist(ctx, &musicbrainzv1.SearchArtistRequest{Query: "  "})
	wantCode(t, err, codes.InvalidArgument)
	_, err = cli.GetArtist(ctx, &musicbrainzv1.GetArtistRequest{})
	wantCode(t, err, codes.InvalidArgument)
	_, err = cli.GetArtist(ctx, &musicbrainzv1.GetArtistRequest{MusicbrainzId: "../etc/passwd"})
	wantCode(t, err, codes.InvalidArgument)
	_, err = cli.GetArtist(ctx, &musicbrainzv1.GetArtistRequest{MusicbrainzId: "00000000-0000-4000-8000-00000000ffff"})
	wantCode(t, err, codes.NotFound)
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v (%v), want %v", got, err, want)
	}
}

// TestGRPCLiveClientAgainstHTTPTest drives the full stack (gRPC -> Client -> HTTP)
// against a local httptest server; nothing touches the real MusicBrainz.
func TestGRPCLiveClientAgainstHTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/2/artist":
			_, _ = w.Write([]byte(`{"count":2,"artists":[{"id":"` + radioheadMBID + `","name":"Radiohead","score":100},{"id":"x","name":"Radiohead Tribute","score":60}]}`))
		case "/ws/2/artist/" + radioheadMBID:
			_, _ = w.Write([]byte(`{"id":"` + radioheadMBID + `","name":"Radiohead","disambiguation":"English rock band","life-span":{"begin":"1985"}}`))
		case "/ws/2/release-group":
			// Two pages of 100 then stop: count says 150.
			off := r.URL.Query().Get("offset")
			n := 100
			if off == "100" {
				n = 50
			}
			var b strings.Builder
			b.WriteString(`{"release-group-count":150,"release-groups":[`)
			for i := range n {
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(`{"id":"rg","title":"T","primary-type":"Album","first-release-date":"1997"}`)
			}
			b.WriteString(`]}`)
			_, _ = w.Write([]byte(b.String()))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	// Build the module around a client with limiting disabled.
	src := NewClient(ClientConfig{BaseURL: srv.URL + "/ws/2", MinInterval: -1})
	cli := startModule(t, Config{Source: src})
	ctx := context.Background()

	sr, err := cli.SearchArtist(ctx, &musicbrainzv1.SearchArtistRequest{Query: "Radiohead"})
	if err != nil || len(sr.GetArtists()) != 2 || sr.GetTotal() != 2 {
		t.Fatalf("%v %v", sr, err)
	}
	ar, err := cli.GetArtist(ctx, &musicbrainzv1.GetArtistRequest{MusicbrainzId: radioheadMBID})
	if err != nil {
		t.Fatal(err)
	}
	if len(ar.GetReleaseGroups()) != 150 || ar.GetReleaseGroups()[0].GetFirstReleaseDateYear() != 1997 {
		t.Fatalf("release groups = %d", len(ar.GetReleaseGroups()))
	}
}

func TestUpstreamFailureMapsToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	cli := startModule(t, Config{Source: NewClient(ClientConfig{BaseURL: srv.URL, MinInterval: -1})})
	_, err := cli.SearchArtist(context.Background(), &musicbrainzv1.SearchArtistRequest{Query: "x"})
	wantCode(t, err, codes.Unavailable)
}

func TestLiveHealthIsCached(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"count":0,"artists":[]}`))
	}))
	t.Cleanup(srv.Close)
	m, err := NewModule(Config{Source: NewClient(ClientConfig{BaseURL: srv.URL, MinInterval: -1})})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := m.Health(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("upstream health probes = %d, want 1", calls)
	}
}

func TestYearOf(t *testing.T) {
	for in, want := range map[string]int32{"": 0, "x": 0, "1997": 1997, "1997-05": 1997, "1997-05-21": 1997, "abcd": 0, "0000": 0} {
		if got := yearOf(in); got != want {
			t.Errorf("yearOf(%q) = %d, want %d", in, got, want)
		}
	}
}
