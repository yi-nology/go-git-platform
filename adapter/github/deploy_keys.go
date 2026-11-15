package github

import (
	"context"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListDeployKeys implements provider.DeploymentKeyManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListDeployKeys(ctx context.Context, owner, repo string) ([]*model.DeployKey, error) {
	keys, err := backendutil.AllPages(func(page int) ([]*github.Key, error) {
		list, _, err := p.client.Repositories.ListKeys(ctx, owner, repo, &github.ListOptions{
			Page: page, PerPage: 100,
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "ListDeployKeys", err)
	}
	result := make([]*model.DeployKey, 0, len(keys))
	for _, k := range keys {
		result = append(result, convertDeployKey(k))
	}
	return result, nil
}

// AddDeployKey implements provider.DeploymentKeyManager.
func (p *Provider) AddDeployKey(ctx context.Context, owner, repo string, opts provider.AddDeployKeyOptions) (*model.DeployKey, error) {
	key := &github.CreateDeployKeyRequest{
		Title:    new(opts.Title),
		Key:      opts.Key,
		ReadOnly: new(opts.ReadOnly),
	}
	created, _, err := p.client.Repositories.CreateKey(ctx, owner, repo, *key)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "AddDeployKey", err)
	}
	return convertDeployKey(created), nil
}

// DeleteDeployKey implements provider.DeploymentKeyManager.
func (p *Provider) DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error {
	if _, err := p.client.Repositories.DeleteKey(ctx, owner, repo, keyID); err != nil {
		return provider.Wrap(model.PlatformGitHub, "DeleteDeployKey", err)
	}
	return nil
}

// convertDeployKey maps a github.Key to a model.DeployKey.
func convertDeployKey(k *github.Key) *model.DeployKey {
	if k == nil {
		return nil
	}
	return &model.DeployKey{
		ID:       k.GetID(),
		Title:    k.GetTitle(),
		Key:      k.GetKey(),
		ReadOnly: k.GetReadOnly(),
	}
}

var _ provider.DeploymentKeyManager = (*Provider)(nil)
