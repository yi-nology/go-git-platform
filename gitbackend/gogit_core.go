package gitbackend

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// --- Core operations ---

func (b *GoGitBackend) Fetch(ctx context.Context, opts FetchOptions) (*FetchResult, error) {
	repo, err := openRepo("Fetch", opts.RepoPath)
	if err != nil {
		return nil, err
	}

	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}
	// The defaulted name must reach the refspecs too: building them from
	// opts.Remote while it is empty maps refs onto refs/remotes//*, which
	// escapes the reference storage and fails the fetch outright.
	opts.Remote = remote

	// Snapshot the refs a fetch can move before running it.
	before := make(map[string]plumbing.Hash)
	if err := collectFetchRefs(repo, remote, before); err != nil {
		return nil, newGitError("Fetch", opts.RepoPath, "", err)
	}

	am, err := buildTransportAuth(opts.Auth)
	if err != nil {
		return nil, newGitError("Fetch", opts.RepoPath, "", err)
	}

	fetchOpts := &git.FetchOptions{
		RemoteName:      remote,
		RefSpecs:        buildFetchRefSpecs(opts),
		Auth:            am,
		Progress:        opts.Progress,
		Tags:            git.NoTags,
		Prune:           opts.Prune,
		Depth:           opts.Depth,
		InsecureSkipTLS: opts.InsecureSkipTLS || opts.Auth.InsecureSkipTLS,
	}
	if opts.Tags {
		fetchOpts.Tags = git.AllTags
	}

	err = repo.FetchContext(ctx, fetchOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, newGitError("Fetch", opts.RepoPath, "", err)
	}

	// Snapshot the refs after the fetch.
	after := make(map[string]plumbing.Hash)
	if err := collectFetchRefs(repo, remote, after); err != nil {
		return nil, newGitError("Fetch", opts.RepoPath, "", err)
	}

	// Diff before/after through the shared classifier (one implementation
	// for both backends). The fetch refspecs map refs/heads/* onto
	// refs/remotes/<remote>/* (see buildFetchRefSpecs) and AllTags writes
	// refs/tags/*, so branch and tag classification key off those two
	// namespaces.
	return diffFetchRefs(remote, hashRefMap(before), hashRefMap(after)), nil
}

// hashRefMap converts a plumbing.Hash ref snapshot into its string-hash form,
// the shape the shared diffFetchRefs classifier consumes.
func hashRefMap(m map[string]plumbing.Hash) map[string]string {
	out := make(map[string]string, len(m))
	for ref, h := range m {
		out[ref] = h.String()
	}
	return out
}

// collectRemoteRefs populates the target map with all remote-tracking refs
// (refs/remotes/<remote>/*) and their current hashes.
func collectRemoteRefs(repo *git.Repository, remote string, target map[string]plumbing.Hash) error {
	prefix := "refs/remotes/" + remote + "/"
	iter, err := repo.References()
	if err != nil {
		return err
	}
	return iter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if strings.HasPrefix(name, prefix) {
			target[name] = ref.Hash()
		}
		return nil
	})
}

// collectFetchRefs snapshots the refs a fetch can move: the remote's
// remote-tracking refs and refs/tags/* (AllTags writes there).
func collectFetchRefs(repo *git.Repository, remote string, target map[string]plumbing.Hash) error {
	if err := collectRemoteRefs(repo, remote, target); err != nil {
		return err
	}
	iter, err := repo.Tags()
	if err != nil {
		return err
	}
	return iter.ForEach(func(ref *plumbing.Reference) error {
		target[ref.Name().String()] = ref.Hash()
		return nil
	})
}

func (b *GoGitBackend) Push(ctx context.Context, opts PushOptions) (*PushResult, error) {
	repo, err := openRepo("Push", opts.RepoPath)
	if err != nil {
		return nil, err
	}

	refSpecs := opts.RefSpecs
	if opts.Mirror {
		refSpecs = []string{"+refs/*:refs/*"}
	}
	if len(refSpecs) == 0 {
		refSpecs = []string{"refs/heads/*:refs/heads/*"}
	}

	rs := make([]config.RefSpec, 0, len(refSpecs))
	for _, s := range refSpecs {
		rs = append(rs, config.RefSpec(s))
	}

	am, err := buildTransportAuth(opts.Auth)
	if err != nil {
		return nil, newGitError("Push", opts.RepoPath, "", err)
	}

	pushOpts := &git.PushOptions{
		RemoteName:      opts.Remote,
		RefSpecs:        rs,
		Auth:            am,
		Progress:        opts.Progress,
		Force:           opts.Force,
		InsecureSkipTLS: opts.InsecureSkipTLS || opts.Auth.InsecureSkipTLS,
	}

	err = repo.PushContext(ctx, pushOpts)
	if err != nil {
		// "already up-to-date" 表示远端已与本地一致(无变化),是幂等成功,
		// 不是错误;否则重复同步同一 commit 会被误判为失败。
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return &PushResult{PushedRefs: nil}, nil
		}
		return nil, newGitError("Push", opts.RepoPath, "", err)
	}
	// go-git's PushContext does not expose per-ref success/failure details,
	// so we return the requested refspecs as PushedRefs. Callers that need
	// granular per-ref error reporting should use the native backend.
	return &PushResult{PushedRefs: refSpecs}, nil
}

