package gitcode

import (
	"context"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// searchPerPage normalizes a caller-supplied per-page value for GitCode's
// search endpoints (page-size ceiling 100).
func searchPerPage(perPage int) int {
	if perPage <= 0 || perPage > provider.MaxPerPage {
		return provider.MaxPerPage
	}
	return perPage
}

// SearchRepos implements provider.SearchManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) SearchRepos(ctx context.Context, opts provider.SearchReposOptions) ([]*model.SearchRepoResult, *int, error) {
	buildOpts := func(page, perPage int) gitcode.SearchRepositoriesOptions {
		return gitcode.SearchRepositoriesOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
			Query:       opts.Query,
			Sort:        opts.Sort,
			Order:       opts.Order,
		}
	}
	var results []*gitcode.SearchRepositoryResult
	if opts.Page > 0 {
		// Caller-driven pagination: serve the requested page only.
		var err error
		if results, err = p.client.SearchRepositories(ctx, buildOpts(opts.Page, searchPerPage(opts.PerPage))); err != nil {
			return nil, nil, provider.Wrap(model.PlatformGitCode, "SearchRepos", err)
		}
	} else {
		var err error
		if results, err = backendutil.AllPages(func(page int) ([]*gitcode.SearchRepositoryResult, error) {
			return p.client.SearchRepositories(ctx, buildOpts(page, provider.MaxPerPage))
		}); err != nil {
			return nil, nil, provider.Wrap(model.PlatformGitCode, "SearchRepos", err)
		}
	}
	out := make([]*model.SearchRepoResult, 0, len(results))
	for _, r := range results {
		out = append(out, &model.SearchRepoResult{
			FullName:      r.FullName,
			Description:   r.Description,
			WebURL:        r.WebURL,
			Stars:         r.StargazersCount,
			Forks:         r.ForksCount,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
		})
	}
	return out, nil, nil
}

// SearchIssues implements provider.SearchManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) SearchIssues(ctx context.Context, opts provider.SearchIssuesOptions) ([]*model.SearchIssueResult, *int, error) {
	buildOpts := func(page, perPage int) gitcode.SearchIssuesOptions {
		return gitcode.SearchIssuesOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
			Query:       opts.Query,
			Sort:        opts.Sort,
			Order:       opts.Order,
			Repo:        opts.Repo,
			State:       opts.State,
		}
	}
	var results []*gitcode.SearchIssueResult
	if opts.Page > 0 {
		// Caller-driven pagination: serve the requested page only.
		var err error
		if results, err = p.client.SearchIssues(ctx, buildOpts(opts.Page, searchPerPage(opts.PerPage))); err != nil {
			return nil, nil, provider.Wrap(model.PlatformGitCode, "SearchIssues", err)
		}
	} else {
		var err error
		if results, err = backendutil.AllPages(func(page int) ([]*gitcode.SearchIssueResult, error) {
			return p.client.SearchIssues(ctx, buildOpts(page, provider.MaxPerPage))
		}); err != nil {
			return nil, nil, provider.Wrap(model.PlatformGitCode, "SearchIssues", err)
		}
	}
	out := make([]*model.SearchIssueResult, 0, len(results))
	for _, r := range results {
		labels := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			labels = append(labels, l.Name)
		}
		out = append(out, &model.SearchIssueResult{
			Number:    r.Number,
			Title:     r.Title,
			Body:      r.Body,
			State:     model.IssueState(r.State),
			WebURL:    r.HTMLURL,
			Labels:    labels,
			Comments:  r.Comments,
			CreatedAt: r.CreatedAt,
		})
	}
	return out, nil, nil
}

// SearchUsers implements provider.SearchManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) SearchUsers(ctx context.Context, opts provider.SearchUsersOptions) ([]*model.SearchUserResult, *int, error) {
	buildOpts := func(page, perPage int) gitcode.SearchUsersOptions {
		return gitcode.SearchUsersOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
			Query:       opts.Query,
			Sort:        opts.Sort,
			Order:       opts.Order,
		}
	}
	var results []*gitcode.SearchUserResult
	if opts.Page > 0 {
		// Caller-driven pagination: serve the requested page only.
		var err error
		if results, err = p.client.SearchUsers(ctx, buildOpts(opts.Page, searchPerPage(opts.PerPage))); err != nil {
			return nil, nil, provider.Wrap(model.PlatformGitCode, "SearchUsers", err)
		}
	} else {
		var err error
		if results, err = backendutil.AllPages(func(page int) ([]*gitcode.SearchUserResult, error) {
			return p.client.SearchUsers(ctx, buildOpts(page, provider.MaxPerPage))
		}); err != nil {
			return nil, nil, provider.Wrap(model.PlatformGitCode, "SearchUsers", err)
		}
	}
	out := make([]*model.SearchUserResult, 0, len(results))
	for _, r := range results {
		out = append(out, &model.SearchUserResult{
			Login:     r.Login,
			Name:      r.Name,
			AvatarURL: r.AvatarURL,
			WebURL:    r.HTMLURL,
		})
	}
	return out, nil, nil
}

var _ provider.SearchManager = (*Provider)(nil)
