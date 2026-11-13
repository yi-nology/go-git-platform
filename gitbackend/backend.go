package gitbackend

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	oekiterrors "github.com/oekit/pkg/errors"
)

// This file defines the public surface of the gitbackend package: the
// authentication model, the option/result types and the GitBackend interface
// that every backend implementation must satisfy.
//
// Two implementations are provided out of the box:
//   - "native": shells out to the local git binary (full feature coverage).
//   - "gogit":  pure-Go implementation based on go-git/v5 (no git binary
//     required, but some operations such as rebase/stash/raw are stubbed).
//
// Backends are created through the factory (NewGitBackend) which auto-selects
// native when available and falls back to gogit.

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// AuthType represents the authentication method used for a git operation.
type AuthType string

const (
	// AuthNone disables authentication (public repositories, local repos).
	AuthNone AuthType = "none"
	// AuthHTTPBasic authenticates over HTTPS using HTTP Basic. The username is
	// an arbitrary placeholder and the secret is carried in Password.
	AuthHTTPBasic AuthType = "http_basic"
	// AuthHTTPToken authenticates over HTTPS using a bearer/token sent as the
	// password of HTTP Basic auth (the convention used by most git hosts).
	AuthHTTPToken AuthType = "http_token"
	// AuthSSH authenticates over the SSH protocol using a private key.
	AuthSSH AuthType = "ssh"
)

// AuthConfig holds authentication credentials for git operations. Build one
// with the New*Auth helpers in auth.go when possible.
type AuthConfig struct {
	// Type selects the authentication method. AuthNone performs no auth.
	Type AuthType

	// Username is the HTTP Basic username (ignored for SSH).
	Username string
	// Password is the HTTP Basic password (used together with Username).
	Password string
	// Token is the HTTPS access token (used by AuthHTTPToken).
	Token string

	// SSHKey is the on-disk path to an SSH private key.
	SSHKey string

	// SSHKeyContent holds the raw PEM content of an SSH private key.
	// When set (and SSHKey is empty), a temporary key file is created
	// automatically for the git operation. This is useful for keys stored
	// in a database rather than on disk.
	SSHKeyContent string

	// Passphrase is the passphrase for decrypting the SSH private key.
	// Used together with SSHKey or SSHKeyContent.
	Passphrase string

	// InsecureSkipTLS disables TLS certificate verification for HTTPS
	// operations (equivalent to http.sslVerify=false). For SSH it also
	// disables host-key verification (explicit opt-out).
	// Carried on AuthConfig so every network operation that
	// takes auth (Fetch, Push, Clone, Pull, FetchAll, PushTag,
	// TestConnection, ...) honors it uniformly.
	InsecureSkipTLS bool

	// KnownHostsPath 指定 known_hosts 文件(空=用 ~/.ssh/known_hosts)。
	// 平台级独立 known_hosts 时使用,避免全局信任面过大。
	KnownHostsPath string

	// HostKeyFingerprint 期望的主机公钥指纹("SHA256:xxx" 或裸 base64)。
	// 设置后:连接时校验服务器公钥指纹必须一致,否则拒绝(防 MITM)。
	// 优先级高于 known_hosts 文件匹配。
	HostKeyFingerprint string
}

// ---------------------------------------------------------------------------
// Option types
// ---------------------------------------------------------------------------

// FetchOptions contains options for fetching from a remote.
type FetchOptions struct {
	// RepoPath is the local working tree to operate on.
	RepoPath string
	// Remote is the remote name to fetch from. Defaults to "origin" when
	// empty.
	Remote string
	// Branches limits the fetch to the given branch/ref names. When empty all
	// branches are fetched. Full 40-char SHAs are ignored as they are not
	// valid fetch refspecs.
	Branches []string
	// Tags fetches all tags when true.
	Tags bool
	// Prune removes remote-tracking refs that no longer exist on the remote.
	// Note: the native backend always passes --prune so that deleted branches
	// actually surface in FetchResult.DeletedBranch; the gogit backend prunes
	// only when this flag is set.
	Prune bool
	// Depth limits the fetch to the given number of commits (shallow fetch).
	// Zero means no depth limit.
	Depth           int
	InsecureSkipTLS bool
	Auth            AuthConfig
	// Progress receives human-readable progress output (optional).
	Progress io.Writer
}

