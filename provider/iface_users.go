package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// UserManager provides user lookup and username-to-ID resolution. It is
// an optional capability interface: consumers should gate on
// Provider.Capabilities().Users (or type-assert) before use.
//
// Not every backend needs this — GitHub, Gitea, Forgejo, Gitee, and
// GitCode accept usernames directly in their APIs. GitLab and TencentCode
// require numeric user IDs for assignees and reviewers, so they implement
// this interface to expose their resolution logic.
//
// Consumers can also use this interface to look up user profiles or
// validate that a username exists before passing it to other operations.
type UserManager interface {
	// GetUser returns the user profile for the given username.
	GetUser(ctx context.Context, username string) (*model.CRUser, error)

	// ResolveUsernames maps a list of usernames to their platform IDs.
	// The returned slice preserves input order; each entry's ID field
	// carries the resolved numeric ID. If a username cannot be resolved,
	// an error is returned for that entry (the partial result up to that
	// point is still valid).
	ResolveUsernames(ctx context.Context, usernames []string) ([]*model.CRUser, error)
}
