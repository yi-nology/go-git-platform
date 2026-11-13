package gitbackend

import (
	"context"
	"fmt"
	"strings"

	oekiterrors "github.com/oekit/pkg/errors"
)

// --- Core operations ---

func (b *NativeGitBackend) Fetch(ctx context.Context, opts FetchOptions) (*FetchResult, error) {
	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}
	if err := checkNameArg("Fetch", remote); err != nil {
		return nil, err
	}

	// Snapshot the refs a fetch can move before running it, so the result can
	// be derived from a before/after diff exactly like the gogit backend.
	before, err := b.snapshotFetchRefs(ctx, opts.RepoPath, remote)
	if err != nil {
		return nil, newGitError("Fetch", opts.RepoPath, "", err)
	}

	// --prune is always passed so DeletedBranch is actually reachable: without
	// pruning, a branch deleted on the remote leaves its remote-tracking ref
	// in place and the before/after diff shows no deletion.
	args := []string{"fetch", "--prune", remote}
	if opts.Tags {
		args = append(args, "--tags")
	} else {
		args = append(args, "--no-tags")
	}
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	if len(opts.Branches) > 0 {
		// Expand branches into explicit refspecs instead of passing them as
		// bare args. A bare "git fetch origin main" only writes FETCH_HEAD
		// (remote-tracking refs stay stale, so the before/after diff here
		// sees nothing) and is refused outright when the fetched branch is
		// the one checked out in this non-bare clone. Explicit refspecs
		// never touch the checked-out branch and update refs/remotes/, which
		// restores parity with the gogit backend's buildFetchRefSpecs.
		args = append(args, buildNativeFetchRefSpecs(remote, opts.Branches)...)
	}

	auth := mergeInsecure(opts.Auth, opts.InsecureSkipTLS)
	_, stderr, err := b.runGit(ctx, opts.RepoPath, args, auth)
	if err != nil {
		return nil, newGitError("Fetch", opts.RepoPath, stderr, err)
	}

	after, err := b.snapshotFetchRefs(ctx, opts.RepoPath, remote)
	if err != nil {
		return nil, newGitError("Fetch", opts.RepoPath, "", err)
	}
	return diffFetchRefs(remote, before, after), nil
}

// buildNativeFetchRefSpecs mirrors the gogit backend's buildFetchRefSpecs so
// both backends fetch branches identically: branch names expand to
// +refs/heads/<b>:refs/remotes/<remote>/<b>, SHA hashes are dropped (they
// can't be used as refspecs), and refs/-prefixed names keep their path below
// refs/heads/. If nothing usable remains, fall back to the full wildcard
// refspec so the fetch still updates remote-tracking refs.
func buildNativeFetchRefSpecs(remote string, branches []string) []string {
	specs := make([]string, 0, len(branches))
	for _, branch := range branches {
		if isCommitSHA(branch) {
			continue
		}
		// ':' 是 git refspec 的 src:dst 分隔符：含 ':' 的条目是调用方
		// 手写的完整 refspec（如 pull/1/head:refs/heads/pr-1），原样透传
		// ——分支名展开只适用于纯分支名。v0.116.0 收紧 Branches 语义时
		// 曾把这类条目也展开成 refs/heads/<refspec> 而炸掉旧调用方。
		if strings.Contains(branch, ":") {
			specs = append(specs, branch)
			continue
		}
		if strings.HasPrefix(branch, "refs/") {
			branchName := strings.TrimPrefix(branch, "refs/heads/")
			specs = append(specs, fmt.Sprintf("+%s:refs/remotes/%s/%s", branch, remote, branchName))
		} else {
			specs = append(specs, fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", branch, remote, branch))
		}
	}
	if len(specs) == 0 {
		return []string{fmt.Sprintf("+refs/heads/*:refs/remotes/%s/*", remote)}
	}
	return specs
}

// snapshotFetchRefs snapshots the refs a fetch can move — the remote's
// remote-tracking refs and all tags — as refname→hash. It mirrors the gogit
// backend's collectFetchRefs.
func (b *NativeGitBackend) snapshotFetchRefs(ctx context.Context, repoPath, remote string) (map[string]string, error) {
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{
		"for-each-ref", "--format=%(refname) %(objectname)",
		"refs/remotes/" + remote + "/", "refs/tags/",
	}, AuthConfig{})
	if err != nil {
		return nil, oekiterrors.Wrap(err, "git for-each-ref", "stderr", strings.TrimSpace(stderr))
	}
	refs := make(map[string]string)
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format is "%(refname) %(objectname)": refnames cannot contain
		// spaces, so everything before the last space is the refname.
		idx := strings.LastIndexByte(line, ' ')
		if idx <= 0 {
			continue
		}
		refs[line[:idx]] = line[idx+1:]
	}
	return refs, nil
}

