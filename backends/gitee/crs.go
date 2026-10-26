package gitee

import (
	"context"
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
	pr, _, err := p.client.PullRequests.Create(ctx, esc(opts.Owner), esc(opts.Repo), createOpts)
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
	pr, _, err := p.client.PullRequests.Get(ctx, esc(owner), esc(repo), n)
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
	prs, resp, err := p.client.PullRequests.List(ctx, esc(opts.Owner), esc(opts.Repo), listOpts)
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
	_, err = p.client.PullRequests.Merge(ctx, esc(owner), esc(repo), n, mergeOpts)
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
	return p.patchCR(ctx, owner, repo, number, &gitee.UpdatePullRequestOptions{
		Title: gitee.String(opts.Title),
		Body:  gitee.String(opts.Description),
	}, "UpdateCR")
}

// patchCR applies a PATCH /pulls/{number} update via the SDK.
func (p *Provider) patchCR(ctx context.Context, owner, repo, number string, opts *gitee.UpdatePullRequestOptions, op string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, op, number)
	if err != nil {
		return nil, err
	}
	pr, _, err := p.client.PullRequests.Edit(ctx, esc(owner), esc(repo), n, opts)
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
	_, _, err = p.client.PullRequests.ReplaceLabels(ctx, esc(owner), esc(repo), n, labels)
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
		list, _, err := p.client.PullRequests.ListComments(ctx, esc(owner), esc(repo), n, &gitee.PullRequestCommentListOptions{
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
	commits, _, err := p.client.PullRequests.ListCommits(ctx, esc(owner), esc(repo), n)
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
