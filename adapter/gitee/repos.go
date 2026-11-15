package gitee

import (
	"context"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListRepos implements provider.RepoManager.
//
// Dual pagination mode: opts.Page > 0 means the caller manages paging
// explicitly and receives exactly that one page (opts.Page/opts.PerPage
// honored via NormalizePageOpts); opts.Page == 0 (the default) exhausts
// pagination via backendutil.AllPages — fetching 100 per page until an
// empty page — so every repository is returned. In full-fetch mode
// opts.PerPage only sizes the underlying requests and defaults to Gitee's
// per_page maximum of 100. Both shapes apply to the authenticated-user
// list and the by-user list alike.
func (p *Provider) ListRepos(ctx context.Context, opts provider.ListRepoOptions) ([]*model.PlatformRepo, error) {
	var repos []*gitee.Project
	var err error
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		if opts.Owner != "" {
			repos, _, err = p.client.Repositories.ListByUser(ctx, opts.Owner, &gitee.RepositoryListByUserOptions{
				Page:    gitee.Int(page),
				PerPage: gitee.Int(perPage),
			})
		} else {
			repos, _, err = p.client.Repositories.List(ctx, &gitee.RepositoryListOptions{
				Page:    gitee.Int(page),
				PerPage: gitee.Int(perPage),
			})
		}
	} else {
		repos, err = backendutil.AllPages(func(page int) ([]*gitee.Project, error) {
			if opts.Owner != "" {
				list, _, err := p.client.Repositories.ListByUser(ctx, opts.Owner, &gitee.RepositoryListByUserOptions{
					Page:    gitee.Int(page),
					PerPage: gitee.Int(100),
				})
				return list, err
			}
			list, _, err := p.client.Repositories.List(ctx, &gitee.RepositoryListOptions{
				Page:    gitee.Int(page),
				PerPage: gitee.Int(100),
			})
			return list, err
		})
	}
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "ListRepos", err)
	}
	result := make([]*model.PlatformRepo, 0, len(repos))
	for _, r := range repos {
		result = append(result, convertProject(r))
	}
	return result, nil
}

// GetRepo implements provider.RepoManager.
func (p *Provider) GetRepo(ctx context.Context, owner, repo string) (*model.PlatformRepo, error) {
	r, _, err := p.client.Repositories.Get(ctx, backendutil.Esc(owner), backendutil.Esc(repo))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "GetRepo", err)
	}
	return convertProject(r), nil
}

// DeleteRepo implements provider.RepoManager.
func (p *Provider) DeleteRepo(ctx context.Context, owner, repo string) error {
	_, err := p.client.Repositories.Delete(ctx, backendutil.Esc(owner), backendutil.Esc(repo))
	if err != nil {
		return provider.Wrap(model.PlatformGitee, "DeleteRepo", err)
	}
	return nil
}

// UpdateRepo implements provider.RepoManager. 「"" = 不变」逐字段兑现
// (issue #88)：go-gitee 的选项字段是 *string+omitempty，空串指针照上
// wire 会清空对侧字段（或 422）——非空才落线，与 UpdateIssue/UpdateCR
// 同做法。
func (p *Provider) UpdateRepo(ctx context.Context, owner, repo string, opts provider.UpdateRepoOptions) (*model.PlatformRepo, error) {
	updateOpts := &gitee.UpdateRepositoryOptions{}
	if opts.Name != "" {
		updateOpts.Name = gitee.String(opts.Name)
	}
	if opts.Description != "" {
		updateOpts.Description = gitee.String(opts.Description)
	}
	if opts.DefaultBranch != "" {
		updateOpts.DefaultBranch = gitee.String(opts.DefaultBranch)
	}
	if opts.Private != nil {
		updateOpts.Private = gitee.Bool(*opts.Private)
	}
	r, _, err := p.client.Repositories.Edit(ctx, backendutil.Esc(owner), backendutil.Esc(repo), updateOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "UpdateRepo", err)
	}
	return convertProject(r), nil
}

// ForkRepo implements provider.RepoManager.
func (p *Provider) ForkRepo(ctx context.Context, owner, repo string, opts provider.ForkRepoOptions) (*model.PlatformRepo, error) {
	forkOpts := &gitee.CreateForkOptions{}
	if opts.Organization != "" {
		forkOpts.Organization = gitee.String(opts.Organization)
	}
	r, _, err := p.client.Repositories.CreateFork(ctx, backendutil.Esc(owner), backendutil.Esc(repo), forkOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "ForkRepo", err)
	}
	return convertProject(r), nil
}

// CreateRepo implements provider.RepoManager.
func (p *Provider) CreateRepo(ctx context.Context, owner string, opts provider.CreateRepoOptions) (*model.PlatformRepo, error) {
	createOpts := &gitee.CreateRepositoryOptions{
		Name:    gitee.String(opts.Name),
		Private: gitee.Bool(opts.Private),
	}
	if opts.Description != "" {
		createOpts.Description = gitee.String(opts.Description)
	}
	if opts.AutoInit {
		createOpts.AutoInit = gitee.Bool(true)
	}

	var r *gitee.Project
	var err error
	if owner != "" {
		orgOpts := &gitee.CreateOrgRepoOptions{
			Name:        gitee.String(opts.Name),
			Description: gitee.String(opts.Description),
			AutoInit:    gitee.Bool(opts.AutoInit),
		}
		if opts.Private {
			orgOpts.Private = gitee.Bool(true)
		}
		r, _, err = p.client.Repositories.CreateInOrg(ctx, backendutil.Esc(owner), orgOpts)
	} else {
		r, _, err = p.client.Repositories.Create(ctx, createOpts)
	}
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitee, "CreateRepo", err)
	}
	return convertProject(r), nil
}

var _ provider.RepoManager = (*Provider)(nil)
