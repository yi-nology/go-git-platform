// Package branchfilter matches branch names against glob patterns.
//
// Patterns follow path.Match semantics. Notably, `*` matches any run of
// non-separator characters and therefore never crosses `/` — on every OS
// (branch names are `/`-layered and OS-independent, so the matcher is the
// path-flavored primitive, not filepath.Match whose separator is `\` on
// Windows; issue #134):
//
//	"release/*"  matches "release/v1"   but not "release/a/b"
//	"release-*"  matches "release-1.0"  but not "release/1.0"
//
// There is no `**`; to match additional path levels, spell them out (e.g.
// "feature/*/*"). Malformed patterns (such as an unclosed "[") are rejected
// by both constructors with an error wrapping path.ErrBadPattern — a bad
// pattern never silently matches nothing. The pre-flight covers patterns
// malformed AFTER a star too (e.g. "release/*["): the matcher validates the
// whole pattern, not just the chunks a failing name happens to reach.
//
// Pattern lists come in two shapes:
//
//   - NewPatterns([]string): each element IS one glob pattern, verbatim —
//     commas are ordinary pattern characters. This is the only way to name
//     a branch whose ref name contains a comma ("release,1.0" is a valid
//     git ref name; git check-ref-format accepts it).
//   - New("main,release-*"): the convenience string form splits on commas,
//     trims whitespace, and skips empty items. The comma is ALWAYS a
//     separator in this form — there is no escape — so a ref name that
//     contains a comma cannot be expressed here; use NewPatterns.
package branchfilter

import (
	"path"
	"strings"

	oekiterrors "github.com/oekit/pkg/errors"
)

// BranchFilter matches branch names against a set of comma-separated glob
// patterns. An empty filter matches all branches.
type BranchFilter struct {
	patterns []string
}

// New creates a BranchFilter from a comma-separated list of glob patterns
// (e.g. "main,release-*,feature/*"). An empty string matches everything.
//
// The comma is ALWAYS a separator in this form (whitespace around each
// pattern is trimmed, empty items skipped) — there is no escape mechanism,
// so a branch name containing a comma can never be expressed here; use
// NewPatterns for that (issue #54).
//
// Each pattern is validated up front with path.Match; a malformed
// pattern yields an error wrapping path.ErrBadPattern instead of a
// filter that silently never matches.
func New(filterStr string) (*BranchFilter, error) {
	if filterStr == "" {
		return &BranchFilter{patterns: nil}, nil
	}

	raw := strings.Split(filterStr, ",")
	patterns := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		patterns = append(patterns, p)
	}
	return newPatterns(patterns)
}

// NewPatterns creates a BranchFilter from explicit patterns: each element IS
// one glob pattern, verbatim — no comma splitting, no trimming, no escaping
// rules to remember (issue #54: "release,1.0" is a valid git ref name and is
// matched literally here). An empty slice matches everything.
//
// Each pattern is validated up front with path.Match; a malformed
// pattern yields an error wrapping path.ErrBadPattern instead of a
// filter that silently never matches.
func NewPatterns(patterns []string) (*BranchFilter, error) {
	return newPatterns(append([]string(nil), patterns...))
}

// newPatterns pre-flights each pattern (path.Match's ErrBadPattern would
// otherwise be swallowed at match time, where a malformed pattern silently
// matches nothing and callers cannot tell exclusion caused by the filter
// apart from a broken pattern) and builds the filter.
//
// The probe name is the EMPTY string: it forces path.Match through the
// whole pattern — Go 1.27's Match validates the syntactic validity of the
// entire remainder before returning a bare false, so every malformed chunk
// (including one hiding behind a star, "release/*[" — the shape the old
// filepath.Match probe silently let through because the leading literal
// chunk failed first, issue #133) surfaces here at construction time.
func newPatterns(patterns []string) (*BranchFilter, error) {
	if len(patterns) == 0 {
		return &BranchFilter{patterns: nil}, nil
	}
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			return nil, oekiterrors.Wrap(err, "branchfilter: bad pattern", "pattern", p)
		}
	}
	return &BranchFilter{patterns: patterns}, nil
}

// Match reports whether branchName matches any of the filter's patterns.
// Returns true (matches everything) when the filter has no patterns.
func (f *BranchFilter) Match(branchName string) bool {
	if len(f.patterns) == 0 {
		return true
	}

	for _, pattern := range f.patterns {
		// path.Match：分隔符恒为 `/`，与 OS 无关（issue #134）——Windows 上
		// filepath.Match 的 `*` 不跨的是 `\`，会翻转「不跨分隔符」契约。构造
		// 期已预检良构性，这里的 err 分支对已构造 filter 不可达。
		matched, err := path.Match(pattern, branchName)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// FilterBranches returns only the branches that match the filter.
func (f *BranchFilter) FilterBranches(branches []string) []string {
	if len(f.patterns) == 0 {
		return branches
	}

	result := make([]string, 0, len(branches))
	for _, b := range branches {
		if f.Match(b) {
			result = append(result, b)
		}
	}
	return result
}

// IsEmpty reports whether the filter has no patterns (matches everything).
func (f *BranchFilter) IsEmpty() bool {
	return len(f.patterns) == 0
}

// Patterns returns the raw glob patterns that were used to create this filter.
func (f *BranchFilter) Patterns() []string {
	return f.patterns
}
