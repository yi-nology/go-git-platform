package provider

import "context"

// CollaboratorManager provides repository collaborator management. It is
// an optional capability interface: consumers should gate on
// Provider.Capabilities().Collaborators (or type-assert) before use.
//
// Platform support: GitHub, GitCode, GitLab, Gitea, Forgejo, Gitee,
// TencentCode. GitLab-family platforms (GitLab, TencentCode) back this
// with the project-members API: mutations resolve the username to a
// numeric user ID first (TTL-cached), and the read/write/admin permission
// vocabulary maps onto platform access levels (registered mappings).
type CollaboratorManager interface {
	// ListCollaborators returns all collaborators on a repository.
	ListCollaborators(ctx context.Context, owner, repo string) ([]*Collaborator, error)
	// AddCollaborator adds a user as a collaborator. Permission is
	// platform-specific ("read", "write", "admin"; empty = default).
	AddCollaborator(ctx context.Context, owner, repo, username string, opts AddCollaboratorOptions) error
	// RemoveCollaborator removes a user from the repository.
	RemoveCollaborator(ctx context.Context, owner, repo, username string) error
}
