package gitbackend

import (
	"context"
	"testing"
)

// 排序契约钉子（v1 契约=返回顺序）：ListLocalBranches 与 GetTagList 双后端
// 均按字母序（探针实证 2026-11-18；gogit 的 storer 迭代顺序上游 go-git 并
// 不保证，钉子防其静默漂移）。注意 tag 是纯字典序非版本感知（"v10" 排在
// "v2" 前）——要版本序调用方自行排序。
func TestOrderingContract_ListLocalBranches_Alphabetical(t *testing.T) {
	for _, tc := range []struct {
		name string
		mk   func(t *testing.T) GitBackend
	}{
		{"native", func(t *testing.T) GitBackend { return newTestNativeBackend(t) }},
		{"gogit", func(t *testing.T) GitBackend { return newTestGoGitBackend(t) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.mk(t)
			repo := createTestRepo(t)
			for _, n := range []string{"zeta", "alpha", "mid"} {
				gitOutput(t, repo, "branch", n)
			}
			bs, err := b.ListLocalBranches(context.Background(), repo)
			if err != nil {
				t.Fatalf("ListLocalBranches: %v", err)
			}
			want := []string{"alpha", "master", "mid", "zeta"}
			if len(bs) != len(want) {
				t.Fatalf("expected %d branches, got %d: %v", len(want), len(bs), bs)
			}
			for i, w := range want {
				if bs[i] != w {
					t.Fatalf("ordering contract broken at %d: got %q, want %q (full: %v)", i, bs[i], w, bs)
				}
			}
		})
	}
}

func TestOrderingContract_GetTagList_Alphabetical(t *testing.T) {
	for _, tc := range []struct {
		name string
		mk   func(t *testing.T) GitBackend
	}{
		{"native", func(t *testing.T) GitBackend { return newTestNativeBackend(t) }},
		{"gogit", func(t *testing.T) GitBackend { return newTestGoGitBackend(t) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.mk(t)
			repo := createTestRepo(t)
			for _, n := range []string{"v2", "v10", "v1"} {
				gitOutput(t, repo, "tag", n)
			}
			infos, err := b.GetTagList(context.Background(), repo)
			if err != nil {
				t.Fatalf("GetTagList: %v", err)
			}
			names := make([]string, 0, len(infos))
			for _, ti := range infos {
				names = append(names, ti.Name)
			}
			want := []string{"v1", "v10", "v2"}
			if len(names) != len(want) {
				t.Fatalf("expected %d tags, got %d: %v", len(want), len(names), names)
			}
			for i, w := range want {
				if names[i] != w {
					t.Fatalf("ordering contract broken at %d: got %q, want %q (full: %v)", i, names[i], w, names)
				}
			}
		})
	}
}
