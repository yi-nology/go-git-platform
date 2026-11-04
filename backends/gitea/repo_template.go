package gitea

import (
	"context"
	"net/http"

	gitea "gitea.dev/sdk"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateRepoFromTemplate implements provider.TemplateRepoManager via the
// platform's generate endpoint (POST /repos/{o}/{r}/generate).
func (p *Provider) CreateRepoFromTemplate(ctx context.Context, templateOwner, templateRepo string, opts provider.CreateRepoFromTemplateOptions) (*provider.PlatformRepo, error) {
	if opts.Name == "" {
		return nil, provider.New(provider.PlatformGitea, "CreateRepoFromTemplate", http.StatusUnprocessableEntity,
			"repo name is required")
	}
	// Gitea 的 generate 线要求显式目标 owner（SDK Validate 拒绝空值）——
	// 统一语义里的「空 = 认证用户」在该平台不可表达，提前以统一 422 拒绝。
	if opts.Owner == "" {
		return nil, provider.New(provider.PlatformGitea, "CreateRepoFromTemplate", http.StatusUnprocessableEntity,
			"owner is required on Gitea (the generate endpoint has no authenticated-user default)")
	}
	opt := gitea.CreateRepoFromTemplateOption{
		Name:        opts.Name,
		Owner:       opts.Owner,
		Description: opts.Description,
		Private:     opts.Private,
	}
	repo, _, err := p.client.Repositories.CreateRepoFromTemplate(ctx, templateOwner, templateRepo, opt)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitea, "CreateRepoFromTemplate", err)
	}
	return giteafamily.ConvertRepo(familyRepo(repo), provider.PlatformGitea), nil
}
