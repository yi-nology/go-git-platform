package tencentcode

import (
	"context"
	"io"
	"strconv"

	gongfeng "github.com/yi-nology/go-gongfeng"

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
		return false, provider.Wrap(provider.PlatformTencentCode, "GetIssueSubscription", err)
	}
	return subscribed, nil
}

// SubscribeIssue implements TencentCodeExtras.
func (p *Provider) SubscribeIssue(ctx context.Context, owner, repo string, issueID int) error {
	if _, err := p.client.Issues.SubscribeIssue(ctx, pid(owner, repo), issueID); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "SubscribeIssue", err)
	}
	return nil
}

// UnsubscribeIssue implements TencentCodeExtras.
func (p *Provider) UnsubscribeIssue(ctx context.Context, owner, repo string, issueID int) error {
	if _, err := p.client.Issues.UnsubscribeIssue(ctx, pid(owner, repo), issueID); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "UnsubscribeIssue", err)
	}
	return nil
}

// GetMRSubscription implements TencentCodeExtras.
func (p *Provider) GetMRSubscription(ctx context.Context, owner, repo string, mrNumber int) (bool, error) {
	return p.mrSubscription(ctx, "GetMRSubscription", owner, repo, mrNumber)
}

// SubscribeMR implements TencentCodeExtras.
func (p *Provider) SubscribeMR(ctx context.Context, owner, repo string, mrNumber int) (bool, error) {
	sub, _, err := p.client.MergeRequests.SubscribeMR(ctx, pid(owner, repo), mrNumber)
	if err != nil {
		return false, provider.Wrap(provider.PlatformTencentCode, "SubscribeMR", err)
	}
	if sub == nil {
		return false, nil
	}
	return sub.Subscribed, nil
}

// UnsubscribeMR implements TencentCodeExtras.
func (p *Provider) UnsubscribeMR(ctx context.Context, owner, repo string, mrNumber int) (bool, error) {
	sub, _, err := p.client.MergeRequests.UnsubscribeMR(ctx, pid(owner, repo), mrNumber)
	if err != nil {
		return false, provider.Wrap(provider.PlatformTencentCode, "UnsubscribeMR", err)
	}
	if sub == nil {
		return false, nil
	}
	return sub.Subscribed, nil
}

// mrSubscription is the shared read path of the MR subscription family
// (the subscribe/unsubscribe endpoints share the wire shape but decode the
// echo themselves).
func (p *Provider) mrSubscription(ctx context.Context, op, owner, repo string, mrNumber int) (bool, error) {
	sub, _, err := p.client.MergeRequests.GetMRSubscription(ctx, pid(owner, repo), mrNumber)
	if err != nil {
		return false, provider.Wrap(provider.PlatformTencentCode, op, err)
	}
	if sub == nil {
		return false, nil
	}
	return sub.Subscribed, nil
}

// ListUserIssues implements TencentCodeExtras: the global /issues listing
// across projects, one caller-driven page per call (Page 0 = first page).
func (p *Provider) ListUserIssues(ctx context.Context, opts ListUserIssuesOptions) ([]*provider.Issue, error) {
	listOpts := &gongfeng.ListIssuesOptions{
		ListOptions: gongfeng.ListOptions{Page: opts.Page, PerPage: opts.PerPage},
	}
	if opts.State != "" {
		s := opts.State
		if s == string(provider.IssueStateOpen) {
			s = "opened" // 工蜂's issues API vocabulary; inbound convertIssue maps back
		}
		listOpts.State = gongfeng.Ptr(s)
	}
	if opts.Labels != "" {
		listOpts.Labels = gongfeng.Ptr(opts.Labels)
	}
	issues, _, err := p.client.Issues.ListUserIssues(ctx, listOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListUserIssues", err)
	}
	return convertIssues(issues), nil
}

// ListMilestoneIssues implements TencentCodeExtras: one caller-driven page
// of a milestone's issues (page 0 = first page).
func (p *Provider) ListMilestoneIssues(ctx context.Context, owner, repo string, milestoneID, page, perPage int) ([]*provider.Issue, error) {
	issues, _, err := p.client.Milestones.ListMilestoneIssues(ctx, pid(owner, repo), milestoneID,
		&gongfeng.ListMilestoneIssuesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage}})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListMilestoneIssues", err)
	}
	result := make([]*provider.Issue, 0, len(issues))
	for _, mi := range issues {
		result = append(result, convertMilestoneIssue(mi))
	}
	return result, nil
}

// DownloadMergeRequestChangedFiles implements TencentCodeExtras.
func (p *Provider) DownloadMergeRequestChangedFiles(ctx context.Context, owner, repo string, mrNumber int, w io.Writer) error {
	if _, err := p.client.MergeRequests.DownloadMergeRequestChangedFiles(ctx, pid(owner, repo), mrNumber, w); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DownloadMergeRequestChangedFiles", err)
	}
	return nil
}

// DownloadCommitReviewChangedFiles implements TencentCodeExtras.
func (p *Provider) DownloadCommitReviewChangedFiles(ctx context.Context, owner, repo string, reviewID int, w io.Writer) error {
	if _, err := p.client.Reviews.DownloadCommitReviewChangedFiles(ctx, pid(owner, repo), reviewID, w); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DownloadCommitReviewChangedFiles", err)
	}
	return nil
}

// convertIssues maps a page of gongfeng issues onto the provider shape.
func convertIssues(issues []*gongfeng.Issue) []*provider.Issue {
	result := make([]*provider.Issue, 0, len(issues))
	for _, i := range issues {
		result = append(result, convertIssue(i))
	}
	return result
}

// convertMilestoneIssue maps the milestone-issues wire type (a slimmer
// Issue: single assignee, no milestone back-reference) onto the provider
// shape. The ID field is the platform-global issue ID — the same identifier
// issue writes take — so it carries in provider.Issue.ID with the IID in
// Number, matching convertIssue.
func convertMilestoneIssue(mi *gongfeng.MilestoneIssue) *provider.Issue {
	if mi == nil {
		return nil
	}
	issue := &provider.Issue{
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
		issue.State = provider.IssueStateOpen
	case "closed":
		issue.State = provider.IssueStateClosed
	}
	if mi.Assignee != nil {
		issue.Assignees = []string{mi.Assignee.Username}
	}
	return issue
}
