package branchfilter

import (
	"errors"
	"path"
	"testing"
)

func mustNew(t *testing.T, filterStr string) *BranchFilter {
	t.Helper()
	f, err := New(filterStr)
	if err != nil {
		t.Fatalf("New(%q): %v", filterStr, err)
	}
	return f
}

func TestNew_Empty(t *testing.T) {
	f := mustNew(t, "")
	if !f.IsEmpty() {
		t.Error("expected empty filter")
	}
}

func TestNew_SinglePattern(t *testing.T) {
	f := mustNew(t, "main")
	if f.IsEmpty() {
		t.Error("expected non-empty filter")
	}
	if len(f.Patterns()) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(f.Patterns()))
	}
}

func TestNew_BadPatternRejected(t *testing.T) {
	f, err := New("main,[bad")
	if err == nil {
		t.Fatal("expected an error for the malformed pattern")
	}
	if f != nil {
		t.Errorf("expected a nil filter alongside the error, got %+v", f)
	}
	if !errors.Is(err, path.ErrBadPattern) {
		t.Errorf("expected the error to wrap path.ErrBadPattern, got %v", err)
	}
	if _, err := New("["); err == nil || !errors.Is(err, path.ErrBadPattern) {
		t.Errorf("expected an unclosed bracket to be rejected, got %v", err)
	}
}

func TestMatch_EmptyMatchesAll(t *testing.T) {
	f := mustNew(t, "")
	if !f.Match("any-branch") {
		t.Error("empty filter should match all branches")
	}
}

func TestMatch_ExactPattern(t *testing.T) {
	f := mustNew(t, "main")
	if !f.Match("main") {
		t.Error("expected match for 'main'")
	}
	if f.Match("develop") {
		t.Error("expected no match for 'develop'")
	}
}

func TestMatch_GlobPattern(t *testing.T) {
	f := mustNew(t, "release/*")
	if !f.Match("release/v1.0") {
		t.Error("expected match for 'release/v1.0'")
	}
	if f.Match("main") {
		t.Error("expected no match for 'main'")
	}
}

// TestMatch_StarDoesNotCrossSlash pins the documented semantics: `*` matches
// only within one path level (path.Match), so "release/*" never reaches
// "release/a/b" and there is no `**`.
func TestMatch_StarDoesNotCrossSlash(t *testing.T) {
	f := mustNew(t, "release/*")
	if f.Match("release/a/b") {
		t.Error(`expected "release/*" NOT to match "release/a/b": * does not cross /`)
	}
	deep := mustNew(t, "release/*/*")
	if !deep.Match("release/a/b") {
		t.Error(`expected "release/*/*" to match "release/a/b"`)
	}
}

func TestMatch_MultiplePatterns(t *testing.T) {
	f := mustNew(t, "main, develop, release/*")
	if !f.Match("main") {
		t.Error("expected match for 'main'")
	}
	if !f.Match("develop") {
		t.Error("expected match for 'develop'")
	}
	if !f.Match("release/v2.0") {
		t.Error("expected match for 'release/v2.0'")
	}
	if f.Match("feature/something") {
		t.Error("expected no match for 'feature/something'")
	}
}

func TestFilterBranches(t *testing.T) {
	f := mustNew(t, "main, release/*")
	branches := []string{"main", "develop", "release/v1.0", "feature/x"}
	filtered := f.FilterBranches(branches)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(filtered))
	}
	if filtered[0] != "main" || filtered[1] != "release/v1.0" {
		t.Errorf("unexpected filtered branches: %v", filtered)
	}
}

func TestFilterBranches_EmptyFilter(t *testing.T) {
	f := mustNew(t, "")
	branches := []string{"main", "develop"}
	filtered := f.FilterBranches(branches)
	if len(filtered) != 2 {
		t.Errorf("empty filter should return all branches, got %d", len(filtered))
	}
}

func TestNew_WhitespaceHandling(t *testing.T) {
	f := mustNew(t, " main , develop , feature/* ")
	if len(f.Patterns()) != 3 {
		t.Errorf("expected 3 patterns after trimming, got %d", len(f.Patterns()))
	}
}

// issue #54：含逗号的合法 git ref 名（git check-ref-format --branch
// 'release,1.0' 实测通过）在逗号切分的字符串形态下永不可匹配且无绕法。
// NewPatterns 提供免切分构造器：每个元素就是一个完整 glob pattern。
func TestNewPatterns_SupportsCommasInRefNames(t *testing.T) {
	f, err := NewPatterns([]string{"release,1.0"})
	if err != nil {
		t.Fatalf("NewPatterns: %v", err)
	}
	cases := []struct {
		branch string
		want   bool
	}{
		{"release,1.0", true}, // 本尊必须匹配得上
		{"release", false},    // 不得被拆开的半个 pattern 误命中
		{"1.0", false},
		{"release,1.0-beta", false},
	}
	for _, tc := range cases {
		if got := f.Match(tc.branch); got != tc.want {
			t.Errorf("Match(%q) = %v, want %v", tc.branch, got, tc.want)
		}
	}
	if got := f.FilterBranches([]string{"main", "release,1.0"}); len(got) != 1 || got[0] != "release,1.0" {
		t.Errorf("FilterBranches = %v, want [release,1.0]", got)
	}
	if len(f.Patterns()) != 1 || f.Patterns()[0] != "release,1.0" {
		t.Errorf("Patterns() = %v, want [release,1.0]", f.Patterns())
	}
}

