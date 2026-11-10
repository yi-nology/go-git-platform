package provider

import "context"

// ReactionManager provides emoji reaction CRUD on issues and their comments
// (both issue comments and change-request comments). It is an optional
// capability interface: consumers should gate on
// Provider.Capabilities().Reactions (or type-assert) before use.
//
// Platform support: GitHub, GitLab (issues/CRs via award-emoji; the
// note-level half is a registered stub family, issue #57), Gitea, Forgejo
// (PR-level reactions ride the issue endpoints — a PR is an issue sharing
// the same index). Gitee, GitCode and TencentCode have no reaction API and
// report Reactions=false in CapabilitySet.
//
// GitLab maps reactions to its "award emoji" API; the SDK normalizes emoji
// names (e.g. +1 ↔ thumbsup, hooray ↔ tada) so callers use the same
// constants across all platforms.
type ReactionManager interface {
	// ListIssueReactions returns all reactions on an issue.
	ListIssueReactions(ctx context.Context, owner, repo, number string) ([]*Reaction, error)
	// AddIssueReaction adds a reaction emoji to an issue. emoji is one of
	// the Reaction* constants (e.g. ReactionHeart).
	AddIssueReaction(ctx context.Context, owner, repo, number, emoji string) (*Reaction, error)
	// RemoveIssueReaction removes a reaction by its platform ID.
	RemoveIssueReaction(ctx context.Context, owner, repo, number string, reactionID int64) error

	// ListIssueCommentReactions returns all reactions on an issue comment.
	ListIssueCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*Reaction, error)
	// AddIssueCommentReaction adds a reaction emoji to an issue comment.
	AddIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*Reaction, error)
	// RemoveIssueCommentReaction removes a reaction by its platform ID.
	RemoveIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, reactionID int64) error

	// ListCRReactions returns all reactions on a change request (PR/MR).
	ListCRReactions(ctx context.Context, owner, repo, number string) ([]*Reaction, error)
	// ListCRCommentReactions returns all reactions on a change-request
	// comment (PR review comment / MR note).
	ListCRCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*Reaction, error)
	// AddCRCommentReaction adds a reaction emoji to a change-request comment.
	AddCRCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*Reaction, error)
	// RemoveCRCommentReaction removes a reaction by its platform ID.
	RemoveCRCommentReaction(ctx context.Context, owner, repo string, commentID int64, reactionID int64) error
}
