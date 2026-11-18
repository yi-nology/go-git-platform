package gitbackend

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestGoGit_GetCommitsBetween_BranchRevs verifies from/to accept branch
// names: a from-branch sitting below to must yield only the in-between
// commits. The old plumbing.NewHash path zeroed branch names, so the walk
// never matched and silently returned to's entire history.
func TestGoGit_GetCommitsBetween_BranchRevs(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	main := currentBranch(t, repo)
	initHash := headHash(t, repo)
	gitOutput(t, repo, "branch", "base", initHash)
	commitFile(t, repo, "file2.txt", "hello", "second")

	commits, err := b.GetCommitsBetween(context.Background(), repo, "base", main)
	if err != nil {
		t.Fatalf("GetCommitsBetween: %v", err)
	}
	if len(commits) != 1 || commits[0].Message != "second" {
		t.Fatalf("expected only [second] between base and %s, got %+v", main, commits)
	}
}

// TestGoGit_GetCommitsBetween_UnresolvableRev verifies an unknown rev is a
// hard error instead of an empty-range / full-history guess.
func TestGoGit_GetCommitsBetween_UnresolvableRev(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	_, err := b.GetCommitsBetween(context.Background(), repo, "no-such-branch", "HEAD")
	if err == nil || !errors.Is(err, errCannotResolveRev) {
		t.Fatalf("expected a cannot-resolve error, got %v", err)
	}
}

// TestGoGit_RebaseAbort_RestoresWorktree verifies abort restores not only the
// branch ref but also the worktree and index: restoring only the ref used to
// leave the half-rebase state in place, and the next commit polluted the old
// branch.
func TestGoGit_RebaseAbort_RestoresWorktree(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo, main, topicTip := rebaseSetup(t)
	ctx := context.Background()
	mainTip := gitOutput(t, repo, "rev-parse", main)

	// Simulate a rebase stopped mid-way: the branch ref moved onto main with
	// a conflicted topic.txt staged on top of it.
	gitOutput(t, repo, "checkout", "-q", "-B", "topic", main)
	if err := os.WriteFile(filepath.Join(repo, "topic.txt"), []byte("<<<<<<< conflict"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", "topic.txt")

	stateDir := filepath.Join(repo, ".git", "rebase-merge")
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("orig-head", topicTip)
	write("head-name", "refs/heads/topic")
	write("onto", mainTip)

	if err := b.RebaseAbort(ctx, repo); err != nil {
		t.Fatalf("RebaseAbort: %v", err)
	}

	if got := headHash(t, repo); got != topicTip {
		t.Errorf("expected HEAD restored to %s, got %s", topicTip, got)
	}
	// Worktree content is back at origHead: the conflicted file shows its
	// original body and the onto-side file is gone.
	if body, err := os.ReadFile(filepath.Join(repo, "topic.txt")); err != nil || string(body) != "topic work" {
		t.Errorf("expected topic.txt restored to the origHead content, got %q (%v)", body, err)
	}
	if _, err := os.Stat(filepath.Join(repo, "main.txt")); !os.IsNotExist(err) {
		t.Errorf("expected the onto-side main.txt gone after abort, stat err = %v", err)
	}
	// A clean status proves the next commit cannot pollute the branch.
	status, err := b.GetStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsClean {
		t.Fatalf("expected a clean status after abort, got %+v", status)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Errorf("expected the rebase state cleaned up, stat err = %v", err)
	}
}

// TestGoGit_GetCommitsBetween_ForkParity 分叉场景下 gogit 与 native 必须逐条一致：
// 「目标侧独有提交」里**不能含共同祖先**。
//
// 旧实现的遍历只在「遇到 from 本身」时停 ⇒ 分叉时永远走不到 from ⇒ 一路往回走过
// merge-base，把共同祖先也收进结果 ⇒ 同一个调用方拿到的「目标侧独有提交清单」
// 随 git.backend 配置而变（运维看到的「将被覆盖的提交」清单是误导的）。
func TestGoGit_GetCommitsBetween_ForkParity(t *testing.T) {
	repo := createTestRepo(t)
	base := gitOutput(t, repo, "rev-parse", "HEAD")

	// 分叉：两侧各一个独有提交，互不为祖先，merge-base = base
	gitOutput(t, repo, "checkout", "-b", "src-side")
	if err := os.WriteFile(filepath.Join(repo, "src.txt"), []byte("src"), 0644); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", ".")
	gitOutput(t, repo, "commit", "-m", "src-only")
	srcHash := gitOutput(t, repo, "rev-parse", "HEAD")

	gitOutput(t, repo, "checkout", "-b", "target-side", base)
	if err := os.WriteFile(filepath.Join(repo, "target.txt"), []byte("target"), 0644); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", ".")
	gitOutput(t, repo, "commit", "-m", "target-only")
	targetHash := gitOutput(t, repo, "rev-parse", "HEAD")

	gogit := newTestGoGitBackend(t)
	native := newTestNativeBackend(t)

	got, err := gogit.GetCommitsBetween(context.Background(), repo, srcHash, targetHash)
	if err != nil {
		t.Fatal(err)
	}
	want, err := native.GetCommitsBetween(context.Background(), repo, srcHash, targetHash)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(want) {
		t.Fatalf("分叉场景 gogit 与 native 不一致：gogit=%d 条，native=%d 条", len(got), len(want))
	}
	for i := range got {
		if got[i].Hash != want[i].Hash {
			t.Fatalf("第 %d 条不一致：gogit=%s native=%s", i, got[i].Hash, want[i].Hash)
		}
	}
	// 共同祖先不得出现在结果里
	for _, c := range got {
		if c.Hash == base {
			t.Fatalf("共同祖先不该出现在「目标侧独有提交」里：%v", got)
		}
	}
}

// TestGoGit_GetCommitsBetween_NewestFirstOrder pins the v1 ordering contract
// (iface.go): newest-first, `git log` semantics. Rebase 的回放环依赖此钉子
// ——它在内部反转成 oldest-first 回放（v1.0.1 修复）。
func TestGoGit_GetCommitsBetween_NewestFirstOrder(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "a", "c1")
	commitFile(t, repo, "b.txt", "b", "c2")
	commitFile(t, repo, "c.txt", "c", "c3")

	commits, err := b.GetCommitsBetween(context.Background(), repo, "HEAD~3", "HEAD")
	if err != nil {
		t.Fatalf("GetCommitsBetween: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(commits))
	}
	want := []string{"c3", "c2", "c1"}
	for i, w := range want {
		if commits[i].Message != w {
			t.Fatalf("ordering contract broken at %d: got %q, want %q (full: %v)", i, commits[i].Message, w, commits)
		}
	}
}

// TestGoGit_GetCommitsBetween_EmptyFromWholeHistory pins the empty-from
// semantics aligned with native（`git log to`）：to 可达的全量历史，无边界。
// 旧行为 gogit 侧空 from 直接 cannot-resolve 报错——同一调用随 git.backend
// 配置而变的后端分歧，本测试钉住 v1.0.3 对齐。
func TestGoGit_GetCommitsBetween_EmptyFromWholeHistory(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	commitFile(t, repo, "a.txt", "a", "c1")

	commits, err := b.GetCommitsBetween(context.Background(), repo, "", "HEAD")
	if err != nil {
		t.Fatalf("GetCommitsBetween: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected whole history (init+c1), got %d commits: %v", len(commits), commits)
	}
	if commits[0].Message != "c1" {
		t.Fatalf("newest-first violated: first = %q", commits[0].Message)
	}
}
