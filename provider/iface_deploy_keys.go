package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// DeploymentKeyManager provides deploy key CRUD for CI/CD. It is an
// optional capability interface: consumers should gate on
// Provider.Capabilities().DeployKeys (or type-assert) before use.
//
// Platform support: GitHub, GitCode, GitLab, Gitea, Forgejo.
// Gitee and TencentCode have no deploy key API.
type DeploymentKeyManager interface {
	// ListDeployKeys returns all deploy keys for a repository.
	ListDeployKeys(ctx context.Context, owner, repo string) ([]*model.DeployKey, error)
	// AddDeployKey adds a new deploy key to the repository.
	AddDeployKey(ctx context.Context, owner, repo string, opts AddDeployKeyOptions) (*model.DeployKey, error)
	// DeleteDeployKey removes a deploy key by its platform ID.
	DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error
}
