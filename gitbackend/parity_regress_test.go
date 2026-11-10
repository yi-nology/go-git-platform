package gitbackend

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 本文件钉跨后端（native / gogit）的语义统一契约。同一夹具仓库分别用两个
// 后端执行同一调用，断言可观测结果一致——语义真值以契约注释（backend.go /
// iface.go）为准，两后端向它对齐。

// --- issue #103：CommitInfo / TagInfo 的 Message 与 Date 口径 ---

// TestParity_CommitInfoFullMessageAndRFC3339 回归：CommitInfo.Message 是
// **完整提交消息**（主题+正文，尾部换行归一），Date 可被 time.Parse
// (RFC3339) 解析——旧 native 用 %s（只有主题行）+ %ai（空格分隔偏移，
// RFC3339 解析必败），gogit 用全文 + RFC3339，同一调用随 git.backend 配置
// 漂移。
func TestParity_CommitInfoFullMessageAndRFC3339(t *testing.T) {
	origin := createTestRepo(t)
	main := currentBranch(t, origin)
	// 多行消息（subject + body）直接经 commitFile 落盘
	commitFile(t, origin, "b.txt", "body content",
		"multi: subject line\n\nbody line one\nbody line two")

	native := newTestNativeBackend(t)
	gogit := newTestGoGitBackend(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		call func() ([]CommitInfo, error)
	}{
		{"GetCommitsBetween", func() ([]CommitInfo, error) {
			return native.GetCommitsBetween(ctx, origin, main+"~1", main)
		}},
		{"GetCommit", func() ([]CommitInfo, error) {
			// GetCommit 的契约入参是完整 SHA（iface.go：commit/ref 参数
			// 除注明外都是 40 位 SHA）
			c, err := native.GetCommit(ctx, origin, headHash(t, origin))
			if err != nil {
				return nil, err
			}
			return []CommitInfo{*c}, nil
		}},
	} {
		t.Run("native/"+tc.name, func(t *testing.T) {
			commits, err := tc.call()
			if err != nil {
				t.Fatal(err)
			}
			assertFullMessageCommit(t, commits[len(commits)-1])
		})
	}
	t.Run("gogit/GetCommitsBetween", func(t *testing.T) {
		commits, err := gogit.GetCommitsBetween(ctx, origin, main+"~1", main)
		if err != nil {
			t.Fatal(err)
		}
		if len(commits) == 0 {
			t.Fatal("no commits returned")
		}
		assertFullMessageCommit(t, commits[len(commits)-1])
	})
	t.Run("gogit/GetCommit", func(t *testing.T) {
		c, err := gogit.GetCommit(ctx, origin, headHash(t, origin))
		if err != nil {
			t.Fatal(err)
		}
		assertFullMessageCommit(t, *c)
	})

	// 两后端在同一提交上的 Message/Date 必须逐字节一致
	fullHash := headHash(t, origin)
	nc, err := native.GetCommit(ctx, origin, fullHash)
	if err != nil {
		t.Fatal(err)
	}
	gc, err := gogit.GetCommit(ctx, origin, fullHash)
	if err != nil {
		t.Fatal(err)
	}
	if nc.Message != gc.Message {
		t.Fatalf("Message split across backends:\n native=%q\n gogit =%q", nc.Message, gc.Message)
	}
	if nc.Date != gc.Date {
		t.Fatalf("Date split across backends: native=%q gogit=%q", nc.Date, gc.Date)
	}
}

// assertFullMessageCommit 校验单个 CommitInfo 的契约口径。
func assertFullMessageCommit(t *testing.T, c CommitInfo) {
	t.Helper()
	want := "multi: subject line\n\nbody line one\nbody line two"
	if c.Message != want {
		t.Fatalf("Message = %q, want full message %q", c.Message, want)
	}
	if _, err := time.Parse(time.RFC3339, c.Date); err != nil {
		t.Fatalf("Date %q is not RFC3339: %v", c.Date, err)
	}
}

// TestParity_TagInfoFullMessage 回归：TagInfo.Message 是完整消息（annotated
// = tag 消息全文，lightweight = commit 消息全文，尾部换行归一）——旧 native
// 用 %(subject) 只有主题行，gogit 用全文。
func TestParity_TagInfoFullMessage(t *testing.T) {
	repo := createTestRepo(t)
	tagMsg := "release v1\n\nfull notes with\nmultiple lines"
	gitOutput(t, repo, "tag", "-a", "annotated", "-m", tagMsg)
	gitOutput(t, repo, "tag", "lightweight")

	native := newTestNativeBackend(t)
	gogit := newTestGoGitBackend(t)
	ctx := context.Background()

	for name, b := range map[string]TagOps{"native": native, "gogit": gogit} {
		t.Run(name, func(t *testing.T) {
			tags, err := b.GetTagList(ctx, repo)
			if err != nil {
				t.Fatal(err)
			}
			byName := map[string]TagInfo{}
			for _, tag := range tags {
				byName[tag.Name] = tag
			}
			a, ok := byName["annotated"]
			if !ok {
				t.Fatalf("annotated tag missing: %+v", tags)
			}
			if a.Message != tagMsg {
				t.Fatalf("annotated Message = %q, want full %q", a.Message, tagMsg)
			}
			if a.Author != "Test" {
				t.Fatalf("annotated Author = %q, want tagger Test", a.Author)
			}
			l, ok := byName["lightweight"]
			if !ok {
				t.Fatalf("lightweight tag missing: %+v", tags)
			}
			if l.Message != "init" {
				t.Fatalf("lightweight Message = %q, want the commit message %q", l.Message, "init")
			}
		})
	}
}

