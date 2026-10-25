package github

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/go-github/v92/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListIssues implements provider.IssueManager.
func (p *Provider) ListIssues(ctx context.Context, opts provider.ListIssuesOptions) ([]*provider.Issue, int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &github.IssueListByRepoOptions{ListOptions: github.ListOptions{Page: page, PerPage: perPage}}
	if opts.State != "" {
		listOpts.State = string(opts.State)
	}
	if opts.Assignee != "" {
		listOpts.Assignee = opts.Assignee
	}
	// 排序(增量备份要按 updated 排,否则"被改过的老 issue"捞不到)。
	if opts.Sort != "" {
		listOpts.Sort = opts.Sort
	}
	if opts.Direction != "" {
		listOpts.Direction = opts.Direction
	}
	if opts.Labels != "" {
		listOpts.Labels = []string{opts.Labels}
	}
	issues, _, err := p.client.Issues.ListByRepo(ctx, opts.Owner, opts.Repo, listOpts)
	if err != nil {
		return nil, 0, provider.Wrap(provider.PlatformGitHub, "ListIssues", err)
	}
	result := make([]*provider.Issue, 0, len(issues))
	for _, i := range issues {
		result = append(result, convertIssue(i))
	}
	return result, len(result), nil
}

// GetIssue implements provider.IssueManager.
func (p *Provider) GetIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "GetIssue", number)
	if err != nil {
		return nil, err
	}
	issue, _, err := p.client.Issues.Get(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetIssue", err)
	}
	return convertIssue(issue), nil
}

// CreateIssue implements provider.IssueManager.
func (p *Provider) CreateIssue(ctx context.Context, opts provider.CreateIssueOptions) (*provider.Issue, error) {
	req := &github.CreateIssueRequest{Title: opts.Title}
	if opts.Body != "" {
		req.Body = new(opts.Body)
	}
	if len(opts.Assignees) > 0 {
		req.Assignees = opts.Assignees
	}
	if len(opts.Labels) > 0 {
		req.Labels = opts.Labels
	}
	if opts.Milestone != "" {
		m64, err := backendutil.ParseMilestoneNumber(provider.PlatformGitHub, "CreateIssue", opts.Milestone)
		if err != nil {
			return nil, err
		}
		req.Milestone = new(int(m64))
	}
	issue, _, err := p.client.Issues.Create(ctx, opts.Owner, opts.Repo, *req)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateIssue", err)
	}
	return convertIssue(issue), nil
}

// UpdateIssue implements provider.IssueManager.
func (p *Provider) UpdateIssue(ctx context.Context, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "UpdateIssue", number)
	if err != nil {
		return nil, err
	}
	req := &github.UpdateIssueRequest{}
	if opts.Title != "" {
		req.Title = new(opts.Title)
	}
	if opts.Body != "" {
		req.Body = new(opts.Body)
	}
	if len(opts.Assignees) > 0 {
		req.Assignees = opts.Assignees
	}
	if len(opts.Labels) > 0 {
		req.Labels = opts.Labels
	}
	if opts.Milestone != "" {
		m64, err := backendutil.ParseMilestoneNumber(provider.PlatformGitHub, "UpdateIssue", opts.Milestone)
		if err != nil {
			return nil, err
		}
		req.Milestone = new(int(m64))
	}
	issue, _, err := p.client.Issues.Update(ctx, owner, repo, n, *req)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateIssue", err)
	}
	return convertIssue(issue), nil
}

// CloseIssue implements provider.IssueManager.
func (p *Provider) CloseIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "CloseIssue", number)
	if err != nil {
		return nil, err
	}
	issue, _, err := p.client.Issues.Update(ctx, owner, repo, n, github.UpdateIssueRequest{State: new("closed")})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CloseIssue", err)
	}
	return convertIssue(issue), nil
}

// ReopenIssue implements provider.IssueManager.
func (p *Provider) ReopenIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "ReopenIssue", number)
	if err != nil {
		return nil, err
	}
	issue, _, err := p.client.Issues.Update(ctx, owner, repo, n, github.UpdateIssueRequest{State: new("open")})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ReopenIssue", err)
	}
	return convertIssue(issue), nil
}

// ListIssueComments implements provider.IssueManager.
func (p *Provider) ListIssueComments(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "ListIssueComments", number)
	if err != nil {
		return nil, err
	}
	comments, err := backendutil.AllPages(func(page int) ([]*github.IssueComment, error) {
		batch, _, err := p.client.Issues.ListComments(ctx, owner, repo, n, &github.IssueListCommentsOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: backendutil.IssueCommentPageSize},
		})
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListIssueComments", err)
	}
	result := make([]*provider.IssueComment, 0, len(comments))
	for _, c := range comments {
		result = append(result, convertIssueComment(c))
	}
	return result, nil
}

