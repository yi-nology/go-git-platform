package gitbackend

import "context"

// This file splits the formerly monolithic GitBackend interface (55 methods)
// into focused, domain-specific sub-interfaces. GitBackend (defined in
// backend.go) is now the composition of all of them, so its method set — and
// therefore the public API — is unchanged. Consumers that only need a subset
// of capabilities can depend on the narrower interface (e.g. BranchOps).
//
// Method arguments and return types are identical to before; only the type
// structure has been refactored.

// CoreOps covers repository-level network and lifecycle operations.
type CoreOps interface {
	// Fetch downloads objects and refs from a remote into the local repository
	// without touching the working tree.
	Fetch(ctx context.Context, opts FetchOptions) (*FetchResult, error)
	// FetchAll fetches tags and every branch from all configured remotes.
	FetchAll(ctx context.Context, repoPath string, auth AuthConfig) error
	// Push uploads local refs to a remote.
	Push(ctx context.Context, opts PushOptions) (*PushResult, error)
	// Pull fetches from and integrates with the current branch (fetch + merge).
	Pull(ctx context.Context, repoPath string, remote string, branch string, auth AuthConfig) error
	// Clone clones a remote repository into a new local working tree.
	Clone(ctx context.Context, opts CloneOptions) error
	// Init creates a new empty git repository at repoPath.
	Init(ctx context.Context, repoPath string) error
	// RunRaw executes an arbitrary git command and returns its combined output.
	// Only supported by the native backend.
	RunRaw(ctx context.Context, repoPath string, args []string) (stdout string, stderr string, err error)
}

// BranchOps covers branch listing, creation, deletion, and checkout.
type BranchOps interface {
	// ListRemoteBranches lists branches that exist on the given remote.
	ListRemoteBranches(ctx context.Context, repoPath string, remote string) ([]string, error)
	// ListLocalBranches lists local branches (refs/heads/*).
	ListLocalBranches(ctx context.Context, repoPath string) ([]string, error)
	// ListBranches lists both local and remote branches with extended details.
	ListBranches(ctx context.Context, repoPath string) ([]BranchDetail, error)
	// CreateBranch creates a branch pointing at ref (or HEAD when empty).
	CreateBranch(ctx context.Context, repoPath string, branch string, ref string) error
	// DeleteBranch deletes a local branch.
	DeleteBranch(ctx context.Context, repoPath string, branch string) error
	// RenameBranch renames a local branch.
	RenameBranch(ctx context.Context, repoPath string, oldName string, newName string) error
	// Checkout switches the working tree to the given branch.
	Checkout(ctx context.Context, repoPath string, branch string) error
	// GetCurrentBranch returns the name of the checked-out branch.
	GetCurrentBranch(ctx context.Context, repoPath string) (string, error)
	// GetBranchSyncInfo returns how many commits branch is ahead/behind upstream.
	GetBranchSyncInfo(ctx context.Context, repoPath string, branch string, upstream string) (ahead int, behind int, err error)
}

// StatusDiffOps covers working-tree status and diff inspection.
type StatusDiffOps interface {
	// GetStatus returns the working tree status.
	GetStatus(ctx context.Context, repoPath string) (*RepoStatus, error)
	// Diff returns a textual diff. With empty From/To it diffs the working tree
	// against HEAD; otherwise it diffs the two commits.
	Diff(ctx context.Context, repoPath string, opts DiffOptions) (string, error)
}

// CommitOps covers history traversal and history-rewriting operations.
type CommitOps interface {
	// GetCommitsBetween returns commits in the range (from, to].
	GetCommitsBetween(ctx context.Context, repoPath string, from string, to string) ([]CommitInfo, error)
	// IsAncestor reports whether ancestor is an ancestor of descendant.
	IsAncestor(ctx context.Context, repoPath string, ancestor string, descendant string) (bool, error)
	// Merge merges branch into the current HEAD according to opts.
	Merge(ctx context.Context, repoPath string, branch string, opts MergeOptions) error
	// CherryPick applies the changes of commitHash onto the current HEAD.
	CherryPick(ctx context.Context, repoPath string, commitHash string) error
	// Rebase rebases the current branch onto onto.
	Rebase(ctx context.Context, repoPath string, onto string) error
	// RebaseAbort aborts an in-progress rebase.
	RebaseAbort(ctx context.Context, repoPath string) error
	// RebaseContinue continues an in-progress rebase after resolving conflicts.
	RebaseContinue(ctx context.Context, repoPath string) error
}

// RemoteOps covers remote configuration and connectivity checks.
type RemoteOps interface {
	// GetRemotes returns the names of all configured remotes.
	GetRemotes(ctx context.Context, repoPath string) ([]string, error)
	// AddRemote adds a new remote.
	AddRemote(ctx context.Context, repoPath string, name string, url string) error
	// RemoveRemote removes a configured remote.
	RemoveRemote(ctx context.Context, repoPath string, name string) error
	// GetRemoteURL returns the URL of a named remote.
	GetRemoteURL(ctx context.Context, repoPath string, name string) (string, error)
	// TestConnection verifies that url is reachable with the given credentials.
	TestConnection(ctx context.Context, url string, auth AuthConfig) error
}

