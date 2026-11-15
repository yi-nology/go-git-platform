package tencentcode

import (
	"context"

	gongfeng "github.com/yi-nology/go-gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListForks implements provider.RepoStatsManager.
//
// TencentCode does not expose a forks list endpoint; this method
// always returns ErrNotImplemented.
func (p *Provider) ListForks(_ context.Context, _, _ string) ([]*model.PlatformRepo, error) {
	return nil, provider.Wrap(model.PlatformTencentCode, "ListForks", provider.ErrNotImplemented)
}

// ListStargazers implements provider.RepoStatsManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListStargazers(ctx context.Context, owner, repo string) ([]*model.CRUser, error) {
	stars, err := backendutil.AllPages(func(page int) ([]*gongfeng.ProjectStar, error) {
		list, _, err := p.client.Projects.ListProjectStars(ctx, pid(owner, repo),
			&gongfeng.ListProjectStarsOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "ListStargazers", err)
	}
	result := make([]*model.CRUser, 0, len(stars))
	for _, s := range stars {
		if s.User != nil {
			result = append(result, &model.CRUser{
				ID:        int64(s.User.ID),
				Username:  s.User.Username,
				Name:      s.User.Name,
				AvatarURL: s.User.AvatarURL,
			})
		}
	}
	return result, nil
}

// ListContributors implements provider.RepoStatsManager.
//
// TencentCode does not expose a contributors list endpoint; this method
// always returns ErrNotImplemented.
func (p *Provider) ListContributors(_ context.Context, _, _ string) ([]*model.Contributor, error) {
	return nil, provider.Wrap(model.PlatformTencentCode, "ListContributors", provider.ErrNotImplemented)
}

var _ provider.RepoStatsManager = (*Provider)(nil)