// --- issue #102：GetStatus 的 Ahead/Behind ---

// TestParity_GetStatus_AheadBehind 回归：落后/领先必须如实上报——旧 gogit
// 从不计算这两个字段（恒 0/0 静默谎报「已同步」）。两后端在同一仓库上
// 给出同一数字。
func TestParity_GetStatus_AheadBehind(t *testing.T) {
	origin := createTestRepo(t)
	clone := t.TempDir()
	cloneRepo(t, origin, clone)

	// 本地领先 2，落后 1
	commitFile(t, origin, "origin-side.txt", "from origin", "origin work")
	commitFile(t, clone, "clone-side-1.txt", "1", "clone work 1")
	commitFile(t, clone, "clone-side-2.txt", "2", "clone work 2")
	gitOutput(t, clone, "fetch", "origin")

	native := newTestNativeBackend(t)
	gogit := newTestGoGitBackend(t)
	ctx := context.Background()

	ns, err := native.GetStatus(ctx, clone)
	if err != nil {
		t.Fatal(err)
	}
	gs, err := gogit.GetStatus(ctx, clone)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Ahead != 2 || ns.Behind != 1 {
		t.Fatalf("native Ahead/Behind = %d/%d, want 2/1", ns.Ahead, ns.Behind)
	}
	if gs.Ahead != ns.Ahead || gs.Behind != ns.Behind {
		t.Fatalf("gogit Ahead/Behind = %d/%d, native = %d/%d — cross-backend split",
			gs.Ahead, gs.Behind, ns.Ahead, ns.Behind)
	}

	// 无上游的仓库：两后端同为 0/0（宽松口径，上游缺失不是 GetStatus 的失败）
	solo := createTestRepo(t)
	for name, b := range map[string]StatusDiffOps{"native": native, "gogit": gogit} {
		t.Run("no-upstream/"+name, func(t *testing.T) {
			st, err := b.GetStatus(ctx, solo)
			if err != nil {
				t.Fatal(err)
			}
			if st.Ahead != 0 || st.Behind != 0 {
				t.Fatalf("Ahead/Behind = %d/%d, want 0/0 without upstream", st.Ahead, st.Behind)
			}
		})
	}
}

// --- issue #108：GetFileAtRevision 的 ref 解析与空 ref 语义 ---

// TestParity_GetFileAtRevision_Refs 回归：分支/标签/短 hash 一律可解析
// （旧 gogit 对非空 ref 直接 plumbing.NewHash 盲 hex 解码，全部失败）；
// 空 ref = HEAD 提交内容而非 index 暂存内容（旧 native 裸 ":path" 读
// index，与 gogit 读 HEAD 分裂——index≠HEAD 时静默返回错内容）。
func TestParity_GetFileAtRevision_Refs(t *testing.T) {
	repo := createTestRepo(t)
	gitOutput(t, repo, "tag", "v1")
	// HEAD 之后：工作区与 index 都改成 v2（未提交）
	if err := osWriteFileInRepo(repo, "README.md", "v2-staged"); err != nil {
		t.Fatal(err)
	}
	shortHash := headHash(t, repo)[:7]

	native := newTestNativeBackend(t)
	gogit := newTestGoGitBackend(t)
	ctx := context.Background()

	for name, b := range map[string]FileOps{"native": native, "gogit": gogit} {
		t.Run(name, func(t *testing.T) {
			// 空 ref = HEAD 内容（不是 index 里的 v2-staged）
			got, err := b.GetFileAtRevision(ctx, repo, "README.md", "")
			if err != nil {
				t.Fatalf("empty ref: %v", err)
			}
			if string(got) != "init" {
				t.Fatalf("empty ref must read HEAD (not the staged index), got %q", got)
			}
			// 分支名
			main := currentBranch(t, repo)
			got, err = b.GetFileAtRevision(ctx, repo, "README.md", main)
			if err != nil {
				t.Fatalf("branch ref: %v", err)
			}
			if string(got) != "init" {
				t.Fatalf("branch ref content = %q", got)
			}
			// 标签名
			got, err = b.GetFileAtRevision(ctx, repo, "README.md", "v1")
			if err != nil {
				t.Fatalf("tag ref: %v", err)
			}
			if string(got) != "init" {
				t.Fatalf("tag ref content = %q", got)
			}
			// 短 hash
			got, err = b.GetFileAtRevision(ctx, repo, "README.md", shortHash)
			if err != nil {
				t.Fatalf("short hash ref: %v", err)
			}
			if string(got) != "init" {
				t.Fatalf("short hash ref content = %q", got)
			}
			// 不可解析 ref 必须报错（gogit 旧行为是垃圾 hash 报错，native 报错——一致）
			if _, err := b.GetFileAtRevision(ctx, repo, "README.md", "no-such-ref"); err == nil {
				t.Fatal("unresolvable ref must error")
			}
		})
	}
}

