package forgejo

import (
	"context"
	"strconv"
	"strings"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListIssues implements provider.IssueManager. The forgejo SDK accepts no
// context (same as its other services).
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListIssues(ctx context.Context, opts provider.ListIssuesOptions) ([]*provider.Issue, int, error) {
	listOpts := forgejo.ListIssueOption{}
	// Unset, the endpoint returns PRs mixed in with the issues.
	listOpts.Type = forgejo.IssueTypeIssue
	if opts.State != "" {
		listOpts.State = forgejo.StateType(opts.State)
	}
	if opts.Labels != "" {
		listOpts.Labels = strings.Split(opts.Labels, ",")
	}
	if opts.Assignee != "" {
		listOpts.AssignedBy = opts.Assignee
	}
	var issues []*forgejo.Issue
	issues, err := backendutil.PageList(opts.Page, opts.PerPage, listPageSize,
		func(page, perPage int) ([]*forgejo.Issue, error) {
			listOpts.ListOptions = forgejo.ListOptions{Page: page, PageSize: perPage}
			list, _, err := p.client.ListRepoIssues(opts.Owner, opts.Repo, listOpts)
			return list, err
		})
	if err != nil {
		return nil, 0, provider.Wrap(provider.PlatformForgejo, "ListIssues", err)
	}
	result := make([]*provider.Issue, 0, len(issues))
	for _, i := range issues {
		result = append(result, convertIssue(i))
	}
	return result, len(result), nil
}

// GetIssue implements provider.IssueManager.
func (p *Provider) GetIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformForgejo, "GetIssue", number)
	if err != nil {
		return nil, err
	}
	issue, _, err := p.client.GetIssue(owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "GetIssue", err)
	}
	return convertIssue(issue), nil
}

// CreateIssue implements provider.IssueManager. Forgejo's create endpoint
// takes label IDs, so names are resolved first (one list call per name;
// label counts are small).
func (p *Provider) CreateIssue(ctx context.Context, opts provider.CreateIssueOptions) (*provider.Issue, error) {
	createOpts := forgejo.CreateIssueOption{
		Title:     opts.Title,
		Body:      opts.Body,
		Assignees: opts.Assignees,
	}
	if opts.Milestone != "" {
		m, err := backendutil.ParseMilestoneNumber(provider.PlatformForgejo, "CreateIssue", opts.Milestone)
		if err != nil {
			return nil, err
		}
		createOpts.Milestone = m
	}
	if len(opts.Labels) > 0 {
		ids, err := p.resolveLabelIDs("CreateIssue", opts.Owner, opts.Repo, opts.Labels)
		if err != nil {
			return nil, err
		}
		createOpts.Labels = ids
	}
	issue, _, err := p.client.CreateIssue(opts.Owner, opts.Repo, createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "CreateIssue", err)
	}
	return convertIssue(issue), nil
}

// UpdateIssue implements provider.IssueManager. Forgejo's EditIssueOption
// always serializes Title (no omitempty), so a caller leaving fields empty
// would clear them; the current title is backfilled via one GET, and an
// all-empty update short-circuits to a plain GetIssue. opts.Labels replaces
// the issue's labels via the dedicated endpoint.
func (p *Provider) UpdateIssue(ctx context.Context, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformForgejo, "UpdateIssue", number)
	if err != nil {
		return nil, err
	}
	if opts.Title == "" && opts.Body == "" && opts.State == "" && len(opts.Assignees) == 0 && len(opts.Labels) == 0 && opts.Milestone == "" {
		return p.GetIssue(ctx, owner, repo, number)
	}
	edit, err := p.buildEditIssueOption("UpdateIssue", owner, repo, n, opts)
	if err != nil {
		return nil, err
	}
	if len(opts.Labels) > 0 {
		ids, err := p.resolveLabelIDs("UpdateIssue", owner, repo, opts.Labels)
		if err != nil {
			return nil, err
		}
		if _, _, err := p.client.ReplaceIssueLabels(owner, repo, n, forgejo.IssueLabelsOption{Labels: ids}); err != nil {
			return nil, provider.Wrap(provider.PlatformForgejo, "UpdateIssue", err)
		}
	}
	issue, _, err := p.client.EditIssue(owner, repo, n, edit)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "UpdateIssue", err)
	}
	return convertIssue(issue), nil
}

