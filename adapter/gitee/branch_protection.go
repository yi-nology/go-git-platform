package gitee

import (
	"context"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranchProtections implements provider.BranchProtectionManager. Gitee has
// no list endpoint for branch protections; this returns nil.
func (p *Provider) ListBranchProtections(ctx context.Context, owner, repo string) ([]*model.BranchProtection, error) {
	return nil, provider.Wrap(model.PlatformGitee, "ListBranchProtections", provider.ErrNotImplemented)
}

// GetBranchProtection implements provider.BranchProtectionManager. Gitee has no
// get endpoint for branch protections; this returns nil.
func (p *Provider) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*model.BranchProtection, error) {
	return nil, provider.Wrap(model.PlatformGitee, "GetBranchProtection", provider.ErrNotImplemented)
}

// CreateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*model.BranchProtection, error) {
	_, _, err := p.client.Repositories.SetBranchProtection(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(opts.BranchName))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "CreateBranchProtection", err)
	}
	return &model.BranchProtection{
		BranchName: opts.BranchName,
	}, nil
}

// UpdateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*model.BranchProtection, error) {
	_, _, err := p.client.Repositories.SetBranchProtection(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(branch))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "UpdateBranchProtection", err)
	}
	return &model.BranchProtection{
		BranchName: branch,
	}, nil
}

// DeleteBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	_, err := p.client.Repositories.RemoveBranchProtection(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(branch))
	if err != nil {
		return provider.Wrap(model.PlatformGitee, "DeleteBranchProtection", err)
	}
	return nil
}

var _ provider.BranchProtectionManager = (*Provider)(nil)
