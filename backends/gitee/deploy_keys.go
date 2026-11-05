package gitee

import (
	"context"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListDeployKeys implements provider.DeploymentKeyManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (Gitee's per_page caps at 100; fetch until an empty
// page).
func (p *Provider) ListDeployKeys(ctx context.Context, owner, repo string) ([]*provider.DeployKey, error) {
	keys, err := backendutil.AllPages(func(page int) ([]*gitee.SSHKey, error) {
		list, _, err := p.client.Repositories.ListKeys(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &gitee.ListOptions{
			Page: page, PerPage: 100,
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListDeployKeys", err)
	}
	result := make([]*provider.DeployKey, 0, len(keys))
	for _, k := range keys {
		result = append(result, convertSSHKey(k))
	}
	return result, nil
}

// AddDeployKey implements provider.DeploymentKeyManager.
func (p *Provider) AddDeployKey(ctx context.Context, owner, repo string, opts provider.AddDeployKeyOptions) (*provider.DeployKey, error) {
	key, _, err := p.client.Repositories.CreateKey(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &gitee.CreateKeyOptions{
		Title: gitee.String(opts.Title),
		Key:   gitee.String(opts.Key),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "AddDeployKey", err)
	}
	return convertSSHKey(key), nil
}

// DeleteDeployKey implements provider.DeploymentKeyManager.
func (p *Provider) DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error {
	_, err := p.client.Repositories.DeleteKey(ctx, backendutil.Esc(owner), backendutil.Esc(repo), keyID)
	return provider.Wrap(provider.PlatformGitee, "DeleteDeployKey", err)
}

func convertSSHKey(k *gitee.SSHKey) *provider.DeployKey {
	if k == nil {
		return nil
	}
	return &provider.DeployKey{
		ID:    int64(deref(k.ID)),
		Title: deref(k.Title),
		Key:   deref(k.Key),
	}
}

var _ provider.DeploymentKeyManager = (*Provider)(nil)