// buildEditIssueOption translates provider update options into forgejo's
// EditIssueOption. Forgejo always serializes Title (no omitempty), so when
// the caller leaves the title empty the current one is backfilled via one
// GET to avoid clearing it. op is the public operation this build serves;
// failures surface under that op. n is the parsed issue number.
//
// KNOWN COST: When opts.Title is empty this issues an extra GET request to
// backfill the current title. This is unavoidable because Forgejo's
// EditIssueOption has no omitempty on Title — sending an empty string would
// clear the title. The cost is one additional API round-trip per update that
// does not explicitly set the title.
func (p *Provider) buildEditIssueOption(op, owner, repo string, n int64, opts provider.UpdateIssueOptions) (forgejo.EditIssueOption, error) {
	edit := forgejo.EditIssueOption{}
	if opts.Title != "" {
		edit.Title = opts.Title
	} else {
		// Extra GET: Forgejo requires Title on every edit; backfill from current.
		current, _, err := p.client.GetIssue(owner, repo, n)
		if err != nil {
			return edit, provider.Wrap(provider.PlatformForgejo, op, err)
		}
		edit.Title = current.Title
	}
	if len(opts.Assignees) > 0 {
		edit.Assignees = opts.Assignees
	}
	if opts.Body != "" {
		body := opts.Body
		edit.Body = &body
	}
	if opts.State != "" {
		s := forgejo.StateType(opts.State)
		edit.State = &s
	}
	if opts.Milestone != "" {
		m, err := backendutil.ParseMilestoneNumber(provider.PlatformForgejo, op, opts.Milestone)
		if err != nil {
			return edit, err
		}
		edit.Milestone = &m
	}
	return edit, nil
}

// CloseIssue implements provider.IssueManager.
func (p *Provider) CloseIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformForgejo, "CloseIssue", number)
	if err != nil {
		return nil, err
	}
	return p.setIssueState(owner, repo, n, forgejo.StateClosed, "CloseIssue")
}

// ReopenIssue implements provider.IssueManager.
func (p *Provider) ReopenIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformForgejo, "ReopenIssue", number)
	if err != nil {
		return nil, err
	}
	return p.setIssueState(owner, repo, n, forgejo.StateOpen, "ReopenIssue")
}

// setIssueState closes/reopens an issue. EditIssue always serializes Title,
// so the current title is fetched first to avoid clearing it. n is the
// parsed issue number.
//
// KNOWN COST: One extra GET per close/reopen to backfill the title (same
// constraint as buildEditIssueOption).
func (p *Provider) setIssueState(owner, repo string, n int64, state forgejo.StateType, op string) (*provider.Issue, error) {
	current, _, err := p.client.GetIssue(owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, op, err)
	}
	issue, _, err := p.client.EditIssue(owner, repo, n, forgejo.EditIssueOption{Title: current.Title, State: &state})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, op, err)
	}
	return convertIssue(issue), nil
}

// ListIssueComments implements provider.IssueManager.
func (p *Provider) ListIssueComments(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformForgejo, "ListIssueComments", number)
	if err != nil {
		return nil, err
	}
	comments, err := backendutil.AllPages(func(page int) ([]*forgejo.Comment, error) {
		batch, _, err := p.client.ListIssueComments(owner, repo, n, forgejo.ListIssueCommentOptions{
			ListOptions: forgejo.ListOptions{Page: page, PageSize: backendutil.IssueCommentPageSize},
		})
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "ListIssueComments", err)
	}
	result := make([]*provider.IssueComment, 0, len(comments))
	for _, c := range comments {
		result = append(result, convertIssueComment(c))
	}
	return result, nil
}

// CreateIssueComment implements provider.IssueManager.
func (p *Provider) CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformForgejo, "CreateIssueComment", number)
	if err != nil {
		return nil, err
	}
	comment, _, err := p.client.CreateIssueComment(owner, repo, n, forgejo.CreateIssueCommentOption{Body: body})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "CreateIssueComment", err)
	}
	return convertIssueComment(comment), nil
}

// UpdateIssueComment implements provider.IssueManager. The edit endpoint
// addresses the comment directly, so number is unused; the SDK accepts no
// context (see the forgejo standing limitation). The platform only lets the
// comment's author perform the edit.
func (p *Provider) UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*provider.IssueComment, error) {
	comment, _, err := p.client.EditIssueComment(owner, repo, commentID, forgejo.EditIssueCommentOption{Body: body})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "UpdateIssueComment", err)
	}
	return convertIssueComment(comment), nil
}