// --- issue #110：Diff 空 From/To 三方统一 ---

// TestParity_Diff_WorktreeAgainstHead 回归：空 From/To 的 Diff 是
// 「工作区 vs HEAD」的 unified patch——staged 与 unstaged 变更都可见
// （旧 native 裸 `git diff` 只看 index 侧、staged-only 完全不可见；旧
// gogit 返回 go-git 的状态清单文本而非 patch），untracked 不出现。
func TestParity_Diff_WorktreeAgainstHead(t *testing.T) {
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "v1", "add a")
	commitFile(t, repo, "b.txt", "v1", "add b")
	commitFile(t, repo, "d.txt", "v1", "add d")

	// staged 修改 a.txt + staged 新增 c.txt + staged 删除 d.txt
	if err := osWriteFileInRepo(repo, "a.txt", "v2-staged"); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", "--", "a.txt")
	if err := osWriteFileInRepo(repo, "c.txt", "new file"); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", "--", "c.txt")
	gitOutput(t, repo, "rm", "--cached", "-q", "d.txt")
	// unstaged 修改 b.txt（staged 后再改）
	if err := osWriteFileInRepo(repo, "b.txt", "v2-worktree"); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", "--", "b.txt")
	if err := osWriteFileInRepo(repo, "b.txt", "v3-worktree"); err != nil {
		t.Fatal(err)
	}
	// untracked e.txt
	if err := osWriteFileInRepo(repo, "e.txt", "untracked"); err != nil {
		t.Fatal(err)
	}

	native := newTestNativeBackend(t)
	gogit := newTestGoGitBackend(t)
	ctx := context.Background()

	assertWorktreeVsHeadPatch := func(name string, diffText string) {
		t.Helper()
		for _, want := range []string{
			"diff --git", // 是 patch 而非状态清单
			"a/a.txt",    // staged 修改可见
			"a/c.txt",    // staged 新增可见（旧 native 裸 diff 看不到 staged-only）
			"a/b.txt",    // unstaged 修改可见
			"d.txt",      // staged 删除可见
		} {
			if !strings.Contains(diffText, want) {
				t.Fatalf("%s: diff missing %q:\n%s", name, want, diffText)
			}
		}
		if strings.Contains(diffText, "e.txt") {
			t.Fatalf("%s: untracked files must not appear in diff:\n%s", name, diffText)
		}
	}

	nd, err := native.Diff(ctx, repo, DiffOptions{})
	if err != nil {
		t.Fatalf("native: %v", err)
	}
	assertWorktreeVsHeadPatch("native", nd)

	gd, err := gogit.Diff(ctx, repo, DiffOptions{})
	if err != nil {
		t.Fatalf("gogit: %v", err)
	}
	assertWorktreeVsHeadPatch("gogit", gd)

	// gogit 的空 From/To 输出必须是 unified patch：带 ---/+++ 与 @@ 头
	if !strings.Contains(gd, "---") || !strings.Contains(gd, "@@") {
		t.Fatalf("gogit empty-diff output is not a unified patch:\n%s", gd)
	}
}

// TestParity_Diff_OneSidedArgsError 回归：单边 From/To 意图不可判，
// 两后端一致显式报错（旧 native 静默忽略单边参数）。
func TestParity_Diff_OneSidedArgsError(t *testing.T) {
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "v2", "second")

	native := newTestNativeBackend(t)
	gogit := newTestGoGitBackend(t)
	ctx := context.Background()

	for name, b := range map[string]StatusDiffOps{"native": native, "gogit": gogit} {
		t.Run(name, func(t *testing.T) {
			if _, err := b.Diff(ctx, repo, DiffOptions{From: "HEAD"}); err == nil {
				t.Fatal("From-only must error")
			}
			if _, err := b.Diff(ctx, repo, DiffOptions{To: "HEAD"}); err == nil {
				t.Fatal("To-only must error")
			}
			if _, err := b.Diff(ctx, repo, DiffOptions{From: "HEAD~1", To: "HEAD"}); err != nil {
				t.Fatalf("both-set must still work: %v", err)
			}
		})
	}
}

// osWriteFileInRepo 写仓库内文件（error 形态，供测试体里直接判错处使用）。
func osWriteFileInRepo(repo, name, content string) error {
	return os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644)
}
