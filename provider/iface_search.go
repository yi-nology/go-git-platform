package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// SearchManager provides cross-platform search for repositories, issues, and users.
//
// The *int return is the server-side total when the platform reports one,
// and nil when it does not — callers must not treat a total as guaranteed.
type SearchManager interface {
	SearchRepos(ctx context.Context, opts SearchReposOptions) ([]*model.SearchRepoResult, *int, error)
	SearchIssues(ctx context.Context, opts SearchIssuesOptions) ([]*model.SearchIssueResult, *int, error)
	SearchUsers(ctx context.Context, opts SearchUsersOptions) ([]*model.SearchUserResult, *int, error)
}
