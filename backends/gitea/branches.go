package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranches implements provider.BranchManager.
func (p *Provider) ListBranches(ctx context.Context, owner, repo string) ([]*provider.PlatformBranch, error) {
	return giteafamily.ListBranches(ctx, p.family, p.ports, owner, repo)
}

// CreateBranch implements provider.BranchManager.
func (p *Provider) CreateBranch(ctx context.Context, owner, repo, branch, ref string) (*provider.PlatformBranch, error) {
	return giteafamily.CreateBranch(ctx, p.family, p.ports, owner, repo, branch, ref)
}

// DeleteBranch implements provider.BranchManager.
func (p *Provider) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	return giteafamily.DeleteBranch(ctx, p.family, p.ports, owner, repo, branch)
}

var _ provider.BranchManager = (*Provider)(nil)
