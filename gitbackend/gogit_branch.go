package gitbackend

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// --- Branch operations ---

func (b *GoGitBackend) ListRemoteBranches(ctx context.Context, repoPath, remote string) ([]string, error) {
	repo, err := openRepo("ListRemoteBranches", repoPath)
	if err != nil {
		return nil, err
	}

	remoteObj, err := repo.Remote(remote)
	if err != nil {
		return nil, newGitError("ListRemoteBranches", repoPath, "", ErrRemoteNotFound)
	}

	refs, err := remoteObj.List(&git.ListOptions{})
	if err != nil {
		return nil, newGitError("ListRemoteBranches", repoPath, "", err)
	}

	var result []string
	prefix := "refs/heads/"
	for _, ref := range refs {
		name := ref.Name().String()
		if strings.HasPrefix(name, prefix) {
			result = append(result, strings.TrimPrefix(name, prefix))
		}
	}
	return result, nil
}

func (b *GoGitBackend) CreateBranch(ctx context.Context, repoPath, branch, ref string) error {
	repo, err := openRepo("CreateBranch", repoPath)
	if err != nil {
		return err
	}

	// Resolve the start point: ref may be a branch/tag name, HEAD, or a raw
	// hash. When empty the branch starts at HEAD. resolveRev rejects anything
	// it cannot resolve, so a branch-name ref can no longer fall through the
	// zero-hash probe and silently start at HEAD instead.
	var hash plumbing.Hash
	if ref == "" {
		head, err := repo.Head()
		if err != nil {
			return newGitError("CreateBranch", repoPath, "", err)
		}
		hash = head.Hash()
	} else {
		hash, err = resolveRev(repo, ref)
		if err != nil {
			return newGitError("CreateBranch", repoPath, "", err)
		}
	}

	newRefName := plumbing.ReferenceName("refs/heads/" + branch)

	// Refuse to clobber: SetReference would silently MOVE an existing branch
	// pointer, so probe for the ref first and fail with the same sentinel the
	// native backend reports.
	if _, err := repo.Reference(newRefName, true); err == nil {
		return newGitError("CreateBranch", repoPath, "", ErrBranchExists)
	}

	newRef := plumbing.NewHashReference(newRefName, hash)
	err = repo.Storer.SetReference(newRef)
	if err != nil {
		return newGitError("CreateBranch", repoPath, "", err)
	}
	return nil
}

func (b *GoGitBackend) DeleteBranch(ctx context.Context, repoPath, branch string) error {
	repo, err := openRepo("DeleteBranch", repoPath)
	if err != nil {
		return err
	}

	// **不能删当前分支**:删掉后 HEAD 悬空 ⇒ 仓库直接不可用
	// (native 报 "cannot delete branch 'main' used by worktree")。
	if head, herr := repo.Head(); herr == nil &&
		head.Name() == plumbing.NewBranchReferenceName(branch) {
		return newGitError("DeleteBranch", repoPath, "", ErrBranchInUse)
	}

	err = repo.Storer.RemoveReference(plumbing.ReferenceName("refs/heads/" + branch))
	if err != nil {
		return newGitError("DeleteBranch", repoPath, "", ErrBranchNotFound)
	}
	return nil
}

func (b *GoGitBackend) RenameBranch(ctx context.Context, repoPath, oldName, newName string) error {
	repo, err := openRepo("RenameBranch", repoPath)
	if err != nil {
		return err
	}

	oldRefName := plumbing.ReferenceName("refs/heads/" + oldName)
	newRefName := plumbing.ReferenceName("refs/heads/" + newName)

	ref, err := repo.Reference(oldRefName, true)
	if err != nil {
		return newGitError("RenameBranch", repoPath, "", ErrBranchNotFound)
	}

	// **目标名已存在 ⇒ 拒绝**:否则会静默把别人的分支指针改掉
	// (native 报 "already exists")。
	if _, rerr := repo.Reference(newRefName, true); rerr == nil {
		return newGitError("RenameBranch", repoPath, "", ErrBranchExists)
	}

	err = repo.Storer.SetReference(plumbing.NewHashReference(newRefName, ref.Hash()))
	if err != nil {
		return newGitError("RenameBranch", repoPath, "", err)
	}

	// **改的是当前分支 ⇒ HEAD 的符号引用要跟着改**:否则 HEAD 指向已删除的
	// refs/heads/<old> ⇒ 仓库不可用(`git rev-parse HEAD` 直接失败)。
	if head, herr := repo.Head(); herr == nil && head.Name() == oldRefName {
		if serr := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, newRefName)); serr != nil {
			return newGitError("RenameBranch", repoPath, "", serr)
		}
	}

	return repo.Storer.RemoveReference(oldRefName)
}

func (b *GoGitBackend) Checkout(ctx context.Context, repoPath, branch string) error {
	repo, err := openRepo("Checkout", repoPath)
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return newGitError("Checkout", repoPath, "", err)
	}

	// **脏工作区不许切**:go-git 的 Checkout 先把 HEAD 切到目标分支、再 Reset
	// 工作区 ⇒ 工作区脏时 Reset 失败,但 **HEAD 已经切走** ⇒ 调用方拿到错误
	// 却面对"半切换"的仓库(HEAD=新分支、文件还是旧分支的内容)⇒
	// 后续 Commit 会把旧分支的内容提交到新分支。
	// ⇒ 先自查脏,脏就**不切**。
	if st, serr := wt.Status(); serr == nil && !st.IsClean() {
		return newGitError("Checkout", repoPath, "", ErrDirtyWorktree)
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/" + branch),
	})
	if err != nil {
		return newGitError("Checkout", repoPath, "", err)
	}
	return nil
}

