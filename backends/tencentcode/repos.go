package tencentcode

import (
	"context"

	gongfeng "github.com/studyzy/gongfeng-sdk-go"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListRepos implements provider.RepoManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListRepos(ctx context.Context, opts provider.ListRepoOptions) ([]*provider.PlatformRepo, error) {
	if opts.Owner != "" {
		// Get group projects via GetGroup (which returns embedded projects).
		group, _, err := p.client.Groups.GetGroup(ctx, opts.Owner)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformTencentCode, "ListRepos", err)
		}
		repos := make([]*provider.PlatformRepo, 0, len(group.Projects))
		for _, proj := range group.Projects {
			repos = append(repos, convertProject(proj))
		}
		return repos, nil
	}

	buildOpts := func(page, perPage int) *gongfeng.ListProjectsOptions {
		return &gongfeng.ListProjectsOptions{
			ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage},
		}
	}
	var projects []*gongfeng.Project
	projects, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gongfeng.Project, error) {
			list, _, err := p.client.Projects.ListProjects(ctx, buildOpts(page, perPage))
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListRepos", err)
	}
	repos := make([]*provider.PlatformRepo, 0, len(projects))
	for _, proj := range projects {
		repos = append(repos, convertProject(proj))
	}
	return repos, nil
}

// GetRepo implements provider.RepoManager.
func (p *Provider) GetRepo(ctx context.Context, owner, repo string) (*provider.PlatformRepo, error) {
	pid := owner + "/" + repo
	proj, _, err := p.client.Projects.GetProject(ctx, pid)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetRepo", err)
	}
	return convertProject(proj), nil
}

// CreateRepo implements provider.RepoManager.
func (p *Provider) CreateRepo(ctx context.Context, owner string, opts provider.CreateRepoOptions) (*provider.PlatformRepo, error) {
	createOpts := &gongfeng.CreateProjectOptions{
		Name: gongfeng.Ptr(opts.Name),
	}
	if opts.Description != "" {
		createOpts.Description = gongfeng.Ptr(opts.Description)
	}
	if opts.Private {
		createOpts.VisibilityLevel = gongfeng.Ptr(0)
	} else {
		createOpts.VisibilityLevel = gongfeng.Ptr(20)
	}
	proj, _, err := p.client.Projects.CreateProject(ctx, createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateRepo", err)
	}
	return convertProject(proj), nil
}

// ForkRepo implements provider.RepoManager.
func (p *Provider) ForkRepo(ctx context.Context, owner, repo string, opts provider.ForkRepoOptions) (*provider.PlatformRepo, error) {
	pid := owner + "/" + repo
	proj, _, err := p.client.Forks.ForkProject(ctx, pid)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ForkRepo", err)
	}
	return convertProject(proj), nil
}

// DeleteRepo implements provider.RepoManager.
func (p *Provider) DeleteRepo(ctx context.Context, owner, repo string) error {
	pid := owner + "/" + repo
	_, err := p.client.Projects.DeleteProject(ctx, pid)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteRepo", err)
	}
	return nil
}

// UpdateRepo implements provider.RepoManager.
func (p *Provider) UpdateRepo(ctx context.Context, owner, repo string, opts provider.UpdateRepoOptions) (*provider.PlatformRepo, error) {
	pid := owner + "/" + repo
	updateOpts := &gongfeng.UpdateProjectOptions{}
	if opts.Name != "" {
		updateOpts.Name = gongfeng.Ptr(opts.Name)
	}
	if opts.Description != "" {
		updateOpts.Description = gongfeng.Ptr(opts.Description)
	}
	if opts.DefaultBranch != "" {
		updateOpts.DefaultBranch = gongfeng.Ptr(opts.DefaultBranch)
	}
	if opts.Private != nil {
		if *opts.Private {
			updateOpts.VisibilityLevel = gongfeng.Ptr(0)
		} else {
			updateOpts.VisibilityLevel = gongfeng.Ptr(20)
		}
	}
	proj, _, err := p.client.Projects.UpdateProject(ctx, pid, updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "UpdateRepo", err)
	}
	return convertProject(proj), nil
}

var _ provider.RepoManager = (*Provider)(nil)
