package gitcode

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
	gitcode "github.com/yi-nology/go-gitcode"
)

// ListIssues implements provider.IssueManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListIssues(ctx context.Context, opts provider.ListIssuesOptions) ([]*provider.Issue, int, error) {
	buildOpts := func(page, perPage int) gitcode.ListIssuesOptions {
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
		return listOpts
	}
	var issues []*gitcode.Issue
	issues, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitcode.Issue, error) {
			return p.client.ListIssues(ctx, opts.Owner, opts.Repo, buildOpts(page, perPage))
		})
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
func (p *Provider) ListIssueComments(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "ListIssueComments", number)
	if err != nil {
		return nil, err
	}
	// The fork's ListIssueComment surface takes no pagination parameters
	// (a bare GET returns the server-default first page only), so the list
	// is driven through the raw transport client with explicit page/
	// per_page query parameters until an empty page (registered detour).
	comments, err := backendutil.AllPages(func(page int) ([]*gitcode.IssueComment, error) {
		var batch []*gitcode.IssueComment
		_, err := p.rawClient.DoJSON(ctx, &transport.Request{
			Method: "GET",
			Path:   fmt.Sprintf("/repos/%s/%s/issues/%d/comments", esc(owner), esc(repo), n),
			Query:  url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(backendutil.IssueCommentPageSize)}},
			Result: &batch,
		})
		return batch, err
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

// ListIssueLabels implements provider.IssueManager. The fork's
// ListIssueLabels surface takes no pagination parameters (a bare GET
// returns the server-default first page only), so the list is driven
// through the raw transport client with explicit page/per_page query
// parameters until an empty page (registered detour).
func (p *Provider) ListIssueLabels(ctx context.Context, owner, repo string) ([]*provider.IssueLabel, error) {
	var labels []gitcode.Label
	labels, err := backendutil.AllPages(func(page int) ([]gitcode.Label, error) {
		var batch []gitcode.Label
		_, err := p.rawClient.DoJSON(ctx, &transport.Request{
			Method: "GET",
			Path:   fmt.Sprintf("/repos/%s/%s/labels", url.PathEscape(owner), url.PathEscape(repo)),
			Query:  url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(backendutil.LabelPageSize)}},
			Result: &batch,
		})
		return batch, err
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
