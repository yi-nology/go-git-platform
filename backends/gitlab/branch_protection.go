package gitlab

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranchProtections implements provider.BranchProtectionManager.
func (p *Provider) ListBranchProtections(ctx context.Context, owner, repo string) ([]*model.BranchProtection, error) {
	branches, err := backendutil.AllPages(func(page int) ([]*gitlab.ProtectedBranch, error) {
		list, _, err := p.client.ProtectedBranches.ListProtectedBranches(pidOf(owner, repo), &gitlab.ListProtectedBranchesOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListBranchProtections", err)
	}
	result := make([]*model.BranchProtection, 0, len(branches))
	for _, b := range branches {
		result = append(result, convertProtectedBranch(b))
	}
	return result, nil
}

// GetBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*model.BranchProtection, error) {
	b, _, err := p.client.ProtectedBranches.GetProtectedBranch(pidOf(owner, repo), branch, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "GetBranchProtection", err)
	}
	return convertProtectedBranch(b), nil
}

// CreateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*model.BranchProtection, error) {
	protectOpts := &gitlab.ProtectRepositoryBranchesOptions{
		Name:           new(opts.BranchName),
		AllowForcePush: new(opts.AllowForcePushes),
	}
	b, _, err := p.client.ProtectedBranches.ProtectRepositoryBranches(pidOf(owner, repo), protectOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CreateBranchProtection", err)
	}
	return convertProtectedBranch(b), nil
}

// UpdateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*model.BranchProtection, error) {
	updateOpts := &gitlab.UpdateProtectedBranchOptions{}
	if opts.AllowForcePushes != nil {
		updateOpts.AllowForcePush = opts.AllowForcePushes
	}
	b, _, err := p.client.ProtectedBranches.UpdateProtectedBranch(pidOf(owner, repo), branch, updateOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "UpdateBranchProtection", err)
	}
	return convertProtectedBranch(b), nil
}

// DeleteBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	if _, err := p.client.ProtectedBranches.UnprotectRepositoryBranches(pidOf(owner, repo), branch, gitlab.WithContext(ctx)); err != nil {
		return provider.Wrap(model.PlatformGitLab, "DeleteBranchProtection", err)
	}
	return nil
}

// convertProtectedBranch maps a gitlab.ProtectedBranch to a
// model.BranchProtection.
func convertProtectedBranch(b *gitlab.ProtectedBranch) *model.BranchProtection {
	return &model.BranchProtection{
		BranchName:       b.Name,
		AllowForcePushes: b.AllowForcePush,
	}
}

var _ provider.BranchProtectionManager = (*Provider)(nil)
