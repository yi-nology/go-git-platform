package gitee

import (
	"context"
	"math"
	"strconv"
	"strings"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListIssues implements provider.IssueManager via the SDK.
func (p *Provider) ListIssues(ctx context.Context, opts provider.ListIssuesOptions) ([]*provider.Issue, int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &gitee.RepoIssueListOptions{
		Page:    gitee.Int(page),
		PerPage: gitee.Int(perPage),
	}
	if opts.State != "" {
		listOpts.State = gitee.String(string(opts.State))
	}
	if opts.Assignee != "" {
		listOpts.Assignee = gitee.String(opts.Assignee)
	}
	if opts.Labels != "" {
		listOpts.Labels = gitee.String(opts.Labels)
	}
	issues, _, err := p.client.Issues.ListByRepo(ctx, backendutil.Esc(opts.Owner), backendutil.Esc(opts.Repo), listOpts)
	if err != nil {
		return nil, 0, provider.Wrap(provider.PlatformGitee, "ListIssues", err)
	}
	result := make([]*provider.Issue, 0, len(issues))
	for _, i := range issues {
		result = append(result, convertIssue(i))
	}
	return result, len(result), nil
}

// GetIssue implements provider.IssueManager via the SDK.
func (p *Provider) GetIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	issue, _, err := p.client.Issues.Get(ctx, backendutil.Esc(owner), backendutil.Esc(repo), number)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetIssue", err)
	}
	return convertIssue(issue), nil
}

// CreateIssue implements provider.IssueManager.
func (p *Provider) CreateIssue(ctx context.Context, opts provider.CreateIssueOptions) (*provider.Issue, error) {
	createOpts := &gitee.CreateIssueOptions{
		Repo:  gitee.String(opts.Repo),
		Title: gitee.String(opts.Title),
	}
	if opts.Body != "" {
		createOpts.Body = gitee.String(opts.Body)
	}
	if len(opts.Labels) > 0 {
		createOpts.Labels = gitee.String(strings.Join(opts.Labels, ","))
	}
	if len(opts.Assignees) > 0 {
		createOpts.Assignee = gitee.String(strings.Join(opts.Assignees, ","))
	}
	if opts.Milestone != "" {
		m, err := strconv.Atoi(opts.Milestone)
		if err != nil {
			return nil, provider.Wrapf(provider.PlatformGitee, "CreateIssue", "invalid milestone number %q", opts.Milestone)
		}
		createOpts.Milestone = gitee.Int(m)
	}
	issue, _, err := p.client.Issues.Create(ctx, backendutil.Esc(opts.Owner), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateIssue", err)
	}
	return convertIssue(issue), nil
}

// UpdateIssue implements provider.IssueManager via the SDK.
func (p *Provider) UpdateIssue(ctx context.Context, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	updateOpts := &gitee.UpdateIssueOptions{
		Repo: gitee.String(repo),
	}
	if opts.Title != "" {
		updateOpts.Title = gitee.String(opts.Title)
	}
	if opts.Body != "" {
		updateOpts.Body = gitee.String(opts.Body)
	}
	if opts.State != "" {
		updateOpts.State = gitee.String(string(opts.State))
	}
	if len(opts.Assignees) > 0 {
		updateOpts.Assignee = gitee.String(strings.Join(opts.Assignees, ","))
	}
	if len(opts.Labels) > 0 {
		updateOpts.Labels = gitee.String(strings.Join(opts.Labels, ","))
	}
	if opts.Milestone != "" {
		m, err := strconv.Atoi(opts.Milestone)
		if err != nil {
			return nil, provider.Wrapf(provider.PlatformGitee, "UpdateIssue", "invalid milestone number %q", opts.Milestone)
		}
		updateOpts.Milestone = gitee.Int(m)
	}
	issue, _, err := p.client.Issues.Edit(ctx, backendutil.Esc(owner), number, updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "UpdateIssue", err)
	}
	return convertIssue(issue), nil
}

// CloseIssue implements provider.IssueManager.
func (p *Provider) CloseIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	return p.patchIssueState(ctx, owner, repo, number, "closed", "CloseIssue")
}

// ReopenIssue implements provider.IssueManager.
func (p *Provider) ReopenIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	return p.patchIssueState(ctx, owner, repo, number, "open", "ReopenIssue")
}

// patchIssueState flips an issue's state via the SDK's owner-scoped update.
func (p *Provider) patchIssueState(ctx context.Context, owner, repo, number, state, op string) (*provider.Issue, error) {
	issue, _, err := p.client.Issues.Edit(ctx, backendutil.Esc(owner), number, &gitee.UpdateIssueOptions{
		Repo:  gitee.String(repo),
		State: gitee.String(state),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, op, err)
	}
	return convertIssue(issue), nil
}