func TestNewPatterns_MatchesAndErrors(t *testing.T) {
	t.Run("empty_slice_matches_everything", func(t *testing.T) {
		f, err := NewPatterns(nil)
		if err != nil {
			t.Fatalf("NewPatterns(nil): %v", err)
		}
		if !f.IsEmpty() || !f.Match("anything") {
			t.Fatal("nil patterns = match-all filter")
		}
	})
	t.Run("glob_semantics_unchanged", func(t *testing.T) {
		f, err := NewPatterns([]string{"main", "release-*", "feature/*"})
		if err != nil {
			t.Fatalf("NewPatterns: %v", err)
		}
		if !f.Match("main") || !f.Match("release-2") || !f.Match("feature/x") || f.Match("release/2") {
			t.Fatal("NewPatterns must follow path.Match semantics, same as New")
		}
	})
	t.Run("bad_pattern_rejected_loud", func(t *testing.T) {
		f, err := NewPatterns([]string{"main", "[bad"})
		if err == nil || f != nil {
			t.Fatal("malformed pattern must error wrapping path.ErrBadPattern")
		}
		if !errors.Is(err, path.ErrBadPattern) {
			t.Fatalf("err = %v, want wrapping path.ErrBadPattern", err)
		}
	})
	t.Run("spaces_inside_one_pattern_are_literal", func(t *testing.T) {
		// 与 New 的 TrimSpace 不同：免切分形态里元素即 pattern，内部空格
		// 是字面量（ref 名允许空格之外的多数符号；空格本身不合法，但
		// 构造器不做二次修剪，保持所见即所得）。
		f, err := NewPatterns([]string{" release "})
		if err != nil {
			t.Fatalf("NewPatterns: %v", err)
		}
		if !f.Match(" release ") {
			t.Fatal("NewPatterns must not trim inside one pattern element")
		}
	})
}

// New 的字符串形态保留逗号切分（便利面，包 doc 写明分隔符地位）：含逗号的
// ref 名在字符串形态下无法表达——这正是 NewPatterns 存在的原因。
func TestNew_CommaIsAlwaysASeparator(t *testing.T) {
	f := mustNew(t, "release,1.0")
	if got := f.Patterns(); len(got) != 2 || got[0] != "release" || got[1] != "1.0" {
		t.Fatalf("Patterns() = %v, want [release 1.0]", got)
	}
	if !f.Match("release") || !f.Match("1.0") || f.Match("release,1.0") {
		t.Fatal("comma-split convenience surface must keep its splitting semantics")
	}
}

// TestNew_MalformedAfterStarRejected pins the issue #133 fix: patterns whose
// malformed section hides BEHIND a star ("release/*[") used to slip past the
// constructor probe — filepath.Match with an empty name fails the leading
// literal chunk first and never scans the rest, so the filter constructed
// fine and silently matched nothing at runtime. path.Match (Go 1.27)
// validates the full pattern before returning a bare false.
func TestNew_MalformedAfterStarRejected(t *testing.T) {
	for _, p := range []string{"release/*[", "feature/*[a", "x*[", "*/release/*[", "a\\", "[a-"} {
		f, err := NewPatterns([]string{p})
		if err == nil {
			t.Errorf("pattern %q must be rejected at construction (it would silently match nothing), got filter %+v", p, f)
		}
		if !errors.Is(err, path.ErrBadPattern) {
			t.Errorf("pattern %q: expected error wrapping path.ErrBadPattern, got %v", p, err)
		}
	}
	// 便捷串形态同样要拦(星号畸形藏在逗号列表第二段)。
	if _, err := New("main,release/*["); err == nil || !errors.Is(err, path.ErrBadPattern) {
		t.Errorf("New comma form must reject a malformed second pattern, got %v", err)
	}
}

// TestMatch_StarDoesNotCrossSlash_PathSemantics pins the issue #134 fix: the
// matcher is path.Match (separator is always `/`), not filepath.Match (whose
// separator is `\` on Windows, where `*` would cross `/` and flip the
// contract). Same assertions as TestMatch_StarDoesNotCrossSlash, kept as its
// primitive-level twin.
func TestMatch_StarDoesNotCrossSlash_PathSemantics(t *testing.T) {
	f := mustNew(t, "release/*")
	if f.Match("release/a/b") {
		t.Error("`release/*` must not match `release/a/b` on any OS (path.Match semantics)")
	}
	if f.Match("release/a") != true {
		t.Error("`release/*` must match `release/a`")
	}
	multi := mustNew(t, "feature/*/*")
	if multi.Match("feature/x") {
		t.Error("`feature/*/*` must not match a single level")
	}
	if !multi.Match("feature/x/y") {
		t.Error("`feature/*/*` must match two levels")
	}
}
