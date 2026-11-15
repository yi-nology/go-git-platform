package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// BranchManager handles branch operations.
type BranchManager interface {
	ListBranches(ctx context.Context, owner, repo string) ([]*model.PlatformBranch, error)
	CreateBranch(ctx context.Context, owner, repo, branch, ref string) (*model.PlatformBranch, error)
	DeleteBranch(ctx context.Context, owner, repo, branch string) error
}
