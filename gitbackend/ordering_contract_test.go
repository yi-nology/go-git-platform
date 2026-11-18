package gitbackend

import (
	"context"
	"os"
	"path/filepath"
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

// ListRemoteBranches 排序契约：字母序（HEAD 引用过滤）——native `git branch
// -r` 与 gogit 远端引用迭代输出一致（探针实证 2026-11-18）。
func TestOrderingContract_ListRemoteBranches_Alphabetical(t *testing.T) {
	for _, tc := range []struct {
		name string
		mk   func(t *testing.T) GitBackend
	}{
		{"native", func(t *testing.T) GitBackend { return newTestNativeBackend(t) }},
		{"gogit", func(t *testing.T) GitBackend { return newTestGoGitBackend(t) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.mk(t)
			// bare 远端 + 非字母序分支 + 被测仓 fetch 建 remote-tracking refs
			origin := filepath.Join(t.TempDir(), "origin.git")
			if err := os.MkdirAll(origin, 0o755); err != nil {
				t.Fatal(err)
			}
			gitOutput(t, origin, "init", "--bare", ".")
			tmp1 := t.TempDir()
			seed := filepath.Join(tmp1, "seed")
			gitOutput(t, tmp1, "clone", origin, seed)
			gitOutput(t, seed, "commit", "--allow-empty", "-m", "init")
			gitOutput(t, seed, "push", "origin", "HEAD")
			for _, n := range []string{"zeta", "alpha", "mid"} {
				gitOutput(t, seed, "branch", n)
			}
			gitOutput(t, seed, "push", "origin", "--all")
			tmp2 := t.TempDir()
			repo := filepath.Join(tmp2, "repo")
			gitOutput(t, tmp2, "clone", origin, repo)

			bs, err := b.ListRemoteBranches(context.Background(), repo, "origin")
			if err != nil {
				t.Fatalf("ListRemoteBranches: %v", err)
			}
			want := []string{"alpha", "master", "mid", "zeta"}
			if len(bs) != len(want) {
				t.Fatalf("expected %d remote branches, got %d: %v", len(want), len(bs), bs)
			}
			for i, w := range want {
				if bs[i] != w {
					t.Fatalf("ordering contract broken at %d: got %q, want %q (full: %v)", i, bs[i], w, bs)
				}
			}
		})
	}
}

// GetStatus 文件序契约：Staged/Untracked 均按路径排序——双后端一致（探针
// 实证 2026-11-18；native porcelain 与 gogit 各自解析后的顺序被钉子锚定）。
func TestOrderingContract_GetStatus_PathsSorted(t *testing.T) {
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
			for _, n := range []string{"zz.txt", "aa.txt", "mm.txt"} {
				if err := os.WriteFile(filepath.Join(repo, n), []byte(n), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			gitOutput(t, repo, "add", "zz.txt")
			st, err := b.GetStatus(context.Background(), repo)
			if err != nil {
				t.Fatalf("GetStatus: %v", err)
			}
			if len(st.Staged) != 1 || st.Staged[0].Path != "zz.txt" {
				t.Fatalf("staged = %v, want [zz.txt]", st.Staged)
			}
			if len(st.Untracked) != 2 || st.Untracked[0] != "aa.txt" || st.Untracked[1] != "mm.txt" {
				t.Fatalf("untracked = %v, want [aa.txt mm.txt]", st.Untracked)
			}
		})
	}
}

// ErrRemoteNotFound 双后端对齐：missing remote config 两侧行为一致
// （gogit 原有契约，native 对齐轮补齐 fail-loud——旧行为返回空列表零报错）。
func TestOrderingContract_ListRemoteBranches_MissingRemote(t *testing.T) {
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
			_, err := b.ListRemoteBranches(context.Background(), repo, "no-such-remote")
			if err == nil || !IsNotFound(err) {
				t.Fatalf("missing remote: expected NotFound, got %v", err)
			}
		})
	}
}

// 快照语义钉子：clone 后远端新增的分支在 fetch 前不得出现——两后端读的都是
// 本地 remote-tracking refs 快照（gogit 对齐轮修复 live ls-remote 旧行为）。
func TestOrderingContract_ListRemoteBranches_SnapshotSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		mk   func(t *testing.T) GitBackend
	}{
		{"native", func(t *testing.T) GitBackend { return newTestNativeBackend(t) }},
		{"gogit", func(t *testing.T) GitBackend { return newTestGoGitBackend(t) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.mk(t)
			origin := createTestRepo(t)
			tmp1 := t.TempDir()
			clone := filepath.Join(tmp1, "clone")
			gitOutput(t, tmp1, "clone", origin, clone)
			// clone 之后远端才长出的分支：fetch 前不得进列表
			gitOutput(t, origin, "branch", "post-clone")
			bs, err := b.ListRemoteBranches(context.Background(), clone, "origin")
			if err != nil {
				t.Fatalf("ListRemoteBranches: %v", err)
			}
			for _, name := range bs {
				if name == "post-clone" {
					t.Fatalf("snapshot semantics broken: post-clone branch visible before any fetch: %v", bs)
				}
			}
			// fetch 后出现（本地快照刷新）
			gitOutput(t, clone, "fetch", "origin")
			bs, err = b.ListRemoteBranches(context.Background(), clone, "origin")
			if err != nil {
				t.Fatalf("ListRemoteBranches after fetch: %v", err)
			}
			found := false
			for _, name := range bs {
				if name == "post-clone" {
					found = true
				}
			}
			if !found {
				t.Fatalf("post-clone branch still missing after fetch: %v", bs)
			}
		})
	}
}