func (b *GoGitBackend) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	repo, err := openRepo("GetCurrentBranch", repoPath)
	if err != nil {
		return "", err
	}

	head, err := repo.Head()
	if err != nil {
		return "", newGitError("GetCurrentBranch", repoPath, "", err)
	}

	name := head.Name().String()
	if strings.HasPrefix(name, "refs/heads/") {
		return strings.TrimPrefix(name, "refs/heads/"), nil
	}
	return name, nil
}

// --- Extended branch operations ---

func (b *GoGitBackend) ListLocalBranches(ctx context.Context, repoPath string) ([]string, error) {
	repo, err := openRepo("ListLocalBranches", repoPath)
	if err != nil {
		return nil, err
	}
	iter, err := repo.Branches()
	if err != nil {
		return nil, newGitError("ListLocalBranches", repoPath, "", err)
	}
	var branches []string
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, ref.Name().Short())
		return nil
	})
	if err != nil {
		return nil, newGitError("ListLocalBranches", repoPath, "", err)
	}
	return branches, nil
}

func (b *GoGitBackend) ListBranches(ctx context.Context, repoPath string) ([]BranchDetail, error) {
	repo, err := openRepo("ListBranches", repoPath)
	if err != nil {
		return nil, err
	}

	headRef, err := repo.Head()
	var headHash plumbing.Hash
	if err == nil {
		headHash = headRef.Hash()
	}

	cfg, err := repo.Config()
	if err != nil {
		return nil, newGitError("ListBranches", repoPath, "", err)
	}

	iter, err := repo.References()
	if err != nil {
		return nil, newGitError("ListBranches", repoPath, "", err)
	}

	var branches []BranchDetail
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		isBranch := ref.Name().IsBranch()
		isRemote := ref.Name().IsRemote()
		if !isBranch && !isRemote {
			return nil
		}

		hash := ref.Hash()
		bd := BranchDetail{
			Name:     ref.Name().Short(),
			Hash:     hash.String(),
			IsRemote: isRemote,
		}

		if isBranch && hash == headHash {
			bd.IsCurrent = true
		}

		if isRemote {
			fullName := ref.Name().String()
			parts := strings.SplitN(strings.TrimPrefix(fullName, "refs/remotes/"), "/", 2)
			if len(parts) == 2 {
				bd.Remote = parts[0]
			}
		}

		if isBranch {
			name := ref.Name().Short()
			if branchCfg, ok := cfg.Branches[name]; ok {
				if branchCfg.Remote != "" && branchCfg.Merge != "" {
					bd.Upstream = fmt.Sprintf("%s/%s", branchCfg.Remote, branchCfg.Merge.Short())
				}
			}
		}

		commit, commitErr := repo.CommitObject(hash)
		if commitErr == nil {
			bd.Author = commit.Author.Name
			bd.Email = commit.Author.Email
			bd.Date = commit.Author.When.Format(time.RFC3339)
			bd.Message = strings.TrimSpace(strings.Split(commit.Message, "\n")[0])
		}

		branches = append(branches, bd)
		return nil
	})
	if err != nil {
		return nil, newGitError("ListBranches", repoPath, "", err)
	}
	return branches, nil
}

func (b *GoGitBackend) GetBranchSyncInfo(ctx context.Context, repoPath, branch, upstream string) (int, int, error) {
	repo, err := openRepo("GetBranchSyncInfo", repoPath)
	if err != nil {
		return 0, 0, err
	}

	branchRef, err := repo.Reference(plumbing.ReferenceName("refs/heads/"+branch), true)
	if err != nil {
		return 0, 0, newGitError("GetBranchSyncInfo", repoPath, "", ErrBranchNotFound)
	}
	upstreamRef, err := repo.Reference(plumbing.ReferenceName("refs/remotes/"+upstream), true)
	if err != nil {
		return 0, 0, newGitError("GetBranchSyncInfo", repoPath, "", err)
	}

	branchCommit, err := repo.CommitObject(branchRef.Hash())
	if err != nil {
		return 0, 0, newGitError("GetBranchSyncInfo", repoPath, "", err)
	}
	upstreamCommit, err := repo.CommitObject(upstreamRef.Hash())
	if err != nil {
		return 0, 0, newGitError("GetBranchSyncInfo", repoPath, "", err)
	}

	bases, err := branchCommit.MergeBase(upstreamCommit)
	if err != nil || len(bases) == 0 {
		return 0, 0, nil
	}
	baseHash := bases[0].Hash

	ahead := 0
	iter, _ := repo.Log(&git.LogOptions{From: branchRef.Hash()})
	if iter != nil {
		_ = iter.ForEach(func(c *object.Commit) error {
			if c.Hash == baseHash {
				return io.EOF
			}
			ahead++
			return nil
		})
		iter.Close()
	}

	behind := 0
	iter2, _ := repo.Log(&git.LogOptions{From: upstreamRef.Hash()})
	if iter2 != nil {
		_ = iter2.ForEach(func(c *object.Commit) error {
			if c.Hash == baseHash {
				return io.EOF
			}
			behind++
			return nil
		})
		iter2.Close()
	}

	return ahead, behind, nil
}
