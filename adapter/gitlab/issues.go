package gitlab

import (
	"context"
	"strconv"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListIssues implements provider.IssueManager.
func (p *Provider) ListIssues(ctx context.Context, opts provider.ListIssuesOptions) ([]*model.Issue, int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &gitlab.ListProjectIssuesOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)}}
	if opts.State != "" {
		s := string(opts.State)
		if s == string(model.IssueStateOpen) {
			s = "opened" // GitLab's issues API vocabulary; inbound convertIssue maps back
		}
		listOpts.State = new(s)
	}
	if opts.Assignee != "" {
		listOpts.AssigneeUsername = new(opts.Assignee)
	}
	if opts.Labels != "" {
		lo := gitlab.LabelOptions(strings.Split(opts.Labels, ","))
		listOpts.Labels = &lo
	}
	// **排序要真的传出去**:增量备份靠它捞"上次备份后被改过的老 issue"
	// (平台默认按 created 排 ⇒ 被改过的老 issue 正好在第一页之外,客户端再按
	// updated 过滤也捞不到)。
	// GitLab 的词表是 created_at / updated_at,方向走 sort=asc|desc。
	if opts.Sort != "" {
		switch opts.Sort {
		case "created":
			listOpts.OrderBy = new("created_at")
		case "updated":
			listOpts.OrderBy = new("updated_at")
		}
	}
	if opts.Direction == "asc" || opts.Direction == "desc" {
		listOpts.Sort = new(opts.Direction)
	}
	issues, _, err := p.client.Issues.ListProjectIssues(pidOf(opts.Owner, opts.Repo), listOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, 0, provider.Wrap(model.PlatformGitLab, "ListIssues", err)
	}
	result := make([]*model.Issue, 0, len(issues))
	for _, i := range issues {
		result = append(result, convertIssue(i))
	}
	return result, len(result), nil
}

// GetIssue implements provider.IssueManager.
func (p *Provider) GetIssue(ctx context.Context, owner, repo, number string) (*model.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "GetIssue", number)
	if err != nil {
		return nil, err
	}
	issue, _, err := p.client.Issues.GetIssue(pidOf(owner, repo), n, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "GetIssue", err)
	}
	return convertIssue(issue), nil
}

// CreateIssue implements provider.IssueManager.
// Assignees are resolved to user IDs via the Users API (cached).
func (p *Provider) CreateIssue(ctx context.Context, opts provider.CreateIssueOptions) (*model.Issue, error) {
	createOpts := &gitlab.CreateIssueOptions{Title: new(opts.Title)}
	if opts.Body != "" {
		createOpts.Description = new(opts.Body)
	}
	if len(opts.Labels) > 0 {
		lo := gitlab.LabelOptions(opts.Labels)
		createOpts.Labels = &lo
	}
	if len(opts.Assignees) > 0 {
		assigneeIDs, err := p.resolveUserIDs(ctx, "CreateIssue", opts.Assignees)
		if err != nil {
			return nil, err
		}
		createOpts.AssigneeIDs = &assigneeIDs
	}
	if opts.Milestone != "" {
		m, err := backendutil.ParseMilestoneNumber(model.PlatformGitLab, "CreateIssue", opts.Milestone)
		if err != nil {
			return nil, err
		}
		createOpts.MilestoneID = new(m)
	}
	issue, _, err := p.client.Issues.CreateIssue(pidOf(opts.Owner, opts.Repo), createOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CreateIssue", err)
	}
	return convertIssue(issue), nil
}

// UpdateIssue implements provider.IssueManager.
// Assignees are resolved to user IDs via the Users API (cached).
func (p *Provider) UpdateIssue(ctx context.Context, owner, repo, number string, opts provider.UpdateIssueOptions) (*model.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "UpdateIssue", number)
	if err != nil {
		return nil, err
	}
	updateOpts := &gitlab.UpdateIssueOptions{}
	if opts.Title != "" {
		updateOpts.Title = new(opts.Title)
	}
	// 三态收编（2A）：Resolve* 非 specified 不动；specified 空值即清空。
	if body, ok := opts.ResolveBody(); ok {
		updateOpts.Description = new(body) // 清空 ⇒ "description":""
	}
	if opts.State != "" {
		event := "close"
		if opts.State == model.IssueStateOpen {
			event = "reopen"
		}
		updateOpts.StateEvent = new(event) // IssueState → state_event 动词
	}
	if ls, ok := opts.ResolveLabels(); ok {
		lo := gitlab.LabelOptions(ls) // 清空 ⇒ 非 nil 空 ⇒ marshal ""
		updateOpts.Labels = &lo
	}
	if as, ok := opts.ResolveAssignees(); ok {
		ids := []int64{} // 清空：跳过 resolve 直接空集 ⇒ "assignee_ids":[]
		if len(as) > 0 {
			var err error
			ids, err = p.resolveUserIDs(ctx, "UpdateIssue", as)
			if err != nil {
				return nil, err
			}
		}
		updateOpts.AssigneeIDs = &ids
	}
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			// 一等公民（矩阵全 A）：ResetMilestoneID 发字面
			// "milestone_id":null；与 MilestoneID 互斥——clear 分支不得同时
			// 设 MilestoneID。
			updateOpts.ResetMilestoneID = true
		} else {
			m64, err := backendutil.ParseMilestoneNumber(model.PlatformGitLab, "UpdateIssue", m)
			if err != nil {
				return nil, err
			}
			updateOpts.MilestoneID = new(m64)
		}
	}
	issue, _, err := p.client.Issues.UpdateIssue(pidOf(owner, repo), n, updateOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "UpdateIssue", err)
	}
	return convertIssue(issue), nil
}

