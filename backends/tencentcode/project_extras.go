package tencentcode

import (
	"context"
	"time"

	gongfeng "github.com/yi-nology/go-git-platform/backends/tencentcode/internal/gongfeng"

	"github.com/yi-nology/go-git-platform/provider"
)

// TencentCodeProjectExtras exposes the 工蜂 project-level surfaces the
// unified provider contract does not model: watchers (关注人), stars,
// fork-relation management, direct tag writes, project↔group shares, and
// the activity feed. Consumers type-assert:
//
//	if px, ok := p.(tencentcode.TencentCodeProjectExtras); ok { ... }
//
// All list methods take page/perPage ints; perPage <= 0 uses the platform
// default and page <= 0 is treated as 1 by the platform.
type TencentCodeProjectExtras interface {
	// ListWatchers lists a project's watchers.
	ListWatchers(ctx context.Context, owner, repo string, page, perPage int) ([]*WatcherInfo, error)
	// GetWatchStatus reports whether the token's user watches the project.
	GetWatchStatus(ctx context.Context, owner, repo string) (bool, error)
	// WatchProject makes the token's user watch the project.
	WatchProject(ctx context.Context, owner, repo string) (*WatcherInfo, error)
	// UnwatchProject removes the token's user's watch.
	UnwatchProject(ctx context.Context, owner, repo string) error

	// StarProject stars the project (reports the previous state).
	StarProject(ctx context.Context, owner, repo string) (bool, error)
	// UnstarProject removes the token's user's star.
	UnstarProject(ctx context.Context, owner, repo string) error
	// GetStarStatus reports whether the token's user starred the project.
	GetStarStatus(ctx context.Context, owner, repo string) (bool, error)
	// ListProjectStars lists the project's stargazers.
	ListProjectStars(ctx context.Context, owner, repo string, page, perPage int) ([]*StarInfo, error)

	// CreateForkRelation links this project to the upstream project ID it
	// was forked from; DeleteForkRelation removes the link.
	CreateForkRelation(ctx context.Context, owner, repo string, forkedFromID int) error
	DeleteForkRelation(ctx context.Context, owner, repo string) error

	// CreateTag creates a lightweight/annotated tag at Ref; GetTag fetches
	// one tag; DeleteTag removes it. (ReleaseManager.ListTags only reads.)
	CreateTag(ctx context.Context, owner, repo string, opts CreateTagOptions) (*TagDetail, error)
	GetTag(ctx context.Context, owner, repo, tag string) (*TagDetail, error)
	DeleteTag(ctx context.Context, owner, repo, tag string) error

	// ShareProject grants a group access to the project; ListProjectShares
	// lists the grants; DeleteProjectShare revokes one by group ID.
	ShareProject(ctx context.Context, owner, repo string, opts ShareProjectOptions) error
	ListProjectShares(ctx context.Context, owner, repo string) ([]*ProjectShareInfo, error)
	DeleteProjectShare(ctx context.Context, owner, repo string, groupID int) error

	// ListProjectEvents lists the project's activity feed.
	ListProjectEvents(ctx context.Context, owner, repo string, opts ListProjectEventsOptions) ([]*ProjectEventInfo, error)

	// --- Project members admin (beyond provider.CollaboratorManager) ---

	// GetProjectMember fetches one member by numeric user ID.
	GetProjectMember(ctx context.Context, owner, repo string, userID int) (*MemberInfo, error)
	// EditProjectMember changes a member's access level (gongfeng
	// AccessLevel* constants).
	EditProjectMember(ctx context.Context, owner, repo string, userID int, opts EditProjectMemberOptions) (*MemberInfo, error)
}

// EditProjectMemberOptions are the EditProjectMember inputs.
type EditProjectMemberOptions struct {
	AccessLevel gongfeng.AccessLevelValue
}

// CreateTagOptions are the CreateTag inputs.
type CreateTagOptions struct {
	TagName string // tag name to create
	Ref     string // commit/branch to point the tag at
	Message string // empty = lightweight tag
}

// ShareProjectOptions are the ShareProject inputs. GroupAccess uses the
// gongfeng AccessLevel* constants.
type ShareProjectOptions struct {
	GroupID     int
	GroupAccess gongfeng.AccessLevelValue
}

// ListProjectEventsOptions are the ListProjectEvents knobs.
type ListProjectEventsOptions struct {
	Page    int
	PerPage int
}

// WatcherInfo is a project watcher as the backend surfaces it.
type WatcherInfo struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Mute     bool   `json:"mute"`
}

// StarInfo is a project stargazer as the backend surfaces it.
type StarInfo struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// TagDetail is a single tag with its message and target commit.
type TagDetail struct {
	Name      string    `json:"name"`
	Message   string    `json:"message"`
	CommitSHA string    `json:"commit_sha"`
	CreatedAt time.Time `json:"created_at"`
}

