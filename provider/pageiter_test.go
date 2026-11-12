package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// TestEachBounded_ReusedBufferNotFalseTruncation 回归（issue #143）：旧实现
// 的伪翻页检测 `prev = batch` 存的是切片头别名——fetch 复用同一底层数组
// （避免每页分配的常规写法）时 batch 与 prev 恒等，第 2 页即被判成无进展
// 静默收手：列表截成第 1 页、err 为 nil。修复后上一页取快照再比较。
func TestEachBounded_ReusedBufferNotFalseTruncation(t *testing.T) {
	buf := make([]int, 3)                             // fetch 每页复用同一底层数组
	want := []int{10, 11, 12, 20, 21, 22, 30, 31, 32} // 三页共 9 个元素
	pages := 0
	var got []int
	err := EachBounded(context.Background(), func(_ context.Context, page int) ([]int, error) {
		pages++
		if page > 3 {
			return nil, nil // 取完
		}
		for i := range buf {
			buf[i] = want[(page-1)*3+i]
		}
		return buf, nil
	}, 10, func(item int) error {
		got = append(got, item)
		return nil
	})
	if err != nil {
		t.Fatalf("reused-buffer walk must not truncate silently: %v", err)
	}
	if pages != 4 { // 三页数据 + 空页收尾
		t.Fatalf("pages = %d, want 4（旧实现第 2 页即误判停手）", pages)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
}

// 复用 buffer 时真·伪翻页（服务端每页回同一批值）仍须被判停：快照比较的
// 语义是「值相同」，不是「内存不同」。
func TestEachBounded_ReusedBufferStillDetectsNoProgress(t *testing.T) {
	buf := []int{1, 2}
	fetches := 0
	err := EachBounded(context.Background(), func(_ context.Context, _ int) ([]int, error) {
		fetches++
		return buf, nil // 平台忽略 page 参数，每页同一 buffer 同一批值
	}, 50, func(int) error { return nil })
	if err != nil {
		t.Fatalf("no-progress soft stop: %v", err)
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d, want 2", fetches)
	}
}
