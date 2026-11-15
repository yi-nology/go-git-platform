package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/adapter/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranches implements provider.BranchManager.
func (p *Provider) ListBranches(ctx context.Context, owner, repo string) ([]*model.PlatformBranch, error) {
	return giteafamily.ListBranches(ctx, p.family, p.ports, owner, repo)
}

// CreateBranch implements provider.BranchManager.
func (p *Provider) CreateBranch(ctx context.Context, owner, repo, branch, ref string) (*model.PlatformBranch, error) {
	return giteafamily.CreateBranch(ctx, p.family, p.ports, owner, repo, branch, ref)
}

// DeleteBranch implements provider.BranchManager.
func (p *Provider) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	return giteafamily.DeleteBranch(ctx, p.family, p.ports, owner, repo, branch)
}

var _ provider.BranchManager = (*Provider)(nil)