// PushOptions contains options for pushing to a remote.
type PushOptions struct {
	RepoPath string
	// Remote is the remote name to push to (e.g. "origin").
	Remote string
	// RefSpecs are the refs to push (e.g. "refs/heads/main:refs/heads/main").
	// Ignored when Mirror is true.
	RefSpecs []string
	// Force forces the push (overwrites remote refs).
	Force bool
	// Mirror pushes all refs as a mirror (--mirror).
	Mirror          bool
	InsecureSkipTLS bool
	Auth            AuthConfig
	// Progress receives human-readable progress output (optional).
	Progress io.Writer
}

// CloneOptions contains options for cloning a repository.
type CloneOptions struct {
	// URL is the remote URL (HTTPS or SSH) to clone from.
	URL string
	// Path is the destination directory for the new working tree.
	Path string
	// Branch checks out the given branch after clone (optional).
	Branch string
	// Depth creates a shallow clone with the given history depth.
	// Zero means a full clone.
	Depth int
	Auth  AuthConfig
	// Progress receives human-readable progress output (optional).
	Progress        io.Writer
	NoCheckout      bool
	SingleBranch    bool
	InsecureSkipTLS bool
	// Filter 部分克隆:"blob:none" / "tree:0" 等;空=全量
	Filter string
	// Submodules clone 后递归初始化子模块
	Submodules bool
	// Reference 本地仓库路径：以该仓库为对象库共享源（等价 git clone
	// --reference <path>，workdir 经 objects/info/alternates 借用其对象，
	// 网络只拉共享仓没有的新对象、磁盘只增元数据）。空 = 无共享（现状）。
	// 指向的目录不存在或非 git 仓 ⇒ 明确报错（ErrNotAGitRepo），不静默
	// 退化成普通克隆（issue #148）。
	//
	// 生命周期警示（探针实测 2026-11-13）：workdir 的 objects/info/alternates
	// 指向共享仓；**共享仓被删除后 fetch 并不失败**（exit 0），对象访问时
	// 打 "unable to normalize alternate object path" 告警并把整段历史重新
	// 下载进 workdir 自有对象库（省流效果归零），仅存在于共享仓的旧对象
	// 读取失败，悬空文件持续告警。需要自包含产物时用 Dissociate；否则由
	// 调用方管理共享仓生命周期——推荐执行前显式检查 alternates 目标存在
	// 性、悬空即清 workdir 全量重建（git-ferry-core cloneFetchStage 即此
	// 模式；Clone Cache 场景即 sync.clone_cache_dir 的职责）。
	//
	// 仅 native 后端支持（gogit 报 ErrReferenceUnsupported，见该哨兵注释）。
	Reference string
	// Dissociate 克隆后断开 alternates、把借用的对象复制进自身（等价
	// --dissociate），产物自包含；仅在 Reference 非空时有意义，单独为真
	// 报错（ErrInvalidGitArg）。
	Dissociate bool
}

// validateCloneReference 在任何后端动手之前统一执行 issue #148 的契约：
//   - Dissociate 脱离 Reference 单独为真是无效组合（git 的 --dissociate
//     只与 --reference 配套）；
//   - Reference 必须指向真实 git 仓形状——缺失/非仓 fail-fast
//     （ErrNotAGitRepo），绝不静默退化成普通克隆。
//
// 形状判定按真实仓结构收深（审计 #153：只查「有 HEAD 文件」的浅探针会被
// 伪造 HEAD 的空壳穿透，穿透后 git 自己报错、哨兵丢失）：
//   - bare：顶层 HEAD 文件 + objects/ + refs/ 三件齐（clone --bare/--mirror
//     /--no-checkout 产物皆此形）；
//   - worktree：.git/ 子目录含 HEAD；
//   - gitfile：.git 是文件且 gitdir: 指向的目标含 HEAD（被掏空共享仓的
//     残骸形态在此拒绝）。
//
// 共享校验保证两个后端对同一份非法 CloneOptions 给出同形错误
// （errors.Is 哨兵 + errors.As *GitError，见 newGitError 契约）。
func validateCloneReference(opts CloneOptions) error {
	if opts.Dissociate && opts.Reference == "" {
		return oekiterrors.Wrap(ErrInvalidGitArg, "--dissociate requires Reference")
	}
	if opts.Reference == "" {
		return nil
	}
	refPath := opts.Reference
	st, err := os.Stat(refPath)
	if err != nil || !st.IsDir() {
		return oekiterrors.Wrap(ErrNotAGitRepo, "reference repository does not exist", "path", refPath)
	}
	// bare 形状:HEAD 文件 + objects/ + refs/ 三件齐
	if isPath(filepath.Join(refPath, "HEAD")) && isDir(filepath.Join(refPath, "objects")) && isDir(filepath.Join(refPath, "refs")) {
		return nil
	}
	// worktree 形状:.git/HEAD
	if isPath(filepath.Join(refPath, ".git", "HEAD")) {
		return nil
	}
	// gitfile 形状:.git 文件解析 gitdir:,目标含 HEAD
	if target, ok := readGitdirTarget(refPath); ok && isPath(filepath.Join(target, "HEAD")) {
		return nil
	}
	return oekiterrors.Wrap(ErrNotAGitRepo, "reference path is not a git repository", "path", refPath)
}

