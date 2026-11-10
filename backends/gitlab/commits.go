package gitlab

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetCommit implements provider.CommitManager.
func (p *Provider) GetCommit(ctx context.Context, owner, repo, sha string) (*provider.CommitInfo, error) {
	c, _, err := p.client.Commits.GetCommit(pidOf(owner, repo), sha, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "GetCommit", err)
	}
	return convertCommit(c), nil
}

// ListCommits implements provider.CommitManager.
func (p *Provider) ListCommits(ctx context.Context, owner, repo string, opts provider.ListCommitsOptions) ([]*provider.CommitInfo, error) {
	// Since/Until 解析失败必须报错（issue #52，批2 #75 LastReadAt 同型）：
	// 静默丢弃会把格式笔误升级成不过滤的全量历史。
	since, err := provider.ParseTimestamp("Since", opts.Since)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListCommits", err)
	}
	until, err := provider.ParseTimestamp("Until", opts.Until)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListCommits", err)
	}
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &gitlab.ListCommitsOptions{
		ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
	}
	if opts.Branch != "" {
		listOpts.RefName = new(opts.Branch)
	}
	if !since.IsZero() {
		listOpts.Since = new(since)
	}
	if !until.IsZero() {
		listOpts.Until = new(until)
	}
	commits, _, err := p.client.Commits.ListCommits(pidOf(owner, repo), listOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListCommits", err)
	}
	result := make([]*provider.CommitInfo, 0, len(commits))
	for _, c := range commits {
		result = append(result, convertCommit(c))
	}
	return result, nil
}

// CompareCommits implements provider.CommitManager.
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*provider.CompareResult, error) {
	cmp, _, err := p.client.Repositories.Compare(pidOf(owner, repo),
		&gitlab.CompareOptions{From: new(base), To: new(head)},
		gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "CompareCommits", err)
	}
	result := &provider.CompareResult{}
	for _, c := range cmp.Commits {
		result.Commits = append(result.Commits, convertCommit(c))
		result.TotalCommits++
	}
	for _, d := range cmp.Diffs {
		add, del := provider.CountDiffLines(d.Diff)
		result.Files = append(result.Files, &provider.ChangedFile{
			OldPath: d.OldPath, NewPath: d.NewPath, Diff: d.Diff,
			Additions: add, Deletions: del,
			IsNew: d.NewFile, IsDeleted: d.DeletedFile, IsRenamed: d.RenamedFile,
		})
	}
	return result, nil
}

// CreateCommitStatus implements provider.CommitStatusManager.
func (p *Provider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts provider.CommitStatusOptions) error {
	statusOpts := &gitlab.SetCommitStatusOptions{
		State:       mapCommitState(opts.State),
		Context:     new(opts.Context),
		Description: new(opts.Description),
		TargetURL:   new(opts.TargetURL),
	}
	_, _, err := p.client.Commits.SetCommitStatus(pidOf(owner, repo), sha, statusOpts, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(provider.PlatformGitLab, "CreateCommitStatus", err)
	}
	return nil
}

// ListCommitStatuses implements provider.CommitStatusManager.
func (p *Provider) ListCommitStatuses(ctx context.Context, owner, repo, sha string) ([]provider.CommitStatus, error) {
	statuses, err := backendutil.AllPages(func(page int) ([]*gitlab.CommitStatus, error) {
		list, _, err := p.client.Commits.GetCommitStatuses(pidOf(owner, repo), sha,
			&gitlab.GetCommitStatusesOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}},
			gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListCommitStatuses", err)
	}
	return convertCommitStatuses(statuses), nil
}

// convertCommitStatuses maps GitLab commit statuses onto the unified
// vocabulary. On the wire the state verb travels under the "status" key
// (client-go's CommitStatus.Status), while "name" is the per-check context;
// GitLab verbs (pending/running/success/failed/canceled) normalize through
// the shared vocabulary.
func convertCommitStatuses(statuses []*gitlab.CommitStatus) []provider.CommitStatus {
	result := make([]provider.CommitStatus, 0, len(statuses))
	for _, s := range statuses {
		if s == nil {
			continue
		}
		result = append(result, provider.CommitStatus{
			State:       provider.NormalizeCommitStatusState(s.Status),
			Context:     s.Name,
			Description: s.Description,
			TargetURL:   s.TargetURL,
		})
	}
	return result
}

var _ provider.CommitManager = (*Provider)(nil)
var _ provider.CommitStatusManager = (*Provider)(nil)
