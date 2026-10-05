# Changelog

All notable changes to this module are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.1] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.0] - 2026-10-05

### Added
- Rebuilt module: `MusicBrainzService` gRPC API (`SearchArtist`, `GetArtist`) over the MusicBrainz WS/2 JSON API.
- Registers with core via the Go SDK and advertises `metadata.music` and `metadata.musicbrainz`.
- Live client with required User-Agent, `fmt=json`, 1 request/s rate limiting and 429/5xx (incl. 503) mapped to `Unavailable`.
- Offline fixture mode (`MUSICBRAINZ_FIXTURE=1`) backed by an embedded corpus; no network access.
- Reproducible generated protobuf code, Makefile, Dockerfile, golangci-lint config and GitHub Actions CI.
- `muxcore.json`, README and AGENTS notes.

### Changed
- Dependencies resolve from published GitHub tags (core v0.6.2; no filesystem `replace`).
