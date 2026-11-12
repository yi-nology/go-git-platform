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
//     trims whitespace, and skips empty items (stringx.SplitCSVNonEmpty:
//     RFC 4180 quoting, so an item opened with a double quote keeps its
//     commas and loses the quotes). Outside quotes the comma is ALWAYS a
//     separator; for a literal comma use NewPatterns.
package branchfilter

import (
	"path"

	oekiterrors "github.com/oekit/pkg/errors"
	"github.com/oekit/pkg/slicex"
	"github.com/oekit/pkg/stringx"
)

// BranchFilter matches branch names against a set of comma-separated glob
// patterns. An empty filter matches all branches.
type BranchFilter struct {
	patterns []string
}

// New creates a BranchFilter from a comma-separated list of glob patterns
// (e.g. "main,release-*,feature/*"). An empty string matches everything.
//
// The comma is a separator in this form (whitespace around each pattern is
// trimmed, empty items skipped; splitting is stringx.SplitCSVNonEmpty, so a
// double-quoted item keeps its commas and drops the quotes — RFC 4180).
// For a literal comma in a pattern, use NewPatterns (issue #54).
//
// Each pattern is validated up front with path.Match; a malformed
// pattern yields an error wrapping path.ErrBadPattern instead of a
// filter that silently never matches.
func New(filterStr string) (*BranchFilter, error) {
	if filterStr == "" {
		return &BranchFilter{patterns: nil}, nil
	}

	patterns := stringx.SplitCSVNonEmpty(filterStr)
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

	return slicex.Filter(branches, f.Match)
}

// IsEmpty reports whether the filter has no patterns (matches everything).
func (f *BranchFilter) IsEmpty() bool {
	return len(f.patterns) == 0
}

// Patterns returns the raw glob patterns that were used to create this filter.
func (f *BranchFilter) Patterns() []string {
	return f.patterns
}
