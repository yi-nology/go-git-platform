package gitcode

import (
	"context"
	"net/url"
	"strings"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// escPath escapes a multi-segment file path, preserving the "/"
// separators: each segment is percent-encoded, the separators are not
// (the same discipline as the gitee backend's methods.go — the SDK
// interpolates path parameters verbatim, issue #84).
func escPath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// GetFileContent implements provider.FileManager.
func (p *Provider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	content, err := p.client.GetRepositoryContent(ctx, backendutil.Esc(owner), backendutil.Esc(repo), escPath(path), backendutil.Esc(ref))
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitCode, "GetFileContent", err)
	}
	// **必须按 Encoding 解码**:GitCode 的内容端点返回 base64,
	// 以前原样返回 ⇒ 调用方拿到的是 base64 串(读文件全错)。
	return provider.DecodeFileContent(provider.PlatformGitCode, content.Encoding, content.Content)
}

// CreateFile implements provider.FileManager.
func (p *Provider) CreateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	result, err := p.client.CreateFile(ctx, backendutil.Esc(owner), backendutil.Esc(repo), escPath(opts.Path), gitcode.CreateFileOptions{
		Message: opts.Message, Content: opts.Content, Branch: opts.Branch,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateFile", err)
	}
	return fileResultFrom(result), nil
}

// UpdateFile implements provider.FileManager.
func (p *Provider) UpdateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	result, err := p.client.UpdateFile(ctx, backendutil.Esc(owner), backendutil.Esc(repo), escPath(opts.Path), gitcode.UpdateFileOptions{
		Message: opts.Message, Content: opts.Content, SHA: opts.SHA, Branch: opts.Branch,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateFile", err)
	}
	return fileResultFrom(result), nil
}

// DeleteFile implements provider.FileManager.
func (p *Provider) DeleteFile(ctx context.Context, owner, repo string, opts provider.FileDeleteOptions) (*provider.FileResult, error) {
	result, err := p.client.DeleteFile(ctx, backendutil.Esc(owner), backendutil.Esc(repo), escPath(opts.Path), gitcode.DeleteFileOptions{
		Message: opts.Message, SHA: opts.SHA, Branch: opts.Branch,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "DeleteFile", err)
	}
	commitSHA := ""
	if result != nil {
		if result.Commit != nil {
			commitSHA = result.Commit.SHA
		}
	}
	return &provider.FileResult{CommitSHA: commitSHA}, nil
}

func fileResultFrom(result *gitcode.FileResult) *provider.FileResult {
	if result == nil {
		return &provider.FileResult{}
	}
	sha := ""
	commitSHA := ""
	if result.Content != nil {
		sha = result.Content.SHA
	}
	if result.Commit != nil {
		commitSHA = result.Commit.SHA
	}
	return &provider.FileResult{SHA: sha, CommitSHA: commitSHA}
}

var _ provider.FileManager = (*Provider)(nil)
