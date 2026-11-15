package tencentcode

import (
	"context"

	gongfeng "github.com/yi-nology/go-gongfeng"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCR implements provider.ChangeRequestManager.
func (p *Provider) CreateCR(ctx context.Context, opts provider.CreateCROptions) (*model.ChangeRequest, error) {
	// #160：gongfeng 创建面无 work_in_progress 参数——Draft=true 显式
	// 拒绝（台账 Field:"opts.Draft" 登记），绝不静默产出非 draft MR。
	if opts.Draft {
		return nil, provider.Wrap(model.PlatformTencentCode, "CreateCR", provider.ErrDraftNotSupported)
	}
	pid := opts.Owner + "/" + opts.Repo
	createOpts := &gongfeng.CreateMergeRequestOptions{
		SourceBranch: gongfeng.Ptr(opts.SourceBranch),
		TargetBranch: gongfeng.Ptr(opts.TargetBranch),
		Title:        gongfeng.Ptr(opts.Title),
	}
	if opts.Description != "" {
		createOpts.Description = gongfeng.Ptr(opts.Description)
	}
	mr, _, err := p.client.MergeRequests.CreateMergeRequest(ctx, pid, createOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "CreateCR", err)
	}
	return convertMR(mr), nil
}

// GetCR implements provider.ChangeRequestManager.
func (p *Provider) GetCR(ctx context.Context, owner, repo, number string) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformTencentCode, "GetCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	mr, _, err := p.client.MergeRequests.GetMergeRequest(ctx, pid, n)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "GetCR", err)
	}
	return convertMR(mr), nil
}

// ListCRs implements provider.ChangeRequestManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListCRs(ctx context.Context, opts provider.ListCROptions) ([]*model.ChangeRequest, *int, error) {
	pid := opts.Owner + "/" + opts.Repo
	buildOpts := func(page, perPage int) *gongfeng.ListMergeRequestsOptions {
		listOpts := &gongfeng.ListMergeRequestsOptions{
			ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage},
		}
		if opts.State != "" {
			listOpts.State = gongfeng.Ptr(string(opts.State))
		}
		return listOpts
	}
	var (
		mrs      []*gongfeng.MergeRequest
		resp     *gongfeng.Response
		allPages bool
	)
	if opts.Page > 0 {
		// Caller-driven pagination: serve the requested page only. The
		// page response still carries the platform total, so it is kept.
		perPage := opts.PerPage
		if perPage <= 0 || perPage > provider.MaxPerPage {
			perPage = provider.MaxPerPage
		}
		list, r, err := p.client.MergeRequests.ListMergeRequests(ctx, pid, buildOpts(opts.Page, perPage))
		if err != nil {
			return nil, nil, provider.Wrap(model.PlatformTencentCode, "ListCRs", err)
		}
		mrs, resp = list, r
	} else {
		allPages = true
		var err error
		if mrs, err = backendutil.AllPages(func(page int) ([]*gongfeng.MergeRequest, error) {
			list, _, err := p.client.MergeRequests.ListMergeRequests(ctx, pid, buildOpts(page, provider.MaxPerPage))
			return list, err
		}); err != nil {
			return nil, nil, provider.Wrap(model.PlatformTencentCode, "ListCRs", err)
		}
	}
	crs := make([]*model.ChangeRequest, 0, len(mrs))
	for _, mr := range mrs {
		crs = append(crs, convertMR(mr))
	}
	if allPages {
		// Merged pages carry no single platform total; the walk merged every
		// page, so the merged size IS the complete count by construction.
		total := len(crs)
		return crs, &total, nil
	}
	// The platform's X-Total is authoritative when present; otherwise the
	// count is unknown (nil) rather than the page length.
	var total *int
	if resp != nil && resp.TotalItems > 0 {
		total = &resp.TotalItems
	}
	return crs, total, nil
}

// MergeCR implements provider.ChangeRequestManager.
//
// 选项面（issue #94）：Squash/RemoveSourceBranch 在 gongfeng SDK 的 Accept
// 线面上没有对应字段（AcceptMergeRequestOptions 只有 merge_commit_message）
// ——两个意图静默丢弃（平台默认：普通合并、保留源分支）按 DivergenceIgnore
// 登记进台账。幂等面与家族对齐（giteafamily crs.go 的 405-merged 恢复）：
// 已合并 MR 重试 MergeCR 返回当前 CR 而非硬失败——重试/幂等场景跨后端一致。
func (p *Provider) MergeCR(ctx context.Context, owner, repo, number string, opts provider.MergeCROptions) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformTencentCode, "MergeCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	// Pre-flight: check MR state. merged = idempotent success (retries of a
	// completed merge must not surface as errors); closed = a genuine hard
	// failure (merging a closed MR is caller error, not convergence).
	existingMR, _, err := p.client.MergeRequests.GetMergeRequest(ctx, pid, n)
	if err == nil {
		state := mapState(existingMR.State)
		if state == model.CRStateMerged {
			return convertMR(existingMR), nil
		}
		if state != model.CRStateOpened {
			return nil, provider.Wrapf(model.PlatformTencentCode, "MergeCR",
				"MR is not in 'opened' state (current: %s)", existingMR.State)
		}
	}
	acceptOpts := &gongfeng.AcceptMergeRequestOptions{}
	if opts.MergeCommitMessage != "" {
		acceptOpts.MergeCommitMessage = gongfeng.Ptr(opts.MergeCommitMessage)
	}
	mr, _, err := p.client.MergeRequests.AcceptMergeRequest(ctx, pid, n, acceptOpts)
	if err != nil {
		// Race recovery: the MR can complete between the pre-flight and the
		// accept — a re-read showing merged is still an idempotent success.
		if latest, _, getErr := p.client.MergeRequests.GetMergeRequest(ctx, pid, n); getErr == nil &&
			mapState(latest.State) == model.CRStateMerged {
			return convertMR(latest), nil
		}
		return nil, provider.Wrap(model.PlatformTencentCode, "MergeCR", err)
	}
	return convertMR(mr), nil
}

