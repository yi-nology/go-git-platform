package tencentcode

import (
	"context"

	gongfeng "github.com/yi-nology/go-gongfeng"

	"github.com/yi-nology/go-git-platform/provider"
)

// GetFileContent implements provider.FileManager.
func (p *Provider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	pid := owner + "/" + repo
	opts := &gongfeng.GetFileOptions{
		FilePath: gongfeng.Ptr(path),
	}
	if ref != "" {
		opts.Ref = gongfeng.Ptr(ref)
	}
	file, _, err := p.client.Repositories.GetFile(ctx, pid, opts)
	if err != nil {
		return "", provider.Wrap(provider.PlatformTencentCode, "GetFileContent", err)
	}
	return provider.DecodeFileContent(provider.PlatformTencentCode, file.Encoding, file.Content)
}

// CreateFile implements provider.FileManager.
func (p *Provider) CreateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	return p.mutateFile(ctx, "CreateFile", owner, repo, opts)
}

// UpdateFile implements provider.FileManager.
func (p *Provider) UpdateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	return p.mutateFile(ctx, "UpdateFile", owner, repo, opts)
}

// DeleteFile implements provider.FileManager.
func (p *Provider) DeleteFile(ctx context.Context, owner, repo string, opts provider.FileDeleteOptions) (*provider.FileResult, error) {
	pid := owner + "/" + repo
	deleteOpts := &gongfeng.DeleteFileOptions{
		FilePath:      gongfeng.Ptr(opts.Path),
		CommitMessage: gongfeng.Ptr(opts.Message),
	}
	if opts.Branch != "" {
		deleteOpts.BranchName = gongfeng.Ptr(opts.Branch)
	}
	_, err := p.client.Repositories.DeleteFile(ctx, pid, deleteOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "DeleteFile", err)
	}
	return &provider.FileResult{}, nil
}

// mutateFile is the shared body for CreateFile and UpdateFile.
func (p *Provider) mutateFile(ctx context.Context, op, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	pid := owner + "/" + repo
	createOpts := &gongfeng.CreateFileOptions{
		FilePath:      gongfeng.Ptr(opts.Path),
		Content:       gongfeng.Ptr(opts.Content),
		CommitMessage: gongfeng.Ptr(opts.Message),
	}
	if opts.Branch != "" {
		createOpts.BranchName = gongfeng.Ptr(opts.Branch)
	}
	if opts.Author != "" || opts.Email != "" {
		if opts.Author != "" {
			createOpts.Encoding = nil // keep default
		}
	}

	if op == "CreateFile" {
		file, _, err := p.client.Repositories.CreateFile(ctx, pid, createOpts)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformTencentCode, op, err)
		}
		return &provider.FileResult{CommitSHA: file.CommitID}, nil
	}

	updateOpts := &gongfeng.UpdateFileOptions{
		FilePath:      gongfeng.Ptr(opts.Path),
		Content:       gongfeng.Ptr(opts.Content),
		CommitMessage: gongfeng.Ptr(opts.Message),
	}
	if opts.Branch != "" {
		updateOpts.BranchName = gongfeng.Ptr(opts.Branch)
	}
	file, _, err := p.client.Repositories.UpdateFile(ctx, pid, updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, op, err)
	}
	return &provider.FileResult{CommitSHA: file.CommitID}, nil
}

var _ provider.FileManager = (*Provider)(nil)
