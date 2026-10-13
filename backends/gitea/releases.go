package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListTags implements provider.ReleaseManager.
func (p *Provider) ListTags(ctx context.Context, owner, repo string) ([]*provider.TagInfo, error) {
	return giteafamily.ListTags(ctx, p.family, p.ports, owner, repo)
}

// ListReleases implements provider.ReleaseManager.
func (p *Provider) ListReleases(ctx context.Context, owner, repo string) ([]*provider.ReleaseInfo, error) {
	return giteafamily.ListReleases(ctx, p.family, p.ports, owner, repo)
}

// CreateRelease implements provider.ReleaseManager.
func (p *Provider) CreateRelease(ctx context.Context, owner, repo string, opts provider.CreateReleaseOptions) (*provider.ReleaseInfo, error) {
	return giteafamily.CreateRelease(ctx, p.family, p.ports, owner, repo, opts)
}

// GetReleaseByTag implements provider.ReleaseManager.
func (p *Provider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*provider.ReleaseInfo, error) {
	return giteafamily.GetReleaseByTag(ctx, p.family, p.ports, owner, repo, tag)
}

// UpdateRelease implements provider.ReleaseManager.
func (p *Provider) UpdateRelease(ctx context.Context, owner, repo, tag string, opts provider.UpdateReleaseOptions) (*provider.ReleaseInfo, error) {
	return giteafamily.UpdateRelease(ctx, p.family, p.ports, owner, repo, tag, opts)
}

// DeleteRelease implements provider.ReleaseManager.
func (p *Provider) DeleteRelease(ctx context.Context, owner, repo, tag string) error {
	return giteafamily.DeleteRelease(ctx, p.family, p.ports, owner, repo, tag)
}

// GetArchive implements provider.ReleaseManager.
func (p *Provider) GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error) {
	return giteafamily.GetArchive(ctx, p.family, p.ports, owner, repo, ref, format)
}

var _ provider.ReleaseManager = (*Provider)(nil)
