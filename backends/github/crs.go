package github

import (
	"context"
	"strings"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCR implements provider.ChangeRequestManager.
func (p *Provider) CreateCR(ctx context.Context, opts provider.CreateCROptions) (*model.ChangeRequest, error) {
	newPR := &github.CreatePullRequest{
		Title: new(opts.Title),
		Body:  new(opts.Description),
		Head:  opts.SourceBranch,
		Base:  opts.TargetBranch,
	}
	// Draft=true 才下发（#160）：false 保持 wire 逐字不变。
	if opts.Draft {
		newPR.Draft = new(opts.Draft)
	}
	pr, _, err := p.client.PullRequests.Create(ctx, opts.Owner, opts.Repo, *newPR)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "CreateCR", err)
	}
	return convertPR(pr), nil
}

// GetCR implements provider.ChangeRequestManager.
func (p *Provider) GetCR(ctx context.Context, owner, repo, number string) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "GetCR", number)
	if err != nil {
		return nil, err
	}
	pr, _, err := p.client.PullRequests.Get(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "GetCR", err)
	}
	return convertPR(pr), nil
}

// ListCRs implements provider.ChangeRequestManager.
//
// SourceBranch 口径(issue #78):GitHub 的 head 过滤参数是
// 「命名空间:分支名」——硬编码 opts.Owner:branch 会把 fork 来源的 PR
// 静默漏掉(fork PR 的 head 在 fork owner 的命名空间下)。契约按两个形态分流:
//
//   - 带 owner("owner:branch")→ 服务端 head 精确过滤,命名空间锁定;
//   - 纯分支名("feature-x")→ 不下发 head 过滤,当前页内按统一
//     SourceBranch(head.ref,与命名空间无关)客户端后过滤——fork 同名
//     分支同样命中,与 GitLab 的服务端纯分支名过滤同果(后过滤与
//     GitLab 服务端过滤的分页语义差异登记 divergence)。
func (p *Provider) ListCRs(ctx context.Context, opts provider.ListCROptions) ([]*model.ChangeRequest, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{Page: page, PerPage: perPage},
	}
	if opts.State != "" {
		listOpts.State = mapCRState(opts.State)
	}
	// 纯分支名形态走客户端后过滤,head 过滤不下发。
	clientSideBranchFilter := ""
	if opts.SourceBranch != "" {
		if owner, branch, found := strings.Cut(opts.SourceBranch, ":"); found && owner != "" && branch != "" {
			listOpts.Head = opts.SourceBranch
		} else {
			clientSideBranchFilter = opts.SourceBranch
		}
	}
	if opts.TargetBranch != "" {
		listOpts.Base = opts.TargetBranch
	}
	prs, resp, err := p.client.PullRequests.List(ctx, opts.Owner, opts.Repo, listOpts)
	if err != nil {
		return nil, nil, provider.Wrap(model.PlatformGitHub, "ListCRs", err)
	}
	crs := make([]*model.ChangeRequest, 0, len(prs))
	for _, pr := range prs {
		cr := convertPR(pr)
		if clientSideBranchFilter != "" && cr.SourceBranch != clientSideBranchFilter {
			continue
		}
		crs = append(crs, cr)
	}
	// GitHub exposes the total only through the Link header's last page;
	// without it the count is unknown (nil) rather than guessed from the
	// page length. LastPage*PerPage is an upper-bound approximation of the
	// platform's own metadata. With the client-side SourceBranch post-filter
	// the bound describes the unfiltered set (see the divergence note above).
	var total *int
	if resp != nil && resp.LastPage > 0 {
		t := resp.LastPage * listOpts.PerPage
		total = &t
	}
	return crs, total, nil
}

// MergeCR implements provider.ChangeRequestManager.
func (p *Provider) MergeCR(ctx context.Context, owner, repo, number string, opts provider.MergeCROptions) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "MergeCR", number)
	if err != nil {
		return nil, err
	}
	mergeOpts := &github.PullRequestOptions{}
	if opts.Squash {
		mergeOpts.MergeMethod = "squash"
	}
	_, _, err = p.client.PullRequests.Merge(ctx, owner, repo, n, opts.MergeCommitMessage, mergeOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "MergeCR", err)
	}
	return p.GetCR(ctx, owner, repo, number)
}

// CloseCR implements provider.ChangeRequestManager.
func (p *Provider) CloseCR(ctx context.Context, owner, repo, number string) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "CloseCR", number)
	if err != nil {
		return nil, err
	}
	pr, _, err := p.client.PullRequests.Edit(ctx, owner, repo, n, &github.PullRequest{
		State: new("closed"),
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "CloseCR", err)
	}
	return convertPR(pr), nil
}

