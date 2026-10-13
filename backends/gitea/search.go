package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// SearchRepos implements provider.SearchManager.
func (p *Provider) SearchRepos(ctx context.Context, opts provider.SearchReposOptions) ([]*provider.SearchRepoResult, *int, error) {
	return giteafamily.SearchRepos(ctx, p.family, p.ports, opts)
}

// SearchIssues implements provider.SearchManager. Repo routes to the
// family's server-side repo-scoped listing; without it the global keyword
// issue search runs restricted to real issues via the "issues" type filter.
func (p *Provider) SearchIssues(ctx context.Context, opts provider.SearchIssuesOptions) ([]*provider.SearchIssueResult, *int, error) {
	return giteafamily.SearchIssues(ctx, p.family, p.ports, opts)
}

// SearchUsers implements provider.SearchManager.
func (p *Provider) SearchUsers(ctx context.Context, opts provider.SearchUsersOptions) ([]*provider.SearchUserResult, *int, error) {
	return giteafamily.SearchUsers(ctx, p.family, p.ports, opts)
}

var _ provider.SearchManager = (*Provider)(nil)
