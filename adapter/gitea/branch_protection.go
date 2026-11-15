package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/adapter/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranchProtections implements provider.BranchProtectionManager.
func (p *Provider) ListBranchProtections(ctx context.Context, owner, repo string) ([]*model.BranchProtection, error) {
	return giteafamily.ListBranchProtections(ctx, p.family, p.ports, owner, repo)
}

// GetBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*model.BranchProtection, error) {
	return giteafamily.GetBranchProtection(ctx, p.family, p.ports, owner, repo, branch)
}

// CreateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*model.BranchProtection, error) {
	return giteafamily.CreateBranchProtection(ctx, p.family, p.ports, owner, repo, opts)
}

// UpdateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*model.BranchProtection, error) {
	return giteafamily.UpdateBranchProtection(ctx, p.family, p.ports, owner, repo, branch, opts)
}

// DeleteBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	return giteafamily.DeleteBranchProtection(ctx, p.family, p.ports, owner, repo, branch)
}

var _ provider.BranchProtectionManager = (*Provider)(nil)
