package github

import (
	"context"

	"github.com/google/go-github/v92/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCR implements provider.ChangeRequestManager.
func (p *Provider) CreateCR(ctx context.Context, opts provider.CreateCROptions) (*provider.ChangeRequest, error) {
	newPR := &github.CreatePullRequest{
		Title: new(opts.Title),
		Body:  new(opts.Description),
		Head:  opts.SourceBranch,
		Base:  opts.TargetBranch,
	}
	pr, _, err := p.client.PullRequests.Create(ctx, opts.Owner, opts.Repo, *newPR)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateCR", err)
	}
	return convertPR(pr), nil
}

// GetCR implements provider.ChangeRequestManager.
func (p *Provider) GetCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "GetCR", number)
	if err != nil {
		return nil, err
	}
	pr, _, err := p.client.PullRequests.Get(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetCR", err)
	}
	return convertPR(pr), nil
}

// ListCRs implements provider.ChangeRequestManager.
func (p *Provider) ListCRs(ctx context.Context, opts provider.ListCROptions) ([]*provider.ChangeRequest, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{Page: page, PerPage: perPage},
	}
	if opts.State != "" {
		listOpts.State = mapCRState(opts.State)
	}
	if opts.SourceBranch != "" {
		listOpts.Head = opts.Owner + ":" + opts.SourceBranch
	}
	if opts.TargetBranch != "" {
		listOpts.Base = opts.TargetBranch
	}
	prs, resp, err := p.client.PullRequests.List(ctx, opts.Owner, opts.Repo, listOpts)
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformGitHub, "ListCRs", err)
	}
	crs := make([]*provider.ChangeRequest, 0, len(prs))
	for _, pr := range prs {
		crs = append(crs, convertPR(pr))
	}
	// GitHub exposes the total only through the Link header's last page;
	// without it the count is unknown (nil) rather than guessed from the
	// page length. LastPage*PerPage is an upper-bound approximation of the
	// platform's own metadata.
	var total *int
	if resp != nil && resp.LastPage > 0 {
		t := resp.LastPage * listOpts.PerPage
		total = &t
	}
	return crs, total, nil
}

// MergeCR implements provider.ChangeRequestManager.
func (p *Provider) MergeCR(ctx context.Context, owner, repo, number string, opts provider.MergeCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "MergeCR", number)
	if err != nil {
		return nil, err
	}
	mergeOpts := &github.PullRequestOptions{}
	if opts.Squash {
		mergeOpts.MergeMethod = "squash"
	}
	_, _, err = p.client.PullRequests.Merge(ctx, owner, repo, n, opts.MergeCommitMessage, mergeOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "MergeCR", err)
	}
	return p.GetCR(ctx, owner, repo, number)
}

// CloseCR implements provider.ChangeRequestManager.
func (p *Provider) CloseCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "CloseCR", number)
	if err != nil {
		return nil, err
	}
	pr, _, err := p.client.PullRequests.Edit(ctx, owner, repo, n, &github.PullRequest{
		State: new("closed"),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CloseCR", err)
	}
	return convertPR(pr), nil
}

// ReopenCR implements provider.ChangeRequestManager.
func (p *Provider) ReopenCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "ReopenCR", number)
	if err != nil {
		return nil, err
	}
	result, _, err := p.client.PullRequests.Edit(ctx, owner, repo, n, &github.PullRequest{
		State: new("open"),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ReopenCR", err)
	}
	return convertPR(result), nil
}

// UpdateCR implements provider.ChangeRequestManager.
func (p *Provider) UpdateCR(ctx context.Context, owner, repo, number string, opts provider.UpdateCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "UpdateCR", number)
	if err != nil {
		return nil, err
	}
	pr := &github.PullRequest{}
	if opts.Title != "" {
		pr.Title = new(opts.Title)
	}
	if opts.Description != "" {
		pr.Body = new(opts.Description)
	}
	if opts.TargetBranch != "" {
		pr.Base = &github.PullRequestBranch{Ref: new(opts.TargetBranch)}
	}
	result, _, err := p.client.PullRequests.Edit(ctx, owner, repo, n, pr)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateCR", err)
	}
	return convertPR(result), nil
}

// UpdateCRLabels implements provider.ChangeRequestManager.
func (p *Provider) UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "UpdateCRLabels", number)
	if err != nil {
		return err
	}
	_, _, err = p.client.Issues.AddLabelsToIssue(ctx, owner, repo, n, labels)
	if err != nil {
		return provider.Wrap(provider.PlatformGitHub, "UpdateCRLabels", err)
	}
	return nil
}

// ListCRComments implements provider.ChangeRequestManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListCRComments(ctx context.Context, owner, repo, number string) ([]*provider.CRComment, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "ListCRComments", number)
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
		return nil, provider.Wrap(provider.PlatformGitHub, "ListCRComments", err)
	}
	result := make([]*provider.CRComment, 0, len(comments))
	for _, c := range comments {
		cc := &provider.CRComment{
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
func (p *Provider) ListCRCommits(ctx context.Context, owner, repo, number string) ([]*provider.CRCommit, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "ListCRCommits", number)
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
		return nil, provider.Wrap(provider.PlatformGitHub, "ListCRCommits", err)
	}
	result := make([]*provider.CRCommit, 0, len(commits))
	for _, c := range commits {
		cc := &provider.CRCommit{
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
