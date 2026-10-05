package forgejo

import (
	"context"
	"strconv"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements the provider.SearchManager surface over the Forgejo
// SDK. Mappings and registrations (divergence ledger) mirror Gitea:
//   - SearchRepos rides the global keyword repo search (/repos/search);
//     Sort passes through in Forgejo's own vocabulary (alpha/created/
//     updated/size/id) — values from other platforms' vocabularies are
//     ignored server-side (registered pass-through).
//   - SearchIssues: without Repo it rides the global keyword issue search
//     (/repos/issues/search, KeyWord param) restricted to real issues via
//     the "issues" type filter (the endpoint also matches pull requests).
//     With Repo set ("owner/repo") it routes to ListRepoIssues — the
//     server-side repo-scoped listing at /repos/{owner}/{repo}/issues,
//     which honors the same KeyWord/type/state/pagination parameters —
//     mirroring the GitLab IssuesByProject routing, so multi-page
//     repo-scoped results and totals stay exact (no client-side
//     filtering). State forwards natively.
//   - SearchUsers rides /users/search.
//   - The SDK returns no totals (nil). The SDK
//     takes no per-call context (as with the rest of this backend).

// SearchRepos implements provider.SearchManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) SearchRepos(ctx context.Context, opts provider.SearchReposOptions) ([]*provider.SearchRepoResult, *int, error) {
	baseOpts := forgejo.SearchRepoOptions{
		Keyword: opts.Query,
		Sort:    opts.Sort,
		Order:   opts.Order,
	}
	var repos []*forgejo.Repository
	repos, err := backendutil.PageList(opts.Page, opts.PerPage, listPageSize,
		func(page, perPage int) ([]*forgejo.Repository, error) {
			baseOpts.ListOptions = forgejo.ListOptions{Page: page, PageSize: perPage}
			list, _, err := p.client.SearchRepos(baseOpts)
			return list, err
		})
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformForgejo, "SearchRepos", err)
	}
	out := make([]*provider.SearchRepoResult, 0, len(repos))
	for _, r := range repos {
		out = append(out, &provider.SearchRepoResult{
			FullName:      r.FullName,
			Description:   r.Description,
			WebURL:        r.HTMLURL,
			Stars:         r.Stars,
			Forks:         r.Forks,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
		})
	}
	return out, nil, nil
}

// SearchIssues implements provider.SearchManager. Repo routes to the
// server-side repo-scoped listing (ListRepoIssues); without it the global
// keyword search runs.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) SearchIssues(ctx context.Context, opts provider.SearchIssuesOptions) ([]*provider.SearchIssueResult, *int, error) {
	listOpts := forgejo.ListIssueOption{
		KeyWord: opts.Query,
		State:   forgejo.StateType(opts.State),
		Type:    forgejo.IssueTypeIssue,
	}
	var repoOwner, repoName string
	if opts.Repo != "" {
		repoOwner, repoName = provider.SplitFullName(opts.Repo)
		if repoOwner == "" || repoName == "" {
			return nil, nil, provider.Wrapf(provider.PlatformForgejo, "SearchIssues", "invalid repo %q, want owner/name", opts.Repo)
		}
	}
	fetch := func() ([]*forgejo.Issue, error) {
		if repoName != "" {
			list, _, err := p.client.ListRepoIssues(repoOwner, repoName, listOpts)
			return list, err
		}
		list, _, err := p.client.ListIssues(listOpts)
		return list, err
	}
	issues, err := backendutil.PageList(opts.Page, opts.PerPage, listPageSize,
		func(page, perPage int) ([]*forgejo.Issue, error) {
			listOpts.ListOptions = forgejo.ListOptions{Page: page, PageSize: perPage}
			return fetch()
		})
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformForgejo, "SearchIssues", err)
	}
	out := make([]*provider.SearchIssueResult, 0, len(issues))
	for _, i := range issues {
		labels := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			labels = append(labels, l.Name)
		}
		out = append(out, &provider.SearchIssueResult{
			Number:    strconv.FormatInt(i.Index, 10),
			Title:     i.Title,
			Body:      i.Body,
			State:     provider.IssueState(i.State),
			WebURL:    i.HTMLURL,
			Labels:    labels,
			Comments:  i.Comments,
			CreatedAt: i.Created,
		})
	}
	return out, nil, nil
}

// SearchUsers implements provider.SearchManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) SearchUsers(ctx context.Context, opts provider.SearchUsersOptions) ([]*provider.SearchUserResult, *int, error) {
	baseOpts := forgejo.SearchUsersOption{KeyWord: opts.Query}
	var users []*forgejo.User
	users, err := backendutil.PageList(opts.Page, opts.PerPage, listPageSize,
		func(page, perPage int) ([]*forgejo.User, error) {
			baseOpts.ListOptions = forgejo.ListOptions{Page: page, PageSize: perPage}
			list, _, err := p.client.SearchUsers(baseOpts)
			return list, err
		})
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformForgejo, "SearchUsers", err)
	}
	out := make([]*provider.SearchUserResult, 0, len(users))
	for _, u := range users {
		out = append(out, &provider.SearchUserResult{
			Login:     u.UserName,
			Name:      u.FullName,
			AvatarURL: u.AvatarURL,
			WebURL:    u.HTMLURL,
		})
	}
	return out, nil, nil
}

var _ provider.SearchManager = (*Provider)(nil)