// TagOps covers tag creation, deletion, listing, and pushing.
type TagOps interface {
	// CreateTag creates a tag pointing at ref (or HEAD when empty).
	CreateTag(ctx context.Context, repoPath string, name string, ref string) error
	// DeleteTag deletes a local tag.
	DeleteTag(ctx context.Context, repoPath string, name string) error
	// PushTag pushes a single tag to remote.
	PushTag(ctx context.Context, repoPath string, remote string, name string, auth AuthConfig) error
	// GetTagList lists all tags with their metadata.
	GetTagList(ctx context.Context, repoPath string) ([]TagInfo, error)
}

// FileOps covers reading files, trees, and blobs at arbitrary revisions.
type FileOps interface {
	// GetFileAtRevision returns the raw bytes of path at the given ref.
	GetFileAtRevision(ctx context.Context, repoPath string, path string, ref string) ([]byte, error)
	// GetFileHistory returns commits that touched path (most recent first),
	// limited to limit entries (0 means all).
	GetFileHistory(ctx context.Context, repoPath string, path string, limit int) ([]CommitInfo, error)
	// GetTree lists entries under dirPath at ref. When recursive is true it
	// lists all files (not directories) below dirPath.
	GetTree(ctx context.Context, repoPath string, ref string, dirPath string, recursive bool) ([]TreeEntry, error)
	// GetBlob returns the content of filePath at ref.
	GetBlob(ctx context.Context, repoPath string, ref string, filePath string) (*BlobContent, error)
	// GetCommit returns metadata for a single commit.
	GetCommit(ctx context.Context, repoPath string, hash string) (*CommitInfo, error)
}

// StashOps covers the stash list and stash lifecycle.
//
// Backend support: the native backend implements stash via git's own
// refs/stash reflog. The go-git backend returns ErrStashUnsupported for
// every stash operation — its storage layer cannot represent the stash
// reflog, and earlier workarounds wrote reference names native git
// rejects (see gogit_stash_config.go).
type StashOps interface {
	// StashList lists the stash entries.
	StashList(ctx context.Context, repoPath string) ([]StashEntry, error)
	// StashSave saves the working tree and index changes to the stash.
	StashSave(ctx context.Context, repoPath string, message string) error
	// StashApply applies a stash entry without removing it from the stash list.
	StashApply(ctx context.Context, repoPath string, index int) error
	// StashPop applies a stash entry and then drops it from the stash list.
	StashPop(ctx context.Context, repoPath string, index int) error
	// StashDrop removes a single stash entry.
	StashDrop(ctx context.Context, repoPath string, index int) error
	// StashClear removes all stash entries.
	StashClear(ctx context.Context, repoPath string) error
}

// ConfigOps covers reading and writing git config values.
type ConfigOps interface {
	// GetConfig reads a config value by dotted key (e.g. "user.name").
	GetConfig(ctx context.Context, repoPath string, key string) (string, error)
	// SetConfig sets a config value by dotted key (e.g. "user.email").
	SetConfig(ctx context.Context, repoPath string, key string, value string) error
}

// AdvancedOps covers lower-level revision, index, and identity operations.
type AdvancedOps interface {
	// RevParse resolves a revision (ref/short-SHA/etc.) to a full SHA.
	RevParse(ctx context.Context, repoPath string, ref string) (string, error)
	// MergeBase returns the best common ancestor of two commits.
	MergeBase(ctx context.Context, repoPath string, a string, b string) (string, error)
	// DiffNames returns the names of files changed between two commits.
	DiffNames(ctx context.Context, repoPath string, from string, to string) ([]string, error)
	// DeletedFiles returns files deleted between two commits.
	DeletedFiles(ctx context.Context, repoPath string, from string, to string) ([]string, error)
	// CheckoutRef checks out ref: branch names attach HEAD to the branch
	// (matching `git checkout <branch>`), anything else detaches.
	CheckoutRef(ctx context.Context, repoPath string, ref string) error
	// CheckoutDetached force-checks out ref in detached HEAD state, even
	// when ref names a branch — pins HEAD to the commit.
	CheckoutDetached(ctx context.Context, repoPath string, ref string) error
	// CheckoutFiles restores the given files from ref into the working tree.
	CheckoutFiles(ctx context.Context, repoPath string, ref string, files []string) error
	// Add stages the given files into the index.
	Add(ctx context.Context, repoPath string, files []string) error
	// CommitWithIdentity creates a commit using an explicit author identity.
	CommitWithIdentity(ctx context.Context, repoPath string, name string, email string, message string) error
}
