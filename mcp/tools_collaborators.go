package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- collaborators (toolset "collaborators") ---

type addCollaboratorIn struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	User       string `json:"user" jsonschema:"username to add"`
	Permission string `json:"permission,omitempty" jsonschema:"read|write|admin; empty = platform default"`
}

func registerCollaborators(s *mcp.Server, st *state) {
	cm, ok := st.p.(provider.CollaboratorManager)
	if !ok {
		return
	}

	add(s, st, "list_collaborators", "List collaborators", "List the collaborators (members) of a repository with their permissions.", false, func(ctx context.Context, in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}) ([]*provider.Collaborator, error) {
		return cm.ListCollaborators(ctx, in.Owner, in.Repo)
	})

	add(s, st, "add_collaborator", "Add collaborator", "Grant a user access to a repository. Permission vocabulary read/write/admin maps onto platform-native levels (e.g. TencentCode access levels — registered mapping).", true, func(ctx context.Context, in addCollaboratorIn) (map[string]any, error) {
		switch in.Permission {
		case "", "read", "write", "admin":
		default:
			return nil, fmt.Errorf("invalid permission %q (read|write|admin)", in.Permission)
		}
		if err := cm.AddCollaborator(ctx, in.Owner, in.Repo, in.User,
			provider.AddCollaboratorOptions{Permission: in.Permission}); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	add(s, st, "remove_collaborator", "Remove collaborator", "Revoke a user's access to a repository.", true, func(ctx context.Context, in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		User  string `json:"user" jsonschema:"username to remove"`
	}) (map[string]any, error) {
		if err := cm.RemoveCollaborator(ctx, in.Owner, in.Repo, in.User); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})
}