// isPath 判断路径存在(文件或目录均可)。
func isPath(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// readGitdirTarget 解析 worktree/.git 文件里的 "gitdir: <path>" 行;
// 相对路径按 git 规范相对该文件所在目录。返回 (解析后的绝对/相对目标, 是否成功)。
func readGitdirTarget(worktree string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(worktree, ".git"))
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(worktree, target)
	}
	return target, true
}

// MergeOptions contains options for merging a branch into HEAD.
type MergeOptions struct {
	// Message overrides the auto-generated merge commit message.
	Message string
	// Squash merges the branch as a single squashed change without
	// recording merge ancestry.
	Squash bool
	// NoCommit performs the merge but leaves the result staged without
	// committing.
	NoCommit bool
	// FFOnly only allows a fast-forward merge; aborts otherwise.
	FFOnly bool
	// AllowUnrelated permits merging histories with no common ancestor.
	AllowUnrelated bool
}

// DiffOptions contains options for getting a diff.
type DiffOptions struct {
	// From is the starting commit SHA. When both From and To are empty the
	// diff is computed between the working tree and HEAD — staged as well as
	// unstaged changes are visible (`git diff HEAD` semantics, on both
	// backends). Giving only one of the two is an error.
	From string
	// To is the ending commit SHA.
	To string
	// Paths restricts the diff to the given paths (optional).
	Paths []string
}

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

// RepoStatus represents the working tree status.
type RepoStatus struct {
	// Branch is the name of the current branch (without the refs/heads/ prefix).
	Branch string
	// IsClean is true when there are no staged, unstaged or untracked changes.
	IsClean bool
	// Staged lists files with changes staged for commit.
	Staged []FileStatus
	// Unstaged lists files with changes in the working tree (not yet staged).
	Unstaged []FileStatus
	// Untracked lists untracked files (not ignored).
	Untracked []string
	// Ahead is the number of local commits not pushed to upstream.
	Ahead int
	// Behind is the number of upstream commits not pulled locally.
	Behind int
}

// StatusCode is the porcelain status code used for a file in the staging area
// or the working tree. These mirror the codes produced by
// `git status --porcelain`.
type StatusCode byte

const (
	StatusUnmodified StatusCode = ' '
	StatusModified   StatusCode = 'M'
	StatusAdded      StatusCode = 'A'
	StatusDeleted    StatusCode = 'D'
	StatusRenamed    StatusCode = 'R'
	StatusCopied     StatusCode = 'C'
	StatusUntracked  StatusCode = '?'
	StatusIgnored    StatusCode = '!'
)

// FileStatus represents the status of a single file.
type FileStatus struct {
	Path string
	// Worktree is the status of the file in the working tree.
	Worktree StatusCode
	// Staging is the status of the file in the staging area (index).
	Staging StatusCode
}

// FetchResult contains the result of a fetch operation.
//
// Both backends populate every field the same way, from a before/after diff
// of the remote-tracking refs and tags:
//   - FetchedRefs: full refnames (refs/remotes/<remote>/..., refs/tags/...)
//     of every ref the fetch created or moved;
//   - NewBranches/UpdatedBranch/DeletedBranch: short branch names (after the
//     refs/remotes/<remote>/ prefix);
//   - NewTags: short tag names (after the refs/tags/ prefix).
type FetchResult struct {
	FetchedRefs   []string
	NewBranches   []string
	UpdatedBranch []string
	DeletedBranch []string
	NewTags       []string
}

