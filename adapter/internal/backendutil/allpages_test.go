package backendutil

import (
	"errors"
	"slices"
	"testing"
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
// report poster) spun forever. v0.77.0 capped the walk and failed loudly;
// v0.99.10 起改为无进展软收手（同页重复 = 列表已到尾）：首页即全量，报错
// 丢弃数据反而把调用方（poster 扫描）逼进新建降级、总结评论碎片化。
func TestAllPagesNoProgressWhenPageParamIgnored(t *testing.T) {
	var fetches int
	fetch := func(page int) ([]string, error) {
		if page == 0 {
			t.Fatal("pages are 1-based")
		}
		fetches++
		return []string{"x"}, nil // server that never advances pagination
	}
	got, err := AllPages(fetch)
	if err != nil {
		t.Fatalf("page-ignoring server 应软收手返回数据，got err %v", err)
	}
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("got = %v, want [x]（首页即全量）", got)
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d, want 2（首页 + 重复页判停）", fetches)
	}
}

func TestAllPagesCollectsUntilEmptyPage(t *testing.T) {
	fetch := func(page int) ([]string, error) {
		switch page {
		case 1:
			return []string{"a", "b"}, nil
		case 2:
			return []string{"c", "d"}, nil
		default:
			return []string{}, nil
		}
	}
	got, err := AllPages(fetch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 items across 2 non-empty pages, got %d", len(got))
	}
}