// ListIssueComments implements provider.IssueManager via the SDK.
func (p *Provider) ListIssueComments(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	notes, err := backendutil.AllPages(func(page int) ([]*gitee.Note, error) {
		opts := &gitee.IssueCommentListByIssueOptions{
			Page:    gitee.Int(page),
			PerPage: gitee.Int(backendutil.IssueCommentPageSize),
		}
		batch, _, err := p.client.Issues.ListIssueComments(ctx, backendutil.Esc(owner), backendutil.Esc(repo), number, opts)
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListIssueComments", err)
	}
	result := make([]*provider.IssueComment, 0, len(notes))
	for _, n := range notes {
		result = append(result, convertIssueComment(n))
	}
	return result, nil
}

// CreateIssueComment implements provider.IssueManager via the SDK.
func (p *Provider) CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*provider.IssueComment, error) {
	opts := &gitee.CreateIssueCommentOptions{
		Body: gitee.String(body),
	}
	note, _, err := p.client.Issues.CreateIssueComment(ctx, backendutil.Esc(owner), backendutil.Esc(repo), number, opts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateIssueComment", err)
	}
	return convertIssueComment(note), nil
}

// UpdateIssueComment implements provider.IssueManager.
func (p *Provider) UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*provider.IssueComment, error) {
	if commentID < 0 || commentID > math.MaxInt32 {
		return nil, provider.Wrapf(provider.PlatformGitee, "UpdateIssueComment", "comment id %d out of gitee's int32 range", commentID)
	}
	opts := &gitee.UpdateIssueCommentOptions{
		Body: gitee.String(body),
	}
	note, _, err := p.client.Issues.EditComment(ctx, backendutil.Esc(owner), backendutil.Esc(repo), commentID, opts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "UpdateIssueComment", err)
	}
	return convertIssueComment(note), nil
}

// DeleteIssueComment implements provider.IssueManager. The delete endpoint
// addresses the comment directly, so number is unused; gitee's comment IDs
// are int32 on the wire, so the same range guard as the edit path applies.
// Deletion is permanent; the platform only lets the comment's author
// perform it.
func (p *Provider) DeleteIssueComment(ctx context.Context, owner, repo, number string, commentID int64) error {
	if commentID < 0 || commentID > math.MaxInt32 {
		return provider.Wrapf(provider.PlatformGitee, "DeleteIssueComment", "comment id %d out of gitee's int32 range", commentID)
	}
	if _, err := p.client.Issues.DeleteComment(ctx, backendutil.Esc(owner), backendutil.Esc(repo), commentID); err != nil {
		return provider.Wrap(provider.PlatformGitee, "DeleteIssueComment", err)
	}
	return nil
}

// AddIssueLabels implements provider.IssueManager via the SDK.
func (p *Provider) AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	_, _, err := p.client.Issues.AddLabels(ctx, backendutil.Esc(owner), backendutil.Esc(repo), number, labels)
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "AddIssueLabels", err)
	}
	return nil
}

// RemoveIssueLabel implements provider.IssueManager via the SDK. The label
// name is a path segment on the delete endpoint (issues #84: a name like
// "C#" truncated the URL at the fragment and deleted the wrong label).
func (p *Provider) RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error {
	_, err := p.client.Issues.RemoveLabel(ctx, backendutil.Esc(owner), backendutil.Esc(repo), number, backendutil.Esc(name))
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "RemoveIssueLabel", err)
	}
	return nil
}

// ListIssueLabels implements provider.IssueManager: repository-level labels.
// The SDK's LabelsService.List takes ListOptions with Page/PerPage, so the
// list is paginated via backendutil.AllPages until an empty page.
func (p *Provider) ListIssueLabels(ctx context.Context, owner, repo string) ([]*provider.IssueLabel, error) {
	labels, err := backendutil.AllPages(func(page int) ([]*gitee.Label, error) {
		batch, _, err := p.client.Labels.List(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &gitee.ListOptions{
			Page:    page,
			PerPage: backendutil.LabelPageSize,
		})
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListIssueLabels", err)
	}
	result := make([]*provider.IssueLabel, 0, len(labels))
	for _, l := range labels {
		result = append(result, &provider.IssueLabel{
			ID:    int64(deref(l.ID)),
			Name:  deref(l.Name),
			Color: strings.TrimPrefix(deref(l.Color), "#"),
		})
	}
	return result, nil
}

var _ provider.IssueManager = (*Provider)(nil)
