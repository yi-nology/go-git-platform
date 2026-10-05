package gitcode

import (
	"context"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetCommit implements provider.CommitManager.
func (p *Provider) GetCommit(ctx context.Context, owner, repo, sha string) (*provider.CommitInfo, error) {
	c, err := p.client.GetCommit(ctx, owner, repo, sha)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetCommit", err)
	}
	return convertCommit(c), nil
}

// ListCommits implements provider.CommitManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListCommits(ctx context.Context, owner, repo string, opts provider.ListCommitsOptions) ([]*provider.CommitInfo, error) {
	fetch := func(page, perPage int) ([]*gitcode.Commit, error) {
		return p.client.ListCommits(ctx, owner, repo, gitcode.ListCommitsOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
			Branch:      opts.Branch,
		})
	}
	var commits []*gitcode.Commit
	commits, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitcode.Commit, error) {
			return fetch(page, perPage)
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListCommits", err)
	}
	result := make([]*provider.CommitInfo, 0, len(commits))
	for _, c := range commits {
		result = append(result, convertCommit(c))
	}
	return result, nil
}

// CompareCommits implements provider.CommitManager.
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*provider.CompareResult, error) {
	cmp, err := p.client.CompareCommits(ctx, owner, repo, base, head)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CompareCommits", err)
	}
	result := &provider.CompareResult{
		TotalCommits: cmp.TotalCommits,
		AheadBy:      cmp.AheadBy,
		BehindBy:     cmp.BehindBy,
	}
	for _, c := range cmp.Commits {
		result.Commits = append(result.Commits, convertCommit(c))
	}
	for _, f := range cmp.Files {
		result.Files = append(result.Files, &provider.ChangedFile{
			OldPath:   f.PreviousFilename,
			NewPath:   f.Filename,
			Additions: f.Additions,
			Deletions: f.Deletions,
			IsNew:     f.Status == "added",
			IsDeleted: f.Status == "removed",
			IsRenamed: f.Status == "renamed",
		})
	}
	return result, nil
}

// CreateCommitStatus implements provider.CommitStatusManager.
//
// provider.CommitStatusOptions map 1:1 onto the SDK's
// CreateCommitStatusOptions (POST /repos/{o}/{r}/statuses/{sha}). State
// passes through verbatim; GitCode expects the GitHub-shaped
// pending/success/error/failure verbs.
func (p *Provider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts provider.CommitStatusOptions) error {
	_, err := p.client.CreateCommitStatus(ctx, owner, repo, sha, gitcode.CreateCommitStatusOptions{
		State:       opts.State,
		TargetURL:   opts.TargetURL,
		Description: opts.Description,
		Context:     opts.Context,
	})
	if err != nil {
		return provider.Wrap(provider.PlatformGitCode, "CreateCommitStatus", err)
	}
	return nil
}

// ListCommitStatuses implements provider.CommitStatusManager.
func (p *Provider) ListCommitStatuses(ctx context.Context, owner, repo, sha string) ([]provider.CommitStatus, error) {
	statuses, err := backendutil.AllPages(func(page int) ([]*gitcode.CommitStatus, error) {
		return p.client.ListCommitStatuses(ctx, owner, repo, sha, gitcode.ListOptions{Page: page, PerPage: 100})
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListCommitStatuses", err)
	}
	return convertCommitStatuses(statuses), nil
}

// convertCommitStatuses maps go-gitcode CommitStatus entries onto the
// unified vocabulary. GitCode speaks the GitHub-shaped verbs
// (pending/success/error/failure) under the "state" key; they pass through
// NormalizeCommitStatusState unchanged.
func convertCommitStatuses(statuses []*gitcode.CommitStatus) []provider.CommitStatus {
	result := make([]provider.CommitStatus, 0, len(statuses))
	for _, s := range statuses {
		if s == nil {
			continue
		}
		result = append(result, provider.CommitStatus{
			State:       provider.NormalizeCommitStatusState(s.State),
			Context:     s.Context,
			Description: s.Description,
			TargetURL:   s.TargetURL,
		})
	}
	return result
}

var _ provider.CommitManager = (*Provider)(nil)

var _ provider.CommitStatusManager = (*Provider)(nil)
