package internal

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Offline corpus for MUSICBRAINZ_FIXTURE=1 (laptop demo, _mvp smoke, tests).
// No network is used while fixture mode is active.
//
//go:embed testdata/artists.json
var fixtureArtistsJSON []byte

// Fixture serves a fixed set of artists and release groups from the embedded corpus.
type Fixture struct {
	artists []Artist
}

// NewFixture parses the embedded corpus.
func NewFixture() (*Fixture, error) {
	return newFixtureFromJSON(fixtureArtistsJSON)
}

func newFixtureFromJSON(data []byte) (*Fixture, error) {
	var artists []Artist
	if err := json.Unmarshal(data, &artists); err != nil {
		return nil, fmt.Errorf("parse fixture corpus: %w", err)
	}
	return &Fixture{artists: artists}, nil
}

// SearchArtists matches case-insensitively on name; an exact match scores 100,
// a prefix 90 and a substring 70.
func (f *Fixture) SearchArtists(ctx context.Context, query string, limit, offset int) (*SearchPage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, fmt.Errorf("%w: empty query", ErrInvalid)
	}
	var hits []Artist
	for _, a := range f.artists {
		name := strings.ToLower(a.Name)
		switch {
		case name == q:
			a.Score = 100
		case strings.HasPrefix(name, q):
			a.Score = 90
		case strings.Contains(name, q):
			a.Score = 70
		default:
			continue
		}
		a.ReleaseGroups = nil
		hits = append(hits, a)
	}
	// Stable insertion sort by score (corpus is tiny).
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Score > hits[j-1].Score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	total := len(hits)
	if offset > total {
		offset = total
	}
	hits = hits[offset:]
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return &SearchPage{Artists: hits, Count: total}, nil
}

func (f *Fixture) find(mbid string) *Artist {
	for i := range f.artists {
		if strings.EqualFold(f.artists[i].ID, mbid) {
			return &f.artists[i]
		}
	}
	return nil
}

// Artist returns the artist without release groups.
func (f *Fixture) Artist(ctx context.Context, mbid string) (*Artist, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a := f.find(mbid)
	if a == nil {
		return nil, fmt.Errorf("%w: artist %s", ErrNotFound, mbid)
	}
	cp := *a
	cp.ReleaseGroups = nil
	return &cp, nil
}

// ReleaseGroups returns a page of the artist's release groups.
func (f *Fixture) ReleaseGroups(ctx context.Context, mbid string, limit, offset int) (*ReleaseGroupPage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a := f.find(mbid)
	if a == nil {
		return &ReleaseGroupPage{}, nil // browse of an unknown artist is empty upstream
	}
	rgs := a.ReleaseGroups
	total := len(rgs)
	if offset > total {
		offset = total
	}
	rgs = rgs[offset:]
	if limit > 0 && len(rgs) > limit {
		rgs = rgs[:limit]
	}
	return &ReleaseGroupPage{ReleaseGroups: append([]ReleaseGroup(nil), rgs...), Count: total}, nil
}
