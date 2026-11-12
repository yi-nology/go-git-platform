package gitbackend

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
)

// divergedRepo builds a repo with main and feature diverged from a common
// base: main commits main-only.txt, feature commits feature-only.txt.
func divergedRepo(t *testing.T) string {
	t.Helper()
	repo := createTestRepo(t)
	main := currentBranch(t, repo)
	gitOutput(t, repo, "checkout", "-b", "feature")
	commitFile(t, repo, "feature-only.txt", "from feature", "feature work")
	gitOutput(t, repo, "checkout", main)
	commitFile(t, repo, "main-only.txt", "from main", "main work")
	return repo
}

func TestGoGit_Merge_FastForward(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	gitOutput(t, repo, "branch", "ff")
	gitOutput(t, repo, "checkout", "ff")
	tip := commitFile(t, repo, "ff.txt", "fast-forwarded", "ff work")
	gitOutput(t, repo, "checkout", currentBranch(t, repo))

	if err := b.Merge(context.Background(), repo, "ff", MergeOptions{}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got := headHash(t, repo); got != tip {
		t.Errorf("expected HEAD to fast-forward to %s, got %s", tip, got)
	}
}

func TestGoGit_Merge_AlreadyUpToDate(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	before := headHash(t, repo)
	if err := b.Merge(context.Background(), repo, currentBranch(t, repo), MergeOptions{}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if after := headHash(t, repo); after != before {
		t.Errorf("self-merge moved HEAD from %s to %s", before, after)
	}
}

func TestGoGit_Merge_ThreeWayCreatesMergeCommit(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := divergedRepo(t)
	featureTip := gitOutput(t, repo, "rev-parse", "feature")

	if err := b.Merge(context.Background(), repo, "feature", MergeOptions{Message: "custom merge"}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got := gitOutput(t, repo, "log", "-1", "--pretty=%s"); got != "custom merge" {
		t.Errorf("expected the custom merge message, got %q", got)
	}
	parents := gitOutput(t, repo, "log", "-1", "--pretty=%P")
	if !strings.Contains(parents, featureTip) {
		t.Errorf("expected a two-parent merge commit recording %s, parents = %q", featureTip, parents)
	}
	for _, file := range []string{"main-only.txt", "feature-only.txt"} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Errorf("expected %s in the working tree after the merge: %v", file, err)
		}
	}
}

func TestGoGit_Merge_FFOnlyRejectsDiverged(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := divergedRepo(t)
	before := headHash(t, repo)

	err := b.Merge(context.Background(), repo, "feature", MergeOptions{FFOnly: true})
	if err == nil || !strings.Contains(err.Error(), "fast-forward") {
		t.Fatalf("expected a not-a-fast-forward error, got %v", err)
	}
	if after := headHash(t, repo); after != before {
		t.Errorf("failed FFOnly merge must not move HEAD (%s → %s)", before, after)
	}
}

func TestGoGit_Merge_ConflictDetected(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	main := currentBranch(t, repo)
	gitOutput(t, repo, "checkout", "-b", "feature")
	commitFile(t, repo, "shared.txt", "feature version", "feature edit")
	gitOutput(t, repo, "checkout", main)
	commitFile(t, repo, "shared.txt", "main version", "main edit")

	err := b.Merge(context.Background(), repo, "feature", MergeOptions{})
	if err == nil || !IsMergeConflict(err) {
		t.Fatalf("expected a merge-conflict error, got %v", err)
	}
}

func TestGoGit_Merge_NoCommitLeavesStaged(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := divergedRepo(t)
	before := headHash(t, repo)

	if err := b.Merge(context.Background(), repo, "feature", MergeOptions{NoCommit: true}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if after := headHash(t, repo); after != before {
		t.Errorf("NoCommit merge moved HEAD (%s → %s)", before, after)
	}
	if _, err := os.Stat(filepath.Join(repo, "feature-only.txt")); err != nil {
		t.Errorf("expected the branch changes applied to the worktree: %v", err)
	}
	staged := gitOutput(t, repo, "diff", "--cached", "--name-only")
	if !strings.Contains(staged, "feature-only.txt") {
		t.Errorf("expected feature-only.txt staged, got %q", staged)
	}
}

func TestGoGit_Merge_SquashSingleParent(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := divergedRepo(t)
	featureTip := gitOutput(t, repo, "rev-parse", "feature")

	if err := b.Merge(context.Background(), repo, "feature", MergeOptions{Squash: true}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	parents := gitOutput(t, repo, "log", "-1", "--pretty=%P")
	if strings.Contains(parents, featureTip) || len(strings.Fields(parents)) != 1 {
		t.Errorf("squash merge must record a single parent excluding %s, parents = %q", featureTip, parents)
	}
}

func TestGoGit_Merge_DefaultMessageAndMissingBranch(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := divergedRepo(t)

	if err := b.Merge(context.Background(), repo, "feature", MergeOptions{}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got := gitOutput(t, repo, "log", "-1", "--pretty=%s"); got != "Merge branch 'feature'" {
		t.Errorf("expected the default merge message, got %q", got)
	}

	err := b.Merge(context.Background(), repo, "missing", MergeOptions{})
	if err == nil || !IsNotFound(err) {
		t.Errorf("missing branch: expected a NotFound error, got %v", err)
	}
}

func TestGoGit_CherryPick(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	main := currentBranch(t, repo)
	gitOutput(t, repo, "checkout", "-b", "side")
	picked := commitFile(t, repo, "note.txt", "picked content", "side commit")
	gitOutput(t, repo, "checkout", main)

	if err := b.CherryPick(context.Background(), repo, picked); err != nil {
		t.Fatalf("CherryPick: %v", err)
	}
	if got := gitOutput(t, repo, "log", "-1", "--pretty=%s"); got != "side commit" {
		t.Errorf("expected the picked message on HEAD, got %q", got)
	}
	if body, err := os.ReadFile(filepath.Join(repo, "note.txt")); err != nil || string(body) != "picked content" {
		t.Errorf("expected note.txt applied to the worktree, got %q (%v)", body, err)
	}

	if err := b.CherryPick(context.Background(), repo, "0000000000000000000000000000000000000000"); err == nil {
		t.Error("expected an error cherry-picking an unknown commit")
	}
}

func TestResolveRev(t *testing.T) {
	repo := createTestRepo(t)
	gogitRepo, err := git.PlainOpen(repo)
	if err != nil {
		t.Fatal(err)
	}
	main := currentBranch(t, repo)
	want := headHash(t, repo)

	for _, ref := range []string{main, "refs/heads/" + main, want} {
		got, err := resolveRev(gogitRepo, ref)
		if err != nil {
			t.Errorf("resolveRev(%q): %v", ref, err)
			continue
		}
		if got.String() != want {
			t.Errorf("resolveRev(%q) = %s, want %s", ref, got, want)
		}
	}
	// Unresolvable revs are errors — never a zero hash that callers would
	// silently act on.
	for _, ref := range []string{"no-such-ref", strings.Repeat("0", 40)} {
		if _, err := resolveRev(gogitRepo, ref); err == nil || !errors.Is(err, errCannotResolveRev) {
			t.Errorf("resolveRev(%q): expected a cannot-resolve error, got %v", ref, err)
		}
	}
	// A tree hash is not a commit: the fallback must reject it too instead of
	// handing back a hash no caller can log or diff.
	treeHash := gitOutput(t, repo, "rev-parse", "HEAD^{tree}")
	if _, err := resolveRev(gogitRepo, treeHash); err == nil || !errors.Is(err, errCannotResolveRev) {
		t.Errorf("resolveRev(tree hash): expected a cannot-resolve error, got %v", err)
	}
}

// TestGoGit_Merge_PreservesExecMode verifies the three-way merge apply path
// restores a changed entry's git mode: an executable touched on the merged
// branch must come back 0o755 in the worktree, not the umask-truncated 0o644
// a plain os.Create used to leave behind. It also proves the merge commit
// still records the mode (status stays clean after the commit).
func TestGoGit_Merge_PreservesExecMode(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)
	main := currentBranch(t, repo)
	commitExecFile(t, repo, "run.sh", "#!/bin/sh\necho one\n", "add script")

	// Feature edits the executable while main moves on its own file.
	gitOutput(t, repo, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repo, "run.sh"), []byte("#!/bin/sh\necho two\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(repo, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", "run.sh")
	gitOutput(t, repo, "commit", "-m", "feature edits script")
	gitOutput(t, repo, "checkout", "-q", main)
	commitFile(t, repo, "main-only.txt", "main", "main work")

	if err := b.Merge(context.Background(), repo, "feature", MergeOptions{}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	info, err := os.Stat(filepath.Join(repo, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("expected run.sh merged with mode 0755, got %v", info.Mode().Perm())
	}
	if body, _ := os.ReadFile(filepath.Join(repo, "run.sh")); string(body) != "#!/bin/sh\necho two\n" {
		t.Errorf("expected the feature content merged in, got %q", body)
	}
	status, err := b.GetStatus(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsClean {
		t.Errorf("expected a clean worktree after the merge commit, got %+v", status)
	}
}

// removeLooseObject 删掉一个 loose object 文件（构造对象库不完整的仓库）：
// git commit 落的是 loose object，哈希前两位是目录名。
func removeLooseObject(t *testing.T, dir, hash string) {
	t.Helper()
	objPath := filepath.Join(dir, ".git", "objects", hash[:2], hash[2:])
	if _, err := os.Stat(objPath); err != nil {
		// 对象被打包（gc 后）时无法按文件删——本测试的夹具仓库不会走到这
		t.Skipf("loose object %s not found: %v", hash, err)
	}
	if err := os.Remove(objPath); err != nil {
		t.Fatal(err)
	}
}

// TestGoGit_Merge_MissingCommitObjectReturnsError 回归（issue #141）：commit
// 对象缺失时必须返回 error 而不是 panic。旧实现丢弃 CommitObject 的错误，
// nil commit 在非快进路径上一路走到 .Tree() 解引用，空指针崩溃打穿宿主
// 进程；native 后端同场景（git 自身）只报 error。两条根因路径（HEAD commit
// 缺失 / 目标分支 commit 缺失）都要 fail-fast。
func TestGoGit_Merge_MissingCommitObjectReturnsError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		branch func(repo string) string // 返回被删对象对应的 rev
	}{
		{"missing HEAD commit object", func(repo string) string { return headHash(t, repo) }},
		{"missing branch commit object", func(repo string) string { return gitOutput(t, repo, "rev-parse", "feature") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newTestGoGitBackend(t)
			repo := divergedRepo(t) // main 与 feature 分叉：走非快进路径
			removeLooseObject(t, repo, tc.branch(repo))

			err := b.Merge(context.Background(), repo, "feature", MergeOptions{AllowUnrelated: true})
			if err == nil {
				t.Fatal("want error for missing commit object, got nil (old code panicked here)")
			}
			if strings.Contains(err.Error(), "runtime error") {
				t.Fatalf("unexpected panic-derived error: %v", err)
			}
		})
	}
}
