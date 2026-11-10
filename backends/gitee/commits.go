package gitee

import (
	"context"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetCommit implements provider.CommitManager.
func (p *Provider) GetCommit(ctx context.Context, owner, repo, sha string) (*provider.CommitInfo, error) {
	commit, _, err := p.client.Repositories.GetCommit(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(sha))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetCommit", err)
	}
	return convertRepoCommitWithFiles(commit), nil
}

// ListCommits implements provider.CommitManager.
//
// Dual pagination mode: opts.Page > 0 means the caller manages paging
// explicitly and receives exactly that one page (opts.Page/opts.PerPage
// honored via NormalizePageOpts); opts.Page == 0 (the default) exhausts
// pagination via backendutil.AllPages — fetching 100 per page until an
// empty page — so every matching commit is returned. In full-fetch mode
// opts.PerPage only sizes the underlying requests and defaults to Gitee's
// per_page maximum of 100.
func (p *Provider) ListCommits(ctx context.Context, owner, repo string, opts provider.ListCommitsOptions) ([]*provider.CommitInfo, error) {
	// 裸串透传平台前先经共享解析器校验（issue #52）：时间戳选项的接受/
	// 拒绝面必须全后端一致——RFC3339 之外的值在这里报 ErrInvalidInput，
	// 与解析型后端同判，而不是把「平台自己报什么」交给 wire。
	if _, err := provider.ParseTimestamp("Since", opts.Since); err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListCommits", err)
	}
	if _, err := provider.ParseTimestamp("Until", opts.Until); err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListCommits", err)
	}
	baseOpts := gitee.CommitListOptions{}
	if opts.Branch != "" {
		baseOpts.SHA = gitee.String(opts.Branch)
	}
	if opts.Since != "" {
		baseOpts.Since = gitee.String(opts.Since)
	}
	if opts.Until != "" {
		baseOpts.Until = gitee.String(opts.Until)
	}

	var commits []*gitee.RepoCommit
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		baseOpts.Page = gitee.Int(page)
		baseOpts.PerPage = gitee.Int(perPage)
		list, _, err := p.client.Repositories.ListCommits(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &baseOpts)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitee, "ListCommits", err)
		}
		commits = list
	} else {
		var err error
		commits, err = backendutil.AllPages(func(page int) ([]*gitee.RepoCommit, error) {
			o := baseOpts
			o.Page = gitee.Int(page)
			o.PerPage = gitee.Int(100)
			list, _, err := p.client.Repositories.ListCommits(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &o)
			return list, err
		})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitee, "ListCommits", err)
		}
	}
	result := make([]*provider.CommitInfo, 0, len(commits))
	for _, c := range commits {
		result = append(result, convertRepoCommit(c))
	}
	return result, nil
}

// CompareCommits implements provider.CommitManager.
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*provider.CompareResult, error) {
	cmp, _, err := p.client.Repositories.CompareCommits(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(base), backendutil.Esc(head), nil)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CompareCommits", err)
	}
	result := &provider.CompareResult{}
	if cmp.Commits != nil {
		result.TotalCommits = len(*cmp.Commits)
		for _, c := range *cmp.Commits {
			result.Commits = append(result.Commits, convertRepoCommit(c))
		}
	}
	if cmp.Files != nil {
		for _, f := range *cmp.Files {
			result.Files = append(result.Files, convertDiffFile(f))
		}
	}
	return result, nil
}

var _ provider.CommitManager = (*Provider)(nil)
