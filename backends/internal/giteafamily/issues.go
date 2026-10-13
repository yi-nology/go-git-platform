package giteafamily

import (
	"context"
	"strconv"
	"strings"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- issues ---

// FamilyIssueListOption mirrors the family's issue list filters (KeyWord,
// State, Assignee, Labels, pagination).
type FamilyIssueListOption struct {
	ListOptions ListOptions
	KeyWord     string
	State       string
	Assignee    string
	Labels      string
	Type        string // "issues" restricts the global search to real issues
}

// FamilyCreateIssueOption mirrors the family's issue create body.
type FamilyCreateIssueOption struct {
	Title     string
	Body      string
	Assignees []string
	Milestone int64
	Labels    []int64
}

// FamilyEditIssueOption mirrors the family's issue edit body. The family's
// edit endpoint always serializes Title (no omitempty), so the engine
// backfills the current title via one GET when the caller leaves it empty
// (an empty title would otherwise clear the field).
type FamilyEditIssueOption struct {
	Title     string
	Body      *string
	State     *string
	Assignees []string
	Milestone *int64
}

// IssueLabelsOption mirrors the family's label-IDs body (add/replace).
type IssueLabelsOption struct {
	Labels []int64
}

// ListIssueCommentsOptions mirrors the family's issue-comment list query.
type ListIssueCommentsOptions struct {
	ListOptions ListOptions
}

// EditIssueCommentOption mirrors the family's issue-comment edit body.
type EditIssueCommentOption struct {
	Body string
}

// IssuePort is the issue + issue-comment endpoint of the family SDK surface.
type IssuePort interface {
	ListRepoIssues(ctx context.Context, owner, repo string, opt FamilyIssueListOption) ([]*Issue, error)
	GetIssue(ctx context.Context, owner, repo string, index int64) (*Issue, error)
	CreateIssue(ctx context.Context, owner, repo string, opt FamilyCreateIssueOption) (*Issue, error)
	EditIssue(ctx context.Context, owner, repo string, index int64, opt FamilyEditIssueOption) (*Issue, error)
	ReplaceIssueLabels(ctx context.Context, owner, repo string, index int64, opt IssueLabelsOption) error
	AddIssueLabels(ctx context.Context, owner, repo string, index int64, opt IssueLabelsOption) error
	DeleteIssueLabel(ctx context.Context, owner, repo string, index int64, labelID int64) error
	ListIssueComments(ctx context.Context, owner, repo string, index int64, opt ListIssueCommentsOptions) ([]*FamilyComment, error)
	EditIssueComment(ctx context.Context, owner, repo string, commentID int64, opt EditIssueCommentOption) (*FamilyComment, error)
}

// IssueLabelCache is the per-provider label-ID cache (issues and labels share
// one cached scan per repo).
type IssueLabelCache = backendutil.IDCache

// ListIssues lists issues (dual-mode pagination via backendutil.PageList;
// with Page == 0 the full list is walked, with Page > 0 exactly that
// caller-driven page). The family mixes PRs into the issue endpoint, so the
// engine filters to real issues (Type: "issues").
func ListIssues(ctx context.Context, f Family, c IssuePort, opts provider.ListIssuesOptions) ([]*provider.Issue, int, error) {
	listOpts := FamilyIssueListOption{Type: "issues"}
	if opts.State != "" {
		listOpts.State = string(opts.State)
	}
	if opts.Labels != "" {
		listOpts.Labels = opts.Labels
	}
	if opts.Assignee != "" {
		listOpts.Assignee = opts.Assignee
	}
	issues, err := backendutil.PageList(opts.Page, opts.PerPage, f.PageSize,
		func(page, perPage int) ([]*Issue, error) {
			listOpts.ListOptions = ListOptions{Page: page, PageSize: perPage}
			return c.ListRepoIssues(ctx, opts.Owner, opts.Repo, listOpts)
		})
	if err != nil {
		return nil, 0, provider.Wrap(f.Platform, "ListIssues", err)
	}
	result := make([]*provider.Issue, 0, len(issues))
	for _, i := range issues {
		result = append(result, convertIssue(i))
	}
	return result, len(result), nil
}

// GetIssue fetches one issue.
func GetIssue(ctx context.Context, f Family, c IssuePort, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "GetIssue", number)
	if err != nil {
		return nil, err
	}
	issue, err := c.GetIssue(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetIssue", err)
	}
	return convertIssue(issue), nil
}

