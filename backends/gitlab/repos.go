package gitlab

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/provider"
)

func pidOf(owner, repo string) string { return owner + "/" + repo }

// ListRepos implements provider.RepoManager.
func (p *Provider) ListRepos(ctx context.Context, opts provider.ListRepoOptions) ([]*provider.PlatformRepo, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	page64, perPage64 := int64(page), int64(perPage)
	var projects []*gitlab.Project
	var err error
	if opts.Owner != "" {
		projects, _, err = p.client.Groups.ListGroupProjects(opts.Owner, &gitlab.ListGroupProjectsOptions{
			ListOptions: gitlab.ListOptions{Page: page64, PerPage: perPage64},
		}, gitlab.WithContext(ctx))
	} else {
		// 无 Owner 时限定 membership,只列当前 token 用户参与的项目;
		// 否则 /projects 会返回全实例可见项目(公共 GitLab 上是海量数据)。
		projects, _, err = p.client.Projects.ListProjects(&gitlab.ListProjectsOptions{
			ListOptions: gitlab.ListOptions{Page: page64, PerPage: perPage64},
			Membership:  new(true),
		}, gitlab.WithContext(ctx))
	}
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListRepos", err)
	}
	repos := make([]*provider.PlatformRepo, 0, len(projects))
	for _, proj := range projects {
		repos = append(repos, convertProject(proj))
	}
	return repos, nil
}

// GetRepo implements provider.RepoManager.
func (p *Provider) GetRepo(ctx context.Context, owner, repo string) (*provider.PlatformRepo, error) {
	proj, _, err := p.client.Projects.GetProject(pidOf(owner, repo), nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "GetRepo", err)
	}
	return convertProject(proj), nil
}

// CreateRepo implements provider.RepoManager.
func (p *Provider) CreateRepo(ctx context.Context, owner string, opts provider.CreateRepoOptions) (*provider.PlatformRepo, error) {
	createOpts := &gitlab.CreateProjectOptions{
		Name:                 new(opts.Name),
		Description:          new(opts.Description),
		Visibility:           new(gitlab.VisibilityValue("private")),
		InitializeWithReadme: new(opts.AutoInit),
	}
	if !opts.Private {
		createOpts.Visibility = new(gitlab.VisibilityValue("public"))
	}
	if opts.DefaultBranch != "" {
		createOpts.DefaultBranch = new(opts.DefaultBranch)
	}
	if owner != "" {
		// Create under a namespace (group) by looking up the namespace ID from the owner path.
		namespaces, _, nsErr := p.client.Namespaces.SearchNamespace(owner, gitlab.WithContext(ctx))
		if nsErr == nil {
			for _, ns := range namespaces {
				if ns.Path == owner || ns.FullPath == owner {
					createOpts.NamespaceID = new(ns.ID)
					break
				}
			}
		}
	}
	proj, _, err := p.client.Projects.CreateProject(createOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "CreateRepo", err)
	}
	return convertProject(proj), nil
}

// ForkRepo implements provider.RepoManager.
func (p *Provider) ForkRepo(ctx context.Context, owner, repo string, opts provider.ForkRepoOptions) (*provider.PlatformRepo, error) {
	forkOpts := &gitlab.ForkProjectOptions{}
	if opts.Organization != "" {
		forkOpts.NamespacePath = new(opts.Organization)
	}
	if opts.Name != "" {
		forkOpts.Name = new(opts.Name)
	}
	proj, _, err := p.client.Projects.ForkProject(pidOf(owner, repo), forkOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ForkRepo", err)
	}
	return convertProject(proj), nil
}

// DeleteRepo implements provider.RepoManager.
func (p *Provider) DeleteRepo(ctx context.Context, owner, repo string) error {
	_, err := p.client.Projects.DeleteProject(pidOf(owner, repo), nil, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(provider.PlatformGitLab, "DeleteRepo", err)
	}
	return nil
}

// UpdateRepo implements provider.RepoManager.
func (p *Provider) UpdateRepo(ctx context.Context, owner, repo string, opts provider.UpdateRepoOptions) (*provider.PlatformRepo, error) {
	updateOpts := &gitlab.EditProjectOptions{}
	if opts.Name != "" {
		updateOpts.Name = new(opts.Name)
	}
	if opts.Description != "" {
		updateOpts.Description = new(opts.Description)
	}
	if opts.DefaultBranch != "" {
		updateOpts.DefaultBranch = new(opts.DefaultBranch)
	}
	if opts.Private != nil {
		vis := "public"
		if *opts.Private {
			vis = "private"
		}
		updateOpts.Visibility = new(gitlab.VisibilityValue(vis))
	}
	proj, _, err := p.client.Projects.EditProject(pidOf(owner, repo), updateOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "UpdateRepo", err)
	}
	return convertProject(proj), nil
}

var _ provider.RepoManager = (*Provider)(nil)
