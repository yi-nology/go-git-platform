package github

import (
	"context"
	"io"

	"github.com/google/go-github/v93/github"
	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/yi-nology/go-git-platform/provider"
)

// GetFileContent implements provider.FileManager.
func (p *Provider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	opts := &github.RepositoryContentGetOptions{Ref: ref}
	rc, _, err := p.client.Repositories.DownloadContents(ctx, owner, repo, path, opts)
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitHub, "GetFileContent", err)
	}
	if rc == nil {
		return "", oekiterrors.Errorf("github: file not found: %s", path)
	}
	defer func() { _ = rc.Close() }()
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
