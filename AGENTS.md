# AGENTS.md — metadata-musicbrainz

MuxCore sidecar module (`metadata-musicbrainz`). Workspace rules: [`../AGENTS.md`](../AGENTS.md). Default ports: [`_mvp/PORTS.md`](../_mvp/PORTS.md) (this module: `:9413`).

## Module identity

| Field | Value |
|-------|-------|
| Directory | `metadata-musicbrainz` |
| Capabilities | `metadata`, `metadata.musicbrainz` (see `muxcore.json`) |
| Service | `MusicBrainzService` (own proto, not a shared contract) |

## Agent rules

- GitHub (`github.com/Muxcore-Media`) is the only origin, tracker and CI host.
- No live-network tests: use the embedded fixture (`MUSICBRAINZ_FIXTURE=1` / `Config{Fixture: true}`) or `httptest`. Fixture mode must never touch the network.
- Keep the MusicBrainz conventions: descriptive User-Agent, `fmt=json`, at most 1 request/s, 429/5xx mapped to `Unavailable`.
- TLS required in production (`MUXCORE_INSECURE_DISABLE_TLS` is dev-only).
- `go.mod` requires published tags only; no `replace` directives.

## Verification

```bash
go build ./... && go vet ./...
go test -race -count=1 ./...
golangci-lint run ./...
gofmt -l .        # must print nothing
```

## Proto regeneration

Generated code in `proto/musicbrainzv1/` must be reproducible with protoc 25.3, protoc-gen-go v1.36.6 and protoc-gen-go-grpc v1.5.1:

```bash
export GOBIN=$(mktemp -d)
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
PATH=$GOBIN:$PATH make proto
git diff --exit-code proto/
```