// CloseIssue implements provider.IssueManager via the state_event API.
func (p *Provider) CloseIssue(ctx context.Context, owner, repo, number string) (*model.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "CloseIssue", number)
	if err != nil {
		return nil, err
	}
	issue, _, err := p.client.Issues.UpdateIssue(pidOf(owner, repo), n,
		&gitlab.UpdateIssueOptions{StateEvent: new("close")}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CloseIssue", err)
	}
	return convertIssue(issue), nil
}

// ReopenIssue implements provider.IssueManager via the state_event API.
func (p *Provider) ReopenIssue(ctx context.Context, owner, repo, number string) (*model.Issue, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "ReopenIssue", number)
	if err != nil {
		return nil, err
	}
	issue, _, err := p.client.Issues.UpdateIssue(pidOf(owner, repo), n,
		&gitlab.UpdateIssueOptions{StateEvent: new("reopen")}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ReopenIssue", err)
	}
	return convertIssue(issue), nil
}

// ListIssueComments implements provider.IssueManager. GitLab models issue
// comments as notes; the loop exhausts GitLab's pagination (advances until
// an empty page), so the result is the complete comment list.
func (p *Provider) ListIssueComments(ctx context.Context, owner, repo, number string) ([]*model.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "ListIssueComments", number)
	if err != nil {
		return nil, err
	}
	notes, err := backendutil.AllPages(func(page int) ([]*gitlab.Note, error) {
		batch, _, err := p.client.Notes.ListIssueNotes(pidOf(owner, repo), n, &gitlab.ListIssueNotesOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: backendutil.IssueCommentPageSize},
		}, gitlab.WithContext(ctx))
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListIssueComments", err)
	}
	result := make([]*model.IssueComment, 0, len(notes))
	for _, note := range notes {
		result = append(result, convertIssueComment(note))
	}
	return result, nil
}

// CreateIssueComment implements provider.IssueManager.
func (p *Provider) CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*model.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "CreateIssueComment", number)
	if err != nil {
		return nil, err
	}
	note, _, err := p.client.Notes.CreateIssueNote(pidOf(owner, repo), n,
		&gitlab.CreateIssueNoteOptions{Body: new(body)}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CreateIssueComment", err)
	}
	return convertIssueComment(note), nil
}

// UpdateIssueComment implements provider.IssueManager. GitLab addresses
// issue notes through the issue, so number must carry the issue's IID; the
// platform only lets the note's author perform the edit.
func (p *Provider) UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*model.IssueComment, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "UpdateIssueComment", number)
	if err != nil {
		return nil, err
	}
	note, _, err := p.client.Notes.UpdateIssueNote(pidOf(owner, repo), n, commentID,
		&gitlab.UpdateIssueNoteOptions{Body: new(body)}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "UpdateIssueComment", err)
	}
	return convertIssueComment(note), nil
}

// DeleteIssueComment implements provider.IssueManager. GitLab deletes issue
// notes through the issue like its edit endpoint, so number must carry the
// issue's IID. Deletion is permanent; the platform only lets the note's
// author perform it.
func (p *Provider) DeleteIssueComment(ctx context.Context, owner, repo, number string, commentID int64) error {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "DeleteIssueComment", number)
	if err != nil {
		return err
	}
	if _, err := p.client.Notes.DeleteIssueNote(pidOf(owner, repo), n, commentID, gitlab.WithContext(ctx)); err != nil {
		return provider.Wrap(model.PlatformGitLab, "DeleteIssueComment", err)
	}
	return nil
}

