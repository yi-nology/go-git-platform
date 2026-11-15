package tencentcode

import (
	"context"
	"io"
	"strconv"

	gongfeng "github.com/yi-nology/go-gongfeng"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// This file carries the 工蜂-native issue/MR workflow extras: subscriptions
// (watch), the cross-project issue listing, milestone issue listings and the
// MR / code-review archive downloads. All are TencentCodeExtras methods over
// the gongfeng SDK's Issues/MergeRequests/Milestones/Reviews services.

// GetIssueSubscription implements TencentCodeExtras.
func (p *Provider) GetIssueSubscription(ctx context.Context, owner, repo string, issueID int) (bool, error) {
	subscribed, _, err := p.client.Issues.GetIssueSubscription(ctx, pid(owner, repo), issueID)
	if err != nil {
		return false, provider.Wrap(model.PlatformTencentCode, "GetIssueSubscription", err)
	}
	return subscribed, nil
}

// SubscribeIssue implements TencentCodeExtras.
func (p *Provider) SubscribeIssue(ctx context.Context, owner, repo string, issueID int) error {
	if _, err := p.client.Issues.SubscribeIssue(ctx, pid(owner, repo), issueID); err != nil {
		return provider.Wrap(model.PlatformTencentCode, "SubscribeIssue", err)
	}
	return nil
}

// UnsubscribeIssue implements TencentCodeExtras.
func (p *Provider) UnsubscribeIssue(ctx context.Context, owner, repo string, issueID int) error {
	if _, err := p.client.Issues.UnsubscribeIssue(ctx, pid(owner, repo), issueID); err != nil {
		return provider.Wrap(model.PlatformTencentCode, "UnsubscribeIssue", err)
	}
	return nil
}

// GetMRSubscription implements TencentCodeExtras.
func (p *Provider) GetMRSubscription(ctx context.Context, owner, repo string, mrNumber int) (bool, error) {
	return p.mrSubscription(ctx, "GetMRSubscription", owner, repo, mrNumber)
}

// SubscribeMR implements TencentCodeExtras.
//
// gongfeng v0.5.0 起 PUT 只回状态码、无响应体（gongfeng issue #7：此前解
// 空 body 必然 EOF），服务端不再回显结果态——成功即已订阅，返回值由**已确认
// 的 PUT 语义**推导（幂等 PUT：2xx ⇒ 请求态已生效），不再读回显。
func (p *Provider) SubscribeMR(ctx context.Context, owner, repo string, mrNumber int) (bool, error) {
	if _, err := p.client.MergeRequests.SubscribeMR(ctx, pid(owner, repo), mrNumber); err != nil {
		return false, provider.Wrap(model.PlatformTencentCode, "SubscribeMR", err)
	}
	return true, nil
}

// UnsubscribeMR implements TencentCodeExtras.
//
// 与 SubscribeMR 同款：gongfeng v0.5.0 的 PUT 无响应体，成功即已退订
// （返回 false = 不再订阅），由已确认的 PUT 语义推导。
func (p *Provider) UnsubscribeMR(ctx context.Context, owner, repo string, mrNumber int) (bool, error) {
	if _, err := p.client.MergeRequests.UnsubscribeMR(ctx, pid(owner, repo), mrNumber); err != nil {
		return false, provider.Wrap(model.PlatformTencentCode, "UnsubscribeMR", err)
	}
	return false, nil
}

// mrSubscription is the shared read path of the MR subscription family.
//
// gongfeng v0.5.0 的 GET 解**裸布尔**（工蜂订阅面真实线形，v0.4.0 解进
// MRSubscription 结构体必然 unmarshal 错——gongfeng issue #7），这里直接
// 透传读到的布尔，没有回显结构可解。
func (p *Provider) mrSubscription(ctx context.Context, op, owner, repo string, mrNumber int) (bool, error) {
	subscribed, _, err := p.client.MergeRequests.GetMRSubscription(ctx, pid(owner, repo), mrNumber)
	if err != nil {
		return false, provider.Wrap(model.PlatformTencentCode, op, err)
	}
	return subscribed, nil
}

// ListUserIssues implements TencentCodeExtras: the global /issues listing
// across projects, one caller-driven page per call (Page 0 = first page).
func (p *Provider) ListUserIssues(ctx context.Context, opts ListUserIssuesOptions) ([]*model.Issue, error) {
	listOpts := &gongfeng.ListIssuesOptions{
		ListOptions: gongfeng.ListOptions{Page: opts.Page, PerPage: opts.PerPage},
	}
	if opts.State != "" {
		s := opts.State
		if s == string(model.IssueStateOpen) {
			s = "opened" // 工蜂's issues API vocabulary; inbound convertIssue maps back
		}
		listOpts.State = gongfeng.Ptr(s)
	}
	if opts.Labels != "" {
		listOpts.Labels = gongfeng.Ptr(opts.Labels)
	}
	issues, _, err := p.client.Issues.ListUserIssues(ctx, listOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "ListUserIssues", err)
	}
	return convertIssues(issues), nil
}