// ProjectShareInfo is a project→group access grant.
type ProjectShareInfo struct {
	ProjectID   int       `json:"project_id"`
	GroupID     int       `json:"group_id"`
	GroupAccess int       `json:"group_access"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProjectEventInfo is one activity-feed entry.
type ProjectEventInfo struct {
	ActionName     string    `json:"action_name"`
	Title          string    `json:"title"`
	TargetType     string    `json:"target_type"`
	TargetTitle    string    `json:"target_title"`
	TargetID       int       `json:"target_id"`
	AuthorUsername string    `json:"author_username"`
	CreatedAt      time.Time `json:"created_at"`
}

// ListWatchers implements TencentCodeProjectExtras.
func (p *Provider) ListWatchers(ctx context.Context, owner, repo string, page, perPage int) ([]*WatcherInfo, error) {
	list, _, err := p.client.Watchers.ListWatchers(ctx, pid(owner, repo),
		&gongfeng.ListWatchersOptions{ListOptions: listOptions(page, perPage)})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListWatchers", err)
	}
	result := make([]*WatcherInfo, 0, len(list))
	for _, w := range list {
		info := &WatcherInfo{Mute: w.Mute}
		if w.User != nil {
			info.Username, info.Name = w.User.Username, w.User.Name
		}
		result = append(result, info)
	}
	return result, nil
}

// GetWatchStatus implements TencentCodeProjectExtras.
func (p *Provider) GetWatchStatus(ctx context.Context, owner, repo string) (bool, error) {
	watching, _, err := p.client.Watchers.GetWatchStatus(ctx, pid(owner, repo))
	if err != nil {
		return false, provider.Wrap(provider.PlatformTencentCode, "GetWatchStatus", err)
	}
	return watching, nil
}

// WatchProject implements TencentCodeProjectExtras.
func (p *Provider) WatchProject(ctx context.Context, owner, repo string) (*WatcherInfo, error) {
	w, _, err := p.client.Watchers.WatchProject(ctx, pid(owner, repo), &gongfeng.WatchProjectOptions{})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "WatchProject", err)
	}
	info := &WatcherInfo{Mute: w.Mute}
	if w.User != nil {
		info.Username, info.Name = w.User.Username, w.User.Name
	}
	return info, nil
}

// UnwatchProject implements TencentCodeProjectExtras.
func (p *Provider) UnwatchProject(ctx context.Context, owner, repo string) error {
	if _, err := p.client.Watchers.UnwatchProject(ctx, pid(owner, repo)); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "UnwatchProject", err)
	}
	return nil
}

// StarProject implements TencentCodeProjectExtras.
func (p *Provider) StarProject(ctx context.Context, owner, repo string) (bool, error) {
	starred, _, err := p.client.Projects.StarProject(ctx, pid(owner, repo))
	if err != nil {
		return false, provider.Wrap(provider.PlatformTencentCode, "StarProject", err)
	}
	return starred, nil
}

// UnstarProject implements TencentCodeProjectExtras.
func (p *Provider) UnstarProject(ctx context.Context, owner, repo string) error {
	if _, err := p.client.Projects.UnstarProject(ctx, pid(owner, repo)); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "UnstarProject", err)
	}
	return nil
}

// GetStarStatus implements TencentCodeProjectExtras.
func (p *Provider) GetStarStatus(ctx context.Context, owner, repo string) (bool, error) {
	starred, _, err := p.client.Projects.GetStarStatus(ctx, pid(owner, repo))
	if err != nil {
		return false, provider.Wrap(provider.PlatformTencentCode, "GetStarStatus", err)
	}
	return starred, nil
}

// ListProjectStars implements TencentCodeProjectExtras.
func (p *Provider) ListProjectStars(ctx context.Context, owner, repo string, page, perPage int) ([]*StarInfo, error) {
	list, _, err := p.client.Projects.ListProjectStars(ctx, pid(owner, repo),
		&gongfeng.ListProjectStarsOptions{ListOptions: listOptions(page, perPage)})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListProjectStars", err)
	}
	result := make([]*StarInfo, 0, len(list))
	for _, s := range list {
		info := &StarInfo{}
		if s.User != nil {
			info.Username, info.Name = s.User.Username, s.User.Name
		}
		result = append(result, info)
	}
	return result, nil
}

// CreateForkRelation implements TencentCodeProjectExtras.
func (p *Provider) CreateForkRelation(ctx context.Context, owner, repo string, forkedFromID int) error {
	if _, err := p.client.Forks.CreateForkRelation(ctx, pid(owner, repo), forkedFromID); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "CreateForkRelation", err)
	}
	return nil
}

// DeleteForkRelation implements TencentCodeProjectExtras.
func (p *Provider) DeleteForkRelation(ctx context.Context, owner, repo string) error {
	if _, err := p.client.Forks.DeleteForkRelation(ctx, pid(owner, repo)); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteForkRelation", err)
	}
	return nil
}

// CreateTag implements TencentCodeProjectExtras.
func (p *Provider) CreateTag(ctx context.Context, owner, repo string, opts CreateTagOptions) (*TagDetail, error) {
	t, _, err := p.client.Tags.CreateTag(ctx, pid(owner, repo), &gongfeng.CreateTagOptions{
		TagName: gongfeng.Ptr(opts.TagName),
		Ref:     gongfeng.Ptr(opts.Ref),
		Message: gongfeng.Ptr(opts.Message),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateTag", err)
	}
	return convertTagDetail(t), nil
}

// GetTag implements TencentCodeProjectExtras.
func (p *Provider) GetTag(ctx context.Context, owner, repo, tag string) (*TagDetail, error) {
	t, _, err := p.client.Tags.GetTag(ctx, pid(owner, repo), tag)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetTag", err)
	}
	return convertTagDetail(t), nil
}

// DeleteTag implements TencentCodeProjectExtras.
func (p *Provider) DeleteTag(ctx context.Context, owner, repo, tag string) error {
	if _, err := p.client.Tags.DeleteTag(ctx, pid(owner, repo), tag); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteTag", err)
	}
	return nil
}

// ShareProject implements TencentCodeProjectExtras.
func (p *Provider) ShareProject(ctx context.Context, owner, repo string, opts ShareProjectOptions) error {
	_, err := p.client.Projects.ShareProject(ctx, pid(owner, repo), &gongfeng.ShareProjectOptions{
		GroupID:     gongfeng.Ptr(opts.GroupID),
		GroupAccess: gongfeng.Ptr(int(opts.GroupAccess)),
	})
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "ShareProject", err)
	}
	return nil
}

// ListProjectShares implements TencentCodeProjectExtras.
func (p *Provider) ListProjectShares(ctx context.Context, owner, repo string) ([]*ProjectShareInfo, error) {
	list, _, err := p.client.Projects.ListProjectShares(ctx, pid(owner, repo))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListProjectShares", err)
	}
	result := make([]*ProjectShareInfo, 0, len(list))
	for _, s := range list {
		result = append(result, &ProjectShareInfo{
			ProjectID:   s.ProjectID,
			GroupID:     s.GroupID,
			GroupAccess: s.GroupAccess,
			CreatedAt:   s.CreatedAt.Time,
			UpdatedAt:   s.UpdatedAt.Time,
		})
	}
	return result, nil
}

// DeleteProjectShare implements TencentCodeProjectExtras.
func (p *Provider) DeleteProjectShare(ctx context.Context, owner, repo string, groupID int) error {
	if _, err := p.client.Projects.DeleteProjectShare(ctx, pid(owner, repo), groupID); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteProjectShare", err)
	}
	return nil
}

// ListProjectEvents implements TencentCodeProjectExtras.
func (p *Provider) ListProjectEvents(ctx context.Context, owner, repo string, opts ListProjectEventsOptions) ([]*ProjectEventInfo, error) {
	list, _, err := p.client.Projects.ListProjectEvents(ctx, pid(owner, repo),
		&gongfeng.ListProjectEventsOptions{ListOptions: listOptions(opts.Page, opts.PerPage)})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListProjectEvents", err)
	}
	result := make([]*ProjectEventInfo, 0, len(list))
	for _, e := range list {
		result = append(result, &ProjectEventInfo{
			ActionName:     e.ActionName,
			Title:          e.Title,
			TargetType:     e.TargetType,
			TargetTitle:    e.TargetTitle,
			TargetID:       e.TargetID,
			AuthorUsername: e.AuthorUsername,
			CreatedAt:      e.CreatedAt.Time,
		})
	}
	return result, nil
}

// GetProjectMember implements TencentCodeProjectExtras.
func (p *Provider) GetProjectMember(ctx context.Context, owner, repo string, userID int) (*MemberInfo, error) {
	m, _, err := p.client.Projects.GetProjectMember(ctx, pid(owner, repo), userID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetProjectMember", err)
	}
	return convertMemberInfo(m), nil
}

// EditProjectMember implements TencentCodeProjectExtras.
func (p *Provider) EditProjectMember(ctx context.Context, owner, repo string, userID int, opts EditProjectMemberOptions) (*MemberInfo, error) {
	m, _, err := p.client.Projects.EditProjectMember(ctx, pid(owner, repo), userID,
		&gongfeng.EditProjectMemberOptions{AccessLevel: gongfeng.Ptr(opts.AccessLevel)})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "EditProjectMember", err)
	}
	return convertMemberInfo(m), nil
}

// convertTagDetail maps a gongfeng.Tag to the TagDetail surface.
func convertTagDetail(t *gongfeng.Tag) *TagDetail {
	detail := &TagDetail{Name: t.Name, Message: t.Message}
	if t.Commit != nil {
		detail.CommitSHA = t.Commit.ID
	}
	return detail
}

// Compile-time guarantee that *Provider satisfies TencentCodeProjectExtras.
var _ TencentCodeProjectExtras = (*Provider)(nil)
