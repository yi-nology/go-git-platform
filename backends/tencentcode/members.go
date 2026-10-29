package tencentcode

import (
	"context"
	"strconv"

	gongfeng "github.com/yi-nology/go-git-platform/backends/tencentcode/internal/gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListCollaborators implements provider.CollaboratorManager over 工蜂's
// project-members API. The endpoint has no caller-facing pagination knobs,
// so every page is fetched via AllPages (工蜂's page-size ceiling is 100).
func (p *Provider) ListCollaborators(ctx context.Context, owner, repo string) ([]*provider.Collaborator, error) {
	pid := owner + "/" + repo
	members, err := backendutil.AllPages(func(page int) ([]*gongfeng.Member, error) {
		list, _, err := p.client.Projects.ListProjectMembers(ctx, pid,
			&gongfeng.ListProjectMembersOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListCollaborators", err)
	}
	result := make([]*provider.Collaborator, 0, len(members))
	for _, m := range members {
		result = append(result, convertMember(m))
	}
	return result, nil
}

// AddCollaborator implements provider.CollaboratorManager. 工蜂 members are
// ID-addressed, so the username resolves through the Users API; the
// read/write/admin permission maps onto the platform's access levels
// (registered mapping) and empty takes the platform default.
func (p *Provider) AddCollaborator(ctx context.Context, owner, repo, username string, opts provider.AddCollaboratorOptions) error {
	ids, err := p.resolveUserIDs(ctx, "AddCollaborator", []string{username})
	if err != nil {
		return err
	}
	addOpts := &gongfeng.AddProjectMemberOptions{UserID: gongfeng.Ptr(ids[0])}
	if level := permissionToAccessLevel(opts.Permission); level != 0 {
		addOpts.AccessLevel = gongfeng.Ptr(level)
	}
	if _, _, err := p.client.Projects.AddProjectMember(ctx, pid(owner, repo), addOpts); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "AddCollaborator", err)
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
	if _, err := p.client.Projects.DeleteProjectMember(ctx, pid(owner, repo), ids[0]); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "RemoveCollaborator", err)
	}
	return nil
}

// permissionToAccessLevel maps the provider permission vocabulary onto
// 工蜂's GitLab-style access levels. Unknown or empty strings return 0 so
// the wire omits access_level and the platform default applies.
func permissionToAccessLevel(permission string) gongfeng.AccessLevelValue {
	switch permission {
	case "read":
		return gongfeng.ReporterPermission
	case "write":
		return gongfeng.DeveloperPermission
	case "admin":
		return gongfeng.MasterPermission
	default:
		return 0
	}
}

// accessLevelName renders an access level as the conventional name, falling
// back to the raw number for values the platform may add later.
func accessLevelName(level gongfeng.AccessLevelValue) string {
	switch level {
	case gongfeng.GuestPermission:
		return "guest"
	case gongfeng.FollowerPermission:
		return "follower"
	case gongfeng.ReporterPermission:
		return "reporter"
	case gongfeng.DeveloperPermission:
		return "developer"
	case gongfeng.MasterPermission:
		return "master"
	case gongfeng.OwnerPermission:
		return "owner"
	default:
		return strconv.Itoa(int(level))
	}
}

// convertMember maps a gongfeng.Member (project or group member) to the
// provider collaborator shape; User stays nil (the member payload already
// carries the flattened user fields).
func convertMember(m *gongfeng.Member) *provider.Collaborator {
	if m == nil {
		return nil
	}
	return &provider.Collaborator{
		ID:         int64(m.ID),
		Username:   m.Username,
		Permission: accessLevelName(m.AccessLevel),
		User: &provider.CRUser{
			ID:        int64(m.ID),
			Username:  m.Username,
			Name:      m.Name,
			AvatarURL: m.AvatarURL,
		},
	}
}

var _ provider.CollaboratorManager = (*Provider)(nil)