// PushResult contains the result of a push operation.
type PushResult struct {
	// PushedRefs are the destination refs the remote accepted, in the short
	// form git prints in its push report (e.g. "main", "v1") — on both
	// backends. Refs that were rejected or merely up to date are not listed.
	PushedRefs []string
	// Errors collects per-ref errors reported by the remote (if any).
	Errors []string
}

// CommitInfo represents a git commit.
type CommitInfo struct {
	// Hash is the full 40-character commit SHA.
	Hash string
	// Message is the FULL commit message (subject and body), trailing
	// newlines trimmed — identical on both backends.
	Message string
	// Author is the commit author's name.
	Author string
	// Date is the author date formatted as RFC3339 (parseable with
	// time.Parse(time.RFC3339, ...) on both backends).
	Date string
}

// TagInfo represents a git tag.
type TagInfo struct {
	Name string
	// Hash is the SHA the tag points at (the tag object for annotated tags,
	// or the commit for lightweight tags).
	Hash string
	// Message is the FULL tag message (annotated tags) or the FULL commit
	// message (lightweight tags), trailing newlines trimmed — identical on
	// both backends.
	Message string
	Author  string
}

// StashEntry represents a stash entry.
type StashEntry struct {
	// Index is the stash position (0 is the most recent, i.e. stash@{0}).
	Index int
	// Message is the stash list line as produced by `git stash list`.
	Message string
}

// BranchDetail represents a branch with extended information.
type BranchDetail struct {
	Name string
	Hash string
	// IsCurrent is true when this is the checked-out branch.
	IsCurrent bool
	// IsRemote is true for remote-tracking branches (refs/remotes/*).
	IsRemote bool
	// Remote is the remote name for remote branches (e.g. "origin").
	Remote string
	// Upstream is the configured upstream tracking branch (e.g. "origin/main").
	Upstream string
	Author   string
	Email    string
	// Date is the tip commit's author date formatted as RFC3339.
	Date string
	// Message is the subject line of the tip commit.
	Message string
}

// TreeEntryType identifies whether a TreeEntry is a file or a directory.
type TreeEntryType string

const (
	TreeEntryFile TreeEntryType = "file"
	TreeEntryDir  TreeEntryType = "dir"
)

// TreeEntry represents a file or directory in a git tree.
type TreeEntry struct {
	Name string
	// Path is the full path relative to the repository root.
	Path string
	// Type is either TreeEntryFile or TreeEntryDir.
	Type TreeEntryType
	// Mode is the git file mode (e.g. "100644").
	Mode string
	Hash string
	// Size is the file size in bytes (populated for files in recursive mode).
	Size int64
}

// BlobEncoding is the encoding used for BlobContent.Content.
type BlobEncoding string

const (
	EncodingUTF8   BlobEncoding = "utf-8"
	EncodingBase64 BlobEncoding = "base64"
)

// BlobContent represents the content of a file at a given revision.
type BlobContent struct {
	// Content is the file payload. When IsBinary it is base64-encoded.
	Content string
	// Encoding is either EncodingUTF8 or EncodingBase64.
	Encoding BlobEncoding
	// Size is the decoded payload size in bytes.
	Size int64
	// IsBinary is true when the blob is detected as binary.
	IsBinary bool
}

// ---------------------------------------------------------------------------
// GitBackend interface
// ---------------------------------------------------------------------------

// GitBackend is the interface for low-level, local git operations.
//
// Every method takes a context (for cancellation/timeouts) and the absolute
// path of a working tree. Commit/ref arguments are full 40-character SHAs
// unless stated otherwise; dates are RFC3339 formatted strings. Methods
// return wrapped errors created by newGitError (see errors.go); use the
// IsNotFound / IsMergeConflict helpers to classify them.
//
// GitBackend composes the focused sub-interfaces defined in iface.go
// (CoreOps, BranchOps, StatusDiffOps, CommitOps, RemoteOps, TagOps, FileOps,
// StashOps, ConfigOps, AdvancedOps). Consumers that only need a subset of
// capabilities may depend on the narrower sub-interface directly.
type GitBackend interface {
	CoreOps
	BranchOps
	StatusDiffOps
	CommitOps
	RemoteOps
	TagOps
	FileOps
	StashOps
	ConfigOps
	AdvancedOps
}
