package forgejo

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// Label management is served by the Gitea-family engine
// (backends/internal/giteafamily); this backend's adapter (sdkPorts) maps
// the SDK's label endpoints. Labels are addressed by name on the provider
// surface and by numeric ID on the family's update/delete endpoints, so
// update/delete resolve the ID through the per-provider TTL cache.

// ListLabels implements provider.LabelManager.
func (p *Provider) ListLabels(ctx context.Context, owner, repo string, opts provider.ListLabelsOptions) ([]*provider.Label, error) {
	return giteafamily.ListLabels(ctx, p.family, p.ports, owner, repo, opts)
}

// CreateLabel implements provider.LabelManager.
func (p *Provider) CreateLabel(ctx context.Context, owner, repo string, opts provider.CreateLabelOptions) (*provider.Label, error) {
	return giteafamily.CreateLabel(ctx, p.family, p.ports, owner, repo, opts)
}

// UpdateLabel implements provider.LabelManager.
func (p *Provider) UpdateLabel(ctx context.Context, owner, repo, name string, opts provider.UpdateLabelOptions) (*provider.Label, error) {
	return giteafamily.UpdateLabel(ctx, p.family, p.ports, p.labelIDs, owner, repo, name, opts)
}

// DeleteLabel implements provider.LabelManager.
func (p *Provider) DeleteLabel(ctx context.Context, owner, repo, name string) error {
	return giteafamily.DeleteLabel(ctx, p.family, p.ports, p.labelIDs, owner, repo, name)
}

var _ provider.LabelManager = (*Provider)(nil)