// CreateIssue creates an issue. The family's create endpoint takes label
// IDs, so names are resolved first through the shared cached resolver.
func CreateIssue(ctx context.Context, f Family, c IssuePort, labels LabelPort, cache *IssueLabelCache, opts provider.CreateIssueOptions) (*provider.Issue, error) {
	createOpts := FamilyCreateIssueOption{
		Title:     opts.Title,
		Body:      opts.Body,
		Assignees: opts.Assignees,
	}
	if opts.Milestone != "" {
		m, err := backendutil.ParseMilestoneNumber(f.Platform, "CreateIssue", opts.Milestone)
		if err != nil {
			return nil, err
		}
		createOpts.Milestone = m
	}
	if len(opts.Labels) > 0 {
		ids, err := ResolveLabelIDs(ctx, f, labels, cache, "CreateIssue", opts.Owner, opts.Repo, opts.Labels)
		if err != nil {
			return nil, err
		}
		createOpts.Labels = ids
	}
	issue, err := c.CreateIssue(ctx, opts.Owner, opts.Repo, createOpts)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateIssue", err)
	}
	return convertIssue(issue), nil
}

// UpdateIssue edits an issue. The family's EditIssueOption always serializes
// Title (no omitempty), so a caller leaving fields empty would clear them;
// the current title is backfilled via one GET, and an all-empty update
// short-circuits to a plain GetIssue. opts.Labels replaces the issue's
// labels via the dedicated endpoint.
//
// KNOWN COST: when opts.Title is empty this issues one extra GET to backfill
// the current title.
func UpdateIssue(ctx context.Context, f Family, c IssuePort, labels LabelPort, cache *IssueLabelCache, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "UpdateIssue", number)
	if err != nil {
		return nil, err
	}
	if opts.Title == "" && opts.Body == "" && opts.State == "" && len(opts.Assignees) == 0 && len(opts.Labels) == 0 && opts.Milestone == "" {
		return GetIssue(ctx, f, c, owner, repo, number)
	}
	edit, err := buildEditIssueOption(ctx, f, c, "UpdateIssue", owner, repo, n, opts)
	if err != nil {
		return nil, err
	}
	if len(opts.Labels) > 0 {
		ids, err := ResolveLabelIDs(ctx, f, labels, cache, "UpdateIssue", owner, repo, opts.Labels)
		if err != nil {
			return nil, err
		}
		if err := c.ReplaceIssueLabels(ctx, owner, repo, n, IssueLabelsOption{Labels: ids}); err != nil {
			return nil, provider.Wrap(f.Platform, "UpdateIssue", err)
		}
	}
	issue, err := c.EditIssue(ctx, owner, repo, n, edit)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateIssue", err)
	}
	return convertIssue(issue), nil
}

// buildEditIssueOption translates provider update options into the family's
// edit option, backfilling the title via one GET when the caller leaves it
// empty (the family's EditIssueOption always serializes Title — no
// omitempty — so an empty string would clear it).
func buildEditIssueOption(ctx context.Context, f Family, c IssuePort, op, owner, repo string, n int64, opts provider.UpdateIssueOptions) (FamilyEditIssueOption, error) {
	edit := FamilyEditIssueOption{}
	if opts.Title != "" {
		edit.Title = opts.Title
	} else {
		current, err := c.GetIssue(ctx, owner, repo, n)
		if err != nil {
			return edit, provider.Wrap(f.Platform, op, err)
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
		s := string(opts.State)
		edit.State = &s
	}
	if opts.Milestone != "" {
		m, err := backendutil.ParseMilestoneNumber(f.Platform, op, opts.Milestone)
		if err != nil {
			return edit, err
		}
		edit.Milestone = &m
	}
	return edit, nil
}

// CloseIssue closes an issue.
func CloseIssue(ctx context.Context, f Family, c IssuePort, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "CloseIssue", number)
	if err != nil {
		return nil, err
	}
	return setIssueState(ctx, f, c, owner, repo, n, "closed", "CloseIssue")
}

// ReopenIssue reopens an issue.
func ReopenIssue(ctx context.Context, f Family, c IssuePort, owner, repo, number string) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "ReopenIssue", number)
	if err != nil {
		return nil, err
	}
	return setIssueState(ctx, f, c, owner, repo, n, "open", "ReopenIssue")
}

// setIssueState closes/reopens an issue. The family's edit endpoint always
// serializes Title, so the current title is fetched first to avoid clearing
// it (one extra GET per close/reopen; same constraint as
// buildEditIssueOption).
func setIssueState(ctx context.Context, f Family, c IssuePort, owner, repo string, n int64, state, op string) (*provider.Issue, error) {
	current, err := c.GetIssue(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(f.Platform, op, err)
	}
	issue, err := c.EditIssue(ctx, owner, repo, n, FamilyEditIssueOption{Title: current.Title, State: &state})
	if err != nil {
		return nil, provider.Wrap(f.Platform, op, err)
	}
	return convertIssue(issue), nil
}

