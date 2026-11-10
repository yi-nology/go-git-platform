package github

import (
	"context"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranchProtections implements provider.BranchProtectionManager. GitHub has
// no list endpoint for branch protections; this returns nil.
func (p *Provider) ListBranchProtections(ctx context.Context, owner, repo string) ([]*provider.BranchProtection, error) {
	return nil, provider.Wrap(provider.PlatformGitHub, "ListBranchProtections", provider.ErrNotImplemented)
}

// GetBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*provider.BranchProtection, error) {
	protection, _, err := p.client.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetBranchProtection", err)
	}
	return convertProtection(branch, protection), nil
}

// CreateBranchProtection implements provider.BranchProtectionManager. GitHub has
// no dedicated create endpoint; this delegates to UpdateBranchProtection.
func (p *Provider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*provider.BranchProtection, error) {
	return p.UpdateBranchProtection(ctx, owner, repo, opts.BranchName, provider.UpdateBranchProtectionOptions{
		RequiredApprovingReviews: &opts.RequiredApprovingReviews,
		RequiredStatusChecks:     &opts.RequiredStatusChecks,
		AllowForcePushes:         &opts.AllowForcePushes,
		AllowDeletions:           &opts.AllowDeletions,
	})
}

// UpdateBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*provider.BranchProtection, error) {
	// Start from the current protection to preserve fields we don't touch.
	current, _, err := p.client.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateBranchProtection", err)
	}
	req := &github.ProtectionRequest{
		RequiredStatusChecks: current.RequiredStatusChecks,
		EnforceAdmins:        true,
	}
	// Carry over existing PR review settings.
	if current.RequiredPullRequestReviews != nil {
		req.RequiredPullRequestReviews = &github.PullRequestReviewsEnforcementRequest{
			DismissStaleReviews:          current.RequiredPullRequestReviews.DismissStaleReviews,
			RequireCodeOwnerReviews:      current.RequiredPullRequestReviews.RequireCodeOwnerReviews,
			RequiredApprovingReviewCount: current.RequiredPullRequestReviews.RequiredApprovingReviewCount,
		}
	}
	// Apply caller overrides.
	if opts.RequiredApprovingReviews != nil {
		if req.RequiredPullRequestReviews == nil {
			req.RequiredPullRequestReviews = &github.PullRequestReviewsEnforcementRequest{}
		}
		req.RequiredPullRequestReviews.RequiredApprovingReviewCount = *opts.RequiredApprovingReviews
	}
	if opts.RequiredStatusChecks != nil {
		if *opts.RequiredStatusChecks {
			req.RequiredStatusChecks = &github.RequiredStatusChecks{Strict: true}
		} else {
			req.RequiredStatusChecks = nil
		}
	}
	if opts.AllowForcePushes != nil {
		req.AllowForcePushes = opts.AllowForcePushes
	}
	if opts.AllowDeletions != nil {
		req.AllowDeletions = opts.AllowDeletions
	}
	protection, _, err := p.client.Repositories.UpdateBranchProtection(ctx, owner, repo, branch, req)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateBranchProtection", err)
	}
	return convertProtection(branch, protection), nil
}

// DeleteBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	if _, err := p.client.Repositories.RemoveBranchProtection(ctx, owner, repo, branch); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DeleteBranchProtection", err)
	}
	return nil
}

// convertProtection maps a github.Protection to a provider.BranchProtection.
func convertProtection(branch string, p *github.Protection) *provider.BranchProtection {
	bp := &provider.BranchProtection{BranchName: branch}
	if p.RequiredStatusChecks != nil {
		bp.RequiredStatusChecks = true
	}
	if p.RequiredPullRequestReviews != nil {
		bp.RequiredApprovingReviews = p.RequiredPullRequestReviews.RequiredApprovingReviewCount
	}
	if p.AllowForcePushes != nil {
		bp.AllowForcePushes = p.AllowForcePushes.Enabled
	}
	if p.AllowDeletions != nil {
		bp.AllowDeletions = p.AllowDeletions.Enabled
	}
	return bp
}

var _ provider.BranchProtectionManager = (*Provider)(nil)