// ListIssueLabels implements provider.IssueManager: repository-level labels,
// exhausting pagination (the loop advances until an empty page).
func (p *Provider) ListIssueLabels(ctx context.Context, owner, repo string) ([]*model.IssueLabel, error) {
	labels, err := backendutil.AllPages(func(page int) ([]*gitlab.Label, error) {
		batch, _, err := p.client.Labels.ListLabels(pidOf(owner, repo),
			&gitlab.ListLabelsOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: backendutil.LabelPageSize}}, gitlab.WithContext(ctx))
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListIssueLabels", err)
	}
	result := make([]*model.IssueLabel, 0, len(labels))
	for _, l := range labels {
		result = append(result, &model.IssueLabel{ID: l.ID, Name: l.Name, Color: strings.TrimPrefix(l.Color, "#")})
	}
	return result, nil
}

// AddIssueLabels implements provider.IssueManager via add_labels.
func (p *Provider) AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "AddIssueLabels", number)
	if err != nil {
		return err
	}
	lo := gitlab.LabelOptions(labels)
	_, _, err = p.client.Issues.UpdateIssue(pidOf(owner, repo), n,
		&gitlab.UpdateIssueOptions{AddLabels: &lo}, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(model.PlatformGitLab, "AddIssueLabels", err)
	}
	return nil
}

// RemoveIssueLabel implements provider.IssueManager via remove_labels.
func (p *Provider) RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "RemoveIssueLabel", number)
	if err != nil {
		return err
	}
	rl := gitlab.LabelOptions{name}
	_, _, err = p.client.Issues.UpdateIssue(pidOf(owner, repo), n,
		&gitlab.UpdateIssueOptions{RemoveLabels: &rl}, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(model.PlatformGitLab, "RemoveIssueLabel", err)
	}
	return nil
}

// RemoveIssueLabels implements provider.IssueManager（#163）：GitLab 原生
// remove_labels 批量面——单请求移除全部目标，保序由服务端语义保证。
func (p *Provider) RemoveIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "RemoveIssueLabels", number)
	if err != nil {
		return err
	}
	rl := gitlab.LabelOptions(labels)
	_, _, err = p.client.Issues.UpdateIssue(pidOf(owner, repo), n,
		&gitlab.UpdateIssueOptions{RemoveLabels: &rl}, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(model.PlatformGitLab, "RemoveIssueLabels", err)
	}
	return nil
}

// convertUser maps a gitlab.IssueAuthor to a model.CRUser. Returns nil if
// a is nil.
func convertUser(a *gitlab.IssueAuthor) *model.CRUser {
	if a == nil {
		return nil
	}
	return &model.CRUser{ID: a.ID, Username: a.Username, Name: a.Name, AvatarURL: a.AvatarURL}
}

// convertNoteAuthor maps a gitlab.NoteAuthor (a value type on Note) to a
// model.CRUser.
func convertNoteAuthor(a gitlab.NoteAuthor) *model.CRUser {
	return &model.CRUser{ID: a.ID, Username: a.Username, Name: a.Name, AvatarURL: a.AvatarURL}
}

// convertIssue maps a gitlab.Issue to a model.Issue. GitLab reports
// state "opened"; the SDK normalizes to "open".
func convertIssue(i *gitlab.Issue) *model.Issue {
	if i == nil {
		return nil
	}
	issue := &model.Issue{
		ID:     i.ID,
		Number: strconv.FormatInt(i.IID, 10),
		Title:  i.Title,
		Body:   i.Description,
		Labels: append([]string(nil), i.Labels...),
		Author: convertUser(i.Author),
		WebURL: i.WebURL,
	}
	switch i.State {
	case "opened", "reopened":
		issue.State = model.IssueStateOpen
	case "closed":
		issue.State = model.IssueStateClosed
	}
	if i.Milestone != nil {
		issue.Milestone = &model.MilestoneRef{Number: strconv.FormatInt(i.Milestone.ID, 10), Title: i.Milestone.Title}
	}
	for _, a := range i.Assignees {
		issue.Assignees = append(issue.Assignees, a.Username)
	}
	issue.CreatedAt, issue.UpdatedAt = timeOrZero(i.CreatedAt), timeOrZero(i.UpdatedAt)
	if i.ClosedAt != nil {
		issue.ClosedAt = i.ClosedAt
	}
	return issue
}

// convertIssueComment maps a gitlab.Note to a model.IssueComment.
func convertIssueComment(n *gitlab.Note) *model.IssueComment {
	if n == nil {
		return nil
	}
	return &model.IssueComment{
		ID:        n.ID,
		Body:      n.Body,
		Author:    convertNoteAuthor(n.Author),
		CreatedAt: timeOrZero(n.CreatedAt),
		UpdatedAt: timeOrZero(n.UpdatedAt),
	}
}

var _ provider.IssueManager = (*Provider)(nil)
