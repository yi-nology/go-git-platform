package github

import (
	"context"

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
	// Since/Until 解析失败必须报错（issue #52，批2 #75 LastReadAt 同型）：
	// 旧的静默丢弃（err==nil 才赋值）让一个格式笔误（如 "2026/01/01"）悄悄
	// 变成不过滤的全量历史，err 仍为 nil，增量同步无从察觉。
	since, err := provider.ParseTimestamp("Since", opts.Since)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListCommits", err)
	}
	until, err := provider.ParseTimestamp("Until", opts.Until)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListCommits", err)
	}
	baseOpts := github.CommitsListOptions{}
	if opts.Branch != "" {
		baseOpts.SHA = opts.Branch
	}
	if !since.IsZero() {
		baseOpts.Since = since
	}
	if !until.IsZero() {
		baseOpts.Until = until
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
//
// commits 走分页拉全（issue #77）：GitHub 的 compare 端点单次至多返回 250
// 条 commits，旧实现传 nil ListOptions 只取一页 ⇒ >250 提交的 compare 静默
// 截断（仅能靠 TotalCommits 对比察觉）。files 平台侧更严：改动文件列表只在
// 首页返回且全量上限 300 条、无分页线可走——len==300 时置
// CompareResult.FilesTruncated，截断可见（对照：GetCRDiff 的 PR files 线有
// 分页，走全）。
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*provider.CompareResult, error) {
	const (
		compareCommitsPerPage = 250 // GitHub compare commits cap per call
		compareMaxChangedFile = 300 // GitHub compare changed-files cap overall
		compareMaxPages       = 50  // pagination budget, mirrors backendutil.AllPages
	)
	var (
		result  *provider.CompareResult
		commits []*github.RepositoryCommit
		files   []*github.CommitFile
	)
	for page := 1; page <= compareMaxPages; page++ {
		cmp, _, err := p.client.Repositories.CompareCommits(ctx, owner, repo, base, head,
			&github.ListOptions{Page: page, PerPage: compareCommitsPerPage})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "CompareCommits", err)
		}
		if result == nil {
			// Metadatas ride every page; changed files ride the first page
			// only — snapshot both there.
			result = &provider.CompareResult{
				TotalCommits: cmp.GetTotalCommits(),
				AheadBy:      cmp.GetAheadBy(),
				BehindBy:     cmp.GetBehindBy(),
			}
			files = cmp.Files
		}
		commits = append(commits, cmp.Commits...)
		if len(cmp.Commits) < compareCommitsPerPage || (cmp.GetTotalCommits() > 0 && len(commits) >= cmp.GetTotalCommits()) {
			break
		}
	}
	result.FilesTruncated = len(files) >= compareMaxChangedFile
	for _, c := range commits {
		result.Commits = append(result.Commits, convertCommit(c))
	}
	for _, f := range files {
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
