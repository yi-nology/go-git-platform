package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetFileContent implements provider.FileManager.
func (p *Provider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	return giteafamily.GetFileContent(ctx, p.family, p.ports, owner, repo, path, ref)
}

// CreateFile implements provider.FileManager.
func (p *Provider) CreateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	return giteafamily.CreateFile(ctx, p.family, p.ports, owner, repo, opts)
}

// UpdateFile implements provider.FileManager.
func (p *Provider) UpdateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	return giteafamily.UpdateFile(ctx, p.family, p.ports, owner, repo, opts)
}

// DeleteFile implements provider.FileManager.
func (p *Provider) DeleteFile(ctx context.Context, owner, repo string, opts provider.FileDeleteOptions) (*provider.FileResult, error) {
	return giteafamily.DeleteFile(ctx, p.family, p.ports, owner, repo, opts)
}

var _ provider.FileManager = (*Provider)(nil)
