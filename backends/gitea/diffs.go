package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetCRDiff implements provider.DiffManager.
func (p *Provider) GetCRDiff(ctx context.Context, owner, repo, number string) (*provider.MergeDiff, error) {
	return giteafamily.GetCRDiff(ctx, p.family, p.ports, owner, repo, number)
}

// GetCRFiles implements provider.DiffManager.
func (p *Provider) GetCRFiles(ctx context.Context, owner, repo, number string) ([]*provider.ChangedFile, error) {
	return giteafamily.GetCRFiles(ctx, p.family, p.ports, owner, repo, number)
}

// CreateNote implements provider.DiffManager.
func (p *Provider) CreateNote(ctx context.Context, owner, repo, number, body string) (string, error) {
	return giteafamily.CreateNote(ctx, p.family, p.ports, owner, repo, number, body)
}

// DeleteNote implements provider.DiffManager.
func (p *Provider) DeleteNote(ctx context.Context, owner, repo, number, noteID string) error {
	return giteafamily.DeleteNote(ctx, p.family, p.ports, owner, repo, number, noteID)
}

// CreateDiscussion implements provider.DiffManager. The family has no
// threaded line comments, so a discussion collapses to one issue comment
// (the same backing store the family itself uses for PR discussions).
func (p *Provider) CreateDiscussion(ctx context.Context, owner, repo, number string, opts provider.DiscussionOptions) (string, error) {
	return giteafamily.CreateDiscussion(ctx, p.family, p.ports, owner, repo, number, opts)
}

var _ provider.DiffManager = (*Provider)(nil)
