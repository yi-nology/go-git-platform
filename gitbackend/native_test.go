package gitbackend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newTestNativeBackend(t *testing.T) *NativeGitBackend {
	t.Helper()
	backend, err := NewNativeGitBackend(Options{})
	if err != nil {
		t.Skip("git not available:", err)
	}
	return backend
}

func createTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// 仓库本地身份：该仓库内后续一切建提交操作（夹具的 commit/merge，
	// 以及被测 backend 的 Merge/Rebase/CherryPick）都不依赖全局 config
	gitOutput(t, dir, "config", "user.email", "test@test.com")
	gitOutput(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init"), 0644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	cmd = exec.Command("git", "-C", dir, "add", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	return dir
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	// 身份统一注入：commit/merge/rebase/cherry-pick 夹具都会创建提交，
	// CI runner 没有全局 git 身份，不注入则 exit 128（CI 曾因此连续全红）
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("git %v: %v\n%s", args, err, ee.Stderr)
		}
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimRight(string(out), "\n")
}

func TestNative_Init(t *testing.T) {
	b := newTestNativeBackend(t)
	dir := t.TempDir()
	err := b.Init(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatal("expected .git directory")
	}
}

func TestNative_GetCurrentBranch(t *testing.T) {
	b := newTestNativeBackend(t)
	repo := createTestRepo(t)

	branch, err := b.GetCurrentBranch(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" && branch != "master" {
		t.Fatalf("expected main or master, got %q", branch)
	}
}

