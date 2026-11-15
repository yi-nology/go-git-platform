package forgejo

import (
	"context"

	"github.com/yi-nology/go-git-platform/adapter/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListForks implements provider.RepoStatsManager.
func (p *Provider) ListForks(ctx context.Context, owner, repo string) ([]*model.PlatformRepo, error) {
	return giteafamily.ListForks(ctx, p.family, p.ports, owner, repo)
}

// ListStargazers implements provider.RepoStatsManager.
func (p *Provider) ListStargazers(ctx context.Context, owner, repo string) ([]*model.CRUser, error) {
	return giteafamily.ListStargazers(ctx, p.family, p.ports, owner, repo)
}

// ListContributors implements provider.RepoStatsManager. The family exposes
// no contributors list endpoint, so it always returns ErrNotImplemented.
func (p *Provider) ListContributors(ctx context.Context, owner, repo string) ([]*model.Contributor, error) {
	return giteafamily.ListContributors(ctx, p.family, p.ports, owner, repo)
}

var _ provider.RepoStatsManager = (*Provider)(nil)
