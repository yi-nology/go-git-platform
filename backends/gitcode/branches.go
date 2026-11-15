package gitcode

import (
	"context"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranches implements provider.BranchManager. The SDK's ListBranches
// endpoint exposes no page parameter (it hard-codes per_page=100), so the
// call stays single-shot.
func (p *Provider) ListBranches(ctx context.Context, owner, repo string) ([]*model.PlatformBranch, error) {
	branches, err := p.client.ListBranches(ctx, owner, repo)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitCode, "ListBranches", err)
	}
	result := make([]*model.PlatformBranch, 0, len(branches))
	for _, b := range branches {
		result = append(result, &model.PlatformBranch{Name: b.Name})
	}
	return result, nil
}

// CreateBranch implements provider.BranchManager.
func (p *Provider) CreateBranch(ctx context.Context, owner, repo, branch, ref string) (*model.PlatformBranch, error) {
	_, err := p.client.CreateBranch(ctx, owner, repo, gitcode.CreateBranchOptions{
		BranchName: branch, Ref: ref,
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitCode, "CreateBranch", err)
	}
	return &model.PlatformBranch{Name: branch}, nil
}

// DeleteBranch implements provider.BranchManager.
func (p *Provider) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	err := p.client.DeleteBranch(ctx, owner, repo, branch)
	if err != nil {
		return provider.Wrap(model.PlatformGitCode, "DeleteBranch", err)
	}
	return nil
}

var _ provider.BranchManager = (*Provider)(nil)
