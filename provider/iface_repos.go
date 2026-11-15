package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// RepoManager handles repository CRUD operations.
type RepoManager interface {
	ListRepos(ctx context.Context, opts ListRepoOptions) ([]*model.PlatformRepo, error)
	GetRepo(ctx context.Context, owner, repo string) (*model.PlatformRepo, error)
	CreateRepo(ctx context.Context, owner string, opts CreateRepoOptions) (*model.PlatformRepo, error)
	DeleteRepo(ctx context.Context, owner, repo string) error
	UpdateRepo(ctx context.Context, owner, repo string, opts UpdateRepoOptions) (*model.PlatformRepo, error)
	ForkRepo(ctx context.Context, owner, repo string, opts ForkRepoOptions) (*model.PlatformRepo, error)
}
