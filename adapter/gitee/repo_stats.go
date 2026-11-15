package gitee

import (
	"context"
	"fmt"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListForks implements provider.RepoStatsManager. The provider interface
// exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (Gitee's per_page caps at 100; fetch until an empty
// page).
func (p *Provider) ListForks(ctx context.Context, owner, repo string) ([]*model.PlatformRepo, error) {
	forks, err := backendutil.AllPages(func(page int) ([]*gitee.Project, error) {
		list, _, err := p.client.Repositories.ListForks(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &gitee.ListForksOptions{
			Page:    gitee.Int(page),
			PerPage: gitee.Int(100),
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "ListForks", err)
	}
	result := make([]*model.PlatformRepo, 0, len(forks))
	for _, f := range forks {
		result = append(result, convertProject(f))
	}
	return result, nil
}

// ListStargazers implements provider.RepoStatsManager.
//
// The new SDK does not expose a dedicated stargazers endpoint, so we use the
// client's generic NewRequest + Do pattern to call the API directly.
func (p *Provider) ListStargazers(ctx context.Context, owner, repo string) ([]*model.CRUser, error) {
	u := fmt.Sprintf("repos/%s/%s/stargazers", backendutil.Esc(owner), backendutil.Esc(repo))
	req, err := p.client.NewRequest(ctx, "GET", u, nil)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "ListStargazers", err)
	}
	var users []*gitee.UserBasic
	_, err = p.client.Do(req, &users)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "ListStargazers", err)
	}
	result := make([]*model.CRUser, 0, len(users))
	for _, u := range users {
		result = append(result, &model.CRUser{
			ID:        int64(deref(u.ID)),
			Username:  deref(u.Login),
			Name:      deref(u.Name),
			AvatarURL: deref(u.AvatarURL),
		})
	}
	return result, nil
}

// ListContributors implements provider.RepoStatsManager. The go-gitee
// SDK's ListContributorsOptions carries no page/per_page fields, so there
// is no pagination parameter to exhaust — the platform returns what it
// returns in a single response.
func (p *Provider) ListContributors(ctx context.Context, owner, repo string) ([]*model.Contributor, error) {
	contributors, _, err := p.client.Repositories.ListContributors(ctx, backendutil.Esc(owner), backendutil.Esc(repo), nil)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "ListContributors", err)
	}
	result := make([]*model.Contributor, 0, len(contributors))
	for _, c := range contributors {
		result = append(result, convertContributor(c))
	}
	return result, nil
}

var _ provider.RepoStatsManager = (*Provider)(nil)