func (b *GoGitBackend) Clone(ctx context.Context, opts CloneOptions) error {
	am, err := buildTransportAuth(opts.Auth)
	if err != nil {
		return newGitError("Clone", opts.Path, "", err)
	}
	cloneOpts := &git.CloneOptions{
		URL:             opts.URL,
		ReferenceName:   plumbing.ReferenceName(opts.Branch),
		Progress:        opts.Progress,
		Auth:            am,
		NoCheckout:      opts.NoCheckout,
		SingleBranch:    opts.SingleBranch,
		InsecureSkipTLS: opts.InsecureSkipTLS || opts.Auth.InsecureSkipTLS,
	}
	if opts.Depth > 0 {
		cloneOpts.Depth = opts.Depth
	}

	_, err = git.PlainCloneContext(ctx, opts.Path, false, cloneOpts)
	if err != nil {
		return newGitError("Clone", opts.Path, "", err)
	}
	return nil
}

func (b *GoGitBackend) Init(ctx context.Context, repoPath string) error {
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		return newGitError("Init", repoPath, "", err)
	}
	return nil
}

// --- Extended core operations ---

func (b *GoGitBackend) FetchAll(ctx context.Context, repoPath string, auth AuthConfig) error {
	repo, err := openRepo("FetchAll", repoPath)
	if err != nil {
		return err
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return newGitError("FetchAll", repoPath, "", err)
	}
	for _, r := range remotes {
		am, err := buildTransportAuth(auth)
		if err != nil {
			return newGitError("FetchAll", repoPath, "", err)
		}
		err = repo.FetchContext(ctx, &git.FetchOptions{
			RemoteName:      r.Config().Name,
			Auth:            am,
			Tags:            git.AllTags,
			InsecureSkipTLS: auth.InsecureSkipTLS,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return newGitError("FetchAll", repoPath, "", err)
		}
	}
	return nil
}

func (b *GoGitBackend) Pull(ctx context.Context, repoPath, remote, branch string, auth AuthConfig) error {
	repo, err := openRepo("Pull", repoPath)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return newGitError("Pull", repoPath, "", err)
	}
	am, err := buildTransportAuth(auth)
	if err != nil {
		return newGitError("Pull", repoPath, "", err)
	}
	err = wt.PullContext(ctx, &git.PullOptions{
		RemoteName:      remote,
		ReferenceName:   plumbing.ReferenceName("refs/heads/" + branch),
		Auth:            am,
		InsecureSkipTLS: auth.InsecureSkipTLS,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return newGitError("Pull", repoPath, "", err)
	}
	return nil
}

// RunRaw is not supported by the pure-Go (gogit) backend because go-git does
// not shell out to the git binary. This method exists to satisfy the
// GitBackend interface; callers that need arbitrary git commands should use
// the native backend instead.
func (b *GoGitBackend) RunRaw(ctx context.Context, repoPath string, args []string) (string, string, error) {
	if _, err := openRepo("RunRaw", repoPath); err != nil {
		return "", "", err
	}
	return "", "", newGitError("RunRaw", repoPath, "", fmt.Errorf("RunRaw not supported in gogit backend (pure-Go); use native backend for arbitrary git commands"))
}

func (b *GoGitBackend) TestConnection(ctx context.Context, url string, auth AuthConfig) error {
	remote := git.NewRemote(nil, &config.RemoteConfig{
		Name: "test",
		URLs: []string{url},
	})
	am, err := buildTransportAuth(auth)
	if err != nil {
		return newGitError("TestConnection", "", "", err)
	}
	_, err = remote.List(&git.ListOptions{
		Auth:            am,
		InsecureSkipTLS: auth.InsecureSkipTLS,
	})
	if err != nil {
		return newGitError("TestConnection", "", "", err)
	}
	return nil
}