// CloseCR implements provider.ChangeRequestManager.
func (p *Provider) CloseCR(ctx context.Context, owner, repo, number string) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformTencentCode, "CloseCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	updateOpts := &gongfeng.UpdateMergeRequestOptions{
		StateEvent: gongfeng.Ptr("close"),
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(ctx, pid, n, updateOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "CloseCR", err)
	}
	return convertMR(mr), nil
}

// ReopenCR implements provider.ChangeRequestManager.
func (p *Provider) ReopenCR(ctx context.Context, owner, repo, number string) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformTencentCode, "ReopenCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	updateOpts := &gongfeng.UpdateMergeRequestOptions{
		StateEvent: gongfeng.Ptr("reopen"),
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(ctx, pid, n, updateOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "ReopenCR", err)
	}
	return convertMR(mr), nil
}

// UpdateCR implements provider.ChangeRequestManager.
//
// 三态收编（2A）：Resolve* 非 specified 不动，specified 空值即清空。
// 描述 ⇒ "description":""（*string+omitempty：非 nil 指针穿过 omitempty，
// 空串照上 wire）；指派单值面 ⇒ "assignee_id":0（Phase 3 验服务端）。
func (p *Provider) UpdateCR(ctx context.Context, owner, repo, number string, opts provider.UpdateCROptions) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformTencentCode, "UpdateCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	updateOpts := &gongfeng.UpdateMergeRequestOptions{}
	// Title/TargetBranch 既有守卫不动（legacy "" = 不变）。
	if opts.Title != "" {
		updateOpts.Title = gongfeng.Ptr(opts.Title)
	}
	if opts.TargetBranch != "" {
		updateOpts.TargetBranch = gongfeng.Ptr(opts.TargetBranch)
	}
	if desc, ok := opts.ResolveDescription(); ok {
		updateOpts.Description = gongfeng.Ptr(desc) // 清空 ⇒ "description":""
	}
	// 工蜂的 MR 线路只收单个 assignee_id——多派取第一个（mapping 分歧已
	// 登记）。三态：非 specified 不动；specified 空集 ⇒ 0 在场。
	if as, ok := opts.ResolveAssignees(); ok {
		if len(as) == 0 {
			updateOpts.AssigneeID = gongfeng.Ptr(0) // 单值面：0 在场（Phase 3 验服务端）
		} else {
			ids, err := p.resolveUserIDs(ctx, "UpdateCR", as[:1]) // 既有 first-login mapping 不变
			if err != nil {
				return nil, err
			}
			if len(ids) > 0 {
				updateOpts.AssigneeID = gongfeng.Ptr(ids[0])
			}
		}
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(ctx, pid, n, updateOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "UpdateCR", err)
	}
	return convertMR(mr), nil
}

// UpdateCRLabels implements provider.ChangeRequestManager.
//
// Tencent Code's API no longer supports setting labels via UpdateMergeRequest.
func (p *Provider) UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	return provider.Wrap(model.PlatformTencentCode, "UpdateCRLabels", provider.ErrNotImplemented)
}

// ListCRComments implements provider.ChangeRequestManager. The endpoint has
// no caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListCRComments(ctx context.Context, owner, repo, number string) ([]*model.CRComment, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformTencentCode, "ListCRComments", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	notes, err := backendutil.AllPages(func(page int) ([]*gongfeng.Note, error) {
		list, _, err := p.client.Notes.ListMergeRequestNotes(ctx, pid, n,
			&gongfeng.ListMergeRequestNotesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "ListCRComments", err)
	}
	result := make([]*model.CRComment, 0, len(notes))
	for _, n := range notes {
		comment := &model.CRComment{
			ID:        int64(n.ID),
			Body:      n.Body,
			CreatedAt: n.CreatedAt.Time,
			UpdatedAt: n.UpdatedAt.Time,
		}
		if n.Author != nil {
			comment.Author = convertUser(n.Author)
		}
		result = append(result, comment)
	}
	return result, nil
}

// ListCRCommits implements provider.ChangeRequestManager. The endpoint has
// no caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListCRCommits(ctx context.Context, owner, repo, number string) ([]*model.CRCommit, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformTencentCode, "ListCRCommits", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	commits, err := backendutil.AllPages(func(page int) ([]*gongfeng.Commit, error) {
		list, _, err := p.client.MergeRequests.ListMergeRequestCommits(ctx, pid, n,
			&gongfeng.ListMergeRequestCommitsOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "ListCRCommits", err)
	}
	result := make([]*model.CRCommit, 0, len(commits))
	for _, c := range commits {
		result = append(result, &model.CRCommit{
			SHA:     c.ID,
			Message: c.Message,
			Author:  &model.CRUser{Name: c.AuthorName},
		})
	}
	return result, nil
}

var _ provider.ChangeRequestManager = (*Provider)(nil)
