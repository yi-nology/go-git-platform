package gitcode

import (
	"context"
	"strings"

	oekiterrors "github.com/oekit/pkg/errors"

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
//
// 三态接线（2A T6）：body/milestone 是 B 级——issue-update wire 是
// string/int64 + omitempty，空值不可表达且没有清空侧端点，patch 清空在
// 任何 HTTP 之前返回 provider.ErrClearNotSupported（整调用失败，零请
// 求）；labels/assignees 是 A 级——置值进主 update（omitempty 裁零），
// 清空走侧端点（PUT ReplaceIssueLabels / DELETE RemoveIssueAssignees）。
//
// 顺序裁定（brief Step 2）：先发主 update（携带全部置值字段），再发侧
// 端点清空——混合 set+clear 的一次调用两个请求都会到，且侧端点是最后
// 一条请求（harness dispatchClear 按「最后一条」断言侧端点形态）。
func (p *Provider) UpdateIssue(ctx context.Context, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitCode, "UpdateIssue", number)
	if err != nil {
		return nil, err
	}
	plan, err := planIssueUpdate(opts)
	if err != nil {
		return nil, err
	}
	// 顺序裁定：主 update 先（携带全部置值字段），侧端点清空后。
	issue, err := p.client.UpdateIssue(ctx, owner, repo, n, plan.update)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateIssue", err)
	}
	if err := p.applyIssueClears(ctx, owner, repo, n, plan); err != nil {
		return nil, err
	}
	return convertIssue(issue), nil
}

// issueUpdatePlan 是 UpdateIssue 的三态解析产物（planIssueUpdate 纯函
// 数、零 HTTP）：update 承载全部置值字段（omitempty 裁零），两个 pending
// 标记把 A 级清空挂到侧端点（主 update 之后执行）。
type issueUpdatePlan struct {
	update         gitcode.UpdateIssueOptions
	clearLabels    bool
	clearAssignees bool
}

// planIssueUpdate 把 provider 三态选项翻译成 wire 选项。B 级字段
// （body/milestone）的 patch 清空在这里就返回 ErrClearNotSupported——
// 调用方在任何 HTTP 之前失败，整调用零请求。
func planIssueUpdate(opts provider.UpdateIssueOptions) (issueUpdatePlan, error) {
	var plan issueUpdatePlan
	// Title/State 既有无条件形态保留（omitempty 裁零；State 无三态面）。
	plan.update.Title = opts.Title
	if opts.State != "" {
		plan.update.State = gitcode.IssueState(opts.State)
	}
	// B 级：string omitempty 丢 "" 且无 body 侧端点。
	if body, ok := opts.ResolveBody(); ok {
		if body == "" {
			return plan, oekiterrors.Wrap(provider.ErrClearNotSupported, "clear body",
				"platform", "gitcode", "field", "body")
		}
		plan.update.Body = body
	}
	// A 级：置值进主 update；清空挂侧端点（主 update 之后执行）。
	if ls, ok := opts.ResolveLabels(); ok {
		if len(ls) == 0 {
			plan.clearLabels = true
		} else {
			plan.update.Labels = ls
		}
	}
	// B 级：milestone 是 int64+omitempty，0 不可表达且无 detach 端点。
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			return plan, oekiterrors.Wrap(provider.ErrClearNotSupported, "clear milestone",
				"platform", "gitcode", "field", "milestone")
		}
		m64, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "UpdateIssue", m)
		if err != nil {
			return plan, err
		}
		plan.update.Milestone = m64
	}
	// A 级：置值走 update 的 omitempty；清空走侧端点 DELETE。
	if as, ok := opts.ResolveAssignees(); ok {
		if len(as) == 0 {
			plan.clearAssignees = true
		} else {
			plan.update.Assignees = as
		}
	}
	return plan, nil
}

// applyIssueClears 执行 plan 挂起的侧端点清空（labels 先、assignees
// 后；顺序裁定要求它们都在主 update 之后到达）。
func (p *Provider) applyIssueClears(ctx context.Context, owner, repo string, n int, plan issueUpdatePlan) error {
	if plan.clearLabels {
		// A：侧端点 PUT——SDK 把 labels 数组本身作为 body（裸 JSON
		// 数组 []，GitHub 形态 wire，非 {"labels":[]} 对象）。恒传非
		// nil 空集：规范化的 [] 而非 null（patch 传 &nil 切片时
		// Resolve 返回 nil）。
		if err := p.client.ReplaceIssueLabels(ctx, owner, repo, n, []string{}); err != nil {
			return provider.Wrap(provider.PlatformGitCode, "UpdateIssue", err)
		}
	}
	if plan.clearAssignees {
		// A：侧端点 DELETE {"assignees":[]}（删光当前指派人）。
		if _, err := p.client.RemoveIssueAssignees(ctx, owner, repo, n, []string{}); err != nil {
			return provider.Wrap(provider.PlatformGitCode, "UpdateIssue", err)
		}
	}
	return nil
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
