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

// CreateBranchProtection implements provider.BranchProtectionManager.
//
// GitHub 的保护端点只有一个 PUT(branches/{branch}/protection),语义是
// create-or-replace——直接落线即可创建(issue #73)。旧实现委托
// UpdateBranchProtection,而 Update 的「读当前保护再合并」前置在未保护
// 分支上必然 404 ⇒ create 路径死锁,EnsureBranchProtection 的
// EnsureCreated 分支(provider/ensure.go)在 GitHub 上不可达。
// 创建按调用方给定的四旋钮直出 wire,不做任何读回。
func (p *Provider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*provider.BranchProtection, error) {
	req := &github.ProtectionRequest{
		RequiredPullRequestReviews: &github.PullRequestReviewsEnforcementRequest{
			RequiredApprovingReviewCount: opts.RequiredApprovingReviews,
		},
		AllowForcePushes: new(opts.AllowForcePushes),
		AllowDeletions:   new(opts.AllowDeletions),
	}
	if opts.RequiredStatusChecks {
		req.RequiredStatusChecks = &github.RequiredStatusChecks{Strict: true}
	}
	protection, _, err := p.client.Repositories.UpdateBranchProtection(ctx, owner, repo, opts.BranchName, req)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateBranchProtection", err)
	}
	return convertProtection(opts.BranchName, protection), nil
}

// UpdateBranchProtection implements provider.BranchProtectionManager.
//
// 「nil 字段不变」契约(options.go)的逐字段兑现(issue #74):GitHub 的
// 保护 PUT 是 create-or-replace 全量替换语义,不带 omitempty 的字段
// (enforce_admins)与显式 null 的字段(restrictions、required_status_checks)
// 一律出线,任何未被回填的字段都会被复位。因此先 GET 当前保护,以
// current 全字段回填,再把 opts 的非 nil 值覆盖其上:
//
//   - enforce_admins 透传当前值——旧实现硬编码 true,一次「只改评审数」
//     的更新会把管理员强制静默打开;
//   - AllowForcePushes/AllowDeletions 为 nil 时回填当前值——omitempty 使
//     nil 不出线,全量替换下等价于复位为 false;
//   - Restrictions 从 current 回填(Users/Teams/Apps)——不回填则 wire 出
//     "restrictions": null,push/merge 限制被清空;
//   - PR review 三布尔与 status checks 的 strict/contexts 原样保留。
//
// 对未保护分支的 Update 返回 GET 的 404(归 ErrNotFound):统一接口的
// 语义是 create 走 CreateBranchProtection / EnsureBranchProtection,
// Update 不隐式承担创建。
func (p *Provider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*provider.BranchProtection, error) {
	current, _, err := p.client.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateBranchProtection", err)
	}
	req := &github.ProtectionRequest{
		RequiredStatusChecks: current.RequiredStatusChecks,
		EnforceAdmins:        current.EnforceAdmins.GetEnabled(),
	}
	// Carry over existing PR review settings.
	if current.RequiredPullRequestReviews != nil {
		req.RequiredPullRequestReviews = &github.PullRequestReviewsEnforcementRequest{
			DismissStaleReviews:          current.RequiredPullRequestReviews.DismissStaleReviews,
			RequireCodeOwnerReviews:      current.RequiredPullRequestReviews.RequireCodeOwnerReviews,
			RequiredApprovingReviewCount: current.RequiredPullRequestReviews.RequiredApprovingReviewCount,
			RequireLastPushApproval:      new(current.RequiredPullRequestReviews.RequireLastPushApproval),
		}
	}
	// Carry over push/merge allowances and restrictions (issue #74).
	if current.AllowForcePushes != nil {
		req.AllowForcePushes = new(current.AllowForcePushes.Enabled)
	}
	if current.AllowDeletions != nil {
		req.AllowDeletions = new(current.AllowDeletions.Enabled)
	}
	if current.Restrictions != nil {
		req.Restrictions = &github.BranchRestrictionsRequest{
			Users: loginsOf(current.Restrictions.Users),
			Teams: slugsOf(current.Restrictions.Teams),
			Apps:  appSlugsOf(current.Restrictions.Apps),
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

// loginsOf/slugsOf/appSlugsOf project the GET-side restriction entities
// onto the PUT-side string slices (issue #74 backfill). Nil-safe: an empty
// entity list round-trips as an empty (non-nil) slice, matching the
// BranchRestrictionsRequest contract ("use []string{} instead of nil").
func loginsOf(users []*github.User) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, u.GetLogin())
	}
	return out
}

func slugsOf(teams []*github.Team) []string {
	out := make([]string, 0, len(teams))
	for _, t := range teams {
		out = append(out, t.GetSlug())
	}
	return out
}

func appSlugsOf(apps []*github.App) []string {
	out := make([]string, 0, len(apps))
	for _, a := range apps {
		out = append(out, a.GetSlug())
	}
	return out
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
