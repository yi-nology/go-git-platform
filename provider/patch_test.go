package provider

import (
	"errors"
	"testing"

	oekiterrors "github.com/oekit/pkg/errors"
)

func TestResolvePatch(t *testing.T) {
	isEmptyStr := func(s string) bool { return s == "" }
	isEmptySlice := func(s []string) bool { return len(s) == 0 }

	cases := []struct {
		name          string
		patch         *string
		legacy        string
		wantVal       string
		wantSpecified bool
	}{
		{"nil patch + empty legacy = 不动", nil, "", "", false},
		{"nil patch + non-empty legacy = 旧字段置值", nil, "old", "old", true},
		{"patch 置值覆盖 legacy", ptr("new"), "old", "new", true},
		{"patch 空串 = 显式清空（覆盖非空 legacy）", ptr(""), "old", "", true},
		{"patch 指向空串 + 空 legacy = 清空", ptr(""), "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			val, ok := ResolvePatch(c.patch, c.legacy, isEmptyStr)
			if val != c.wantVal || ok != c.wantSpecified {
				t.Fatalf("got (%q, %v), want (%q, %v)", val, ok, c.wantVal, c.wantSpecified)
			}
		})
	}

	// 切片形：空切片 patch = 清空
	sp := []string{}
	val, ok := ResolvePatch(&sp, []string{"a"}, isEmptySlice)
	if !ok || len(val) != 0 {
		t.Fatalf("empty slice patch: got (%v, %v), want ( [], true )", val, ok)
	}
}

func ptr[T any](v T) *T { return &v }

func TestErrClearNotSupported(t *testing.T) {
	err := oekiterrors.Wrap(ErrClearNotSupported, "clear milestone",
		"platform", "github", "field", "milestone")
	if !errors.Is(err, ErrClearNotSupported) {
		t.Fatalf("wrapped sentinel must be Is-matchable: %v", err)
	}
}
