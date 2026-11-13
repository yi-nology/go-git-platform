package giteafamily

import (
	"context"
	"errors"
	"slices"
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

// ListIssues lists issues (caller-paged family, issue #51 — see
// provider.NormalizePageOpts: Page == 0 serves the platform's first page in
// exactly one request, Page > 0 exactly that caller-driven page; the int
// return is len(result), the served rows' count). The family mixes PRs into
// the issue endpoint, so the engine filters to real issues (Type: "issues").
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
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts.ListOptions = ListOptions{Page: page, PageSize: perPage}
	issues, err := c.ListRepoIssues(ctx, opts.Owner, opts.Repo, listOpts)
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
// labels via the dedicated endpoint; a labels-only update skips the
// trailing EditIssue entirely — the side-endpoint PUT is the complete
// update, and a PATCH after it would only rewrite the backfilled title
// while leaking body:null/assignees:null onto the wire (T2 review
// 放行条件②).
//
// KNOWN COST: when opts.Title is empty this issues one extra GET to backfill
// the current title.
func UpdateIssue(ctx context.Context, f Family, c IssuePort, labels LabelPort, cache *IssueLabelCache, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "UpdateIssue", number)
	if err != nil {
		return nil, err
	}
	if issueUpdateAllEmpty(opts) {
		return GetIssue(ctx, f, c, owner, repo, number)
	}
	edit, backfilled, err := buildEditIssueOption(ctx, f, c, "UpdateIssue", owner, repo, n, opts)
	if err != nil {
		return nil, err
	}
	ls, lsOK, err := applyIssueLabelsUpdate(ctx, f, c, labels, cache, owner, repo, n, opts)
	if err != nil {
		return nil, err
	}
	// T2 放行条件②：编辑面全零 ⇒ labels 侧端点 PUT 即完整更新，跳过尾随
	// EditIssue（PATCH 只会回写 backfill 的原题并尾漏 body:null）。
	if editIssueNoChange(opts, edit) {
		issue := convertIssue(backfilled)
		if lsOK && issue != nil {
			// 快照取自 PUT 之前，把已落线的标签集回写到返回值上。
			issue.Labels = slices.Clone(ls)
		}
		return issue, nil
	}
	issue, err := c.EditIssue(ctx, owner, repo, n, edit)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateIssue", err)
	}
	return convertIssue(issue), nil
}

// issueUpdateAllEmpty reports whether every legacy and patch field of opts
// is zero — the all-empty short-circuit (a plain GET, no write). The four
// patch-nil checks matter: without them a patch-only update (e.g.
// BodyPatch:&"") would degrade into the GET short-circuit and the clear
// would silently no-op.
func issueUpdateAllEmpty(opts provider.UpdateIssueOptions) bool {
	return opts.Title == "" && opts.Body == "" && opts.State == "" &&
		len(opts.Assignees) == 0 && len(opts.Labels) == 0 && opts.Milestone == "" &&
		opts.BodyPatch == nil && opts.AssigneesPatch == nil &&
		opts.LabelsPatch == nil && opts.MilestonePatch == nil
}

// applyIssueLabelsUpdate runs the labels side endpoint when ResolveLabels
// specifies a set (三态收编 2A：非 specified 不动；specified 空集 ⇒ 不查
// 直接 [] —— ResolveLabelIDs 对空输入零 API 调用 —— ⇒ wire
// {"labels":[]} PUT). It returns the resolved name set for the caller's
// return-value sync.
func applyIssueLabelsUpdate(ctx context.Context, f Family, c IssuePort, labels LabelPort, cache *IssueLabelCache, owner, repo string, n int64, opts provider.UpdateIssueOptions) ([]string, bool, error) {
	ls, ok := opts.ResolveLabels()
	if !ok {
		return nil, false, nil
	}
	ids, err := ResolveLabelIDs(ctx, f, labels, cache, "UpdateIssue", owner, repo, ls)
	if err != nil {
		return nil, false, err
	}
	if err := c.ReplaceIssueLabels(ctx, owner, repo, n, IssueLabelsOption{Labels: ids}); err != nil {
		// 2xx 状态在链上 = 写入已落库（family SDK 却强制把 2xx 响应解析
		// 成 []*Label——契约夹具对所有写请求统一回 issue 对象会在此解析
		// 失败；本端点签名只回 error、响应体引擎不消费，真实 Gitea/Forgejo
		// 回 label 数组可正常解析）。4xx/5xx 与无状态的传输错误照常上抛。
		if !writeLanded(err) {
			return nil, false, provider.Wrap(f.Platform, "UpdateIssue", err)
		}
	}
	return ls, true, nil
}

