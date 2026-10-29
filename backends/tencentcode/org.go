package tencentcode

import (
	"context"

	gongfeng "github.com/yi-nology/go-git-platform/backends/tencentcode/internal/gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// TencentCodeOrgExtras exposes the 工蜂 organization surface the unified
// provider contract does not model: project-group (项目组) CRUD plus group
// member management, and namespace lookup for resolving where new projects
// can live. Consumers type-assert:
//
//	if ox, ok := p.(tencentcode.TencentCodeOrgExtras); ok { ... }
//
// Group IDs are the platform's numeric group identifiers (visible in the
// web URL), not "owner" strings. Member mutations take numeric user IDs —
// resolve usernames through provider.UsersManager.ResolveUsernames first.
type TencentCodeOrgExtras interface {
	// ListGroups walks the group list (0 = all pages, budget-capped).
	ListGroups(ctx context.Context, opts ListGroupsOptions) ([]*GroupInfo, error)
	// GetGroup fetches one group by numeric ID.
	GetGroup(ctx context.Context, groupID int) (*GroupInfo, error)
	// CreateGroup creates a project group.
	CreateGroup(ctx context.Context, opts CreateGroupOptions) (*GroupInfo, error)
	// EditGroup updates a group's editable fields.
	EditGroup(ctx context.Context, groupID int, opts EditGroupOptions) (*GroupInfo, error)
	// DeleteGroup removes a group.
	DeleteGroup(ctx context.Context, groupID int) error
	// ListGroupMembers walks a group's member list (0 = all pages).
	ListGroupMembers(ctx context.Context, groupID int, opts ListGroupMembersOptions) ([]*MemberInfo, error)
	// AddGroupMember adds a user to the group at the given access level
	// (gongfeng AccessLevel* constants).
	AddGroupMember(ctx context.Context, groupID int, opts AddGroupMemberOptions) (*MemberInfo, error)
	// EditGroupMember changes a member's access level.
	EditGroupMember(ctx context.Context, groupID, userID int, opts EditGroupMemberOptions) (*MemberInfo, error)
	// DeleteGroupMember removes a member from the group.
	DeleteGroupMember(ctx context.Context, groupID, userID int) error
	// ListNamespaces walks the namespaces visible to the token
	// (0 = all pages); Search narrows by name/path.
	ListNamespaces(ctx context.Context, opts ListNamespacesOptions) ([]*NamespaceInfo, error)
}

// ListGroupsOptions are the ListGroups knobs.
type ListGroupsOptions struct {
	Page    int    // 0 walks every page
	PerPage int    // 0 = platform default
	Search  string // empty = no filter
}

// CreateGroupOptions are the CreateGroup inputs; Name and Path are required
// by the platform.
type CreateGroupOptions struct {
	Name        string
	Path        string
	Description string
}

// EditGroupOptions are the EditGroup inputs; zero values stay unchanged.
type EditGroupOptions struct {
	Name        string
	Description string
}

// ListGroupMembersOptions are the ListGroupMembers knobs.
type ListGroupMembersOptions struct {
	Page    int // 0 walks every page
	PerPage int // 0 = platform default
}

// AddGroupMemberOptions are the AddGroupMember inputs. AccessLevel uses the
// gongfeng AccessLevel* constants (0 = platform default).
type AddGroupMemberOptions struct {
	UserID      int
	AccessLevel gongfeng.AccessLevelValue
}

// EditGroupMemberOptions are the EditGroupMember inputs.
type EditGroupMemberOptions struct {
	AccessLevel gongfeng.AccessLevelValue
}

// ListNamespacesOptions are the ListNamespaces knobs.
type ListNamespacesOptions struct {
	Page    int    // 0 walks every page
	PerPage int    // 0 = platform default
	Search  string // empty = no filter
}

// GroupInfo is the platform group as the backend surfaces it.
type GroupInfo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	WebURL      string `json:"web_url"`
	Description string `json:"description"`
}

