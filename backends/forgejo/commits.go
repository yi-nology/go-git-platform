package forgejo

import (
	"context"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetCommit implements provider.CommitManager.
func (p *Provider) GetCommit(ctx context.Context, owner, repo, sha string) (*provider.CommitInfo, error) {
	c, _, err := p.client.GetSingleCommit(owner, repo, sha)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "GetCommit", err)
	}
	return convertCommit(c), nil
}

// ListCommits implements provider.CommitManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListCommits(ctx context.Context, owner, repo string, opts provider.ListCommitsOptions) ([]*provider.CommitInfo, error) {
	commits, err := backendutil.PageList(opts.Page, opts.PerPage, listPageSize,
		func(page, perPage int) ([]*forgejo.Commit, error) {
			list, _, err := p.client.ListRepoCommits(owner, repo, forgejo.ListCommitOptions{
				ListOptions: forgejo.ListOptions{Page: page, PageSize: perPage},
			})
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "ListCommits", err)
	}
	result := make([]*provider.CommitInfo, 0, len(commits))
	for _, c := range commits {
		result = append(result, convertCommit(c))
	}
	return result, nil
}

// CompareCommits implements provider.CommitManager.
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*provider.CompareResult, error) {
	cmp, _, err := p.client.CompareCommits(owner, repo, base, head)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "CompareCommits", err)
	}
	result := &provider.CompareResult{TotalCommits: cmp.TotalCommits}
	for _, c := range cmp.Commits {
		result.Commits = append(result.Commits, convertCommit(c))
	}
	return result, nil
}

// CreateCommitStatus implements provider.CommitStatusManager.
func (p *Provider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts provider.CommitStatusOptions) error {
	stateMap := map[string]forgejo.StatusState{
		"success": forgejo.StatusSuccess,
		"failed":  forgejo.StatusFailure,
		"pending": forgejo.StatusPending,
		"error":   forgejo.StatusError,
	}
	state := stateMap[opts.State]
	if state == "" {
		state = forgejo.StatusPending
	}
	_, _, err := p.client.CreateStatus(owner, repo, sha, forgejo.CreateStatusOption{
		State:       state,
		Context:     opts.Context,
		Description: opts.Description,
		TargetURL:   opts.TargetURL,
	})
	if err != nil {
		return provider.Wrap(provider.PlatformForgejo, "CreateCommitStatus", err)
	}
	return nil
}

// ListCommitStatuses implements provider.CommitStatusManager.
//
// The forgejo SDK accepts no context parameter (registered platform
// limitation), so ctx is unused here beyond signature conformance.
func (p *Provider) ListCommitStatuses(ctx context.Context, owner, repo, sha string) ([]provider.CommitStatus, error) {
	statuses, err := backendutil.AllPages(func(page int) ([]*forgejo.Status, error) {
		list, _, err := p.client.ListStatuses(owner, repo, sha, forgejo.ListStatusesOption{
			ListOptions: forgejo.ListOptions{Page: page, PageSize: 100},
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "ListCommitStatuses", err)
	}
	return convertCommitStatuses(statuses), nil
}

// convertCommitStatuses maps forgejo Status entries onto the unified
// vocabulary. On the wire the verb rides the "status" key (Status.State);
// forgejo's extra "warning" terminal-passing verb normalizes to success via
// the shared vocabulary.
func convertCommitStatuses(statuses []*forgejo.Status) []provider.CommitStatus {
	result := make([]provider.CommitStatus, 0, len(statuses))
	for _, s := range statuses {
		if s == nil {
			continue
		}
		result = append(result, provider.CommitStatus{
			State:       provider.NormalizeCommitStatusState(string(s.State)),
			Context:     s.Context,
			Description: s.Description,
			TargetURL:   s.TargetURL,
		})
	}
	return result
}

var _ provider.CommitManager = (*Provider)(nil)
var _ provider.CommitStatusManager = (*Provider)(nil)
