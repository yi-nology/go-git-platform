package gitcode

import (
	"context"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListForks implements provider.RepoStatsManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (GitCode's page-size ceiling is 100).
func (p *Provider) ListForks(ctx context.Context, owner, repo string) ([]*provider.PlatformRepo, error) {
	forks, err := backendutil.AllPages(func(page int) ([]*gitcode.Repository, error) {
		return p.client.ListForks(ctx, owner, repo, gitcode.ListOptions{Page: page, PerPage: 100})
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListForks", err)
	}
	result := make([]*provider.PlatformRepo, 0, len(forks))
	for _, r := range forks {
		result = append(result, convertGitcodeRepo(r))
	}
	return result, nil
}

// ListStargazers implements provider.RepoStatsManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (GitCode's page-size ceiling is 100).
func (p *Provider) ListStargazers(ctx context.Context, owner, repo string) ([]*provider.CRUser, error) {
	stargazers, err := backendutil.AllPages(func(page int) ([]*gitcode.User, error) {
		return p.client.ListStargazers(ctx, owner, repo, gitcode.ListOptions{Page: page, PerPage: 100})
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListStargazers", err)
	}
	result := make([]*provider.CRUser, 0, len(stargazers))
	for _, u := range stargazers {
		id, _ := parseGitCodeID(u.ID)
		result = append(result, &provider.CRUser{
			ID:        id,
			Username:  u.Login,
			Name:      u.Name,
			AvatarURL: u.AvatarURL,
		})
	}
	return result, nil
}

// ListContributors implements provider.RepoStatsManager. The SDK's
// ListContributors endpoint exposes no page parameter (it hard-codes
// per_page=100), so the call stays single-shot.
func (p *Provider) ListContributors(ctx context.Context, owner, repo string) ([]*provider.Contributor, error) {
	contributors, err := p.client.ListContributors(ctx, owner, repo)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListContributors", err)
	}
	result := make([]*provider.Contributor, 0, len(contributors))
	for _, c := range contributors {
		result = append(result, &provider.Contributor{
			Username:      c.Login,
			Contributions: c.Contributions,
		})
	}
	return result, nil
}

var _ provider.RepoStatsManager = (*Provider)(nil)
