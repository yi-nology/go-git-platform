package gitee

import (
	"context"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListCollaborators implements provider.CollaboratorManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (Gitee's per_page caps at 100; fetch until an empty
// page).
func (p *Provider) ListCollaborators(ctx context.Context, owner, repo string) ([]*provider.Collaborator, error) {
	members, err := backendutil.AllPages(func(page int) ([]*gitee.ProjectMember, error) {
		list, _, err := p.client.Repositories.ListCollaborators(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &gitee.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListCollaborators", err)
	}
	result := make([]*provider.Collaborator, 0, len(members))
	for _, m := range members {
		result = append(result, convertCollaborator(m))
	}
	return result, nil
}

// AddCollaborator implements provider.CollaboratorManager.
func (p *Provider) AddCollaborator(ctx context.Context, owner, repo, username string, opts provider.AddCollaboratorOptions) error {
	addOpts := &gitee.AddCollaboratorOptions{}
	if opts.Permission != "" {
		addOpts.Permission = gitee.String(opts.Permission)
	}
	_, _, err := p.client.Repositories.AddCollaborator(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(username), addOpts)
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "AddCollaborator", err)
	}
	return nil
}

// RemoveCollaborator implements provider.CollaboratorManager.
func (p *Provider) RemoveCollaborator(ctx context.Context, owner, repo, username string) error {
	_, err := p.client.Repositories.RemoveCollaborator(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(username))
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "RemoveCollaborator", err)
	}
	return nil
}

var _ provider.CollaboratorManager = (*Provider)(nil)
