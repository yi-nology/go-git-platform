package gitlab

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListDeployKeys implements provider.DeploymentKeyManager.
func (p *Provider) ListDeployKeys(ctx context.Context, owner, repo string) ([]*model.DeployKey, error) {
	keys, err := backendutil.AllPages(func(page int) ([]*gitlab.ProjectDeployKey, error) {
		list, _, err := p.client.DeployKeys.ListProjectDeployKeys(pidOf(owner, repo), &gitlab.ListProjectDeployKeysOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListDeployKeys", err)
	}
	result := make([]*model.DeployKey, 0, len(keys))
	for _, k := range keys {
		result = append(result, convertDeployKey(k))
	}
	return result, nil
}

// AddDeployKey implements provider.DeploymentKeyManager.
func (p *Provider) AddDeployKey(ctx context.Context, owner, repo string, opts provider.AddDeployKeyOptions) (*model.DeployKey, error) {
	// GitLab uses CanPush (true = read-write); provider uses ReadOnly (true =
	// read-only), so the mapping is ReadOnly = !CanPush.
	canPush := !opts.ReadOnly
	createOpts := &gitlab.AddDeployKeyOptions{
		Key:     new(opts.Key),
		Title:   new(opts.Title),
		CanPush: new(canPush),
	}
	key, _, err := p.client.DeployKeys.AddDeployKey(pidOf(owner, repo), createOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "AddDeployKey", err)
	}
	return convertDeployKey(key), nil
}

// DeleteDeployKey implements provider.DeploymentKeyManager.
func (p *Provider) DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error {
	if _, err := p.client.DeployKeys.DeleteDeployKey(pidOf(owner, repo), keyID, gitlab.WithContext(ctx)); err != nil {
		return provider.Wrap(model.PlatformGitLab, "DeleteDeployKey", err)
	}
	return nil
}

// convertDeployKey maps a gitlab.ProjectDeployKey to a model.DeployKey.
// GitLab's CanPush is the inverse of provider's ReadOnly.
func convertDeployKey(k *gitlab.ProjectDeployKey) *model.DeployKey {
	if k == nil {
		return nil
	}
	return &model.DeployKey{
		ID:       k.ID,
		Title:    k.Title,
		Key:      k.Key,
		ReadOnly: !k.CanPush,
	}
}

var _ provider.DeploymentKeyManager = (*Provider)(nil)
