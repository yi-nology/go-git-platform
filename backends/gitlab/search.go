package gitlab

import (
	"context"
	"strconv"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements the provider.SearchManager surface over the GitLab
// client-go SDK, via the typed SearchService scopes (projects/issues/users).
//
// Registrations (divergence ledger):
//   - SearchIssuesOptions.Repo routes to Search.IssuesByProject (the
//     project-scoped search endpoint) when set; otherwise the global
//     Search.Issues runs.
//   - GitLab's search API exposes no state, sort, or order parameters, so
//     SearchIssuesOptions.State/Sort/Order and the sort/order fields of the
//     repo/user options are not forwarded (registered ignore).
//   - The SDK returns no total for searches (nil).
//   - Wire states "opened"/"reopened" normalize to open (as elsewhere).

// SearchRepos implements provider.SearchManager.
func (p *Provider) SearchRepos(ctx context.Context, opts provider.SearchReposOptions) ([]*model.SearchRepoResult, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	projects, _, err := p.client.Search.Projects(opts.Query, &gitlab.SearchOptions{
		ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, nil, provider.Wrap(model.PlatformGitLab, "SearchRepos", err)
	}
	out := make([]*model.SearchRepoResult, 0, len(projects))
	for _, pr := range projects {
		out = append(out, &model.SearchRepoResult{
			FullName:      pr.PathWithNamespace,
			Description:   pr.Description,
			WebURL:        pr.WebURL,
			Stars:         int(pr.StarCount),
			Forks:         int(pr.ForksCount),
			DefaultBranch: pr.DefaultBranch,
			Private:       pr.Visibility != "public",
		})
	}
	return out, nil, nil
}

// SearchIssues implements provider.SearchManager.
func (p *Provider) SearchIssues(ctx context.Context, opts provider.SearchIssuesOptions) ([]*model.SearchIssueResult, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	searchOpts := &gitlab.SearchOptions{
		ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
	}
	var issues []*gitlab.Issue
	var err error
	if opts.Repo != "" {
		issues, _, err = p.client.Search.IssuesByProject(opts.Repo, opts.Query, searchOpts, gitlab.WithContext(ctx))
	} else {
		issues, _, err = p.client.Search.Issues(opts.Query, searchOpts, gitlab.WithContext(ctx))
	}
	if err != nil {
		return nil, nil, provider.Wrap(model.PlatformGitLab, "SearchIssues", err)
	}
	out := make([]*model.SearchIssueResult, 0, len(issues))
	for _, i := range issues {
		state := model.IssueStateOpen
		if i.State == "closed" {
			state = model.IssueStateClosed
		}
		out = append(out, &model.SearchIssueResult{
			Number:    strconv.FormatInt(i.IID, 10),
			Title:     i.Title,
			Body:      i.Description,
			State:     state,
			WebURL:    i.WebURL,
			Labels:    append([]string(nil), i.Labels...),
			Comments:  int(i.UserNotesCount),
			CreatedAt: timeOrZero(i.CreatedAt),
		})
	}
	return out, nil, nil
}

// SearchUsers implements provider.SearchManager.
func (p *Provider) SearchUsers(ctx context.Context, opts provider.SearchUsersOptions) ([]*model.SearchUserResult, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	users, _, err := p.client.Search.Users(opts.Query, &gitlab.SearchOptions{
		ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, nil, provider.Wrap(model.PlatformGitLab, "SearchUsers", err)
	}
	out := make([]*model.SearchUserResult, 0, len(users))
	for _, u := range users {
		out = append(out, &model.SearchUserResult{
			Login:     u.Username,
			Name:      u.Name,
			AvatarURL: u.AvatarURL,
			WebURL:    u.WebURL,
		})
	}
	return out, nil, nil
}

var _ provider.SearchManager = (*Provider)(nil)
