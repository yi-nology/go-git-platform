package gitcode

import (
	"context"
	"strings"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	gitcode "github.com/yi-nology/go-gitcode"
)

// ListIssues implements provider.IssueManager.
//
// Caller-paged family (issue #51 — see provider.NormalizePageOpts):
// opts.Page == 0 serves the platform's first page in exactly one request;
// opts.Page > 0 returns exactly that caller-driven page. The int return is
// len(result), the served rows' count.
func (p *Provider) ListIssues(ctx context.Context, opts provider.ListIssuesOptions) ([]*provider.Issue, int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := gitcode.ListIssuesOptions{
		ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
	}
	if opts.State != "" {
		listOpts.State = gitcode.IssueState(opts.State)
	}
	if opts.Assignee != "" {
		listOpts.Assignee = opts.Assignee
	}
	if opts.Labels != "" {
		listOpts.Labels = opts.Labels
	}
	issues, err := p.client.ListIssues(ctx, opts.Owner, opts.Repo, listOpts)
	if err != nil {
		return nil, 0, provider.Wrap(provider.PlatformGitCode, "ListIssues", err)
	}
	result := make([]*provider.Issue, 0, len(issues))
	for _, i := range issues {
		result = append(result, convertIssue(i))
	}
	return result, len(result), nil
}

// GetIssue implements provider.IssueManager.
func (p *Provider) GetIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "GetIssue", number)
	if err != nil {
		return nil, err
	}
	issue, err := p.client.GetIssue(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetIssue", err)
	}
	return convertIssue(issue), nil
}

// CreateIssue implements provider.IssueManager.
func (p *Provider) CreateIssue(ctx context.Context, opts provider.CreateIssueOptions) (*provider.Issue, error) {
	createOpts := gitcode.CreateIssueOptions{
		Title:     opts.Title,
		Body:      opts.Body,
		Assignees: opts.Assignees,
		Labels:    opts.Labels,
	}
	if opts.Milestone != "" {
		m, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "CreateIssue", opts.Milestone)
		if err != nil {
			return nil, err
		}
		createOpts.Milestone = m
	}
	issue, err := p.client.CreateIssue(ctx, opts.Owner, opts.Repo, createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateIssue", err)
	}
	return convertIssue(issue), nil
}

// UpdateIssue implements provider.IssueManager.
func (p *Provider) UpdateIssue(ctx context.Context, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "UpdateIssue", number)
	if err != nil {
		return nil, err
	}
	updateOpts := gitcode.UpdateIssueOptions{
		Title:     opts.Title,
		Body:      opts.Body,
		Assignees: opts.Assignees,
		Labels:    opts.Labels,
	}
	if opts.Milestone != "" {
		m, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "UpdateIssue", opts.Milestone)
		if err != nil {
			return nil, err
		}
		updateOpts.Milestone = m
	}
	if opts.State != "" {
		updateOpts.State = gitcode.IssueState(opts.State)
	}
	issue, err := p.client.UpdateIssue(ctx, owner, repo, n, updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateIssue", err)
	}
	return convertIssue(issue), nil
}

// CloseIssue implements provider.IssueManager.
func (p *Provider) CloseIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "CloseIssue", number)
	if err != nil {
		return nil, err
	}
	issue, err := p.client.CloseIssue(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CloseIssue", err)
	}
	return convertIssue(issue), nil
}

// ReopenIssue implements provider.IssueManager.
func (p *Provider) ReopenIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "ReopenIssue", number)
	if err != nil {
		return nil, err
	}
	issue, err := p.client.ReopenIssue(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ReopenIssue", err)
	}
	return convertIssue(issue), nil
}

// ListIssueComments implements provider.IssueManager.
//
// go-gitcode v0.8.0 exposes the documented page/per_page/order/since query
// on the issue-comment list endpoint, so the walk runs through the SDK.
func (p *Provider) ListIssueComments(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "ListIssueComments", number)
	if err != nil {
		return nil, err
	}
	comments, err := backendutil.AllPages(func(page int) ([]*gitcode.IssueComment, error) {
		return p.client.ListIssueCommentsWithOptions(ctx, owner, repo, n, gitcode.ListIssueCommentsOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: backendutil.IssueCommentPageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListIssueComments", err)
	}
	result := make([]*provider.IssueComment, 0, len(comments))
	for _, c := range comments {
		result = append(result, convertIssueComment(c))
	}
	return result, nil
}

// CreateIssueComment implements provider.IssueManager.
func (p *Provider) CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "CreateIssueComment", number)
	if err != nil {
		return nil, err
	}
	comment, err := p.client.CreateIssueComment(ctx, owner, repo, n, body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateIssueComment", err)
	}
	return convertIssueComment(comment), nil
}

// UpdateIssueComment implements provider.IssueManager. The edit endpoint
// addresses the comment directly, so number is unused. The platform only
// lets the comment's author perform the edit.
func (p *Provider) UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*provider.IssueComment, error) {
	comment, err := p.client.UpdateIssueComment(ctx, owner, repo, commentID, body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateIssueComment", err)
	}
	return convertIssueComment(comment), nil
}

// DeleteIssueComment implements provider.IssueManager. The delete endpoint
// addresses the comment directly, so number is unused. Deletion is
// permanent; the platform only lets the comment's author perform it.
func (p *Provider) DeleteIssueComment(ctx context.Context, owner, repo, number string, commentID int64) error {
	if err := p.client.DeleteIssueComment(ctx, owner, repo, commentID); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteIssueComment", err)
	}
	return nil
}

// ListIssueLabels implements provider.IssueManager. go-gitcode v0.8.0
// exposes the documented page/per_page query on the label-list endpoint,
// so the walk runs through the SDK (server default is 100 per page).
func (p *Provider) ListIssueLabels(ctx context.Context, owner, repo string) ([]*provider.IssueLabel, error) {
	var labels []*gitcode.Label
	labels, err := backendutil.AllPages(func(page int) ([]*gitcode.Label, error) {
		return p.client.ListIssueLabelsWithOptions(ctx, owner, repo, gitcode.ListOptions{
			Page: page, PerPage: backendutil.LabelPageSize,
		})
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListIssueLabels", err)
	}
	result := make([]*provider.IssueLabel, 0, len(labels))
	for _, l := range labels {
		result = append(result, &provider.IssueLabel{
			ID:   l.ID,
			Name: l.Name,
			// Same canonicalization as convertLabel in labels.go: GitCode
			// labels carry '#'-prefixed colors on the wire; the SDK type is
			// '#' free.
			Color: strings.TrimPrefix(l.Color, "#"),
		})
	}
	return result, nil
}

// AddIssueLabels implements provider.IssueManager.
func (p *Provider) AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "AddIssueLabels", number)
	if err != nil {
		return err
	}
	if err := p.client.AddIssueLabels(ctx, owner, repo, n, labels); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "AddIssueLabels", err)
	}
	return nil
}

// RemoveIssueLabel implements provider.IssueManager.
func (p *Provider) RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "RemoveIssueLabel", number)
	if err != nil {
		return err
	}
	if err := p.client.RemoveIssueLabel(ctx, owner, repo, n, name); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "RemoveIssueLabel", err)
	}
	return nil
}

var _ provider.IssueManager = (*Provider)(nil)
