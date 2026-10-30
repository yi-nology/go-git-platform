package gitlab

import (
	"context"
	"strconv"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListCollaborators implements provider.CollaboratorManager over GitLab's
// project-members API. The endpoint has no caller-facing pagination knobs,
// so every page is fetched via AllPages (GitLab's page-size ceiling is 100).
func (p *Provider) ListCollaborators(ctx context.Context, owner, repo string) ([]*provider.Collaborator, error) {
	members, err := backendutil.AllPages(func(page int) ([]*gitlab.ProjectMember, error) {
		list, _, err := p.client.ProjectMembers.ListProjectMembers(pidOf(owner, repo),
			&gitlab.ListProjectMembersOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}},
			gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListCollaborators", err)
	}
	result := make([]*provider.Collaborator, 0, len(members))
	for _, m := range members {
		result = append(result, convertMember(m))
	}
	return result, nil
}

// AddCollaborator implements provider.CollaboratorManager. GitLab members
// are ID-addressed and the add-member endpoint requires an explicit access
// level (no platform default), so an empty permission maps to developer
// (registered mapping).
func (p *Provider) AddCollaborator(ctx context.Context, owner, repo, username string, opts provider.AddCollaboratorOptions) error {
	ids, err := p.resolveUserIDs(ctx, "AddCollaborator", []string{username})
	if err != nil {
		return err
	}
	_, _, err = p.client.ProjectMembers.AddProjectMember(pidOf(owner, repo),
		&gitlab.AddProjectMemberOptions{
			UserID:      new(ids[0]),
			AccessLevel: new(permissionToAccessLevel(opts.Permission)),
		},
		gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(provider.PlatformGitLab, "AddCollaborator", err)
	}
	return nil
}

// RemoveCollaborator implements provider.CollaboratorManager via the
// username→ID resolution (the members API is ID-addressed).
func (p *Provider) RemoveCollaborator(ctx context.Context, owner, repo, username string) error {
	ids, err := p.resolveUserIDs(ctx, "RemoveCollaborator", []string{username})
	if err != nil {
		return err
	}
	_, err = p.client.ProjectMembers.DeleteProjectMember(pidOf(owner, repo), ids[0], gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(provider.PlatformGitLab, "RemoveCollaborator", err)
	}
	return nil
}

// permissionToAccessLevel maps the provider permission vocabulary onto
// GitLab access levels. Unknown or empty strings map to developer: the
// add-member endpoint has no platform default and rejects a missing
// access_level outright.
func permissionToAccessLevel(permission string) gitlab.AccessLevelValue {
	switch permission {
	case "read":
		return gitlab.ReporterPermissions
	case "write":
		return gitlab.DeveloperPermissions
	case "admin":
		return gitlab.MaintainerPermissions
	default:
		return gitlab.DeveloperPermissions
	}
}

// accessLevelName renders an access level as the conventional GitLab name,
// falling back to the raw number for values the platform may add later.
func accessLevelName(level gitlab.AccessLevelValue) string {
	switch level {
	case gitlab.GuestPermissions:
		return "guest"
	case gitlab.ReporterPermissions:
		return "reporter"
	case gitlab.DeveloperPermissions:
		return "developer"
	case gitlab.MaintainerPermissions:
		return "maintainer"
	case gitlab.OwnerPermissions:
		return "owner"
	default:
		return strconv.Itoa(int(level))
	}
}

// convertMember maps a gitlab.ProjectMember to the provider collaborator
// shape; User stays nil (the member payload already carries the flattened
// user fields).
func convertMember(m *gitlab.ProjectMember) *provider.Collaborator {
	if m == nil {
		return nil
	}
	return &provider.Collaborator{
		ID:         m.ID,
		Username:   m.Username,
		Permission: accessLevelName(m.AccessLevel),
		User: &provider.CRUser{
			ID:        m.ID,
			Username:  m.Username,
			Name:      m.Name,
			AvatarURL: m.AvatarURL,
		},
	}
}

var _ provider.CollaboratorManager = (*Provider)(nil)
