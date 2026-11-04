package github

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements the provider.SearchManager surface over go-github.
//
// Registrations (divergence ledger):
//   - GitHub's search API is global: SearchReposOptions carries no repo
//     scoping, so the query travels as-is. SearchIssuesOptions.Repo becomes
//     a "repo:owner/name" qualifier, State a "state:" qualifier.
//   - GitHub's issue-search endpoint matches issues and pull requests alike,
//     so an "is:issue" qualifier is appended to SearchIssues to honor the
//     method's contract (real issues only).
//   - The total is GitHub's reported total_count when present and positive;
//     nil when absent (GitHub sometimes omits it).

// SearchRepos implements provider.SearchManager.
func (p *Provider) SearchRepos(ctx context.Context, opts provider.SearchReposOptions) ([]*provider.SearchRepoResult, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	result, _, err := p.client.Search.Repositories(ctx, opts.Query, &github.SearchOptions{
		Sort:        opts.Sort,
		Order:       opts.Order,
		ListOptions: github.ListOptions{Page: page, PerPage: perPage},
	})
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformGitHub, "SearchRepos", err)
	}
	out := make([]*provider.SearchRepoResult, 0, len(result.Repositories))
	for _, r := range result.Repositories {
		out = append(out, &provider.SearchRepoResult{
			FullName:      r.GetFullName(),
			Description:   r.GetDescription(),
			WebURL:        r.GetHTMLURL(),
			Stars:         r.GetStargazersCount(),
			Forks:         r.GetForksCount(),
			DefaultBranch: r.GetDefaultBranch(),
			Private:       r.GetPrivate(),
		})
	}
	if total := result.GetTotal(); total > 0 {
		return out, &total, nil
	}
	return out, nil, nil
}

// SearchIssues implements provider.SearchManager. Repo/State map onto query
// qualifiers; "is:issue" keeps pull requests out of the results.
func (p *Provider) SearchIssues(ctx context.Context, opts provider.SearchIssuesOptions) ([]*provider.SearchIssueResult, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	var qualifiers []string
	if opts.Repo != "" {
		qualifiers = append(qualifiers, "repo:"+opts.Repo)
	}
	if opts.State != "" {
		qualifiers = append(qualifiers, "state:"+opts.State)
	}
	qualifiers = append(qualifiers, "is:issue")
	query := strings.Join(qualifiers, " ") + " " + opts.Query
	result, _, err := p.client.Search.Issues(ctx, query, &github.SearchOptions{
		Sort:        opts.Sort,
		Order:       opts.Order,
		ListOptions: github.ListOptions{Page: page, PerPage: perPage},
	})
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformGitHub, "SearchIssues", err)
	}
	out := make([]*provider.SearchIssueResult, 0, len(result.Issues))
	for _, i := range result.Issues {
		labels := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			labels = append(labels, l.GetName())
		}
		out = append(out, &provider.SearchIssueResult{
			Number:    strconv.Itoa(i.GetNumber()),
			Title:     i.GetTitle(),
			Body:      i.GetBody(),
			State:     provider.IssueState(i.GetState()),
			WebURL:    i.GetHTMLURL(),
			Labels:    labels,
			Comments:  i.GetComments(),
			CreatedAt: tsOrZero(i.GetCreatedAt()),
		})
	}
	if total := result.GetTotal(); total > 0 {
		return out, &total, nil
	}
	return out, nil, nil
}

// SearchUsers implements provider.SearchManager.
func (p *Provider) SearchUsers(ctx context.Context, opts provider.SearchUsersOptions) ([]*provider.SearchUserResult, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	result, _, err := p.client.Search.Users(ctx, opts.Query, &github.SearchOptions{
		Sort:        opts.Sort,
		Order:       opts.Order,
		ListOptions: github.ListOptions{Page: page, PerPage: perPage},
	})
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformGitHub, "SearchUsers", err)
	}
	out := make([]*provider.SearchUserResult, 0, len(result.Users))
	for _, u := range result.Users {
		out = append(out, &provider.SearchUserResult{
			Login:     u.GetLogin(),
			Name:      u.GetName(),
			AvatarURL: u.GetAvatarURL(),
			WebURL:    u.GetHTMLURL(),
		})
	}
	if total := result.GetTotal(); total > 0 {
		return out, &total, nil
	}
	return out, nil, nil
}

var _ provider.SearchManager = (*Provider)(nil)