// ReopenCR implements provider.ChangeRequestManager.
func (p *Provider) ReopenCR(ctx context.Context, owner, repo, number string) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "ReopenCR", number)
	if err != nil {
		return nil, err
	}
	result, _, err := p.client.PullRequests.Edit(ctx, owner, repo, n, &github.PullRequest{
		State: new("open"),
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "ReopenCR", err)
	}
	return convertPR(result), nil
}

// UpdateCR implements provider.ChangeRequestManager.
func (p *Provider) UpdateCR(ctx context.Context, owner, repo, number string, opts provider.UpdateCROptions) (*model.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "UpdateCR", number)
	if err != nil {
		return nil, err
	}
	pr := &github.PullRequest{}
	if opts.Title != "" {
		pr.Title = new(opts.Title)
	}
	// 三态收编（2A）：ResolveDescription 非 specified 不动；specified
	// 空值即清空。
	if desc, ok := opts.ResolveDescription(); ok {
		pr.Body = new(desc) // 清空 ⇒ new("") ⇒ "body":""
	}
	if opts.TargetBranch != "" {
		pr.Base = &github.PullRequestBranch{Ref: new(opts.TargetBranch)}
	}
	// PR 编号即 issue 编号：assignees 走 issue 线（PullRequests.Edit 的 wire
	// 不带 assignee 字段）——三态同线：清空时 []string{} 经 omitzero 发
	// "assignees":[]。
	if as, ok := opts.ResolveAssignees(); ok {
		issue := github.UpdateIssueRequest{Assignees: as}
		if _, _, err := p.client.Issues.Update(ctx, owner, repo, n, issue); err != nil {
			return nil, provider.Wrap(model.PlatformGitHub, "UpdateCR", err)
		}
	}
	result, _, err := p.client.PullRequests.Edit(ctx, owner, repo, n, pr)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "UpdateCR", err)
	}
	return convertPR(result), nil
}

// UpdateCRLabels implements provider.ChangeRequestManager.
func (p *Provider) UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "UpdateCRLabels", number)
	if err != nil {
		return err
	}
	_, _, err = p.client.Issues.AddLabelsToIssue(ctx, owner, repo, n, labels)
	if err != nil {
		return provider.Wrap(model.PlatformGitHub, "UpdateCRLabels", err)
	}
	return nil
}

// ListCRComments implements provider.ChangeRequestManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListCRComments(ctx context.Context, owner, repo, number string) ([]*model.CRComment, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "ListCRComments", number)
	if err != nil {
		return nil, err
	}
	comments, err := backendutil.AllPages(func(page int) ([]*github.PullRequestComment, error) {
		list, _, err := p.client.PullRequests.ListComments(ctx, owner, repo, n, &github.PullRequestListCommentsOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: 100},
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "ListCRComments", err)
	}
	result := make([]*model.CRComment, 0, len(comments))
	for _, c := range comments {
		cc := &model.CRComment{
			ID:        c.GetID(),
			Body:      c.GetBody(),
			CreatedAt: tsOrZero(c.GetCreatedAt()),
			UpdatedAt: tsOrZero(c.GetUpdatedAt()),
		}
		if c.GetUser() != nil {
			cc.Author = convertUser(c.GetUser())
		}
		result = append(result, cc)
	}
	return result, nil
}

// ListCRCommits implements provider.ChangeRequestManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListCRCommits(ctx context.Context, owner, repo, number string) ([]*model.CRCommit, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "ListCRCommits", number)
	if err != nil {
		return nil, err
	}
	commits, err := backendutil.AllPages(func(page int) ([]*github.RepositoryCommit, error) {
		list, _, err := p.client.PullRequests.ListCommits(ctx, owner, repo, n, &github.ListOptions{
			Page: page, PerPage: 100,
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "ListCRCommits", err)
	}
	result := make([]*model.CRCommit, 0, len(commits))
	for _, c := range commits {
		cc := &model.CRCommit{
			SHA:       c.GetSHA(),
			Message:   c.GetCommit().GetMessage(),
			CreatedAt: c.GetCommit().GetAuthor().GetDate().Time,
		}
		if c.GetAuthor() != nil {
			cc.Author = convertUser(c.GetAuthor())
		}
		result = append(result, cc)
	}
	return result, nil
}

// compile-time guard
var _ provider.ChangeRequestManager = (*Provider)(nil)