func (b *NativeGitBackend) Push(ctx context.Context, opts PushOptions) (*PushResult, error) {
	if err := checkNameArg("Push", opts.Remote); err != nil {
		return nil, err
	}
	if err := checkNameArgs("Push", opts.RefSpecs...); err != nil {
		return nil, err
	}
	args := []string{"push", opts.Remote}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Mirror {
		args = []string{"push", "--mirror", opts.Remote}
	} else {
		args = append(args, opts.RefSpecs...)
	}

	auth := mergeInsecure(opts.Auth, opts.InsecureSkipTLS)
	stdout, stderr, err := b.runGit(ctx, opts.RepoPath, args, auth)
	if err != nil {
		return nil, newGitError("Push", opts.RepoPath, stderr, err)
	}
	out := stdout + stderr
	pushed := parsePushRefs(out)
	// 解析不出任何 ref 且远端确实更新了（非 up-to-date）时，回填请求的
	// refspecs（归一成 dst 短名，与解析口径一致）
	if len(pushed) == 0 && !strings.Contains(out, "Everything up-to-date") &&
		len(opts.RefSpecs) > 0 && !opts.Mirror {
		pushed = pushedRefsFromRefSpecs(opts.RefSpecs)
	}
	return &PushResult{PushedRefs: pushed}, nil
}

func (b *NativeGitBackend) Clone(ctx context.Context, opts CloneOptions) error {
	if err := checkNameArgs("Clone", opts.URL, opts.Path); err != nil {
		return err
	}
	args := []string{"clone"}
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	if opts.NoCheckout {
		args = append(args, "--no-checkout")
	}
	if opts.SingleBranch {
		args = append(args, "--single-branch")
	}
	// 部分克隆:blob:none / tree:0,显著降冷备/首次同步体积
	if f := strings.TrimSpace(opts.Filter); f != "" {
		args = append(args, "--filter", f)
	}
	if opts.Submodules {
		args = append(args, "--recurse-submodules")
	}
	// Reference clone（issue #148）：--reference <path> 让 git 经 alternates
	// 借用共享仓对象库（网络只拉差异、磁盘只增元数据），--dissociate 克隆后
	// 复制对象断开借用。参数直通 git；前置校验走 validateCloneReference
	// （无效路径 fail-fast，不静默退化成普通克隆）。
	if err := validateCloneReference(opts); err != nil {
		// newGitError 契约(backend.go):两后端错误同形——native 校验失败
		// 也要可 errors.As(*GitError),不能裸 Wrap(#152)。
		return newGitError("Clone", opts.Path, "", err)
	}
	if opts.Reference != "" {
		args = append(args, "--reference", opts.Reference)
		if opts.Dissociate {
			args = append(args, "--dissociate")
		}
	}
	args = append(args, opts.URL, opts.Path)

	auth := mergeInsecure(opts.Auth, opts.InsecureSkipTLS)
	_, stderr, err := b.runGit(ctx, "", args, auth)
	if err != nil {
		return newGitError("Clone", opts.Path, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) Init(ctx context.Context, repoPath string) error {
	_, stderr, err := b.runGit(ctx, repoPath, []string{"init"}, AuthConfig{})
	if err != nil {
		return newGitError("Init", repoPath, stderr, err)
	}
	return nil
}

// --- Extended core operations ---

func (b *NativeGitBackend) FetchAll(ctx context.Context, repoPath string, auth AuthConfig) error {
	// 逐 remote 单独 fetch（`git fetch --all` 的内部语义就是按序遍历），
	// 而不是单条 `git fetch --all`：后者一次调用会向多个 remote 各自的
	// 主机请求凭据，而凭证会话只能钉一个信任域——多 remote 混合信任域时
	// 为 A 准备的令牌会被发给 B（issue #106）。逐 remote 调用让每次的
	// credentialHostsFor 都能解析出当前 remote 自己的主机。
	remotes, err := b.GetRemotes(ctx, repoPath)
	if err != nil {
		return err
	}
	for _, remote := range remotes {
		_, stderr, err := b.runGit(ctx, repoPath, []string{"fetch", "--prune", "--tags", remote}, auth)
		if err != nil {
			return newGitError("FetchAll", repoPath, stderr, err)
		}
	}
	return nil
}

func (b *NativeGitBackend) Pull(ctx context.Context, repoPath, remote, branch string, auth AuthConfig) error {
	if err := checkNameArgs("Pull", remote, branch); err != nil {
		return err
	}
	args := []string{"pull", remote}
	if branch != "" {
		args = append(args, branch)
	}
	_, stderr, err := b.runGit(ctx, repoPath, args, auth)
	if err != nil {
		return newGitError("Pull", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) RunRaw(ctx context.Context, repoPath string, args []string) (string, string, error) {
	return b.runGit(ctx, repoPath, args, AuthConfig{})
}

func (b *NativeGitBackend) TestConnection(ctx context.Context, url string, auth AuthConfig) error {
	if err := checkNameArg("TestConnection", url); err != nil {
		return err
	}
	_, stderr, err := b.runGit(ctx, "", []string{"ls-remote", "--heads", url}, auth)
	if err != nil {
		return newGitError("TestConnection", "", stderr, err)
	}
	return nil
}
