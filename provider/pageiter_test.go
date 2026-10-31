package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestEachWalksAllPages(t *testing.T) {
	var pages []int
	var items []int
	err := Each(context.Background(), func(_ context.Context, page int) ([]int, error) {
		pages = append(pages, page)
		if page == 3 {
			return nil, nil // empty page terminates
		}
		return []int{page*10 + 1, page*10 + 2}, nil
	}, func(item int) error {
		items = append(items, item)
		return nil
	})
	if err != nil {
		t.Fatalf("Each: %v", err)
	}
	if got, want := pages, []int{1, 2, 3}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("fetched pages = %v, want %v", got, want)
	}
	if len(items) != 4 || items[0] != 11 || items[3] != 22 {
		t.Fatalf("items = %v, want [11 12 21 22]", items)
	}
}

func TestEachStopIteration(t *testing.T) {
	var seen []int
	err := Each(context.Background(), func(_ context.Context, page int) ([]int, error) {
		return []int{page}, nil // never empty on its own
	}, func(item int) error {
		seen = append(seen, item)
		if item == 2 {
			return ErrStopIteration
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Each with ErrStopIteration: %v", err)
	}
	if got, want := seen, []int{1, 2}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("seen = %v, want %v", got, want)
	}
}

func TestEachPropagatesCallbackError(t *testing.T) {
	boom := errors.New("boom")
	err := Each(context.Background(), func(_ context.Context, _ int) ([]int, error) {
		return []int{1}, nil
	}, func(int) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestEachPropagatesFetchError(t *testing.T) {
	boom := errors.New("fetch boom")
	err := Each(context.Background(), func(_ context.Context, _ int) ([]int, error) {
		return nil, boom
	}, func(int) error { return nil })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

// 伪翻页无进展检测（Forgejo 15 实测形态：忽略 page 参数、每页返回同一批
// 全量数据）：同页重复即按到尾收手，已收集数据保留、不报错不重复。
func TestEachBoundedNoProgressStops(t *testing.T) {
	var fetches int
	var items []int
	err := EachBounded(context.Background(), func(_ context.Context, _ int) ([]int, error) {
		fetches++
		return []int{1, 2}, nil // platform ignores the page parameter
	}, 50, func(item int) error {
		items = append(items, item)
		return nil
	})
	if err != nil {
		t.Fatalf("no-progress 应软收手不报错: %v", err)
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d, want 2（首页 + 重复页判停）", fetches)
	}
	if len(items) != 2 || items[0] != 1 || items[1] != 2 {
		t.Fatalf("items = %v, want [1 2]（重复页不重复消费）", items)
	}
}

// Collect 同口径：伪翻页返回首页数据（即全量），nil error。
func TestCollectNoProgressReturnsData(t *testing.T) {
	got, err := CollectBounded(context.Background(), func(_ context.Context, _ int) ([]int, error) {
		return []int{7, 8}, nil
	}, 5)
	if err != nil {
		t.Fatalf("CollectBounded: %v", err)
	}
	if len(got) != 2 || got[0] != 7 || got[1] != 8 {
		t.Fatalf("got = %v, want [7 8]", got)
	}
}

// 真超长列表（每页不同）仍走预算哨兵——无进展检测不得吞掉真实越界。
func TestEachBoundedBudgetExceeded(t *testing.T) {
	err := EachBounded(context.Background(), func(_ context.Context, page int) ([]int, error) {
		return []int{page}, nil // 页页不同 = 真在推进
	}, 3, func(int) error { return nil })
	if !errors.Is(err, ErrPageBudgetExceeded) {
		t.Fatalf("err = %v, want ErrPageBudgetExceeded", err)
	}
}

func TestEachHonorsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Each(ctx, func(_ context.Context, _ int) ([]int, error) {
		return []int{1}, nil
	}, func(int) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestEachHonorsMidIterationCancel(t *testing.T) {
	// Cancellation raised inside a page fetch must take effect on the next
	// loop pass instead of grinding through the remaining pages.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls int
	err := EachBounded(ctx, func(_ context.Context, _ int) ([]int, error) {
		calls++
		cancel()
		return []int{1}, nil
	}, 10, func(int) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("fetches = %d, want 1 (abort on next pass)", calls)
	}
}

func TestCollect(t *testing.T) {
	got, err := Collect(context.Background(), func(_ context.Context, page int) ([]int, error) {
		if page == 2 {
			return nil, nil
		}
		return []int{1, 2}, nil
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("got = %v, want [1 2]", got)
	}
}

func TestCollectErrorReturnsNil(t *testing.T) {
	got, err := CollectBounded(context.Background(), func(_ context.Context, _ int) ([]int, error) {
		return []int{1}, nil
	}, 1)
	if err == nil {
		t.Fatal("expected ErrPageBudgetExceeded")
	}
	if got != nil {
		t.Fatalf("got = %v, want nil on error", got)
	}
}
