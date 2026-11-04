package github

import (
	"context"
	"time"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetCommit implements provider.CommitManager.
func (p *Provider) GetCommit(ctx context.Context, owner, repo, sha string) (*provider.CommitInfo, error) {
	c, _, err := p.client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetCommit", err)
	}
	return convertCommit(c), nil
}

// ListCommits implements provider.CommitManager.
//
// Dual pagination mode: opts.Page > 0 means the caller manages paging
// explicitly and receives exactly that one page (opts.Page/opts.PerPage
// honored via NormalizePageOpts); opts.Page == 0 (the default) exhausts
// pagination via backendutil.AllPages — fetching 100 per page until an
// empty page — so every matching commit is returned. In full-fetch mode
// opts.PerPage only sizes the underlying requests and defaults to the
// platform maximum of 100.
func (p *Provider) ListCommits(ctx context.Context, owner, repo string, opts provider.ListCommitsOptions) ([]*provider.CommitInfo, error) {
	baseOpts := github.CommitsListOptions{}
	if opts.Branch != "" {
		baseOpts.SHA = opts.Branch
	}
	if opts.Since != "" {
		if t, err := time.Parse(time.RFC3339, opts.Since); err == nil {
			baseOpts.Since = t
		}
	}
	if opts.Until != "" {
		if t, err := time.Parse(time.RFC3339, opts.Until); err == nil {
			baseOpts.Until = t
		}
	}

	var commits []*github.RepositoryCommit
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		baseOpts.ListOptions = github.ListOptions{Page: page, PerPage: perPage}
		list, _, err := p.client.Repositories.ListCommits(ctx, owner, repo, &baseOpts)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "ListCommits", err)
		}
		commits = list
	} else {
		var err error
		commits, err = backendutil.AllPages(func(page int) ([]*github.RepositoryCommit, error) {
			o := baseOpts
			o.ListOptions = github.ListOptions{Page: page, PerPage: 100}
			list, _, err := p.client.Repositories.ListCommits(ctx, owner, repo, &o)
			return list, err
		})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "ListCommits", err)
		}
	}
	result := make([]*provider.CommitInfo, 0, len(commits))
	for _, c := range commits {
		result = append(result, convertCommit(c))
	}
	return result, nil
}

// CompareCommits implements provider.CommitManager.
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*provider.CompareResult, error) {
	cmp, _, err := p.client.Repositories.CompareCommits(ctx, owner, repo, base, head, nil)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CompareCommits", err)
	}
	result := &provider.CompareResult{
		TotalCommits: cmp.GetTotalCommits(),
		AheadBy:      cmp.GetAheadBy(),
		BehindBy:     cmp.GetBehindBy(),
	}
	for _, c := range cmp.Commits {
		result.Commits = append(result.Commits, convertCommit(c))
	}
	for _, f := range cmp.Files {
		result.Files = append(result.Files, &provider.ChangedFile{
			OldPath:   f.GetPreviousFilename(),
			NewPath:   f.GetFilename(),
			Additions: f.GetAdditions(),
			Deletions: f.GetDeletions(),
			IsNew:     f.GetStatus() == "added",
			IsDeleted: f.GetStatus() == "removed",
			IsRenamed: f.GetStatus() == "renamed",
		})
	}
	return result, nil
}

// CreateCommitStatus implements provider.CommitStatusManager.
func (p *Provider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts provider.CommitStatusOptions) error {
	_, _, err := p.client.Repositories.CreateStatus(ctx, owner, repo, sha, github.RepoStatus{
		State:       new(opts.State),
		Context:     new(opts.Context),
		Description: new(opts.Description),
		TargetURL:   new(opts.TargetURL),
	})
	if err != nil {
		return provider.Wrap(provider.PlatformGitHub, "CreateCommitStatus", err)
	}
	return nil
}

// ListCommitStatuses implements provider.CommitStatusManager.
func (p *Provider) ListCommitStatuses(ctx context.Context, owner, repo, sha string) ([]provider.CommitStatus, error) {
	statuses, err := backendutil.AllPages(func(page int) ([]*github.RepoStatus, error) {
		list, _, err := p.client.Repositories.ListStatuses(ctx, owner, repo, sha,
			&github.ListOptions{Page: page, PerPage: 100})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListCommitStatuses", err)
	}
	return convertCommitStatuses(statuses), nil
}

// convertCommitStatuses maps go-github RepoStatus entries onto the unified
// vocabulary. GitHub already speaks the canonical verbs (error/failure/
// pending/success); they pass through NormalizeCommitStatusState unchanged.
func convertCommitStatuses(statuses []*github.RepoStatus) []provider.CommitStatus {
	result := make([]provider.CommitStatus, 0, len(statuses))
	for _, s := range statuses {
		if s == nil {
			continue
		}
		result = append(result, provider.CommitStatus{
			State:       provider.NormalizeCommitStatusState(s.GetState()),
			Context:     s.GetContext(),
			Description: s.GetDescription(),
			TargetURL:   s.GetTargetURL(),
		})
	}
	return result
}

// compile-time guard
var _ provider.CommitManager = (*Provider)(nil)
var _ provider.CommitStatusManager = (*Provider)(nil)
