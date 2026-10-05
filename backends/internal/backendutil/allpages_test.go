package backendutil

import (
	"errors"
	"slices"

	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestAllPagesMergesUntilEmpty verifies the loop advances pages and stops
// on the first empty page, merging everything seen along the way. Stopping
// on empty (not on "short page") keeps the result complete even when the
// server caps the page size below the requested per-page value.
func TestAllPagesMergesUntilEmpty(t *testing.T) {
	pages := [][]int{{1, 2, 3}, {4}, {}}
	var fetched []int
	got, err := AllPages(func(page int) ([]int, error) {
		if page > len(pages) {
			t.Fatalf("fetched past the empty terminating page: %d", page)
		}
		fetched = append(fetched, page)
		return pages[page-1], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4}
	if !slices.Equal(got, want) {
		t.Errorf("AllPages = %v, want %v", got, want)
	}
	if !slices.Equal(fetched, []int{1, 2, 3}) {
		t.Errorf("fetched pages %v, want [1 2 3] (stop on the empty page)", fetched)
	}
}

// TestAllPagesSingleEmptyPage verifies an empty first page yields an empty
// result without a second fetch.
func TestAllPagesSingleEmptyPage(t *testing.T) {
	fetches := 0
	got, err := AllPages(func(page int) ([]string, error) {
		fetches++
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || fetches != 1 {
		t.Errorf("got %d items in %d fetches, want 0 items in 1 fetch", len(got), fetches)
	}
}

// TestAllPagesPropagatesErrors verifies a failing fetch aborts the loop
// with the error surfaced.
func TestAllPagesPropagatesErrors(t *testing.T) {
	want := errors.New("boom")
	_, err := AllPages(func(page int) ([]int, error) {
		if page == 2 {
			return nil, want
		}
		return []int{1}, nil
	})
	if !errors.Is(err, want) {
		t.Errorf("expected the fetch error to surface, got %v", err)
	}
}

// Regression: a Forgejo 15.0.1 instance ignored the page parameter on the
// issue-comment list endpoint and returned the identical full page for every
// page number, so "stop on first empty page" never fired and callers (argus's
// report poster) spun forever. AllPages must cap itself instead of hanging.
func TestAllPagesStopsAtCapWhenPageParamIgnored(t *testing.T) {
	item := "x"
	fetch := func(page int) ([]string, error) {
		if page == 0 {
			t.Fatal("pages are 1-based")
		}
		return []string{item}, nil // server that never advances pagination
	}
	// Since v0.77.0 AllPages delegates to the provider iterator and a budget
	// overrun is an error wrapping provider.ErrPageBudgetExceeded — silent
	// truncation is gone, so a platform pagination bug fails loudly.
	_, err := AllPages(fetch)
	if !errors.Is(err, provider.ErrPageBudgetExceeded) {
		t.Fatalf("expected ErrPageBudgetExceeded from a page-ignoring server, got %v", err)
	}
}

func TestAllPagesCollectsUntilEmptyPage(t *testing.T) {
	fetch := func(page int) ([]string, error) {
		if page < 3 {
			return []string{"a", "b"}, nil
		}
		return []string{}, nil
	}
	got, err := AllPages(fetch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 items across 2 non-empty pages, got %d", len(got))
	}
}
