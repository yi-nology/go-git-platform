package gitbackend

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	xhttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// bareOrigin creates a bare repo (HEAD on main) and returns its path.
func bareOrigin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out, err := exec.Command("git", "init", "--bare", "--initial-branch=main", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	return dir
}

// pushSetup seeds a bare origin with main, clones it, and commits one local
// change in the clone (unpushed). The seed working repo is returned for
// tests that need to advance origin.
func pushSetup(t *testing.T) (origin, seed, clone string) {
	t.Helper()
	origin = bareOrigin(t)
	seed = t.TempDir()
	gitOutput(t, seed, "init", "--initial-branch=main")
	gitOutput(t, seed, "config", "user.email", "test@test.com")
	gitOutput(t, seed, "config", "user.name", "Test")
	gitOutput(t, seed, "remote", "add", "origin", origin)
	{
		cmd := exec.Command("git", "-C", seed, "commit", "--allow-empty", "-m", "seed")
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("seed commit: %v\n%s", err, out)
		}
	}
	gitOutput(t, seed, "push", "-q", "origin", "main")

	clone = t.TempDir()
	cloneRepo(t, origin, clone)
	commitFile(t, clone, "local.txt", "local work", "local commit")
	return origin, seed, clone
}

func TestGoGit_Clone(t *testing.T) {
	b := newTestGoGitBackend(t)
	origin, _, _ := pushSetup(t)

	dst := filepath.Join(t.TempDir(), "clone")
	if err := b.Clone(context.Background(), CloneOptions{URL: origin, Path: dst}); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		t.Errorf("expected a cloned repository at %s: %v", dst, err)
	}
}

func TestGoGit_Push_AndIdempotentRepush(t *testing.T) {
	b := newTestGoGitBackend(t)
	origin, _, clone := pushSetup(t)

	res, err := b.Push(context.Background(), PushOptions{RepoPath: clone, Remote: "origin"})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(res.PushedRefs) == 0 {
		t.Error("expected the push result to report pushed refs")
	}
	if got := gitOutput(t, origin, "log", "-1", "--pretty=%s"); got != "local commit" {
		t.Errorf("expected the local commit on origin after push, got %q", got)
	}

	// Pushing again with nothing new must be an idempotent success, not an error.
	if _, err := b.Push(context.Background(), PushOptions{RepoPath: clone, Remote: "origin"}); err != nil {
		t.Errorf("re-push with no new commits: %v", err)
	}
}

func TestGoGit_Fetch_ClassifiesNewAndUpdated(t *testing.T) {
	b := newTestGoGitBackend(t)
	origin := bareOrigin(t)
	seed := t.TempDir()
	gitOutput(t, seed, "init", "--initial-branch=main")
	gitOutput(t, seed, "config", "user.email", "test@test.com")
	gitOutput(t, seed, "config", "user.name", "Test")
	gitOutput(t, seed, "remote", "add", "origin", origin)
	commitFile(t, seed, "a.txt", "one", "first")
	gitOutput(t, seed, "push", "-q", "origin", "main")

	clone := t.TempDir()
	cloneRepo(t, origin, clone)

	// Origin gains a new branch and moves main forward.
	gitOutput(t, seed, "branch", "feature")
	commitFile(t, seed, "a.txt", "two", "second")
	gitOutput(t, seed, "push", "-q", "origin", "main", "feature")

	// No Remote on the options: the default origin path must work.
	res, err := b.Fetch(context.Background(), FetchOptions{RepoPath: clone})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !contains(res.NewBranches, "feature") {
		t.Errorf("expected NewBranches to report feature, got %+v", res)
	}
	if !contains(res.UpdatedBranch, "main") {
		t.Errorf("expected UpdatedBranch to report main, got %+v", res)
	}
	if !contains(res.FetchedRefs, "refs/remotes/origin/feature") {
		t.Errorf("expected FetchedRefs to carry the new remote-tracking ref, got %+v", res)
	}
}