// CreateIssueComment implements provider.IssueManager.
func (p *Provider) CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "CreateIssueComment", number)
	if err != nil {
		return nil, err
	}
	comment, _, err := p.client.Issues.CreateComment(ctx, owner, repo, n, github.IssueCommentRequest{Body: body})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateIssueComment", err)
	}
	return convertIssueComment(comment), nil
}

// UpdateIssueComment implements provider.IssueManager. The edit endpoint
// addresses the comment directly, so number is unused. The platform only
// lets the comment's author perform the edit.
func (p *Provider) UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*provider.IssueComment, error) {
	comment, _, err := p.client.Issues.UpdateComment(ctx, owner, repo, commentID, github.IssueCommentRequest{Body: body})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateIssueComment", err)
	}
	return convertIssueComment(comment), nil
}

// ListIssueLabels implements provider.IssueManager: repository-level labels,
// exhausting pagination (the loop advances until an empty page).
func (p *Provider) ListIssueLabels(ctx context.Context, owner, repo string) ([]*provider.IssueLabel, error) {
	labels, err := backendutil.AllPages(func(page int) ([]*github.Label, error) {
		batch, _, err := p.client.Issues.ListLabels(ctx, owner, repo, &github.ListOptions{Page: page, PerPage: backendutil.LabelPageSize})
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListIssueLabels", err)
	}
	result := make([]*provider.IssueLabel, 0, len(labels))
	for _, l := range labels {
		result = append(result, &provider.IssueLabel{ID: l.GetID(), Name: l.GetName(), Color: trimHash(l.GetColor())})
	}
	return result, nil
}

// AddIssueLabels implements provider.IssueManager.
func (p *Provider) AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "AddIssueLabels", number)
	if err != nil {
		return err
	}
	if _, _, err := p.client.Issues.AddLabelsToIssue(ctx, owner, repo, n, labels); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "AddIssueLabels", err)
	}
	return nil
}

// RemoveIssueLabel implements provider.IssueManager.
func (p *Provider) RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "RemoveIssueLabel", number)
	if err != nil {
		return err
	}
	if _, err := p.client.Issues.RemoveLabelForIssue(ctx, owner, repo, n, name); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "RemoveIssueLabel", err)
	}
	return nil
}

// convertIssue maps a github.Issue to a provider.Issue.
func convertIssue(i *github.Issue) *provider.Issue {
	if i == nil {
		return nil
	}
	var milestone *provider.MilestoneRef
	if i.Milestone != nil {
		milestone = &provider.MilestoneRef{Number: strconv.Itoa(i.Milestone.GetNumber()), Title: i.Milestone.GetTitle()}
	}
	issue := &provider.Issue{
		ID:        i.GetID(),
		Number:    strconv.Itoa(i.GetNumber()),
		Title:     i.GetTitle(),
		Body:      i.GetBody(),
		Author:    convertUser(i.GetUser()),
		Milestone: milestone,
		WebURL:    i.GetHTMLURL(),
		// GitHub 的 issue 列表端点**把 PR 一起返回**;
		// 载荷里的 pull_request 字段是"这条其实是 PR"的唯一标记。
		// 不标记的话消费方分不开 issue 与 PR。
		IsPullRequest: i.PullRequestLinks != nil,
	}
	if s := i.GetState(); s != "" {
		issue.State = provider.IssueState(s)
	}
	for _, l := range i.Labels {
		issue.Labels = append(issue.Labels, l.GetName())
	}
	for _, a := range i.Assignees {
		issue.Assignees = append(issue.Assignees, a.GetLogin())
	}
	if t := i.GetCreatedAt(); !t.IsZero() {
		issue.CreatedAt = t.Time
	}
	if t := i.GetUpdatedAt(); !t.IsZero() {
		issue.UpdatedAt = t.Time
	}
	// GetClosedAt returns the Timestamp by value; a zero timestamp means the
	// issue was never closed.
	if t := i.GetClosedAt(); !t.IsZero() {
		issue.ClosedAt = &t.Time
	}
	return issue
}

// convertIssueComment maps a github.IssueComment to a provider.IssueComment.
func convertIssueComment(c *github.IssueComment) *provider.IssueComment {
	if c == nil {
		return nil
	}
	comment := &provider.IssueComment{
		ID:     c.GetID(),
		Body:   c.GetBody(),
		Author: convertUser(c.GetUser()),
	}
	if t := c.GetCreatedAt(); !t.IsZero() {
		comment.CreatedAt = t.Time
	}
	if t := c.GetUpdatedAt(); !t.IsZero() {
		comment.UpdatedAt = t.Time
	}
	return comment
}

// trimHash strips a leading '#' from a color so the SDK's canonical form is
// '#' free, mirroring convertLabel in labels.go.
func trimHash(s string) string { return strings.TrimPrefix(s, "#") }

var _ provider.IssueManager = (*Provider)(nil)