// ListMilestoneIssues implements TencentCodeExtras: one caller-driven page
// of a milestone's issues (page 0 = first page).
func (p *Provider) ListMilestoneIssues(ctx context.Context, owner, repo string, milestoneID, page, perPage int) ([]*model.Issue, error) {
	issues, _, err := p.client.Milestones.ListMilestoneIssues(ctx, pid(owner, repo), milestoneID,
		&gongfeng.ListMilestoneIssuesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage}})
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "ListMilestoneIssues", err)
	}
	result := make([]*model.Issue, 0, len(issues))
	for _, mi := range issues {
		result = append(result, convertMilestoneIssue(mi))
	}
	return result, nil
}

// DownloadMergeRequestChangedFiles implements TencentCodeExtras.
func (p *Provider) DownloadMergeRequestChangedFiles(ctx context.Context, owner, repo string, mrNumber int, w io.Writer) error {
	if _, err := p.client.MergeRequests.DownloadMergeRequestChangedFiles(ctx, pid(owner, repo), mrNumber, w); err != nil {
		return provider.Wrap(model.PlatformTencentCode, "DownloadMergeRequestChangedFiles", err)
	}
	return nil
}

// DownloadCommitReviewChangedFiles implements TencentCodeExtras.
func (p *Provider) DownloadCommitReviewChangedFiles(ctx context.Context, owner, repo string, reviewID int, w io.Writer) error {
	if _, err := p.client.Reviews.DownloadCommitReviewChangedFiles(ctx, pid(owner, repo), reviewID, w); err != nil {
		return provider.Wrap(model.PlatformTencentCode, "DownloadCommitReviewChangedFiles", err)
	}
	return nil
}

// convertIssues maps a page of gongfeng issues onto the provider shape.
func convertIssues(issues []*gongfeng.Issue) []*model.Issue {
	result := make([]*model.Issue, 0, len(issues))
	for _, i := range issues {
		result = append(result, convertIssue(i))
	}
	return result
}

// convertMilestoneIssue maps the milestone-issues wire type (a slimmer
// Issue: single assignee, no milestone back-reference) onto the provider
// shape. The ID field is the platform-global issue ID — the same identifier
// issue writes take — so it carries in model.Issue.ID with the IID in
// Number, matching convertIssue.
func convertMilestoneIssue(mi *gongfeng.MilestoneIssue) *model.Issue {
	if mi == nil {
		return nil
	}
	issue := &model.Issue{
		ID:        int64(mi.ID),
		Number:    strconv.Itoa(mi.IID),
		Title:     mi.Title,
		Body:      mi.Description,
		Labels:    append([]string(nil), mi.Labels...),
		Author:    convertUser(mi.Author),
		WebURL:    "",
		CreatedAt: mi.CreatedAt.Time,
		UpdatedAt: mi.UpdatedAt.Time,
	}
	switch mi.State {
	case "opened", "reopened":
		issue.State = model.IssueStateOpen
	case "closed":
		issue.State = model.IssueStateClosed
	}
	if mi.Assignee != nil {
		issue.Assignees = []string{mi.Assignee.Username}
	}
	return issue
}
