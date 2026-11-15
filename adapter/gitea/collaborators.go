package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/adapter/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListCollaborators implements provider.CollaboratorManager.
func (p *Provider) ListCollaborators(ctx context.Context, owner, repo string) ([]*model.Collaborator, error) {
	return giteafamily.ListCollaborators(ctx, p.family, p.ports, owner, repo)
}

// AddCollaborator implements provider.CollaboratorManager.
func (p *Provider) AddCollaborator(ctx context.Context, owner, repo, username string, opts provider.AddCollaboratorOptions) error {
	return giteafamily.AddCollaborator(ctx, p.family, p.ports, owner, repo, username, opts)
}

// RemoveCollaborator implements provider.CollaboratorManager.
func (p *Provider) RemoveCollaborator(ctx context.Context, owner, repo, username string) error {
	return giteafamily.RemoveCollaborator(ctx, p.family, p.ports, owner, repo, username)
}

var _ provider.CollaboratorManager = (*Provider)(nil)
