package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListIssueReactions implements provider.ReactionManager.
func (p *Provider) ListIssueReactions(ctx context.Context, owner, repo, number string) ([]*provider.Reaction, error) {
	return giteafamily.ListIssueReactions(ctx, p.family, p.ports, owner, repo, number)
}

// AddIssueReaction implements provider.ReactionManager.
func (p *Provider) AddIssueReaction(ctx context.Context, owner, repo, number, emoji string) (*provider.Reaction, error) {
	return giteafamily.AddIssueReaction(ctx, p.family, p.ports, owner, repo, number, emoji)
}

// RemoveIssueReaction implements provider.ReactionManager.
func (p *Provider) RemoveIssueReaction(ctx context.Context, owner, repo, number string, reactionID int64) error {
	return giteafamily.RemoveIssueReaction(ctx, p.family, p.ports, owner, repo, number, reactionID)
}

// ListCRReactions implements provider.ReactionManager.
func (p *Provider) ListCRReactions(ctx context.Context, owner, repo, number string) ([]*provider.Reaction, error) {
	return giteafamily.ListCRReactions(ctx, p.family, p.ports, owner, repo, number)
}

// ListIssueCommentReactions implements provider.ReactionManager.
func (p *Provider) ListIssueCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*provider.Reaction, error) {
	return giteafamily.ListIssueCommentReactions(ctx, p.family, p.ports, owner, repo, commentID)
}

// AddIssueCommentReaction implements provider.ReactionManager.
func (p *Provider) AddIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*provider.Reaction, error) {
	return giteafamily.AddIssueCommentReaction(ctx, p.family, p.ports, owner, repo, commentID, emoji)
}

// RemoveIssueCommentReaction implements provider.ReactionManager.
func (p *Provider) RemoveIssueCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) error {
	return giteafamily.RemoveIssueCommentReaction(ctx, p.family, p.ports, owner, repo, commentID, reactionID)
}

// ListCRCommentReactions implements provider.ReactionManager (the family has
// no PR-comment reaction API; returns empty).
func (p *Provider) ListCRCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*provider.Reaction, error) {
	return giteafamily.ListCRCommentReactions(ctx, p.family, p.ports, owner, repo, commentID)
}

// AddCRCommentReaction implements provider.ReactionManager (unsupported on
// the family).
func (p *Provider) AddCRCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*provider.Reaction, error) {
	return giteafamily.AddCRCommentReaction(ctx, p.family, p.ports, owner, repo, commentID, emoji)
}

// RemoveCRCommentReaction implements provider.ReactionManager (unsupported on
// the family).
func (p *Provider) RemoveCRCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) error {
	return giteafamily.RemoveCRCommentReaction(ctx, p.family, p.ports, owner, repo, commentID, reactionID)
}

var _ provider.ReactionManager = (*Provider)(nil)