// editIssueNoChange reports whether the built edit option carries no caller
// change: Title is only the backfill (opts.Title == "" ⇒ rewriting the
// current title back is a no-op) and Body/State/Assignees/Milestone were
// never specified (Resolve* left them nil). Reaching this past the
// all-empty short-circuit implies labels were specified — the side-endpoint
// PUT was the whole update.
func editIssueNoChange(opts provider.UpdateIssueOptions, edit FamilyEditIssueOption) bool {
	return opts.Title == "" && edit.Body == nil && edit.State == nil &&
		edit.Assignees == nil && edit.Milestone == nil
}

// writeLanded reports whether the port's error still carries a 2xx HTTP
// status on the chain (the family adapters' withStatus threads it through
// provider.WrapStatusError): the request reached the server and was
// accepted, so only reading the — unused — response body failed.
func writeLanded(err error) bool {
	var se *provider.StatusError
	return errors.As(err, &se) && se.StatusCode() >= 200 && se.StatusCode() < 300
}

// buildEditIssueOption translates provider update options into the family's
// edit option, backfilling the title via one GET when the caller leaves it
// empty (the family's EditIssueOption always serializes Title — no
// omitempty — so an empty string would clear it). The backfilled issue is
// returned alongside the option so a labels-only update (EditIssue skipped)
// still has a value to return without a second GET after the PUT.
func buildEditIssueOption(ctx context.Context, f Family, c IssuePort, op, owner, repo string, n int64, opts provider.UpdateIssueOptions) (FamilyEditIssueOption, *Issue, error) {
	edit := FamilyEditIssueOption{}
	var current *Issue
	if opts.Title != "" {
		edit.Title = opts.Title
	} else {
		var err error
		current, err = c.GetIssue(ctx, owner, repo, n)
		if err != nil {
			return edit, nil, provider.Wrap(f.Platform, op, err)
		}
		edit.Title = current.Title
	}
	// 三态收编（2A）：Resolve* 非 specified 不动；specified 空值即清空
	// （Body new("") ⇒ wire "body":""、Assignees [] ⇒ wire "assignees":[]、
	// Milestone "" ⇒ wire "milestone":0——0-vs-null 语义 Phase 3 实测）。
	if as, ok := opts.ResolveAssignees(); ok {
		edit.Assignees = as
	}
	if body, ok := opts.ResolveBody(); ok {
		edit.Body = new(body)
	}
	if opts.State != "" {
		s := string(opts.State)
		edit.State = &s
	}
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			edit.Milestone = new(int64(0)) // wire "milestone":0
		} else {
			m64, err := backendutil.ParseMilestoneNumber(f.Platform, op, m)
			if err != nil {
				return edit, nil, err
			}
			edit.Milestone = &m64
		}
	}
	return edit, current, nil
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

// DeleteIssueComment removes a comment. The family's delete endpoint lives
// on DiffPort and addresses the comment directly (number is unused);
// deletion is permanent and the platform only lets the comment's author
// perform it.
func DeleteIssueComment(ctx context.Context, f Family, c DiffPort, owner, repo, number string, commentID int64) error {
	if _, err := c.DeleteIssueComment(ctx, owner, repo, commentID); err != nil {
		return provider.Wrap(f.Platform, "DeleteIssueComment", err)
	}
	return nil
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