// ListIssueComments lists an issue's comments, walking the full endpoint
// pagination.
func ListIssueComments(ctx context.Context, f Family, c IssuePort, owner, repo, number string) ([]*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "ListIssueComments", number)
	if err != nil {
		return nil, err
	}
	comments, err := backendutil.AllPages(func(page int) ([]*FamilyComment, error) {
		return c.ListIssueComments(ctx, owner, repo, n, ListIssueCommentsOptions{
			ListOptions: ListOptions{Page: page, PageSize: backendutil.IssueCommentPageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListIssueComments", err)
	}
	result := make([]*provider.IssueComment, 0, len(comments))
	for _, c := range comments {
		result = append(result, convertIssueComment(c))
	}
	return result, nil
}

// CreateIssueComment adds a comment to an issue.
func CreateIssueComment(ctx context.Context, f Family, c DiffPort, owner, repo, number, body string) (*provider.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "CreateIssueComment", number)
	if err != nil {
		return nil, err
	}
	comment, _, err := c.CreateIssueComment(ctx, owner, repo, n, CreateIssueCommentOption{Body: body})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateIssueComment", err)
	}
	return convertIssueComment(comment), nil
}

// UpdateIssueComment edits a comment. The family's edit endpoint addresses
// the comment directly (number is unused); the platform only lets the
// comment's author perform the edit.
func UpdateIssueComment(ctx context.Context, f Family, c IssuePort, owner, repo, number string, commentID int64, body string) (*provider.IssueComment, error) {
	comment, err := c.EditIssueComment(ctx, owner, repo, commentID, EditIssueCommentOption{Body: body})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateIssueComment", err)
	}
	return convertIssueComment(comment), nil
}

// ListIssueLabels lists the repository-level labels, exhausting pagination.
func ListIssueLabels(ctx context.Context, f Family, c LabelPort, owner, repo string) ([]*provider.IssueLabel, error) {
	labels, err := backendutil.AllPages(func(page int) ([]*Label, error) {
		return c.ListRepoLabels(ctx, owner, repo, ListLabelsOptions{
			ListOptions: ListOptions{Page: page, PageSize: backendutil.LabelPageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListIssueLabels", err)
	}
	result := make([]*provider.IssueLabel, 0, len(labels))
	for _, l := range labels {
		result = append(result, &provider.IssueLabel{ID: l.ID, Name: l.Name, Color: strings.TrimPrefix(l.Color, "#")})
	}
	return result, nil
}

// AddIssueLabels adds labels to an issue (names resolved to IDs first).
func AddIssueLabels(ctx context.Context, f Family, c IssuePort, labels LabelPort, cache *IssueLabelCache, owner, repo, number string, names []string) error {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "AddIssueLabels", number)
	if err != nil {
		return err
	}
	ids, err := ResolveLabelIDs(ctx, f, labels, cache, "AddIssueLabels", owner, repo, names)
	if err != nil {
		return err
	}
	if err := c.AddIssueLabels(ctx, owner, repo, n, IssueLabelsOption{Labels: ids}); err != nil {
		return provider.Wrap(f.Platform, "AddIssueLabels", err)
	}
	return nil
}

// RemoveIssueLabel removes a label from an issue (name resolved to ID first).
func RemoveIssueLabel(ctx context.Context, f Family, c IssuePort, labels LabelPort, cache *IssueLabelCache, owner, repo, number, name string) error {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "RemoveIssueLabel", number)
	if err != nil {
		return err
	}
	id, err := ResolveLabelID(ctx, f, labels, cache, "RemoveIssueLabel", owner, repo, name)
	if err != nil {
		return err
	}
	if err := c.DeleteIssueLabel(ctx, owner, repo, n, id); err != nil {
		return provider.Wrap(f.Platform, "RemoveIssueLabel", err)
	}
	return nil
}

// convertIssue maps a family Issue to the provider shape.
func convertIssue(i *Issue) *provider.Issue {
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
		if l != nil {
			issue.Labels = append(issue.Labels, l.Name)
		}
	}
	for _, a := range i.Assignees {
		if a != nil {
			issue.Assignees = append(issue.Assignees, a.UserName)
		}
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

// convertIssueComment maps a family comment to the provider shape.
func convertIssueComment(c *FamilyComment) *provider.IssueComment {
	if c == nil {
		return nil
	}
	return &provider.IssueComment{
		ID:        c.ID,
		Body:      c.Body,
		Author:    convertUser(c.Poster),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// ResolveLabelIDs resolves label names to numeric IDs through the family's
// cached resolver (one cached scan per repo, shared with the label
// manager's update/delete path).
func ResolveLabelIDs(ctx context.Context, f Family, c LabelPort, cache *backendutil.IDCache, op, owner, repo string, names []string) ([]int64, error) {
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		id, err := ResolveLabelID(ctx, f, c, cache, op, owner, repo, name)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
