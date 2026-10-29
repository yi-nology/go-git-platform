package tencentcode

import (
	"context"

	gongfeng "github.com/yi-nology/go-git-platform/backends/tencentcode/internal/gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListForks implements provider.RepoStatsManager.
//
// TencentCode does not expose a forks list endpoint; this method
// always returns ErrNotImplemented.
func (p *Provider) ListForks(_ context.Context, _, _ string) ([]*provider.PlatformRepo, error) {
	return nil, provider.Wrapf(provider.PlatformTencentCode, "ListForks", "not supported on this platform")
}

// ListStargazers implements provider.RepoStatsManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListStargazers(ctx context.Context, owner, repo string) ([]*provider.CRUser, error) {
	stars, err := backendutil.AllPages(func(page int) ([]*gongfeng.ProjectStar, error) {
		list, _, err := p.client.Projects.ListProjectStars(ctx, pid(owner, repo),
			&gongfeng.ListProjectStarsOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListStargazers", err)
	}
	result := make([]*provider.CRUser, 0, len(stars))
	for _, s := range stars {
		if s.User != nil {
			result = append(result, &provider.CRUser{
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
func (p *Provider) ListContributors(_ context.Context, _, _ string) ([]*provider.Contributor, error) {
	return nil, provider.Wrapf(provider.PlatformTencentCode, "ListContributors", "not supported on this platform")
}

var _ provider.RepoStatsManager = (*Provider)(nil)
