# Metadata MusicBrainz

MusicBrainz artist and release-group metadata provider for MuxCore music libraries. It is a gRPC sidecar that talks to the MusicBrainz WS/2 JSON API (or an embedded offline corpus) and registers with core through the Go SDK (`github.com/Muxcore-Media/core/sdk/go/module`). `media-music` discovers it with `Discovery.FindByCapability(ctx, "metadata.musicbrainz")`.

## Capability

`metadata`, `metadata.musicbrainz`

## RPCs

`MusicBrainzService` (`proto/muxcore/musicbrainz/v1/musicbrainz.proto`, Go package `proto/musicbrainzv1`):

| RPC | Description |
|-----|-------------|
| `SearchArtist` | Free-text artist search (`query`, `limit` default 25 / max 100, `offset`); returns MBID, name, score, sort name, disambiguation, type, country |
| `GetArtist` | Artist lookup by MBID (UUID) including release groups (browsed, up to 500) |

Errors map to gRPC codes: `InvalidArgument` (empty query, non-UUID MBID), `NotFound`, `Unavailable` (HTTP 429/5xx, including 503, or network failure), `DeadlineExceeded`/`Canceled`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `MUSICBRAINZ_FIXTURE` | unset | `1`/`true` serves the embedded corpus; the network is never touched |
| `MUSICBRAINZ_BASE_URL` | `https://musicbrainz.org/ws/2` | WS/2 root (point at a mirror or a test server) |
| `MUSICBRAINZ_USER_AGENT` | `MuxCore-metadata-musicbrainz/<version> ( https://github.com/Muxcore-Media/metadata-musicbrainz )` | MusicBrainz requires a descriptive User-Agent; set a contact address in production |
| `METADATA_GRPC_ADDR` | `:9413` | gRPC listen address |
| `MUXCORE_GRPC_ADDR` | - | Core mesh address used by the SDK to register |
| `MUXCORE_MODULE_ID` | `metadata-musicbrainz` | Module ID override (SDK) |
| `MUXCORE_TLS_CERT` / `MUXCORE_TLS_KEY` | unset | Serve TLS; plaintext with a warning otherwise |
| `MUXCORE_INSECURE_DISABLE_TLS` / `MUXCORE_GRPC_INSECURE` | unset | `true` for plaintext gRPC to core (dev only) |

Live requests always send `fmt=json`, the User-Agent above, and are limited to one request per second (MusicBrainz policy). Health checks in live mode probe upstream at most once per minute.

## Fixture mode

With `MUSICBRAINZ_FIXTURE=1` the module serves a small embedded corpus (`internal/testdata/artists.json`) with no network access. Metadata and acquisition paths are fixture-only in CI ([ADR-0008](../docs/adr/0008-fixture-only-acquisition-gate.md)); all tests use the fixture or a local `httptest` server.

## Build and test

```bash
make build
make test      # go test -race -count=1 ./...
make lint      # golangci-lint
make proto     # needs protoc 25.3, protoc-gen-go v1.36.6, protoc-gen-go-grpc v1.5.1
```

## Dependencies

- `github.com/Muxcore-Media/core` — MuxCore SDK and contracts (published tags; no `replace`)
