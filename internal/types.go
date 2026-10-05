// Package internal implements the metadata-musicbrainz MuxCore module: a gRPC
// facade over the MusicBrainz WS/2 JSON API with an offline fixture mode.
package internal

import (
	"context"
	"errors"
)

// Sentinel errors returned by a Source; the gRPC layer maps them to status codes.
var (
	ErrNotFound    = errors.New("musicbrainz: not found")
	ErrInvalid     = errors.New("musicbrainz: invalid request")
	ErrUnavailable = errors.New("musicbrainz: upstream unavailable")
)

// Source is where artist data comes from: the live API or the fixture corpus.
type Source interface {
	// SearchArtists runs a free-text artist search.
	SearchArtists(ctx context.Context, query string, limit, offset int) (*SearchPage, error)
	// Artist looks up one artist by MBID (without release groups).
	Artist(ctx context.Context, mbid string) (*Artist, error)
	// ReleaseGroups browses release groups credited to the artist.
	ReleaseGroups(ctx context.Context, mbid string, limit, offset int) (*ReleaseGroupPage, error)
}

// SearchPage is one page of artist search results.
type SearchPage struct {
	Artists []Artist `json:"artists"`
	Count   int      `json:"count"`
}

// ReleaseGroupPage is one page of an artist's release groups.
type ReleaseGroupPage struct {
	ReleaseGroups []ReleaseGroup `json:"release-groups"`
	Count         int            `json:"release-group-count"`
}

// Artist mirrors the subset of MusicBrainz WS/2 artist JSON that is used.
type Artist struct {
	LifeSpan struct {
		Begin string `json:"begin"`
	} `json:"life-span"`
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	SortName       string         `json:"sort-name"`
	Disambiguation string         `json:"disambiguation"`
	Type           string         `json:"type"`
	Country        string         `json:"country"`
	ReleaseGroups  []ReleaseGroup `json:"release-groups,omitempty"`
	Score          int            `json:"score"`
}

// ReleaseGroup mirrors the subset of MusicBrainz WS/2 release-group JSON that is used.
type ReleaseGroup struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	PrimaryType      string   `json:"primary-type"`
	FirstReleaseDate string   `json:"first-release-date"`
	SecondaryTypes   []string `json:"secondary-types"`
}
