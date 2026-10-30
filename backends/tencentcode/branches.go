package tencentcode

import (
	"context"

	gongfeng "github.com/yi-nology/go-gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranches implements provider.BranchManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListBranches(ctx context.Context, owner, repo string) ([]*provider.PlatformBranch, error) {
	pid := owner + "/" + repo
	branches, err := backendutil.AllPages(func(page int) ([]*gongfeng.Branch, error) {
		list, _, err := p.client.Branches.ListBranches(ctx, pid,
			&gongfeng.ListBranchesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListBranches", err)
	}
	result := make([]*provider.PlatformBranch, 0, len(branches))
	for _, b := range branches {
		result = append(result, convertBranch(b))
	}
	return result, nil
}

// CreateBranch implements provider.BranchManager.
func (p *Provider) CreateBranch(ctx context.Context, owner, repo, branch, ref string) (*provider.PlatformBranch, error) {
	pid := owner + "/" + repo
	opts := &gongfeng.CreateBranchOptions{
		BranchName: gongfeng.Ptr(branch),
		Ref:        gongfeng.Ptr(ref),
	}
	b, _, err := p.client.Branches.CreateBranch(ctx, pid, opts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateBranch", err)
	}
	return convertBranch(b), nil
}

// DeleteBranch implements provider.BranchManager.
func (p *Provider) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	pid := owner + "/" + repo
	_, err := p.client.Branches.DeleteBranch(ctx, pid, branch)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteBranch", err)
	}
	return nil
}

var _ provider.BranchManager = (*Provider)(nil)
