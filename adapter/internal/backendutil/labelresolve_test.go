package backendutil

import (
	"errors"
	"testing"
	"time"
)

func TestResolveLabelIDFindsOnSecondPage(t *testing.T) {
	pages := map[int][]LabelRef{
		1: {{ID: 1, Name: "a"}, {ID: 2, Name: "b"}},
		2: {{ID: 3, Name: "target"}},
	}
	id, err := ResolveLabelID(func(page, perPage int) ([]LabelRef, error) {
		return pages[page], nil
	}, "target", 5, 2)
	if err != nil || id != 3 {
		t.Fatalf("ResolveLabelID = (%d, %v), want (3, nil)", id, err)
	}
}

func TestResolveLabelIDStopsAtShortPage(t *testing.T) {
	calls := 0
	_, err := ResolveLabelID(func(page, perPage int) ([]LabelRef, error) {
		calls++
		return []LabelRef{{ID: 1, Name: "a"}}, nil // short page: no more pages
	}, "missing", 50, 100)
	if !errors.Is(err, ErrLabelScanLimit) {
		t.Fatalf("err = %v, want ErrLabelScanLimit", err)
	}
	if calls != 1 {
		t.Fatalf("scan calls = %d, want 1 (short page must stop the scan)", calls)
	}
}

func TestResolveLabelIDScanLimitDistinctFromShortPage(t *testing.T) {
	_, err := ResolveLabelID(func(page, perPage int) ([]LabelRef, error) {
		return []LabelRef{{ID: int64(page), Name: "other"}}, nil // always full page? no: 1 < 100
	}, "missing", 50, 100)
	// 1 result < perPage 100 → short page → stop → scan limit error, 1 call.
	if !errors.Is(err, ErrLabelScanLimit) {
		t.Fatalf("err = %v, want ErrLabelScanLimit", err)
	}
}

func TestIDCacheResolveLabelCaches(t *testing.T) {
	c := NewIDCache(time.Minute)
	scans := 0
	scan := func(page, perPage int) ([]LabelRef, error) {
		scans++
		return []LabelRef{{ID: 7, Name: "x"}}, nil
	}
	for i := 0; i < 3; i++ {
		id, err := c.ResolveLabel("owner/repo", "x", scan, 50, 100)
		if err != nil || id != 7 {
			t.Fatalf("ResolveLabel #%d = (%d, %v)", i, id, err)
		}
	}
	if scans != 1 {
		t.Fatalf("scans = %d, want 1 (second and third resolve must hit the cache)", scans)
	}
}

func TestNewScanLimitErrorPreservesSentinel(t *testing.T) {
	const msg = `label "missing" not found within 50 pages (scan limit)`
	err := NewScanLimitError(msg)
	if !errors.Is(err, ErrLabelScanLimit) {
		t.Fatalf("errors.Is(err, ErrLabelScanLimit) = false for %v, want true", err)
	}
	if err.Error() != msg {
		t.Fatalf("err.Error() = %q, want %q", err.Error(), msg)
	}
}

func TestResolveLabelIDsBulk(t *testing.T) {
	pages := map[int][]LabelRef{
		1: {{ID: 1, Name: "bug"}, {ID: 2, Name: "feature"}, {ID: 3, Name: "docs"}},
	}
	result, err := ResolveLabelIDs(func(page, perPage int) ([]LabelRef, error) {
		return pages[page], nil
	}, []string{"bug", "feature"}, 5, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result["bug"] != 1 {
		t.Errorf("bug = %d, want 1", result["bug"])
	}
	if result["feature"] != 2 {
		t.Errorf("feature = %d, want 2", result["feature"])
	}
}

func TestResolveLabelIDsBulkPartialMiss(t *testing.T) {
	scanCalls := 0
	result, err := ResolveLabelIDs(func(page, perPage int) ([]LabelRef, error) {
		scanCalls++
		if page == 1 {
			return []LabelRef{{ID: 1, Name: "bug"}}, nil
		}
		return nil, nil
	}, []string{"bug", "missing"}, 5, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result["bug"] != 1 {
		t.Errorf("bug = %d, want 1", result["bug"])
	}
	// Should stop after first page (short page)
	if scanCalls != 1 {
		t.Errorf("scan calls = %d, want 1", scanCalls)
	}
}

func TestResolveLabelIDsWithCacheBulk(t *testing.T) {
	c := NewIDCache(time.Minute)
	// Pre-populate cache for "bug"
	c.Put("owner/repo/bug", 42)

	scanCalls := 0
	result, err := c.ResolveLabelIDsWithCache("owner/repo", []string{"bug", "feature"}, func(page, perPage int) ([]LabelRef, error) {
		scanCalls++
		return []LabelRef{{ID: 99, Name: "feature"}}, nil
	}, 5, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// bug should come from cache
	if result["bug"] != 42 {
		t.Errorf("bug = %d, want 42 (from cache)", result["bug"])
	}
	// feature should come from scan
	if result["feature"] != 99 {
		t.Errorf("feature = %d, want 99 (from scan)", result["feature"])
	}
	// Scan should only be called for the miss
	if scanCalls != 1 {
		t.Errorf("scan calls = %d, want 1 (only for cache miss)", scanCalls)
	}
}
