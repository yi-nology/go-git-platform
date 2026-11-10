package provider

import (
	"net/http"
	"strconv"

	"github.com/oekit/pkg/pagination"
)

const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// pagePolicy 本仓分页口径：默认每页 DefaultPerPage、上限 MaxPerPage、超限钳到上限。
// 口径数字仍由本包的导出常量给出（对外 API 不变），归一化逻辑单源于
// oekit/pkg/pagination.Policy。
var pagePolicy = pagination.Policy{
	DefaultPageSize: DefaultPerPage,
	MaxPageSize:     MaxPerPage,
}

// NormalizePageOpts applies default values for page/perPage: Page falls
// back to DefaultPage when below 1, and PerPage to DefaultPerPage when out
// of the (0, MaxPerPage] range.
//
// NormalizePageOpts is also the documentation anchor for the SDK-wide Page
// semantics (issue #60) — the contract every list option type's Page field
// follows. Two families exist, chosen per manager by the boundedness of the
// result set; each option type's doc states which one it belongs to:
//
//   - Exhaustive family (bounded sets — labels, milestones, notifications).
//     Page == 0 (the zero value, the default) walks EVERY page of the result
//     set and returns all items; Page > 0 returns exactly that single
//     caller-driven page at the normalized sizes. Walks are budget-capped
//     and fail loud (error wrapping ErrPageBudgetExceeded) when a platform's
//     pagination never advances, so a silent partial list is impossible.
//
//   - Caller-paged family (unbounded sets — change requests; the issue
//     family's ListIssues is tracked by issue #51). Page == 0 returns the
//     platform's FIRST page at the normalized page size — one request, no
//     walk, because a default call's request volume must not scale silently
//     with the repository's size. Page > 0 returns exactly that page;
//     callers page through explicitly until a short page comes back.
//
// Per-backend departures from either family are machine-readable in each
// backend's divergence ledger (Provider.Divergences, regenerated into
// docs/divergence-ledger.md) — never prose-only.
func NormalizePageOpts(page, perPage int) (int, int) {
	return pagePolicy.Normalize(page, perPage)
}

// ParseTotalCountHeader reads X-Total-Count or X-Total from response headers.
// Falls back to the provided default if neither header is present or valid.
func ParseTotalCountHeader(headers http.Header, fallback int) int {
	for _, key := range []string{"X-Total-Count", "X-Total"} {
		if v := headers.Get(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				return n
			}
		}
	}
	return fallback
}
