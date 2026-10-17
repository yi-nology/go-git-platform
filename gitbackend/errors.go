package gitbackend

import (
	"errors"
	"fmt"

	oekiterrors "github.com/oekit/pkg/errors"
)

var (
	ErrGitNotFound       = errors.New("git binary not found")
	ErrRepoNotFound      = errors.New("repository not found")
	ErrBranchExists      = errors.New("branch already exists")
	ErrBranchNotFound    = errors.New("branch not found")
	ErrMergeConflict     = errors.New("merge conflict")
	ErrNothingToMerge    = errors.New("nothing to merge")
	ErrDirtyWorktree     = errors.New("dirty worktree")
	ErrRemoteNotFound    = errors.New("remote not found")
	ErrTagExists         = errors.New("tag already exists")
	ErrFileNotFound      = errors.New("file not found at revision")
	ErrAuthFailed        = errors.New("authentication failed")
	ErrAlreadyUpToDate   = errors.New("already up to date")
	ErrNotAGitRepo       = errors.New("not a git repository")
	ErrInvalidGitArg     = errors.New("invalid git argument")
	ErrConfigKeyNotFound = errors.New("config key not found")
	ErrStashUnsupported  = errors.New("stash requires the native git backend")
)

// GitError is a structured error from a git operation.
type GitError struct {
	Op      string // operation name, e.g., "Fetch"
	Path    string // repo path
	Command string // git command that failed
	Stderr  string // stderr output
	Err     error  // underlying error
}

func (e *GitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("gitbackend: %s: %s: %s: %v", e.Op, e.Path, e.Stderr, e.Err)
	}
	return fmt.Sprintf("gitbackend: %s: %s: %v", e.Op, e.Path, e.Err)
}

func (e *GitError) Unwrap() error {
	return e.Err
}

func (e *GitError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// newGitError creates a GitError.
func newGitError(op, path, stderr string, err error) *GitError {
	return &GitError{
		Op:     op,
		Path:   path,
		Stderr: stderr,
		Err:    err,
	}
}

// IsNotFound checks if an error is a not-found error. oekiterrors.In walks
// the chain once for all four sentinels instead of four separate walks.
func IsNotFound(err error) bool {
	return oekiterrors.In(err, ErrRepoNotFound, ErrBranchNotFound, ErrRemoteNotFound, ErrFileNotFound)
}

// IsAuthFailed checks if an error is an authentication error.
func IsAuthFailed(err error) bool {
	return errors.Is(err, ErrAuthFailed)
}

// IsMergeConflict checks if an error is a merge conflict.
func IsMergeConflict(err error) bool {
	return errors.Is(err, ErrMergeConflict)
}