func TestGoGit_Fetch_Tags(t *testing.T) {
	b := newTestGoGitBackend(t)
	_, seed, clone := pushSetup(t)
	gitOutput(t, seed, "tag", "v1")
	gitOutput(t, seed, "push", "-q", "origin", "v1")

	res, err := b.Fetch(context.Background(), FetchOptions{RepoPath: clone, Tags: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !contains(res.NewTags, "v1") {
		t.Errorf("expected NewTags to report v1, got %+v", res)
	}
}

// TestGoGit_Fetch_DeletedBranchShortName verifies a remote-tracking branch
// pruned away by the fetch is reported with its SHORT name, matching the
// native backend (it used to carry the full refs/remotes/... refname).
func TestGoGit_Fetch_DeletedBranchShortName(t *testing.T) {
	b := newTestGoGitBackend(t)
	origin, seed, clone := pushSetup(t)
	gitOutput(t, seed, "push", "-q", "origin", "main:feature")

	// Make the clone see feature, then delete it on the remote.
	if _, err := b.Fetch(context.Background(), FetchOptions{RepoPath: clone}); err != nil {
		t.Fatalf("Fetch (first): %v", err)
	}
	gitOutput(t, origin, "update-ref", "-d", "refs/heads/feature")

	res, err := b.Fetch(context.Background(), FetchOptions{RepoPath: clone, Prune: true})
	if err != nil {
		t.Fatalf("Fetch (after remote delete): %v", err)
	}
	if !contains(res.DeletedBranch, "feature") {
		t.Errorf("expected DeletedBranch to report the short name feature, got %+v", res)
	}
	if contains(res.FetchedRefs, "refs/remotes/origin/feature") {
		t.Errorf("expected the pruned ref not to be reported as fetched, got %+v", res)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestGoGit_Pull(t *testing.T) {
	b := newTestGoGitBackend(t)
	origin := bareOrigin(t)
	seed := t.TempDir()
	gitOutput(t, seed, "init", "--initial-branch=main")
	gitOutput(t, seed, "config", "user.email", "test@test.com")
	gitOutput(t, seed, "config", "user.name", "Test")
	gitOutput(t, seed, "remote", "add", "origin", origin)
	commitFile(t, seed, "a.txt", "one", "first")
	gitOutput(t, seed, "push", "-q", "origin", "main")

	clone := t.TempDir()
	cloneRepo(t, origin, clone) // clean clone, strictly behind after the push below

	tip := commitFile(t, seed, "remote.txt", "remote work", "remote commit")
	gitOutput(t, seed, "push", "-q", "origin", "main")

	if err := b.Pull(context.Background(), clone, "origin", "main", AuthConfig{}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got := headHash(t, clone); got != tip {
		t.Errorf("expected clone HEAD at %s after pull, got %s", tip, got)
	}
	if _, err := os.Stat(filepath.Join(clone, "remote.txt")); err != nil {
		t.Errorf("expected remote.txt checked out by the pull: %v", err)
	}
}

func TestGoGit_FetchAll(t *testing.T) {
	b := newTestGoGitBackend(t)
	_, _, clone := pushSetup(t)
	if err := b.FetchAll(context.Background(), clone, AuthConfig{}); err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
}

func TestGoGit_TestConnection_LocalURL(t *testing.T) {
	b := newTestGoGitBackend(t)
	origin, _, _ := pushSetup(t)
	if err := b.TestConnection(context.Background(), origin, AuthConfig{}); err != nil {
		t.Errorf("TestConnection to a local origin: %v", err)
	}
	if err := b.TestConnection(context.Background(), filepath.Join(t.TempDir(), "missing"), AuthConfig{}); err == nil {
		t.Error("expected TestConnection to fail for a missing repo")
	}
}

func TestGoGit_RunRaw_Unsupported(t *testing.T) {
	b := newTestGoGitBackend(t)
	_, _, clone := pushSetup(t)
	_, _, err := b.RunRaw(context.Background(), clone, []string{"status"})
	if err == nil || !strings.Contains(err.Error(), "native backend") {
		t.Errorf("expected the unsupported error pointing at the native backend, got %v", err)
	}
}

func TestGoGit_Remotes(t *testing.T) {
	b := newTestGoGitBackend(t)
	origin, _, clone := pushSetup(t)

	url, err := b.GetRemoteURL(context.Background(), clone, "origin")
	if err != nil {
		t.Fatalf("GetRemoteURL: %v", err)
	}
	if url != origin {
		t.Errorf("expected origin URL %q, got %q", origin, url)
	}
	names, err := b.GetRemotes(context.Background(), clone)
	if err != nil || !contains(names, "origin") {
		t.Errorf("expected remotes to include origin, got %v (%v)", names, err)
	}
	if _, err := b.GetRemoteURL(context.Background(), clone, "nonexistent"); err == nil || !IsNotFound(err) {
		t.Errorf("missing remote: expected a NotFound error, got %v", err)
	}
}

func TestGoGit_Tags(t *testing.T) {
	b := newTestGoGitBackend(t)
	origin, _, clone := pushSetup(t)
	gitOutput(t, clone, "tag", "v1")

	tags, err := b.GetTagList(context.Background(), clone)
	if err != nil {
		t.Fatalf("GetTagList: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "v1" || tags[0].Author != "Test" {
		t.Errorf("unexpected tag list: %+v", tags)
	}

	if err := b.PushTag(context.Background(), clone, "origin", "v1", AuthConfig{}); err != nil {
		t.Fatalf("PushTag: %v", err)
	}
	if got := gitOutput(t, origin, "tag"); got != "v1" {
		t.Errorf("expected v1 on origin, got %q", got)
	}

	if err := b.DeleteTag(context.Background(), clone, "v1"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	tags, err = b.GetTagList(context.Background(), clone)
	if err != nil || len(tags) != 0 {
		t.Errorf("expected no tags after deletion, got %+v (%v)", tags, err)
	}
}

func TestBuildTransportAuth(t *testing.T) {
	if got, err := buildTransportAuth(AuthConfig{Type: AuthNone}); got != nil || err != nil {
		t.Errorf("AuthNone: expected (nil, nil), got %T, %v", got, err)
	}
	basic, err := buildTransportAuth(AuthConfig{Type: AuthHTTPBasic, Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("AuthHTTPBasic: unexpected error %v", err)
	}
	if ba, ok := basic.(*xhttp.BasicAuth); !ok || ba.Username != "u" || ba.Password != "p" {
		t.Errorf("AuthHTTPBasic: unexpected %T %+v", basic, basic)
	}

	// Invalid key content and missing key files must surface the real cause
	// wrapped in ErrAuthFailed — silently falling back to anonymous auth
	// used to masquerade as "authentication required".
	if _, err := buildTransportAuth(AuthConfig{Type: AuthSSH, SSHKeyContent: "not a key"}); !errors.Is(err, ErrAuthFailed) {
		t.Errorf("invalid SSHKeyContent: err = %v, want ErrAuthFailed", err)
	}
	if _, err := buildTransportAuth(AuthConfig{Type: AuthSSH, SSHKey: "/nonexistent/id_ed25519"}); !errors.Is(err, ErrAuthFailed) {
		t.Errorf("missing SSHKey file: err = %v, want ErrAuthFailed", err)
	}
}

func TestInsecureHostKeyCallbackAcceptsAnyHost(t *testing.T) {
	// The callback backs SkipTLS-style SSH against self-hosted forges: it
	// must accept an arbitrary-looking host key without consulting known
	// hosts — but only when the caller explicitly asked for insecure mode.
	cb, err := hostKeyCallbackWithConfig(AuthConfig{InsecureSkipTLS: true})
	if err != nil {
		t.Fatalf("hostKeyCallbackWithConfig(insecure): %v", err)
	}
	if err := cb("git.example.com:22", nil, nil); err != nil {
		t.Errorf("expected the insecure callback to accept any host key, got %v", err)
	}

	// Secure default: a missing known_hosts file must fail loudly instead
	// of silently trusting unknown hosts.
	if _, err := hostKeyCallbackWithConfig(AuthConfig{}); err == nil {
		t.Skip("this environment has ~/.ssh/known_hosts; nothing to assert")
	}
}
