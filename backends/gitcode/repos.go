package gitcode

import (
	"context"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListRepos implements provider.RepoManager. Owner rides the SDK options
// but is interpolated into the /users/{owner}/repos path verbatim, so it
// passes through Esc like every other path segment (issue #84).
func (p *Provider) ListRepos(ctx context.Context, opts provider.ListRepoOptions) ([]*provider.PlatformRepo, error) {
	repos, err := p.client.ListRepositories(ctx, gitcode.ListRepositoriesOptions{
		ListOptions: gitcode.ListOptions{Page: opts.Page, PerPage: opts.PerPage},
		Owner:       backendutil.Esc(opts.Owner),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListRepos", err)
	}
	result := make([]*provider.PlatformRepo, 0, len(repos))
	for _, r := range repos {
		owner := ""
		if r.Owner != nil {
			owner = r.Owner.Login
		}
		result = append(result, &provider.PlatformRepo{
			ID:            r.ID,
			FullName:      r.FullName,
			Name:          r.Name,
			Owner:         owner,
			Description:   r.Description,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
			Platform:      provider.PlatformGitCode,
		})
	}
	return result, nil
}

// GetRepo implements provider.RepoManager.
func (p *Provider) GetRepo(ctx context.Context, owner, repo string) (*provider.PlatformRepo, error) {
	r, err := p.client.GetRepository(ctx, backendutil.Esc(owner), backendutil.Esc(repo))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetRepo", err)
	}
	ownerName := ""
	if r.Owner != nil {
		ownerName = r.Owner.Login
	}
	return &provider.PlatformRepo{
		ID:            r.ID,
		FullName:      r.FullName,
		Name:          r.Name,
		Owner:         ownerName,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Platform:      provider.PlatformGitCode,
	}, nil
}

// CreateRepo implements provider.RepoManager.
func (p *Provider) CreateRepo(ctx context.Context, owner string, opts provider.CreateRepoOptions) (*provider.PlatformRepo, error) {
	var r *gitcode.Repository
	var err error
	if owner != "" {
		orgOpts := gitcode.CreateOrgRepoOptions{
			Name:        opts.Name,
			Description: opts.Description,
			Private:     &opts.Private,
			AutoInit:    &opts.AutoInit,
		}
		if opts.DefaultBranch != "" {
			orgOpts.DefaultBranch = opts.DefaultBranch
		}
		r, err = p.client.CreateOrgRepository(ctx, owner, orgOpts)
	} else {
		r, err = p.client.CreateRepository(ctx, gitcode.CreateRepositoryOptions{
			Name:        opts.Name,
			Description: opts.Description,
			Private:     &opts.Private,
			AutoInit:    &opts.AutoInit,
		})
	}
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateRepo", err)
	}
	ownerName := ""
	if r.Owner != nil {
		ownerName = r.Owner.Login
	}
	return &provider.PlatformRepo{
		ID:            r.ID,
		FullName:      r.FullName,
		Name:          r.Name,
		Owner:         ownerName,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Platform:      provider.PlatformGitCode,
	}, nil
}

// ForkRepo implements provider.RepoManager. opts.Name maps onto the fork
// body's name field (renaming the fork at creation, issue #87); the fork
// endpoint takes no organization — forks always land under the
// authenticated user's namespace (registered divergence).
func (p *Provider) ForkRepo(ctx context.Context, owner, repo string, opts provider.ForkRepoOptions) (*provider.PlatformRepo, error) {
	var forkOpts *gitcode.CreateRepositoryOptions
	if opts.Name != "" {
		forkOpts = &gitcode.CreateRepositoryOptions{Name: opts.Name}
	}
	r, err := p.client.ForkRepository(ctx, backendutil.Esc(owner), backendutil.Esc(repo), forkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ForkRepo", err)
	}
	ownerName := ""
	if r.Owner != nil {
		ownerName = r.Owner.Login
	}
	return &provider.PlatformRepo{
		ID:            r.ID,
		FullName:      r.FullName,
		Name:          r.Name,
		Owner:         ownerName,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Platform:      provider.PlatformGitCode,
	}, nil
}

// DeleteRepo implements provider.RepoManager.
func (p *Provider) DeleteRepo(ctx context.Context, owner, repo string) error {
	err := p.client.DeleteRepository(ctx, backendutil.Esc(owner), backendutil.Esc(repo))
	if err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteRepo", err)
	}
	return nil
}

// UpdateRepo implements provider.RepoManager.
func (p *Provider) UpdateRepo(ctx context.Context, owner, repo string, opts provider.UpdateRepoOptions) (*provider.PlatformRepo, error) {
	r, err := p.client.UpdateRepository(ctx, backendutil.Esc(owner), backendutil.Esc(repo), gitcode.UpdateRepositoryOptions{
		Name:          opts.Name,
		Description:   opts.Description,
		DefaultBranch: opts.DefaultBranch,
		Private:       opts.Private,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateRepo", err)
	}
	ownerName := ""
	if r.Owner != nil {
		ownerName = r.Owner.Login
	}
	return &provider.PlatformRepo{
		ID:            r.ID,
		FullName:      r.FullName,
		Name:          r.Name,
		Owner:         ownerName,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Platform:      provider.PlatformGitCode,
	}, nil
}

var _ provider.RepoManager = (*Provider)(nil)
