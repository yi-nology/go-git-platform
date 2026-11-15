package gitlab

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranches implements provider.BranchManager.
func (p *Provider) ListBranches(ctx context.Context, owner, repo string) ([]*model.PlatformBranch, error) {
	branches, err := backendutil.AllPages(func(page int) ([]*gitlab.Branch, error) {
		list, _, err := p.client.Branches.ListBranches(pidOf(owner, repo),
			&gitlab.ListBranchesOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}},
			gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListBranches", err)
	}
	result := make([]*model.PlatformBranch, 0, len(branches))
	for _, b := range branches {
		result = append(result, convertBranch(b))
	}
	return result, nil
}

// CreateBranch implements provider.BranchManager.
func (p *Provider) CreateBranch(ctx context.Context, owner, repo, branch, ref string) (*model.PlatformBranch, error) {
	b, _, err := p.client.Branches.CreateBranch(pidOf(owner, repo),
		&gitlab.CreateBranchOptions{Branch: new(branch), Ref: new(ref)},
		gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CreateBranch", err)
	}
	return convertBranch(b), nil
}

// DeleteBranch implements provider.BranchManager.
func (p *Provider) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	_, err := p.client.Branches.DeleteBranch(pidOf(owner, repo), branch, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(model.PlatformGitLab, "DeleteBranch", err)
	}
	return nil
}

var _ provider.BranchManager = (*Provider)(nil)