func TestNative_CreateAndDeleteBranch(t *testing.T) {
	b := newTestNativeBackend(t)
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

func TestNative_CreateBranchExists(t *testing.T) {
	b := newTestNativeBackend(t)
	repo := createTestRepo(t)

	err := b.CreateBranch(context.Background(), repo, "feature", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	err = b.CreateBranch(context.Background(), repo, "feature", "HEAD")
	if err == nil {
		t.Fatal("expected error for existing branch")
	}
	ge, ok := err.(*GitError)
	if !ok || ge.Err != ErrBranchExists {
		t.Fatalf("expected ErrBranchExists, got %v", err)
	}
}

func TestNative_Checkout(t *testing.T) {
	b := newTestNativeBackend(t)
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

func TestNative_GetStatus(t *testing.T) {
	b := newTestNativeBackend(t)
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
	if len(status.Untracked) != 1 {
		t.Fatalf("expected 1 untracked, got %d", len(status.Untracked))
	}
}

func TestNative_GetCommitsBetween(t *testing.T) {
	b := newTestNativeBackend(t)
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

func TestNative_IsAncestor(t *testing.T) {
	b := newTestNativeBackend(t)
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

	ok, err = b.IsAncestor(context.Background(), repo, secondHash, initHash)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected secondHash NOT to be ancestor of initHash")
	}
}

func TestNative_CreateTag(t *testing.T) {
	b := newTestNativeBackend(t)
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

func TestNative_GetFileAtRevision(t *testing.T) {
	b := newTestNativeBackend(t)
	repo := createTestRepo(t)

	content, err := b.GetFileAtRevision(context.Background(), repo, "README.md", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "init" {
		t.Fatalf("expected 'init', got %q", string(content))
	}
}

func TestNative_GetFileNotFound(t *testing.T) {
	b := newTestNativeBackend(t)
	repo := createTestRepo(t)

	_, err := b.GetFileAtRevision(context.Background(), repo, "nonexistent.txt", "HEAD")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNative_AddRemoveRemote(t *testing.T) {
	b := newTestNativeBackend(t)
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

func TestNative_FetchNonexistentRepo(t *testing.T) {
	b := newTestNativeBackend(t)

	_, err := b.Fetch(context.Background(), FetchOptions{
		RepoPath: "/nonexistent/path",
		Remote:   "origin",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFactory_NewGitBackend(t *testing.T) {
	backend, err := NewGitBackend(Options{Type: "native"})
	if err != nil {
		t.Fatal(err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestFactory_AutoDetect(t *testing.T) {
	backend, err := NewGitBackend(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestFactory_UnknownType(t *testing.T) {
	_, err := NewGitBackend(Options{Type: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

// captureLogger records Warn calls so tests can assert the auto-detect
// fallback is audible instead of silent.
type captureLogger struct {
	warnMsgs []string
}

func (l *captureLogger) Debug(string, ...any) {}
func (l *captureLogger) Info(string, ...any)  {}
func (l *captureLogger) Warn(msg string, _ ...any) {
	l.warnMsgs = append(l.warnMsgs, msg)
}
func (l *captureLogger) Error(string, ...any) {}

// TestNewAutoBackend_WarnsOnNativeFallback verifies the native->gogit
// degradation logs exactly one Warn, the success path logs none, a missing
// logger does not panic, and no backends at all still errors.
func TestNewAutoBackend_WarnsOnNativeFallback(t *testing.T) {
	gogitBackend := NewGoGitBackend(Options{})
	failing := func(Options) (GitBackend, error) { return nil, fmt.Errorf("git not installed") }
	succeeding := func(Options) (GitBackend, error) { return gogitBackend, nil }

	logger := &captureLogger{}
	backend, err := newAutoBackend(Options{Logger: logger}, failing, true, succeeding, true)
	if err != nil {
		t.Fatalf("newAutoBackend (fallback): %v", err)
	}
	if _, ok := backend.(*GoGitBackend); !ok {
		t.Errorf("expected the gogit backend after degradation, got %T", backend)
	}
	if len(logger.warnMsgs) != 1 {
		t.Fatalf("expected exactly one Warn on degradation, got %q", logger.warnMsgs)
	}
	if !strings.Contains(logger.warnMsgs[0], "gogit") {
		t.Errorf("expected the Warn to name the gogit fallback, got %q", logger.warnMsgs[0])
	}

	okLogger := &captureLogger{}
	if _, err := newAutoBackend(Options{Logger: okLogger}, succeeding, true, succeeding, true); err != nil {
		t.Fatalf("newAutoBackend (native ok): %v", err)
	}
	if len(okLogger.warnMsgs) != 0 {
		t.Errorf("expected no Warn when native constructs fine, got %q", okLogger.warnMsgs)
	}

	if _, err := newAutoBackend(Options{}, failing, true, succeeding, true); err != nil {
		t.Errorf("degradation without a Logger must not fail or panic, got %v", err)
	}

	if _, err := newAutoBackend(Options{}, nil, false, nil, false); err == nil {
		t.Error("expected an error when no backend is available")
	}
}

func TestBuildFetchRefSpecs_Empty(t *testing.T) {
	opts := FetchOptions{Remote: "origin"}
	specs := buildFetchRefSpecs(opts)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	expected := "+refs/heads/*:refs/remotes/origin/*"
	if string(specs[0]) != expected {
		t.Fatalf("expected %q, got %q", expected, string(specs[0]))
	}
}

func TestBuildFetchRefSpecs_Branches(t *testing.T) {
	opts := FetchOptions{Remote: "origin", Branches: []string{"main", "feature"}}
	specs := buildFetchRefSpecs(opts)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	expected := "+refs/heads/main:refs/remotes/origin/main"
	if string(specs[0]) != expected {
		t.Fatalf("expected %q, got %q", expected, string(specs[0]))
	}
	expected = "+refs/heads/feature:refs/remotes/origin/feature"
	if string(specs[1]) != expected {
		t.Fatalf("expected %q, got %q", expected, string(specs[1]))
	}
}

func TestBuildFetchRefSpecs_SHA(t *testing.T) {
	// SHA hashes should be skipped, fallback to all branches
	opts := FetchOptions{
		Remote:   "origin",
		Branches: []string{"abc123def456789012345678901234567890abcd"},
	}
	specs := buildFetchRefSpecs(opts)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec (fallback), got %d", len(specs))
	}
	expected := "+refs/heads/*:refs/remotes/origin/*"
	if string(specs[0]) != expected {
		t.Fatalf("expected %q, got %q", expected, string(specs[0]))
	}
}

func TestBuildFetchRefSpecs_MixedBranchAndSHA(t *testing.T) {
	// Mix of branch and SHA - should only include branch
	opts := FetchOptions{
		Remote:   "origin",
		Branches: []string{"main", "abc123def456789012345678901234567890abcd"},
	}
	specs := buildFetchRefSpecs(opts)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	expected := "+refs/heads/main:refs/remotes/origin/main"
	if string(specs[0]) != expected {
		t.Fatalf("expected %q, got %q", expected, string(specs[0]))
	}
}

func TestBuildFetchRefSpecs_WithRefPrefix(t *testing.T) {
	// Branch with full ref prefix
	opts := FetchOptions{Remote: "origin", Branches: []string{"refs/heads/main"}}
	specs := buildFetchRefSpecs(opts)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	expected := "+refs/heads/main:refs/remotes/origin/main"
	if string(specs[0]) != expected {
		t.Fatalf("expected %q, got %q", expected, string(specs[0]))
	}
}

func TestBuildNativeFetchRefSpecs_Branches(t *testing.T) {
	specs := buildNativeFetchRefSpecs("origin", []string{"main", "feature"})
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	expected := []string{
		"+refs/heads/main:refs/remotes/origin/main",
		"+refs/heads/feature:refs/remotes/origin/feature",
	}
	for i, want := range expected {
		if specs[i] != want {
			t.Fatalf("spec[%d]: expected %q, got %q", i, want, specs[i])
		}
	}
}

func TestBuildNativeFetchRefSpecs_SHA(t *testing.T) {
	// SHA hashes can't be refspecs - should only include the branch
	specs := buildNativeFetchRefSpecs("origin", []string{"main", "abc123def456789012345678901234567890abcd"})
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	expected := "+refs/heads/main:refs/remotes/origin/main"
	if specs[0] != expected {
		t.Fatalf("expected %q, got %q", expected, specs[0])
	}
}

func TestBuildNativeFetchRefSpecs_WithRefPrefix(t *testing.T) {
	specs := buildNativeFetchRefSpecs("origin", []string{"refs/heads/main"})
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	expected := "+refs/heads/main:refs/remotes/origin/main"
	if specs[0] != expected {
		t.Fatalf("expected %q, got %q", expected, specs[0])
	}
}

func TestBuildNativeFetchRefSpecs_SHAOnlyFallsBackToWildcard(t *testing.T) {
	// All-SHA input mirrors the gogit backend: fall back to the wildcard
	// refspec so the fetch still updates remote-tracking refs.
	specs := buildNativeFetchRefSpecs("origin", []string{"abc123def456789012345678901234567890abcd"})
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	expected := "+refs/heads/*:refs/remotes/origin/*"
	if specs[0] != expected {
		t.Fatalf("expected %q, got %q", expected, specs[0])
	}
}

// TestNative_FetchCheckedOutBranch reproduces the production failure behind
// issue #37: fetching a branch that is checked out in the non-bare clone used
// to die with "refusing to fetch into branch ... checked out", and even where
// it didn't, a bare branch fetch only wrote FETCH_HEAD and left the
// remote-tracking ref stale. Explicit refspecs must do both.
func TestNative_FetchCheckedOutBranch(t *testing.T) {
	b := newTestNativeBackend(t)
	upstream := createTestRepo(t)
	branch := gitOutput(t, upstream, "rev-parse", "--abbrev-ref", "HEAD")

	cloneDir := filepath.Join(t.TempDir(), "clone")
	if err := b.Clone(context.Background(), CloneOptions{
		URL:    upstream,
		Path:   cloneDir,
		Branch: branch,
	}); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Advance the remote branch while the clone keeps the same branch checked
	// out - the exact workspace shape the review service runs on.
	if err := os.WriteFile(filepath.Join(upstream, "advance.txt"), []byte("advance"), 0644); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, upstream, "add", ".")
	gitOutput(t, upstream, "commit", "-m", "advance")
	remoteSHA := gitOutput(t, upstream, "rev-parse", "HEAD")
	localSHA := gitOutput(t, cloneDir, "rev-parse", "HEAD")

	res, err := b.Fetch(context.Background(), FetchOptions{
		RepoPath: cloneDir,
		Remote:   "origin",
		Branches: []string{branch},
	})
	if err != nil {
		t.Fatalf("fetch must not refuse a checked-out branch: %v", err)
	}
	if got := gitOutput(t, cloneDir, "rev-parse", "refs/remotes/origin/"+branch); got != remoteSHA {
		t.Fatalf("remote-tracking ref not updated: got %s, want %s", got, remoteSHA)
	}
	if got := gitOutput(t, cloneDir, "rev-parse", "refs/heads/"+branch); got != localSHA {
		t.Fatalf("checked-out branch must stay untouched: got %s, want %s", got, localSHA)
	}
	// origin/HEAD (symbolic) rides along in the snapshot and reports itself
	// as updated too, so assert membership rather than an exact list.
	found := false
	for _, b := range res.UpdatedBranch {
		if b == branch {
			found = true
		}
	}
	if !found {
		t.Fatalf("FetchResult.UpdatedBranch = %v, want it to contain %s", res.UpdatedBranch, branch)
	}
}

func TestBuildNativeFetchRefSpecs_RefSpecPassthrough(t *testing.T) {
	// 含 ':' 的条目是调用方手写的完整 refspec：原样透传，不做分支名展开。
	specs := buildNativeFetchRefSpecs("origin", []string{"pull/1/head:refs/heads/pr-1"})
	if len(specs) != 1 || specs[0] != "pull/1/head:refs/heads/pr-1" {
		t.Fatalf("refspec must pass through verbatim, got %v", specs)
	}
}

// TestNative_FetchRefSpecIntoCheckedOutClone：PR head refspec 抓取在默认
// 分支已检出的工作区 clone 上必须照常工作（code-review-service 的生产形态）。
func TestNative_FetchRefSpecIntoCheckedOutClone(t *testing.T) {
	b := newTestNativeBackend(t)
	upstream := createTestRepo(t)
	branch := gitOutput(t, upstream, "rev-parse", "--abbrev-ref", "HEAD")

	cloneDir := filepath.Join(t.TempDir(), "clone")
	if err := b.Clone(context.Background(), CloneOptions{URL: upstream, Path: cloneDir, Branch: branch}); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// 用 refspec 把远端同名分支抓成本地 pr-1（模拟 PR head ref），
	// 不触碰检出的分支
	if _, err := b.Fetch(context.Background(), FetchOptions{
		RepoPath: cloneDir,
		Remote:   "origin",
		Branches: []string{branch + ":refs/heads/pr-1"},
	}); err != nil {
		t.Fatalf("refspec fetch must pass through verbatim: %v", err)
	}
	if got := gitOutput(t, cloneDir, "rev-parse", "refs/heads/pr-1"); got == "" {
		t.Fatal("pr-1 ref not created")
	}
}
