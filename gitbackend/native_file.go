package gitbackend

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

// --- File operations ---

func (b *NativeGitBackend) GetFileAtRevision(ctx context.Context, repoPath, path, ref string) ([]byte, error) {
	if err := checkNameArg("GetFileAtRevision", ref); err != nil {
		return nil, err
	}
	// 契约口径：空 ref = HEAD。旧实现裸 ":path" 读的是 **index**——index
	// 与 HEAD 不一致时静默返回暂存内容而非提交内容（与 gogit 后端读 HEAD
	// 分裂，issue #108）
	if ref == "" {
		ref = "HEAD"
	}
	spec := fmt.Sprintf("%s:%s", ref, path)
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"show", spec}, AuthConfig{})
	if err != nil {
		if strings.Contains(stderr, "does not exist") || strings.Contains(stderr, "exists on disk") {
			return nil, newGitError("GetFileAtRevision", repoPath, stderr, ErrFileNotFound)
		}
		return nil, newGitError("GetFileAtRevision", repoPath, stderr, err)
	}
	return []byte(stdout), nil
}

func (b *NativeGitBackend) GetFileHistory(ctx context.Context, repoPath, path string, limit int) ([]CommitInfo, error) {
	// The -<limit> flag must precede the "--" separator: anything after it
	// is a pathspec, and --follow rejects a second one outright.
	args := []string{"log", commitLogFormat, "--follow"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-%d", limit))
	}
	args = append(args, "--", path)
	stdout, stderr, err := b.runGit(ctx, repoPath, args, AuthConfig{})
	if err != nil {
		return nil, newGitError("GetFileHistory", repoPath, stderr, err)
	}
	return parseCommitLines(stdout), nil
}

// --- Tree and blob queries ---

func (b *NativeGitBackend) GetTree(ctx context.Context, repoPath, ref, dirPath string, recursive bool) ([]TreeEntry, error) {
	if err := checkNameArg("GetTree", ref); err != nil {
		return nil, err
	}
	lsArg := ref
	if dirPath != "" && dirPath != "." {
		lsArg = ref + ":" + dirPath
	}
	args := []string{"ls-tree", lsArg}
	if recursive {
		args = []string{"ls-tree", "-r", lsArg}
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, args, AuthConfig{})
	if err != nil {
		return nil, newGitError("GetTree", repoPath, stderr, err)
	}

	var entries []TreeEntry
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// format: <mode> <type> <hash>\t<name>
		tabIdx := strings.Index(line, "\t")
		if tabIdx < 0 {
			continue
		}
		meta := strings.Fields(line[:tabIdx])
		name := line[tabIdx+1:]
		if len(meta) < 3 {
			continue
		}
		entryType := TreeEntryFile
		if meta[1] == "tree" {
			entryType = TreeEntryDir
		}
		path := name
		if dirPath != "" && dirPath != "." {
			path = dirPath + "/" + name
		}
		entries = append(entries, TreeEntry{
			Name: name,
			Path: path,
			Type: entryType,
			Mode: meta[0],
			Hash: meta[2],
		})
	}
	return entries, nil
}

func (b *NativeGitBackend) GetBlob(ctx context.Context, repoPath, ref, filePath string) (*BlobContent, error) {
	if err := checkNameArg("GetBlob", ref); err != nil {
		return nil, err
	}
	if ref == "" {
		ref = "HEAD"
	}
	spec := ref + ":" + filePath
	data, stderr, err := b.runGit(ctx, repoPath, []string{"show", spec}, AuthConfig{})
	if err != nil {
		if strings.Contains(stderr, "does not exist") || strings.Contains(stderr, "exists on disk") {
			return nil, newGitError("GetBlob", repoPath, stderr, ErrFileNotFound)
		}
		return nil, newGitError("GetBlob", repoPath, stderr, err)
	}

	raw := []byte(data)
	isBinary := !isText(raw)
	result := &BlobContent{
		Size:     int64(len(raw)),
		IsBinary: isBinary,
	}
	if isBinary {
		result.Content = base64.StdEncoding.EncodeToString(raw)
		result.Encoding = EncodingBase64
	} else {
		result.Content = data
		result.Encoding = EncodingUTF8
	}
	return result, nil
}

// --- Checkout helpers ---

func (b *NativeGitBackend) CheckoutRef(ctx context.Context, repoPath, ref string) error {
	// checkout 不吃 `--`（那是 pathspec 语义），ref="-b evil" 这类形态
	// 靠名字校验挡（issue #109）
	if err := checkNameArg("CheckoutRef", ref); err != nil {
		return err
	}
	_, stderr, err := b.runGit(ctx, repoPath, []string{"checkout", "--force", ref}, AuthConfig{})
	if err != nil {
		return newGitError("CheckoutRef", repoPath, stderr, err)
	}
	return nil
}

// CheckoutDetached implements BranchOps: `git checkout --detach --force`
// pins HEAD to ref's commit even when ref names a branch.
func (b *NativeGitBackend) CheckoutDetached(ctx context.Context, repoPath, ref string) error {
	if err := checkNameArg("CheckoutDetached", ref); err != nil {
		return err
	}
	_, stderr, err := b.runGit(ctx, repoPath, []string{"checkout", "--detach", "--force", ref}, AuthConfig{})
	if err != nil {
		return newGitError("CheckoutDetached", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) CheckoutFiles(ctx context.Context, repoPath, ref string, files []string) error {
	if err := checkNameArg("CheckoutFiles", ref); err != nil {
		return err
	}
	args := append([]string{"checkout", ref, "--"}, files...)
	_, stderr, err := b.runGit(ctx, repoPath, args, AuthConfig{})
	if err != nil {
		return newGitError("CheckoutFiles", repoPath, stderr, err)
	}
	return nil
}
