package internal

import (
	"context"
	"errors"
	"testing"
)

const radioheadMBID = "a74b1b7f-71a5-4011-9441-d0b5e4122711"

func TestFixtureCorpusParses(t *testing.T) {
	f, err := NewFixture()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.artists) < 3 {
		t.Fatalf("want >=3 artists, got %d", len(f.artists))
	}
	seen := map[string]bool{}
	for _, a := range f.artists {
		if !mbidRE.MatchString(a.ID) || a.Name == "" || len(a.ReleaseGroups) == 0 {
			t.Errorf("bad artist %+v", a)
		}
		for _, rg := range a.ReleaseGroups {
			if !mbidRE.MatchString(rg.ID) || rg.Title == "" || seen[rg.ID] {
				t.Errorf("bad or duplicate release group %+v", rg)
			}
			seen[rg.ID] = true
		}
	}
}

func TestFixtureSearch(t *testing.T) {
	f, _ := NewFixture()
	ctx := context.Background()

	page, err := f.SearchArtists(ctx, "  radiohead ", 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Artists) != 1 || page.Artists[0].ID != radioheadMBID || page.Artists[0].Score != 100 {
		t.Fatalf("unexpected: %+v", page)
	}
	if len(page.Artists[0].ReleaseGroups) != 0 {
		t.Fatal("search results must not carry release groups")
	}

	page, err = f.SearchArtists(ctx, "p", 10, 0) // substring/prefix matches, ordered by score
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Artists) < 2 {
		t.Fatalf("want multiple partial matches, got %d", len(page.Artists))
	}
	for i := 1; i < len(page.Artists); i++ {
		if page.Artists[i].Score > page.Artists[i-1].Score {
			t.Fatal("results not sorted by score")
		}
	}

	page, _ = f.SearchArtists(ctx, "p", 1, 0)
	if len(page.Artists) != 1 || page.Count < 2 {
		t.Fatalf("limit not applied: %+v", page)
	}
	page, _ = f.SearchArtists(ctx, "nobody-like-this", 5, 0)
	if len(page.Artists) != 0 || page.Count != 0 {
		t.Fatalf("expected empty, got %+v", page)
	}
	if _, err := f.SearchArtists(ctx, " ", 5, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestFixtureLookup(t *testing.T) {
	f, _ := NewFixture()
	ctx := context.Background()
	a, err := f.Artist(ctx, radioheadMBID)
	if err != nil || a.Name != "Radiohead" {
		t.Fatalf("got %+v, %v", a, err)
	}
	if _, err := f.Artist(ctx, "00000000-0000-4000-8000-00000000ffff"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	p, err := f.ReleaseGroups(ctx, radioheadMBID, 2, 1)
	if err != nil || len(p.ReleaseGroups) != 2 || p.Count < 3 {
		t.Fatalf("got %+v, %v", p, err)
	}
}
