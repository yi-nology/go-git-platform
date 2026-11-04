package github

import (
	"context"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListCollaborators implements provider.CollaboratorManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListCollaborators(ctx context.Context, owner, repo string) ([]*provider.Collaborator, error) {
	users, err := backendutil.AllPages(func(page int) ([]*github.User, error) {
		list, _, err := p.client.Repositories.ListCollaborators(ctx, owner, repo, &github.ListCollaboratorsOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: 100},
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListCollaborators", err)
	}
	result := make([]*provider.Collaborator, 0, len(users))
	for _, u := range users {
		result = append(result, convertCollaborator(u))
	}
	return result, nil
}

// AddCollaborator implements provider.CollaboratorManager.
func (p *Provider) AddCollaborator(ctx context.Context, owner, repo, username string, opts provider.AddCollaboratorOptions) error {
	// go-github v93 moved collaborator invitations onto AddCollaboratorRequest.
	body := github.AddCollaboratorRequest{Permission: new(opts.Permission)}
	if _, _, err := p.client.Repositories.AddCollaborator(ctx, owner, repo, username, body); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "AddCollaborator", err)
	}
	return nil
}

// RemoveCollaborator implements provider.CollaboratorManager.
func (p *Provider) RemoveCollaborator(ctx context.Context, owner, repo, username string) error {
	if _, err := p.client.Repositories.RemoveCollaborator(ctx, owner, repo, username); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "RemoveCollaborator", err)
	}
	return nil
}

// convertCollaborator maps a github.User to a provider.Collaborator.
// GitHub's ListCollaborators returns User objects; the permission flags are
// collapsed to the highest role found (admin > maintain > push > triage > pull).
func convertCollaborator(u *github.User) *provider.Collaborator {
	if u == nil {
		return nil
	}
	c := &provider.Collaborator{
		ID:       u.GetID(),
		Username: u.GetLogin(),
	}
	if perms := u.GetPermissions(); perms != nil {
		// GitHub returns flags like {"admin": true, "push": true}; pick the
		// highest-level permission that is set.
		switch {
		case perms.GetAdmin():
			c.Permission = "admin"
		case perms.GetMaintain():
			c.Permission = "maintain"
		case perms.GetPush():
			c.Permission = "push"
		case perms.GetTriage():
			c.Permission = "triage"
		case perms.GetPull():
			c.Permission = "pull"
		}
	}
	return c
}

var _ provider.CollaboratorManager = (*Provider)(nil)
