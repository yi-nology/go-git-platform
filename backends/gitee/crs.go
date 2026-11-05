package gitee

import (
	"context"
	"strconv"
	"strings"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCR implements provider.ChangeRequestManager.
func (p *Provider) CreateCR(ctx context.Context, opts provider.CreateCROptions) (*provider.ChangeRequest, error) {
	createOpts := &gitee.CreatePullRequestOptions{
		Title:             gitee.String(opts.Title),
		Head:              gitee.String(opts.SourceBranch),
		Base:              gitee.String(opts.TargetBranch),
		Body:              gitee.String(opts.Description),
		Labels:            gitee.String(strings.Join(opts.Labels, ",")),
		PruneSourceBranch: gitee.Bool(opts.RemoveSourceBranch),
	}
	pr, _, err := p.client.PullRequests.Create(ctx, backendutil.Esc(opts.Owner), backendutil.Esc(opts.Repo), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateCR", err)
	}
	return convertPullRequest(pr), nil
}

// GetCR implements provider.ChangeRequestManager.
func (p *Provider) GetCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, "GetCR", number)
	if err != nil {
		return nil, err
	}
	pr, _, err := p.client.PullRequests.Get(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetCR", err)
	}
	return convertPullRequest(pr), nil
}

// ListCRs implements provider.ChangeRequestManager.
func (p *Provider) ListCRs(ctx context.Context, opts provider.ListCROptions) ([]*provider.ChangeRequest, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &gitee.PullRequestListOptions{
		State:   gitee.String(mapCRStateForGitee(opts.State)),
		Page:    gitee.Int(page),
		PerPage: gitee.Int(perPage),
	}
	if opts.SourceBranch != "" {
		listOpts.Head = gitee.String(opts.SourceBranch)
	}
	if opts.TargetBranch != "" {
		listOpts.Base = gitee.String(opts.TargetBranch)
	}
	prs, resp, err := p.client.PullRequests.List(ctx, backendutil.Esc(opts.Owner), backendutil.Esc(opts.Repo), listOpts)
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformGitee, "ListCRs", err)
	}
	result := make([]*provider.ChangeRequest, 0, len(prs))
	for _, pr := range prs {
		result = append(result, convertPullRequest(pr))
	}
	// X-Total-Count is authoritative when present; without it the count is
	// unknown (nil) rather than the page length.
	var total *int
	if t := provider.ParseTotalCountHeader(resp.Header, 0); t > 0 {
		total = &t
	}
	return result, total, nil
}

// MergeCR implements provider.ChangeRequestManager.
func (p *Provider) MergeCR(ctx context.Context, owner, repo, number string, opts provider.MergeCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, "MergeCR", number)
	if err != nil {
		return nil, err
	}
	mergeOpts := &gitee.MergePullRequestOptions{
		PruneSourceBranch: gitee.Bool(opts.RemoveSourceBranch),
		Description:       gitee.String(opts.MergeCommitMessage),
	}
	if opts.Squash {
		mergeOpts.MergeMethod = gitee.String("squash")
	}
	_, err = p.client.PullRequests.Merge(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n, mergeOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "MergeCR", err)
	}
	return p.GetCR(ctx, owner, repo, number)
}

// CloseCR implements provider.ChangeRequestManager.
func (p *Provider) CloseCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	return p.patchCR(ctx, owner, repo, number, &gitee.UpdatePullRequestOptions{
		State: gitee.String("closed"),
	}, "CloseCR")
}

// ReopenCR implements provider.ChangeRequestManager.
func (p *Provider) ReopenCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	return p.patchCR(ctx, owner, repo, number, &gitee.UpdatePullRequestOptions{
		State: gitee.String("open"),
	}, "ReopenCR")
}

// UpdateCR implements provider.ChangeRequestManager.
func (p *Provider) UpdateCR(ctx context.Context, owner, repo, number string, opts provider.UpdateCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, "UpdateCR", number)
	if err != nil {
		return nil, err
	}
	// Gitee 的 assignee 线路是独立的 add/remove 端点（PATCH pull 不带
	// assignee 字段）——全量替换走读改写收敛差集。nil/空 = 不变（len>0
	// 才落线，见 UpdateCROptions）。并发编辑下有丢更新窗口（mapping 分歧
	// 已登记）。
	if len(opts.Assignees) > 0 {
		if err := p.setCRAssignees(ctx, owner, repo, n, opts.Assignees); err != nil {
			return nil, err
		}
	}
	return p.patchCR(ctx, owner, repo, number, &gitee.UpdatePullRequestOptions{
		Title: gitee.String(opts.Title),
		Body:  gitee.String(opts.Description),
	}, "UpdateCR")
}

