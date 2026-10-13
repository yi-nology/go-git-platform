package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListRepos implements provider.RepoManager.
func (p *Provider) ListRepos(ctx context.Context, opts provider.ListRepoOptions) ([]*provider.PlatformRepo, error) {
	return giteafamily.ListRepos(ctx, p.family, p.ports, opts)
}

// GetRepo implements provider.RepoManager.
func (p *Provider) GetRepo(ctx context.Context, owner, repo string) (*provider.PlatformRepo, error) {
	return giteafamily.GetRepo(ctx, p.family, p.ports, owner, repo)
}

// CreateRepo implements provider.RepoManager.
func (p *Provider) CreateRepo(ctx context.Context, owner string, opts provider.CreateRepoOptions) (*provider.PlatformRepo, error) {
	return giteafamily.CreateRepo(ctx, p.family, p.ports, owner, opts)
}

// DeleteRepo implements provider.RepoManager.
func (p *Provider) DeleteRepo(ctx context.Context, owner, repo string) error {
	return giteafamily.DeleteRepo(ctx, p.family, p.ports, owner, repo)
}

// UpdateRepo implements provider.RepoManager.
func (p *Provider) UpdateRepo(ctx context.Context, owner, repo string, opts provider.UpdateRepoOptions) (*provider.PlatformRepo, error) {
	return giteafamily.UpdateRepo(ctx, p.family, p.ports, owner, repo, opts)
}

// ForkRepo implements provider.RepoManager.
func (p *Provider) ForkRepo(ctx context.Context, owner, repo string, opts provider.ForkRepoOptions) (*provider.PlatformRepo, error) {
	return giteafamily.ForkRepo(ctx, p.family, p.ports, owner, repo, opts)
}

var _ provider.RepoManager = (*Provider)(nil)
