package github

import (
	"context"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListRepos implements provider.RepoManager.
func (p *Provider) ListRepos(ctx context.Context, opts provider.ListRepoOptions) ([]*provider.PlatformRepo, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{Page: page, PerPage: perPage},
	}
	var repos []*github.Repository
	var err error
	if opts.Owner != "" {
		repos, _, err = p.client.Repositories.ListByOrg(ctx, opts.Owner, &github.RepositoryListByOrgOptions{
			ListOptions: listOpts.ListOptions,
		})
	} else {
		// Same wire call the deprecated Repositories.List makes when user is
		// empty: list repositories of the authenticated user.
		repos, _, err = p.client.Repositories.ListByAuthenticatedUser(ctx, &github.RepositoryListByAuthenticatedUserOptions{
			ListOptions: listOpts.ListOptions,
		})
	}
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListRepos", err)
	}
	result := make([]*provider.PlatformRepo, 0, len(repos))
	for _, r := range repos {
		result = append(result, convertRepo(r))
	}
	return result, nil
}

// GetRepo implements provider.RepoManager.
func (p *Provider) GetRepo(ctx context.Context, owner, repo string) (*provider.PlatformRepo, error) {
	r, _, err := p.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetRepo", err)
	}
	return convertRepo(r), nil
}

// CreateRepo implements provider.RepoManager.
func (p *Provider) CreateRepo(ctx context.Context, owner string, opts provider.CreateRepoOptions) (*provider.PlatformRepo, error) {
	// go-github v93 moved repo creation onto a dedicated request type
	// (CreateRepositoryRequest); only the fields provider.CreateRepoOptions
	// carries are populated.
	body := github.CreateRepositoryRequest{
		Name:        opts.Name,
		Description: new(opts.Description),
		Private:     new(opts.Private),
		AutoInit:    new(opts.AutoInit),
	}
	result, _, err := p.client.Repositories.Create(ctx, owner, body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateRepo", err)
	}
	return convertRepo(result), nil
}

// ForkRepo implements provider.RepoManager.
func (p *Provider) ForkRepo(ctx context.Context, owner, repo string, opts provider.ForkRepoOptions) (*provider.PlatformRepo, error) {
	body := github.CreateForkRequest{
		Organization: new(opts.Organization),
		Name:         new(opts.Name),
	}
	r, _, err := p.client.Repositories.CreateFork(ctx, owner, repo, body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ForkRepo", err)
	}
	return convertRepo(r), nil
}

// DeleteRepo implements provider.RepoManager.
func (p *Provider) DeleteRepo(ctx context.Context, owner, repo string) error {
	_, err := p.client.Repositories.Delete(ctx, owner, repo)
	if err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DeleteRepo", err)
	}
	return nil
}

// UpdateRepo implements provider.RepoManager.
func (p *Provider) UpdateRepo(ctx context.Context, owner, repo string, opts provider.UpdateRepoOptions) (*provider.PlatformRepo, error) {
	r := &github.Repository{}
	if opts.Name != "" {
		r.Name = new(opts.Name)
	}
	if opts.Description != "" {
		r.Description = new(opts.Description)
	}
	if opts.DefaultBranch != "" {
		r.DefaultBranch = new(opts.DefaultBranch)
	}
	if opts.Private != nil {
		r.Private = opts.Private
	}
	result, _, err := p.client.Repositories.Edit(ctx, owner, repo, r)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateRepo", err)
	}
	return convertRepo(result), nil
}
