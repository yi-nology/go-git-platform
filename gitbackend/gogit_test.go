package gitbackend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newTestGoGitBackend(t *testing.T) *GoGitBackend {
	t.Helper()
	return NewGoGitBackend(Options{})
}

func TestGoGit_Init(t *testing.T) {
	b := newTestGoGitBackend(t)
	dir := t.TempDir()
	err := b.Init(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatal("expected .git directory")
	}
}

func TestGoGit_GetCurrentBranch(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	branch, err := b.GetCurrentBranch(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" && branch != "master" {
		t.Fatalf("expected main or master, got %q", branch)
	}
}

func TestGoGit_CreateAndDeleteBranch(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	err := b.CreateBranch(context.Background(), repo, "feature", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	err = b.DeleteBranch(context.Background(), repo, "feature")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGoGit_Checkout(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	b.CreateBranch(context.Background(), repo, "feature", "HEAD")
	err := b.Checkout(context.Background(), repo, "feature")
	if err != nil {
		t.Fatal(err)
	}

	branch, _ := b.GetCurrentBranch(context.Background(), repo)
	if branch != "feature" {
		t.Fatalf("expected feature, got %q", branch)
	}
}

func TestGoGit_GetStatus(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	status, err := b.GetStatus(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsClean {
		t.Fatal("expected clean repo")
	}

	os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0644)
	status, err = b.GetStatus(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if status.IsClean {
		t.Fatal("expected dirty repo")
	}
}

func TestGoGit_IsAncestor(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	initHash := gitOutput(t, repo, "rev-parse", "HEAD")

	os.WriteFile(filepath.Join(repo, "file2.txt"), []byte("hello"), 0644)
	gitOutput(t, repo, "add", ".")
	gitOutput(t, repo, "commit", "-m", "second")

	secondHash := gitOutput(t, repo, "rev-parse", "HEAD")

	ok, err := b.IsAncestor(context.Background(), repo, initHash, secondHash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected initHash to be ancestor of secondHash")
	}
}

func TestGoGit_CreateTag(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	err := b.CreateTag(context.Background(), repo, "v1.0", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	out := gitOutput(t, repo, "tag", "-l", "v1.0")
	if out != "v1.0" {
		t.Fatalf("expected tag v1.0, got %q", out)
	}
}

func TestGoGit_GetFileAtRevision(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	content, err := b.GetFileAtRevision(context.Background(), repo, "README.md", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "init" {
		t.Fatalf("expected 'init', got %q", string(content))
	}
}

func TestGoGit_AddRemoveRemote(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	err := b.AddRemote(context.Background(), repo, "myremote", "https://example.com/repo.git")
	if err != nil {
		t.Fatal(err)
	}

	err = b.RemoveRemote(context.Background(), repo, "myremote")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGoGit_GetCommitsBetween(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	initHash := gitOutput(t, repo, "rev-parse", "HEAD")

	os.WriteFile(filepath.Join(repo, "file2.txt"), []byte("hello"), 0644)
	gitOutput(t, repo, "add", ".")
	gitOutput(t, repo, "commit", "-m", "second")

	secondHash := gitOutput(t, repo, "rev-parse", "HEAD")

	commits, err := b.GetCommitsBetween(context.Background(), repo, initHash, secondHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0].Message != "second" {
		t.Fatalf("expected 'second', got %q", commits[0].Message)
	}
}

func TestGoGit_Merge(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	// Create a feature branch and add a commit
	b.CreateBranch(context.Background(), repo, "feature", "HEAD")
	b.Checkout(context.Background(), repo, "feature")
	os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature"), 0644)
	gitOutput(t, repo, "add", ".")
	gitOutput(t, repo, "commit", "-m", "feature commit")

	// Switch back to main and merge
	b.Checkout(context.Background(), repo, "main")
	err := b.Merge(context.Background(), repo, "feature", MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the file exists
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Fatal("expected feature.txt to exist after merge")
	}
}

func TestGoGit_Diff(t *testing.T) {
	b := newTestGoGitBackend(t)
	repo := createTestRepo(t)

	initHash := gitOutput(t, repo, "rev-parse", "HEAD")

	os.WriteFile(filepath.Join(repo, "file2.txt"), []byte("hello"), 0644)
	gitOutput(t, repo, "add", ".")
	gitOutput(t, repo, "commit", "-m", "second")

	secondHash := gitOutput(t, repo, "rev-parse", "HEAD")

	diff, err := b.Diff(context.Background(), repo, DiffOptions{
		From: initHash,
		To:   secondHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
}

// TestGoGit_IsAncestor_RevsParity pins the resolveRev migration (issue #128,
// the #108 mirror-method escape): gogit IsAncestor must accept the same rev
// vocabulary as native — branch names, HEAD, short hashes — not just 40-char
// SHAs. native_regress_test.go pins the same table for the native backend;
// the two backends are interchangeable (distroless picks gogit) so a rev
// input that succeeds on one must succeed on the other.
func TestGoGit_IsAncestor_RevsParity(t *testing.T) {
	b := newTestGoGitBackend(t)
	ctx := context.Background()
	repo := createTestRepo(t)
	base := headHash(t, repo)
	commitFile(t, repo, "a.txt", "v2", "second")
	tip := headHash(t, repo)

	cur := currentBranch(t, repo) // CI 的 git init 默认分支可能是 master
	cases := []struct {
		name       string
		ancestor   string
		descendant string
		wantOK     bool
		wantErr    bool
	}{
		{"full hashes", base, tip, true, false},
		{"branch names instead of hashes", cur, "HEAD", true, false},
		{"HEAD vs branch name (same tip, equality counts)", "HEAD", cur, true, false},
		{"short hash ancestor", base[:7], tip, true, false},
		{"invalid ancestor ref", "no-such-ref", tip, false, true},
		{"invalid descendant ref", base, "no-such-ref", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := b.IsAncestor(ctx, repo, tc.ancestor, tc.descendant)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("IsAncestor(%q, %q) = (false, nil), want error for unresolvable ref", tc.ancestor, tc.descendant)
				}
				return
			}
			if err != nil {
				t.Fatalf("IsAncestor(%q, %q): %v", tc.ancestor, tc.descendant, err)
			}
			if ok != tc.wantOK {
				t.Fatalf("IsAncestor(%q, %q) = %v, want %v", tc.ancestor, tc.descendant, ok, tc.wantOK)
			}
		})
	}
}
