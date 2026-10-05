package gitbackend

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// --- File operations ---

func (b *GoGitBackend) GetFileAtRevision(ctx context.Context, repoPath, path, ref string) ([]byte, error) {
	repo, err := openRepo("GetFileAtRevision", repoPath)
	if err != nil {
		return nil, err
	}

	var hash plumbing.Hash
	if ref == "" || ref == "HEAD" {
		head, err := repo.Head()
		if err != nil {
			return nil, newGitError("GetFileAtRevision", repoPath, "", err)
		}
		hash = head.Hash()
	} else {
		hash = plumbing.NewHash(ref)
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, newGitError("GetFileAtRevision", repoPath, "", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, newGitError("GetFileAtRevision", repoPath, "", err)
	}

	file, err := tree.File(path)
	if err != nil {
		return nil, newGitError("GetFileAtRevision", repoPath, "", ErrFileNotFound)
	}

	content, err := file.Contents()
	if err != nil {
		return nil, newGitError("GetFileAtRevision", repoPath, "", err)
	}
	return []byte(content), nil
}

func (b *GoGitBackend) GetFileHistory(ctx context.Context, repoPath, path string, limit int) ([]CommitInfo, error) {
	repo, err := openRepo("GetFileHistory", repoPath)
	if err != nil {
		return nil, err
	}

	head, err := repo.Head()
	if err != nil {
		return nil, newGitError("GetFileHistory", repoPath, "", err)
	}

	commitIter, err := repo.Log(&git.LogOptions{
		From:     head.Hash(),
		FileName: &path,
	})
	if err != nil {
		return nil, newGitError("GetFileHistory", repoPath, "", err)
	}
	defer commitIter.Close()

	var commits []CommitInfo
	count := 0
	err = commitIter.ForEach(func(c *object.Commit) error {
		if limit > 0 && count >= limit {
			return io.EOF
		}
		commits = append(commits, CommitInfo{
			Hash:    c.Hash.String(),
			Message: strings.TrimRight(c.Message, "\n"),
			Author:  c.Author.Name,
			Date:    c.Author.When.Format(time.RFC3339),
		})
		count++
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, newGitError("GetFileHistory", repoPath, "", err)
	}
	return commits, nil
}

// --- Tree and blob queries ---

func (b *GoGitBackend) GetTree(ctx context.Context, repoPath, ref, dirPath string, recursive bool) ([]TreeEntry, error) {
	repo, err := openRepo("GetTree", repoPath)
	if err != nil {
		return nil, err
	}

	var hash plumbing.Hash
	if ref == "" || ref == "HEAD" {
		head, err := repo.Head()
		if err != nil {
			return nil, newGitError("GetTree", repoPath, "", err)
		}
		hash = head.Hash()
	} else {
		resolved, err := repo.ResolveRevision(plumbing.Revision(ref))
		if err != nil {
			return nil, newGitError("GetTree", repoPath, "", err)
		}
		hash = *resolved
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, newGitError("GetTree", repoPath, "", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, newGitError("GetTree", repoPath, "", err)
	}

	dirPath = strings.TrimSpace(dirPath)
	if dirPath == "." || dirPath == "" {
		dirPath = ""
	}
	if dirPath != "" && dirPath != "/" {
		dirPath = strings.TrimPrefix(dirPath, "/")
		tree, err = tree.Tree(dirPath)
		if err != nil {
			return nil, newGitError("GetTree", repoPath, "", err)
		}
	}

	var entries []TreeEntry
	if recursive {
		_ = tree.Files().ForEach(func(f *object.File) error {
			entries = append(entries, TreeEntry{
				Name: filepath.Base(f.Name),
				Path: f.Name,
				Type: TreeEntryFile,
				Size: f.Size,
				Mode: f.Mode.String(),
				Hash: f.Hash.String(),
			})
			return nil
		})
	} else {
		for _, entry := range tree.Entries {
			entryType := TreeEntryFile
			if entry.Mode == filemode.Dir {
				entryType = TreeEntryDir
			}
			path := entry.Name
			if dirPath != "" {
				path = filepath.Join(dirPath, entry.Name)
			}
			entries = append(entries, TreeEntry{
				Name: entry.Name,
				Path: path,
				Type: entryType,
				Mode: entry.Mode.String(),
				Hash: entry.Hash.String(),
			})
		}
	}
	return entries, nil
}

func (b *GoGitBackend) GetBlob(ctx context.Context, repoPath, ref, filePath string) (*BlobContent, error) {
	repo, err := openRepo("GetBlob", repoPath)
	if err != nil {
		return nil, err
	}

	var hash plumbing.Hash
	if ref == "" || ref == "HEAD" {
		head, err := repo.Head()
		if err != nil {
			return nil, newGitError("GetBlob", repoPath, "", err)
		}
		hash = head.Hash()
	} else {
		resolved, err := repo.ResolveRevision(plumbing.Revision(ref))
		if err != nil {
			return nil, newGitError("GetBlob", repoPath, "", err)
		}
		hash = *resolved
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, newGitError("GetBlob", repoPath, "", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, newGitError("GetBlob", repoPath, "", err)
	}
	file, err := tree.File(filePath)
	if err != nil {
		return nil, newGitError("GetBlob", repoPath, "", ErrFileNotFound)
	}
	content, err := file.Contents()
	if err != nil {
		return nil, newGitError("GetBlob", repoPath, "", err)
	}

	data := []byte(content)
	isBinary := !utf8.Valid(data) || containsNullByte(data)
	result := &BlobContent{
		Size:     int64(len(data)),
		IsBinary: isBinary,
	}
	if isBinary {
		result.Content = base64.StdEncoding.EncodeToString(data)
		result.Encoding = EncodingBase64
	} else {
		result.Content = content
		result.Encoding = EncodingUTF8
	}
	return result, nil
}

// --- Checkout helpers ---

// worktreeFileMode maps a git entry mode onto the permission bits the
// working-tree file should carry. Git only tracks the executable bit:
// executables get 0o755, everything else 0o644.
func worktreeFileMode(mode filemode.FileMode) os.FileMode {
	if mode == filemode.Executable {
		return 0o755
	}
	return 0o644
}

// writeWorktreeFile writes blob content to the working tree at relPath with
// the entry's mode applied. Copy and close errors are returned instead of
// swallowed: a silently truncated write used to end up staged and committed
// as if it were complete. Symlink entries are recreated as links rather than
// clobbered into plain files holding the target path.
func writeWorktreeFile(repoPath, relPath string, blob *object.Blob, mode filemode.FileMode) (err error) {
	fullPath := filepath.Join(repoPath, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return fmt.Errorf("mkdir for %s: %w", relPath, err)
	}

	reader, err := blob.Reader()
	if err != nil {
		return fmt.Errorf("open blob reader for %s: %w", relPath, err)
	}
	defer func() {
		if cerr := reader.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close blob reader for %s: %w", relPath, cerr)
		}
	}()

	if mode == filemode.Symlink {
		target, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("read symlink target for %s: %w", relPath, err)
		}
		_ = os.Remove(fullPath)
		if err := os.Symlink(string(target), fullPath); err != nil {
			return fmt.Errorf("create symlink %s: %w", relPath, err)
		}
		return nil
	}

	perm := worktreeFileMode(mode)
	// os.OpenFile with the final permission bits so freshly created files
	// never sit at the wrong mode mid-write.
	f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("create file %s: %w", relPath, err)
	}
	if _, err := io.Copy(f, reader); err != nil {
		_ = f.Close()
		return fmt.Errorf("write file %s: %w", relPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close file %s: %w", relPath, err)
	}
	// The process umask strips bits from OpenFile's mode parameter; chmod
	// restores the exact entry mode so the executable bit survives.
	if err := os.Chmod(fullPath, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", relPath, err)
	}
	return nil
}

// CheckoutRef force-checks out ref. A ref that names a local branch (bare
// name or refs/heads/... form) is checked out ATTACHED — HEAD follows the
// branch, matching the native backend and `git checkout <branch>`. Anything
// else that resolves to a bare commit (raw hash, tag, HEAD~n) is checked out
// detached. CheckoutRef used to detach unconditionally, silently drifting
// from the native backend's attached semantics for branch names.
func (b *GoGitBackend) CheckoutRef(ctx context.Context, repoPath, ref string) error {
	repo, err := openRepo("CheckoutRef", repoPath)
	if err != nil {
		return err
	}
	hash, err := resolveRev(repo, ref)
	if err != nil {
		return newGitError("CheckoutRef", repoPath, "", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return newGitError("CheckoutRef", repoPath, "", err)
	}

	branch := plumbing.ReferenceName(ref)
	if !branch.IsBranch() {
		branch = plumbing.ReferenceName("refs/heads/" + ref)
	}
	if _, err := repo.Reference(branch, true); err == nil {
		return wt.Checkout(&git.CheckoutOptions{Branch: branch, Force: true})
	}
	return wt.Checkout(&git.CheckoutOptions{Hash: hash, Force: true})
}

// CheckoutDetached force-checks out ref in detached HEAD state, even when the
// ref names a branch. Use it when the caller explicitly wants to pin HEAD to
// a commit rather than follow a branch (CheckoutRef attaches for branches).
func (b *GoGitBackend) CheckoutDetached(ctx context.Context, repoPath, ref string) error {
	repo, err := openRepo("CheckoutDetached", repoPath)
	if err != nil {
		return err
	}
	hash, err := resolveRev(repo, ref)
	if err != nil {
		return newGitError("CheckoutDetached", repoPath, "", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return newGitError("CheckoutDetached", repoPath, "", err)
	}
	return wt.Checkout(&git.CheckoutOptions{Hash: hash, Force: true})
}

func (b *GoGitBackend) CheckoutFiles(ctx context.Context, repoPath, ref string, files []string) error {
	repo, err := openRepo("CheckoutFiles", repoPath)
	if err != nil {
		return err
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return newGitError("CheckoutFiles", repoPath, "", err)
	}
	commitObj, err := repo.CommitObject(*hash)
	if err != nil {
		return newGitError("CheckoutFiles", repoPath, "", err)
	}
	tree, err := commitObj.Tree()
	if err != nil {
		return newGitError("CheckoutFiles", repoPath, "", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return newGitError("CheckoutFiles", repoPath, "", err)
	}
	var lastErr error
	for _, file := range files {
		treeFile, err := tree.File(file)
		if err != nil {
			lastErr = fmt.Errorf("file %s not found in tree: %w", file, err)
			continue
		}
		if err := writeWorktreeFile(repoPath, file, &treeFile.Blob, treeFile.Mode); err != nil {
			lastErr = err
			continue
		}
		if _, err := wt.Add(file); err != nil {
			lastErr = fmt.Errorf("git add %s: %w", file, err)
		}
	}
	return lastErr
}

// --- Internal helpers for advanced operations ---

func containsNullByte(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func (b *GoGitBackend) treeChanges(repoPath, from, to string) (object.Changes, error) {
	repo, err := openRepo("treeChanges", repoPath)
	if err != nil {
		return nil, err
	}
	commitFromHash, err := resolveRev(repo, from)
	if err != nil {
		return nil, newGitError("treeChanges", repoPath, "", err)
	}
	commitFrom, err := repo.CommitObject(commitFromHash)
	if err != nil {
		return nil, newGitError("treeChanges", repoPath, "", err)
	}
	commitToHash, err := resolveRev(repo, to)
	if err != nil {
		return nil, newGitError("treeChanges", repoPath, "", err)
	}
	commitTo, err := repo.CommitObject(commitToHash)
	if err != nil {
		return nil, newGitError("treeChanges", repoPath, "", err)
	}
	treeFrom, err := commitFrom.Tree()
	if err != nil {
		return nil, newGitError("treeChanges", repoPath, "", err)
	}
	treeTo, err := commitTo.Tree()
	if err != nil {
		return nil, newGitError("treeChanges", repoPath, "", err)
	}
	return object.DiffTree(treeFrom, treeTo)
}

func changePath(c *object.Change) string {
	if c.To.Name != "" {
		return c.To.Name
	}
	return c.From.Name
}
