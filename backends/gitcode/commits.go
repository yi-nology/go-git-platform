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
	// Since/Until 先经共享解析器校验（issue #52）——malformed 报
	// ErrInvalidInput，与其他后端同判；校验通过的值本平台无法下传
	// （SDK 的 commit-list 选项不带时间过滤），按 divergence 台账登记。
	if _, err := provider.ParseTimestamp("Since", opts.Since); err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListCommits", err)
	}
	if _, err := provider.ParseTimestamp("Until", opts.Until); err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListCommits", err)
	}
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

var _ provider.CommitManager = (*Provider)(nil)
