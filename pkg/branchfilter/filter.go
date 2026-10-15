// Package branchfilter matches branch names against comma-separated glob
// patterns (e.g. "main,release-*,feature/*").
//
// Patterns follow filepath.Match semantics. Notably, `*` matches any run of
// non-separator characters and therefore never crosses `/`:
//
//	"release/*"  matches "release/v1"   but not "release/a/b"
//	"release-*"  matches "release-1.0"  but not "release/1.0"
//
// There is no `**`; to match additional path levels, spell them out (e.g.
// "feature/*/*"). Malformed patterns (such as an unclosed "[") are rejected
// by New with an error wrapping filepath.ErrBadPattern — a bad pattern never
// silently matches nothing.
package branchfilter

import (
	"path/filepath"
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
// Each pattern is validated up front with filepath.Match; a malformed
// pattern yields an error wrapping filepath.ErrBadPattern instead of a
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
		// Pre-flight every pattern: filepath.Match's ErrBadPattern would
		// otherwise be swallowed at match time, where a malformed pattern
		// silently matches nothing and callers cannot tell exclusion caused
		// by the filter apart from a broken pattern.
		if _, err := filepath.Match(p, ""); err != nil {
			return nil, oekiterrors.Wrap(err, "branchfilter: bad pattern", "pattern", p)
		}
		patterns = append(patterns, p)
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
		matched, err := filepath.Match(pattern, branchName)
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
