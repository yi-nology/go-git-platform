package provider

import (
	"context"
	"errors"

	oekiterrors "github.com/oekit/pkg/errors"
)

// DefaultIterMaxPages is the page budget applied by Each/Collect when no
// explicit bound is given. At the default page size (20) this caps a walk at
// 2000 items; with PerPage=100 it caps at 10000 — far beyond any real list.
// The budget is a safety net against platforms that ignore the page
// parameter (observed in the field: Forgejo 15.0.1 on issue-comment lists),
// so exhausting it is surfaced as ErrPageBudgetExceeded instead of spinning.
const DefaultIterMaxPages = 100

// ErrStopIteration is the sentinel a Each callback returns to stop the walk
// early without reporting an error. Everything fetched so far stands.
var ErrStopIteration = errors.New("iteration stopped")

// ErrPageBudgetExceeded is returned when the walk hits its MaxPages bound
// before the platform returns an empty page. Either the list is genuinely
// longer than the budget, or (more likely) the platform ignores the page
// parameter. Retry with a higher bound via EachBounded/CollectBounded if
// the former.
var ErrPageBudgetExceeded = errors.New("page budget exceeded before an empty page")

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
// item, until the platform returns an empty page. Pages are fetched
// lazily: fn sees items page by page, so a walk over a long list does not
// buffer it in memory. Use Collect instead when the full slice is wanted.
//
// fn returning ErrStopIteration ends the walk with a nil error; any other
// error aborts immediately and is returned as-is. ctx cancellation between
// items and between page fetches is honored.
func Each[T any](ctx context.Context, fetch PageFetch[T], fn func(T) error) error {
	return EachBounded(ctx, fetch, DefaultIterMaxPages, fn)
}

// EachBounded is Each with an explicit page budget. maxPages <= 0 falls
// back to DefaultIterMaxPages. Exceeding the budget fails the walk with an
// error wrapping ErrPageBudgetExceeded (unlike the internal backend
// helper, a public walk must not truncate silently).
func EachBounded[T any](ctx context.Context, fetch PageFetch[T], maxPages int, fn func(T) error) error {
	if maxPages <= 0 {
		maxPages = DefaultIterMaxPages
	}
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
		for _, item := range batch {
			if err := fn(item); err != nil {
				if errors.Is(err, ErrStopIteration) {
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