// setCRAssignees converges the CR's assignee set to target via the platform's
// add/remove endpoints (each call comma-joins its batch).
func (p *Provider) setCRAssignees(ctx context.Context, owner, repo string, n int, target []string) error {
	cr, err := p.GetCR(ctx, owner, repo, strconv.Itoa(n))
	if err != nil {
		return err
	}
	toAdd, toRemove := assigneeDiff(cr.Assignees, target)
	if len(toAdd) > 0 {
		if _, _, err := p.client.PullRequests.AddAssignees(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n, toAdd); err != nil {
			return provider.Wrap(provider.PlatformGitee, "UpdateCR", err)
		}
	}
	if len(toRemove) > 0 {
		if _, _, err := p.client.PullRequests.RemoveAssignees(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n, toRemove); err != nil {
			return provider.Wrap(provider.PlatformGitee, "UpdateCR", err)
		}
	}
	return nil
}

// assigneeDiff computes the add/remove batches that converge the current
// assignee list to target (login matching is case-insensitive).
func assigneeDiff(current []*provider.CRUser, target []string) (toAdd, toRemove []string) {
	currentSet := make(map[string]struct{}, len(current))
	for _, a := range current {
		if a != nil && a.Username != "" {
			currentSet[strings.ToLower(a.Username)] = struct{}{}
		}
	}
	targetSet := make(map[string]struct{}, len(target))
	for _, t := range target {
		targetSet[strings.ToLower(t)] = struct{}{}
	}
	for _, t := range target {
		if _, ok := currentSet[strings.ToLower(t)]; !ok {
			toAdd = append(toAdd, t)
		}
	}
	for _, a := range current {
		if a != nil && a.Username != "" {
			if _, ok := targetSet[strings.ToLower(a.Username)]; !ok {
				toRemove = append(toRemove, a.Username)
			}
		}
	}
	return toAdd, toRemove
}

// patchCR applies a PATCH /pulls/{number} update via the SDK.
func (p *Provider) patchCR(ctx context.Context, owner, repo, number string, opts *gitee.UpdatePullRequestOptions, op string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, op, number)
	if err != nil {
		return nil, err
	}
	pr, _, err := p.client.PullRequests.Edit(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n, opts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, op, err)
	}
	return convertPullRequest(pr), nil
}

// UpdateCRLabels implements provider.ChangeRequestManager.
func (p *Provider) UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, "UpdateCRLabels", number)
	if err != nil {
		return err
	}
	_, _, err = p.client.PullRequests.ReplaceLabels(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n, labels)
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "UpdateCRLabels", err)
	}
	return nil
}

// ListCRComments implements provider.ChangeRequestManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (Gitee's per_page caps at 100; fetch until an empty
// page).
func (p *Provider) ListCRComments(ctx context.Context, owner, repo, number string) ([]*provider.CRComment, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, "ListCRComments", number)
	if err != nil {
		return nil, err
	}
	comments, err := backendutil.AllPages(func(page int) ([]*gitee.PullRequestComments, error) {
		list, _, err := p.client.PullRequests.ListComments(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n, &gitee.PullRequestCommentListOptions{
			Page:    gitee.Int(page),
			PerPage: gitee.Int(100),
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListCRComments", err)
	}
	result := make([]*provider.CRComment, 0, len(comments))
	for _, c := range comments {
		result = append(result, convertPRComment(c))
	}
	return result, nil
}

// ListCRCommits implements provider.ChangeRequestManager. The go-gitee
// SDK's PullRequests.ListCommits takes no options at all, so there is no
// pagination parameter to exhaust — the platform returns what it returns
// in a single response.
func (p *Provider) ListCRCommits(ctx context.Context, owner, repo, number string) ([]*provider.CRCommit, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, "ListCRCommits", number)
	if err != nil {
		return nil, err
	}
	commits, _, err := p.client.PullRequests.ListCommits(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListCRCommits", err)
	}
	result := make([]*provider.CRCommit, 0, len(commits))
	for _, c := range commits {
		result = append(result, convertPRCommit(c))
	}
	return result, nil
}

var _ provider.ChangeRequestManager = (*Provider)(nil)

// mapCRStateForGitee maps the SDK CRState to gitee's pull-list vocabulary.
//
// **未指定状态(零值)⇒ 不过滤**(平台上不加 state = 全部)。
// 以前落到 "open" ⇒ closed/merged 的 CR **静默丢失**。
func mapCRStateForGitee(s provider.CRState) string {
	switch s {
	case provider.CRStateClosed:
		return "closed"
	case provider.CRStateMerged:
		return "merged"
	case provider.CRStateOpened:
		return "open"
	default:
		return ""
	}
}
