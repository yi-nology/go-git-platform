package github

import (
	"context"
	"io"

	"github.com/google/go-github/v93/github"
	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/yi-nology/go-git-platform/provider"
)

// GetFileContent implements provider.FileManager.
//
// DownloadContents 是两跳(contents 元数据 + download_url 取字节),go-github
// 的契约明示第二跳可能"失败却不报错":非 2xx 时返回 (错误页 body, nil err)
// (issue #71)——所以除了 err 之外**必须**检查 resp.StatusCode,429/5xx/404
// 的错误页文本不得当归档内容返回。非 2xx 经 provider.New 归类
// (404→ErrNotFound、429→ErrRateLimited……)。
func (p *Provider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	opts := &github.RepositoryContentGetOptions{Ref: ref}
	rc, resp, err := p.client.Repositories.DownloadContents(ctx, owner, repo, path, opts)
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitHub, "GetFileContent", err)
	}
	if rc == nil {
		return "", oekiterrors.Errorf("github: file not found: %s", path)
	}
	defer func() { _ = rc.Close() }()
	if resp != nil && resp.StatusCode >= 400 {
		// 错误体只截前 4KB 进错误信息,避免把超大错误页整个吞进内存。
		body, _ := io.ReadAll(io.LimitReader(rc, 4096))
		return "", provider.New(provider.PlatformGitHub, "GetFileContent", resp.StatusCode, string(body))
	}
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitHub, "GetFileContent", err)
	}
	return string(data), nil
}

// CreateFile implements provider.FileManager.
func (p *Provider) CreateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	optsReq := &github.RepositoryContentFileOptions{
		Message: new(opts.Message),
		Content: []byte(opts.Content),
	}
	if opts.Branch != "" {
		optsReq.Branch = new(opts.Branch)
	}
	if opts.Author != "" || opts.Email != "" {
		optsReq.Author = &github.CommitAuthor{Name: new(opts.Author), Email: new(opts.Email)}
	}
	resp, _, err := p.client.Repositories.CreateFile(ctx, owner, repo, opts.Path, optsReq)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateFile", err)
	}
	return &provider.FileResult{CommitSHA: resp.GetSHA()}, nil
}

// UpdateFile implements provider.FileManager.
func (p *Provider) UpdateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	optsReq := &github.RepositoryContentFileOptions{
		Message: new(opts.Message),
		Content: []byte(opts.Content),
	}
	if opts.SHA != "" {
		optsReq.SHA = new(opts.SHA)
	}
	if opts.Branch != "" {
		optsReq.Branch = new(opts.Branch)
	}
	if opts.Author != "" || opts.Email != "" {
		optsReq.Author = &github.CommitAuthor{Name: new(opts.Author), Email: new(opts.Email)}
	}
	resp, _, err := p.client.Repositories.UpdateFile(ctx, owner, repo, opts.Path, optsReq)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateFile", err)
	}
	return &provider.FileResult{CommitSHA: resp.GetSHA()}, nil
}

// DeleteFile implements provider.FileManager.
func (p *Provider) DeleteFile(ctx context.Context, owner, repo string, opts provider.FileDeleteOptions) (*provider.FileResult, error) {
	optsReq := &github.RepositoryContentFileOptions{
		Message: new(opts.Message),
	}
	if opts.SHA != "" {
		optsReq.SHA = new(opts.SHA)
	}
	if opts.Branch != "" {
		optsReq.Branch = new(opts.Branch)
	}
	if opts.Author != "" || opts.Email != "" {
		optsReq.Author = &github.CommitAuthor{Name: new(opts.Author), Email: new(opts.Email)}
	}
	resp, _, err := p.client.Repositories.DeleteFile(ctx, owner, repo, opts.Path, optsReq)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "DeleteFile", err)
	}
	return &provider.FileResult{CommitSHA: resp.GetSHA()}, nil
}

// compile-time guard
var _ provider.FileManager = (*Provider)(nil)
