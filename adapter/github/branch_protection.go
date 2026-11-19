package github

import (
	"context"
	"slices"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranchProtections implements provider.BranchProtectionManager. GitHub has
// no list endpoint for branch protections; this returns nil.
func (p *Provider) ListBranchProtections(ctx context.Context, owner, repo string) ([]*model.BranchProtection, error) {
	return nil, provider.Wrap(model.PlatformGitHub, "ListBranchProtections", provider.ErrNotImplemented)
}

// GetBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*model.BranchProtection, error) {
	protection, _, err := p.client.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "GetBranchProtection", err)
	}
	return convertProtection(branch, protection), nil
}

// CreateBranchProtection implements provider.BranchProtectionManager.
//
// GitHub 的保护端点只有一个 PUT(branches/{branch}/protection),语义是
// create-or-replace——直接落线即可创建(issue #73)。旧实现委托
// UpdateBranchProtection,而 Update 的「读当前保护再合并」前置在未保护
// 分支上必然 404 ⇒ create 路径死锁,EnsureBranchProtection 的
// EnsureCreated 分支(runtime/ensure.go)在 GitHub 上不可达。
// 创建按调用方给定的旋钮直出 wire,不做任何读回。三扩展字段(issue #171):
// Contexts 非空蕴含启用 required_status_checks(与 RequiredStatusChecks
// 显式 false 冲突时以 contexts 为准);RequireCodeOwnerReviews 进 PR
// reviews 请求体;EnforceAdmins 是请求体的裸 bool(go-github 形态),
// nil 取 false 缺省。
func (p *Provider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*model.BranchProtection, error) {
	req := &github.ProtectionRequest{
		RequiredPullRequestReviews: &github.PullRequestReviewsEnforcementRequest{
			RequiredApprovingReviewCount: opts.RequiredApprovingReviews,
		},
		AllowForcePushes: new(opts.AllowForcePushes),
		AllowDeletions:   new(opts.AllowDeletions),
	}
	if opts.RequireCodeOwnerReviews != nil {
		req.RequiredPullRequestReviews.RequireCodeOwnerReviews = *opts.RequireCodeOwnerReviews
	}
	if opts.EnforceAdmins != nil {
		req.EnforceAdmins = *opts.EnforceAdmins
	}
	if contexts := opts.Contexts; len(contexts) > 0 {
		// Contexts 非空蕴含启用 required_status_checks——与
		// RequiredStatusChecks 显式 false 冲突时以 contexts 为准
		// (options.go 类型注释的冲突处置,钉在
		// TestCreateBranchProtection_ContextsWinOverExplicitFalse)。
		req.RequiredStatusChecks = &github.RequiredStatusChecks{Strict: true, Contexts: &contexts}
	} else if opts.RequiredStatusChecks {
		req.RequiredStatusChecks = &github.RequiredStatusChecks{Strict: true}
	}
	protection, _, err := p.client.Repositories.UpdateBranchProtection(ctx, owner, repo, opts.BranchName, req)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "CreateBranchProtection", err)
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
//   - PR review 三布尔与 status checks 的 strict/contexts 原样保留;
//   - 三扩展字段(issue #171)nil 走保值:Contexts nil 保留 current 的
//     contexts(显式启用 checks 时也不再重建丢 contexts),RequireCodeOwner
//     Reviews / EnforceAdmins 从 current 回填;非 nil 按显式值覆盖,显式
//     false 照常落线(能关掉)。
//
// 对未保护分支的 Update 返回 GET 的 404(归 ErrNotFound):统一接口的
// 语义是 create 走 CreateBranchProtection / EnsureBranchProtection,
// Update 不隐式承担创建。
func (p *Provider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*model.BranchProtection, error) {
	current, _, err := p.client.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "UpdateBranchProtection", err)
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
	applyProtectionOverrides(req, current, opts)
	protection, _, err := p.client.Repositories.UpdateBranchProtection(ctx, owner, repo, branch, req)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "UpdateBranchProtection", err)
	}
	return convertProtection(branch, protection), nil
}

// DeleteBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	if _, err := p.client.Repositories.RemoveBranchProtection(ctx, owner, repo, branch); err != nil {
		return provider.Wrap(model.PlatformGitHub, "DeleteBranchProtection", err)
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

// applyProtectionOverrides 把 opts 的显式非 nil 值覆盖到已按 current
// 回填的 req 上(#74 保值族的「显式优先」半边;issue #171 扩展到三扩展
// 字段):
//
//   - RequiredApprovingReviews / RequireCodeOwnerReviews 覆盖 PR reviews
//     请求体,后者 req 缺席时先造空壳;
//   - EnforceAdmins 直接覆盖(request 体是裸 bool);
//   - RequiredStatusChecks 显式启用时保留 current 的 contexts(issue
//     #171)——旧实现重建 Strict:true 会把已配置的检查列表静默清空;
//   - Contexts nil = 不动(保值);显式空切片 = 清空 contexts 但 checks
//     保持启用;非空 = 覆盖且蕴含启用(与 RequiredStatusChecks 显式
//     false 冲突时以 contexts 为准,同 Create 的冲突处置)。启用时
//     strict 保留 current 值,current 缺席取 true。
func applyProtectionOverrides(req *github.ProtectionRequest, current *github.Protection, opts provider.UpdateBranchProtectionOptions) {
	if opts.RequiredApprovingReviews != nil {
		ensureReviewsRequest(req).RequiredApprovingReviewCount = *opts.RequiredApprovingReviews
	}
	if opts.RequireCodeOwnerReviews != nil {
		ensureReviewsRequest(req).RequireCodeOwnerReviews = *opts.RequireCodeOwnerReviews
	}
	if opts.EnforceAdmins != nil {
		req.EnforceAdmins = *opts.EnforceAdmins
	}
	strict := true
	if current.RequiredStatusChecks != nil {
		strict = current.RequiredStatusChecks.Strict
	}
	if opts.RequiredStatusChecks != nil {
		if *opts.RequiredStatusChecks {
			req.RequiredStatusChecks = &github.RequiredStatusChecks{Strict: strict}
			if current.RequiredStatusChecks != nil {
				req.RequiredStatusChecks.Contexts = current.RequiredStatusChecks.Contexts
			}
		} else {
			req.RequiredStatusChecks = nil
		}
	}
	if opts.Contexts != nil {
		contexts := slices.Clone(*opts.Contexts)
		if req.RequiredStatusChecks == nil && len(contexts) > 0 {
			req.RequiredStatusChecks = &github.RequiredStatusChecks{Strict: strict}
		}
		if req.RequiredStatusChecks != nil {
			req.RequiredStatusChecks.Contexts = &contexts
		}
	}
	if opts.AllowForcePushes != nil {
		req.AllowForcePushes = opts.AllowForcePushes
	}
	if opts.AllowDeletions != nil {
		req.AllowDeletions = opts.AllowDeletions
	}
}

func ensureReviewsRequest(req *github.ProtectionRequest) *github.PullRequestReviewsEnforcementRequest {
	if req.RequiredPullRequestReviews == nil {
		req.RequiredPullRequestReviews = &github.PullRequestReviewsEnforcementRequest{}
	}
	return req.RequiredPullRequestReviews
}

// convertProtection maps a github.Protection to a model.BranchProtection.
// 三扩展字段(issue #171)按对象在场性回填:平台响应缺对应对象时指针
// 保持 nil(「未返回」≠「false」)。
func convertProtection(branch string, p *github.Protection) *model.BranchProtection {
	bp := &model.BranchProtection{BranchName: branch}
	if p.RequiredStatusChecks != nil {
		bp.RequiredStatusChecks = true
		bp.Contexts = slices.Clone(p.RequiredStatusChecks.GetContexts())
	}
	if p.RequiredPullRequestReviews != nil {
		bp.RequiredApprovingReviews = p.RequiredPullRequestReviews.RequiredApprovingReviewCount
		bp.RequireCodeOwnerReviews = new(p.RequiredPullRequestReviews.RequireCodeOwnerReviews)
	}
	if p.EnforceAdmins != nil {
		bp.EnforceAdmins = new(p.EnforceAdmins.Enabled)
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
