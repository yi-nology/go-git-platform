package gitbackend

import (
	"context"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// --- Status and diff ---

func (b *GoGitBackend) GetStatus(ctx context.Context, repoPath string) (*RepoStatus, error) {
	repo, err := openRepo("GetStatus", repoPath)
	if err != nil {
		return nil, err
	}

	status := &RepoStatus{}

	branch, err := b.GetCurrentBranch(ctx, repoPath)
	if err == nil {
		status.Branch = branch
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, newGitError("GetStatus", repoPath, "", err)
	}

	wtStatus, err := wt.Status()
	if err != nil {
		return nil, newGitError("GetStatus", repoPath, "", err)
	}

	for path, fileStatus := range wtStatus {
		fs := FileStatus{
			Path:     path,
			Staging:  StatusCode(fileStatus.Staging),
			Worktree: StatusCode(fileStatus.Worktree),
		}
		if fileStatus.Staging == git.Untracked {
			status.Untracked = append(status.Untracked, path)
		} else {
			if fileStatus.Staging != git.Unmodified {
				status.Staged = append(status.Staged, fs)
			}
			if fileStatus.Worktree != git.Unmodified {
				status.Unstaged = append(status.Unstaged, fs)
			}
		}
	}

	status.IsClean = len(status.Staged) == 0 && len(status.Unstaged) == 0 && len(status.Untracked) == 0
	return status, nil
}

func (b *GoGitBackend) Diff(ctx context.Context, repoPath string, opts DiffOptions) (string, error) {
	repo, err := openRepo("Diff", repoPath)
	if err != nil {
		return "", err
	}

	// If no From/To specified, diff working tree against HEAD
	if opts.From == "" && opts.To == "" {
		wt, err := repo.Worktree()
		if err != nil {
			return "", newGitError("Diff", repoPath, "", err)
		}
		status, err := wt.Status()
		if err != nil {
			return "", newGitError("Diff", repoPath, "", err)
		}
		return status.String(), nil
	}

	// Diff between two commits
	fromHash, err := resolveRev(repo, opts.From)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}
	fromCommit, err := repo.CommitObject(fromHash)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	toHash, err := resolveRev(repo, opts.To)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}
	toCommit, err := repo.CommitObject(toHash)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	fromTree, err := fromCommit.Tree()
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	toTree, err := toCommit.Tree()
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	var result strings.Builder
	for _, change := range changes {
		patch, err := change.Patch()
		if err != nil {
			continue
		}
		result.WriteString(patch.String())
	}
	return result.String(), nil
}

// --- Advanced diff operations ---

func (b *GoGitBackend) DiffNames(ctx context.Context, repoPath, from, to string) ([]string, error) {
	changes, err := b.treeChanges(repoPath, from, to)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(changes))
	for _, c := range changes {
		result = append(result, changePath(c))
	}
	return result, nil
}

func (b *GoGitBackend) DeletedFiles(ctx context.Context, repoPath, from, to string) ([]string, error) {
	changes, err := b.treeChanges(repoPath, from, to)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, c := range changes {
		action, err := c.Action()
		if err != nil {
			continue
		}
		if action == merkletrie.Delete {
			result = append(result, c.From.Name)
		}
	}
	return result, nil
}

// --- Revision parsing ---

func (b *GoGitBackend) RevParse(ctx context.Context, repoPath, ref string) (string, error) {
	repo, err := openRepo("RevParse", repoPath)
	if err != nil {
		return "", err
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return "", newGitError("RevParse", repoPath, "", err)
	}
	return hash.String(), nil
}

func (b *GoGitBackend) MergeBase(ctx context.Context, repoPath, a, other string) (string, error) {
	repo, err := openRepo("MergeBase", repoPath)
	if err != nil {
		return "", err
	}
	hashA, err := resolveRev(repo, a)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	commitA, err := repo.CommitObject(hashA)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	hashB, err := resolveRev(repo, other)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	commitB, err := repo.CommitObject(hashB)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	bases, err := commitA.MergeBase(commitB)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	if len(bases) == 0 {
		return "", nil
	}
	return bases[0].Hash.String(), nil
}
