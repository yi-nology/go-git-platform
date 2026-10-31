package provider

import (
	"context"
	"reflect"

	oekiterrors "github.com/oekit/pkg/errors"
)

// DefaultIterMaxPages is the page budget applied by Each/Collect when no
// explicit bound is given. At the default page size (20) this caps a walk at
// 2000 items; with PerPage=100 it caps at 10000 — far beyond any real list.
// The budget is a safety net against genuinely endless list walks; platforms
// that ignore the page parameter (observed in the field: Forgejo 15.0.1 on
// issue-comment lists) are cut short earlier by EachBounded's no-progress
// page detection.
const DefaultIterMaxPages = 100

// ErrStopIteration is the sentinel a Each callback returns to stop the walk
// early without reporting an error. Everything fetched so far stands.
var ErrStopIteration = oekiterrors.New("iteration stopped")

// ErrPageBudgetExceeded is returned when the walk hits its MaxPages bound
// before the platform returns an empty page: the list is genuinely longer
// than the budget (服务器不推进分页的伪翻页形态由 EachBounded 的无进展检测
// 在预算内收手，不走本错误). Retry with a higher bound via
// EachBounded/CollectBounded if the former.
var ErrPageBudgetExceeded = oekiterrors.New("page budget exceeded before an empty page")

// PageFetch fetches one page of a paginated list. page is the 1-based page
// number; the implementation must forward it into the platform call's
// page options (and should set an explicit PerPage so page sizes stay
// stable across the walk):
//
//	err := provider.Each(ctx, func(ctx context.Context, page int) ([]provider.PlatformRepo, error) {
//	    return p.ListRepos(ctx, provider.ListRepoOptions{Owner: "org", Page: page, PerPage: 100})
//	}, func(r provider.PlatformRepo) error {
//	    fmt.Println(r.FullName)
//	    return nil
//	})
type PageFetch[T any] func(ctx context.Context, page int) ([]T, error)

// Each walks every page of a paginated list call, invoking fn once per
// item, until the platform returns an empty page or stops advancing
// pagination. Pages are fetched lazily: fn sees items page by page, so a
// walk over a long list does not buffer it in memory. Use Collect instead
// when the full slice is wanted.
//
// fn returning ErrStopIteration ends the walk with a nil error; any other
// error aborts immediately and is returned as-is. ctx cancellation between
// items and between page fetches is honored.
func Each[T any](ctx context.Context, fetch PageFetch[T], fn func(T) error) error {
	return EachBounded(ctx, fetch, DefaultIterMaxPages, fn)
}

// EachBounded is Each with an explicit page budget. maxPages <= 0 falls
// back to DefaultIterMaxPages. Exceeding the budget fails the walk with an
// error wrapping ErrPageBudgetExceeded (a public walk must not truncate
// silently).
//
// 伪翻页无进展检测（v0.99.10）：与上一页**逐值相同**的页视为服务器未推进
// 分页（实测 Forgejo 15.0.1 的 issue 评论列表忽略 page 参数、每页返回同一批
// 全量数据），按"已到列表末尾"收手并返回已收集数据——继续翻页只会把同一页
// 重复到预算耗尽再报错丢弃全部数据。该形态下首页即全量（这正是服务器无法
// 分页的原因），截断风险仅出现在"忽略 page 但遵守 limit"的另一种服务器形态，
// 此时返回首页数据也优于整体报错。
func EachBounded[T any](ctx context.Context, fetch PageFetch[T], maxPages int, fn func(T) error) error {
	if maxPages <= 0 {
		maxPages = DefaultIterMaxPages
	}
	var prev []T
	for page := 1; ; page++ {
		if page > maxPages {
			return oekiterrors.Wrap(ErrPageBudgetExceeded, "page budget exhausted", "max_pages", maxPages)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		batch, err := fetch(ctx, page)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		if page > 1 && reflect.DeepEqual(batch, prev) {
			return nil // 伪翻页：同页重复 = 列表已到尾（见函数文档）
		}
		prev = batch
		for _, item := range batch {
			if err := fn(item); err != nil {
				if oekiterrors.Is(err, ErrStopIteration) {
					return nil
				}
				return err
			}
		}
	}
}

// Collect walks every page of a paginated list call and returns all items
// as one slice. It is the convenient counterpart of Each for callers that
// want the whole list in memory.
func Collect[T any](ctx context.Context, fetch PageFetch[T]) ([]T, error) {
	return CollectBounded(ctx, fetch, DefaultIterMaxPages)
}

// CollectBounded is Collect with an explicit page budget
// (maxPages <= 0 falls back to DefaultIterMaxPages).
func CollectBounded[T any](ctx context.Context, fetch PageFetch[T], maxPages int) ([]T, error) {
	var all []T
	err := EachBounded(ctx, fetch, maxPages, func(item T) error {
		all = append(all, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}