// NamespaceInfo is a namespace (user or group) projects can be created in.
type NamespaceInfo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	WebURL      string `json:"web_url"`
	Description string `json:"description"`
}

// MemberInfo is a project/group member with the platform-native access
// level (gongfeng AccessLevel* constants).
type MemberInfo struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	AccessLevel int    `json:"access_level"`
	State       string `json:"state"`
	WebURL      string `json:"web_url"`
	AvatarURL   string `json:"avatar_url"`
}

// ListGroups implements TencentCodeOrgExtras.
func (p *Provider) ListGroups(ctx context.Context, opts ListGroupsOptions) ([]*GroupInfo, error) {
	groups, err := walkOrgPages(opts.Page, func(page int) ([]*gongfeng.Group, error) {
		list, _, err := p.client.Groups.ListGroups(ctx, &gongfeng.ListGroupsOptions{
			ListOptions: listOptions(page, opts.PerPage),
			Search:      gongfeng.Ptr(opts.Search),
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListGroups", err)
	}
	result := make([]*GroupInfo, 0, len(groups))
	for _, g := range groups {
		result = append(result, convertGroup(g))
	}
	return result, nil
}

// GetGroup implements TencentCodeOrgExtras.
func (p *Provider) GetGroup(ctx context.Context, groupID int) (*GroupInfo, error) {
	g, _, err := p.client.Groups.GetGroup(ctx, groupID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetGroup", err)
	}
	return convertGroup(g), nil
}

// CreateGroup implements TencentCodeOrgExtras.
func (p *Provider) CreateGroup(ctx context.Context, opts CreateGroupOptions) (*GroupInfo, error) {
	g, _, err := p.client.Groups.CreateGroup(ctx, &gongfeng.CreateGroupOptions{
		Name:        gongfeng.Ptr(opts.Name),
		Path:        gongfeng.Ptr(opts.Path),
		Description: gongfeng.Ptr(opts.Description),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateGroup", err)
	}
	return convertGroup(g), nil
}

// EditGroup implements TencentCodeOrgExtras. The platform's edit endpoint
// is a PUT to the group collection carrying the group ID in the body.
func (p *Provider) EditGroup(ctx context.Context, groupID int, opts EditGroupOptions) (*GroupInfo, error) {
	editOpts := &gongfeng.EditGroupOptions{ID: gongfeng.Ptr(groupID)}
	if opts.Name != "" {
		editOpts.Name = gongfeng.Ptr(opts.Name)
	}
	if opts.Description != "" {
		editOpts.Description = gongfeng.Ptr(opts.Description)
	}
	g, _, err := p.client.Groups.EditGroup(ctx, editOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "EditGroup", err)
	}
	return convertGroup(g), nil
}

// DeleteGroup implements TencentCodeOrgExtras.
func (p *Provider) DeleteGroup(ctx context.Context, groupID int) error {
	if _, err := p.client.Groups.DeleteGroup(ctx, groupID); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteGroup", err)
	}
	return nil
}

// ListGroupMembers implements TencentCodeOrgExtras.
func (p *Provider) ListGroupMembers(ctx context.Context, groupID int, opts ListGroupMembersOptions) ([]*MemberInfo, error) {
	members, err := walkOrgPages(opts.Page, func(page int) ([]*gongfeng.Member, error) {
		list, _, err := p.client.Groups.ListGroupMembers(ctx, groupID, &gongfeng.ListGroupMembersOptions{
			ListOptions: listOptions(page, opts.PerPage),
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListGroupMembers", err)
	}
	result := make([]*MemberInfo, 0, len(members))
	for _, m := range members {
		result = append(result, convertMemberInfo(m))
	}
	return result, nil
}

// AddGroupMember implements TencentCodeOrgExtras.
func (p *Provider) AddGroupMember(ctx context.Context, groupID int, opts AddGroupMemberOptions) (*MemberInfo, error) {
	addOpts := &gongfeng.AddGroupMemberOptions{UserID: gongfeng.Ptr(opts.UserID)}
	if opts.AccessLevel != 0 {
		addOpts.AccessLevel = gongfeng.Ptr(opts.AccessLevel)
	}
	m, _, err := p.client.Groups.AddGroupMember(ctx, groupID, addOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "AddGroupMember", err)
	}
	return convertMemberInfo(m), nil
}

// EditGroupMember implements TencentCodeOrgExtras.
func (p *Provider) EditGroupMember(ctx context.Context, groupID, userID int, opts EditGroupMemberOptions) (*MemberInfo, error) {
	m, _, err := p.client.Groups.EditGroupMember(ctx, groupID, userID,
		&gongfeng.EditGroupMemberOptions{AccessLevel: gongfeng.Ptr(opts.AccessLevel)})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "EditGroupMember", err)
	}
	return convertMemberInfo(m), nil
}

// DeleteGroupMember implements TencentCodeOrgExtras.
func (p *Provider) DeleteGroupMember(ctx context.Context, groupID, userID int) error {
	if _, err := p.client.Groups.DeleteGroupMember(ctx, groupID, userID); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteGroupMember", err)
	}
	return nil
}

// ListNamespaces implements TencentCodeOrgExtras.
func (p *Provider) ListNamespaces(ctx context.Context, opts ListNamespacesOptions) ([]*NamespaceInfo, error) {
	namespaces, err := walkOrgPages(opts.Page, func(page int) ([]*gongfeng.Namespace, error) {
		list, _, err := p.client.Namespaces.ListNamespaces(ctx, &gongfeng.ListNamespacesOptions{
			ListOptions: listOptions(page, opts.PerPage),
			Search:      gongfeng.Ptr(opts.Search),
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListNamespaces", err)
	}
	result := make([]*NamespaceInfo, 0, len(namespaces))
	for _, n := range namespaces {
		result = append(result, convertNamespace(n))
	}
	return result, nil
}

// walkOrgPages either walks every page through backendutil.AllPages (page
// 0) or fetches the single requested page, mirroring the PageList idiom of
// the Gitea-family backends.
func walkOrgPages[T any](page int, fetch func(page int) ([]T, error)) ([]T, error) {
	if page > 0 {
		return fetch(page)
	}
	return backendutil.AllPages(fetch)
}

// listOptions normalizes caller knobs into the SDK's ListOptions.
func listOptions(page, perPage int) gongfeng.ListOptions {
	lo := gongfeng.ListOptions{Page: page}
	if perPage > 0 {
		lo.PerPage = perPage
	}
	return lo
}

func convertGroup(g *gongfeng.Group) *GroupInfo {
	if g == nil {
		return nil
	}
	return &GroupInfo{
		ID:          g.ID,
		Name:        g.Name,
		Path:        g.Path,
		WebURL:      g.WebURL,
		Description: g.Description,
	}
}

func convertNamespace(n *gongfeng.Namespace) *NamespaceInfo {
	if n == nil {
		return nil
	}
	return &NamespaceInfo{
		ID:          n.ID,
		Name:        n.Name,
		Path:        n.Path,
		Kind:        n.Kind,
		WebURL:      n.WebURL,
		Description: n.Description,
	}
}

func convertMemberInfo(m *gongfeng.Member) *MemberInfo {
	if m == nil {
		return nil
	}
	return &MemberInfo{
		ID:          m.ID,
		Username:    m.Username,
		Name:        m.Name,
		AccessLevel: int(m.AccessLevel),
		State:       m.State,
		WebURL:      m.WebURL,
		AvatarURL:   m.AvatarURL,
	}
}

// Compile-time guarantee that *Provider satisfies TencentCodeOrgExtras.
var _ TencentCodeOrgExtras = (*Provider)(nil)
