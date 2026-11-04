package github

import (
	"context"
	"net/http"

	sdkgithub "github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateRepoFromTemplate implements provider.TemplateRepoManager.
func (p *Provider) CreateRepoFromTemplate(ctx context.Context, templateOwner, templateRepo string, opts provider.CreateRepoFromTemplateOptions) (*provider.PlatformRepo, error) {
	if opts.Name == "" {
		return nil, provider.New(provider.PlatformGitHub, "CreateRepoFromTemplate", http.StatusUnprocessableEntity,
			"repo name is required")
	}
	body := sdkgithub.TemplateRepoRequest{
		Name:               opts.Name,
		Owner:              new(opts.Owner),
		Description:        new(opts.Description),
		Private:            new(opts.Private),
		IncludeAllBranches: new(opts.IncludeAllBranches),
	}
	repo, _, err := p.client.Repositories.CreateFromTemplate(ctx, templateOwner, templateRepo, body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateRepoFromTemplate", err)
	}
	return convertRepo(repo), nil
}

// SyncFork implements provider.ForkSyncManager via the merge-upstream
// endpoint: the fork's named branch is fast-forwarded (or merged) from its
// upstream counterpart; conflicts error and stay on the fork for manual
// resolution.
func (p *Provider) SyncFork(ctx context.Context, owner, repo, branch string) error {
	if branch == "" {
		return provider.New(provider.PlatformGitHub, "SyncFork", http.StatusUnprocessableEntity,
			"branch is required")
	}
	if _, _, err := p.client.Repositories.MergeUpstream(ctx, owner, repo, sdkgithub.RepoMergeUpstreamRequest{
		Branch: branch,
	}); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "SyncFork", err)
	}
	return nil
}
