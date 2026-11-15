package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/adapter/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListDeployKeys implements provider.DeploymentKeyManager.
func (p *Provider) ListDeployKeys(ctx context.Context, owner, repo string) ([]*model.DeployKey, error) {
	return giteafamily.ListDeployKeys(ctx, p.family, p.ports, owner, repo)
}

// AddDeployKey implements provider.DeploymentKeyManager.
func (p *Provider) AddDeployKey(ctx context.Context, owner, repo string, opts provider.AddDeployKeyOptions) (*model.DeployKey, error) {
	return giteafamily.AddDeployKey(ctx, p.family, p.ports, owner, repo, opts)
}

// DeleteDeployKey implements provider.DeploymentKeyManager.
func (p *Provider) DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error {
	return giteafamily.DeleteDeployKey(ctx, p.family, p.ports, owner, repo, keyID)
}

var _ provider.DeploymentKeyManager = (*Provider)(nil)