// ListIssueLabels implements provider.IssueManager: repository-level labels,
// exhausting pagination (the loop advances until an empty page).
func (p *Provider) ListIssueLabels(ctx context.Context, owner, repo string) ([]*provider.IssueLabel, error) {
	labels, err := backendutil.AllPages(func(page int) ([]*forgejo.Label, error) {
		batch, _, err := p.client.ListRepoLabels(owner, repo, forgejo.ListLabelsOptions{
			ListOptions: forgejo.ListOptions{Page: page, PageSize: backendutil.LabelPageSize},
		})
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "ListIssueLabels", err)
	}
	result := make([]*provider.IssueLabel, 0, len(labels))
	for _, l := range labels {
		result = append(result, &provider.IssueLabel{ID: l.ID, Name: l.Name, Color: strings.TrimPrefix(l.Color, "#")})
	}
	return result, nil
}

// AddIssueLabels implements provider.IssueManager.
func (p *Provider) AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformForgejo, "AddIssueLabels", number)
	if err != nil {
		return err
	}
	ids, err := p.resolveLabelIDs("AddIssueLabels", owner, repo, labels)
	if err != nil {
		return err
	}
	if _, _, err := p.client.AddIssueLabels(owner, repo, n, forgejo.IssueLabelsOption{Labels: ids}); err != nil {
		return provider.Wrap(provider.PlatformForgejo, "AddIssueLabels", err)
	}
	return nil
}

// RemoveIssueLabel implements provider.IssueManager.
func (p *Provider) RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformForgejo, "RemoveIssueLabel", number)
	if err != nil {
		return err
	}
	id, err := p.resolveLabelID("RemoveIssueLabel", owner, repo, name)
	if err != nil {
		return err
	}
	if _, err := p.client.DeleteIssueLabel(owner, repo, n, id); err != nil {
		return provider.Wrap(provider.PlatformForgejo, "RemoveIssueLabel", err)
	}
	return nil
}

// resolveLabelIDs resolves label names to numeric IDs, one list call per
// name (labels.go owns the single-name resolver).
func (p *Provider) resolveLabelIDs(op, owner, repo string, names []string) ([]int64, error) {
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		id, err := p.resolveLabelID(op, owner, repo, name)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// convertIssue maps a forgejo.Issue to a provider.Issue.
func convertIssue(i *forgejo.Issue) *provider.Issue {
	if i == nil {
		return nil
	}
	issue := &provider.Issue{
		ID:     i.ID,
		Number: strconv.FormatInt(i.Index, 10),
		Title:  i.Title,
		Body:   i.Body,
		State:  provider.IssueState(i.State),
		Author: convertUser(i.Poster),
		WebURL: i.HTMLURL,
	}
	for _, l := range i.Labels {
		issue.Labels = append(issue.Labels, l.Name)
	}
	for _, a := range i.Assignees {
		issue.Assignees = append(issue.Assignees, a.UserName)
	}
	if i.Milestone != nil {
		issue.Milestone = &provider.MilestoneRef{Number: strconv.FormatInt(i.Milestone.ID, 10), Title: i.Milestone.Title}
	}
	issue.CreatedAt, issue.UpdatedAt = i.Created, i.Updated
	if i.Closed != nil {
		issue.ClosedAt = i.Closed
	}
	return issue
}

// convertIssueComment maps a forgejo.Comment to a provider.IssueComment.
func convertIssueComment(c *forgejo.Comment) *provider.IssueComment {
	if c == nil {
		return nil
	}
	return &provider.IssueComment{
		ID:        c.ID,
		Body:      c.Body,
		Author:    convertUser(c.Poster),
		CreatedAt: c.Created,
		UpdatedAt: c.Updated,
	}
}

// convertUser maps a forgejo.User to a provider.CRUser. Returns nil if u is nil.
func convertUser(u *forgejo.User) *provider.CRUser {
	if u == nil {
		return nil
	}
	return &provider.CRUser{ID: u.ID, Username: u.UserName, Name: u.FullName, AvatarURL: u.AvatarURL}
}

var _ provider.IssueManager = (*Provider)(nil)
