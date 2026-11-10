package gitee

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranchProtections implements provider.BranchProtectionManager. Gitee has
// no list endpoint for branch protections; this returns nil.
func (p *Provider) ListBranchProtections(ctx context.Context, owner, repo string) ([]*provider.BranchProtection, error) {
	return nil, provider.Wrap(provider.PlatformGitee, "ListBranchProtections", provider.ErrNotImplemented)
}

// GetBranchProtection implements provider.BranchProtectionManager. Gitee has no
// get endpoint for branch protections; this returns nil.
func (p *Provider) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*provider.BranchProtection, error) {
	return nil, provider.Wrap(provider.PlatformGitee, "GetBranchProtection", provider.ErrNotImplemented)
}

// CreateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*provider.BranchProtection, error) {
	_, _, err := p.client.Repositories.SetBranchProtection(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(opts.BranchName))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateBranchProtection", err)
	}
	return &provider.BranchProtection{
		BranchName: opts.BranchName,
	}, nil
}

// UpdateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*provider.BranchProtection, error) {
	_, _, err := p.client.Repositories.SetBranchProtection(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(branch))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "UpdateBranchProtection", err)
	}
	return &provider.BranchProtection{
		BranchName: branch,
	}, nil
}

// DeleteBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	_, err := p.client.Repositories.RemoveBranchProtection(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(branch))
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "DeleteBranchProtection", err)
	}
	return nil
}

var _ provider.BranchProtectionManager = (*Provider)(nil)
