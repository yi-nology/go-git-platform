package backendutil

import (
	"context"

	"github.com/yi-nology/go-git-platform/provider"
)

// maxAllPages bounds the pagination walks run by backend list methods as a
// safety net against platforms that do not advance pagination. Observed in
// the field: a Forgejo 15.0.1 instance ignored the page parameter on the
// issue-comment list endpoint and returned the identical full page for every
// page number, so the "stop on first empty page" rule never fired and callers
// spun forever. maxAllPages×pageSize items is far beyond any real list.
//
// The bound delegates to the provider's public iterator (single ITERATOR
// implementation for the whole SDK). Unlike pre-0.77.0 AllPages — which
// logged and silently truncated at the cap — exhausting the budget now fails
// the walk with an error wrapping provider.ErrPageBudgetExceeded, so a
// platform pagination bug surfaces instead of quietly dropping data.
const maxAllPages = 50

// AllPages fetches every page of a paginated list by advancing the page
// number until the platform returns an empty page. fetch receives the
// 1-based page number and must request that page with whatever page size
// the caller chose. Stopping on the first empty page (rather than on a
// short page) keeps the result complete even when the server caps the page
// size below the requested per-page value; the platform's list endpoint
// must honor the page parameter, which every supported platform's list API
// does.
func AllPages[T any](fetch func(page int) ([]T, error)) ([]T, error) {
	return provider.CollectBounded(context.Background(), func(_ context.Context, page int) ([]T, error) {
		return fetch(page)
	}, maxAllPages)
}

// PageList is the shared dual-mode list idiom of the Gitea-family backends:
// page == 0 walks every page of the endpoint (AllPages, budget-capped) at
// walkPageSize; page > 0 fetches exactly the caller-driven single page at
// the normalized caller per-page size. fetch is invoked as fetch(page,
// perPage) with both values ready to forward into the SDK list options.
func PageList[T any](page, perPage, walkPageSize int, fetch func(page, perPage int) ([]T, error)) ([]T, error) {
	if page == 0 {
		return AllPages(func(p int) ([]T, error) { return fetch(p, walkPageSize) })
	}
	p, pp := provider.NormalizePageOpts(page, perPage)
	return fetch(p, pp)
}
