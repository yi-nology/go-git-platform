package tencentcode

import (
	"context"
	"net/http"
	"strconv"
	"time"

	gongfeng "github.com/yi-nology/go-gongfeng"

	"github.com/yi-nology/go-git-platform/provider"
)

// TencentCodeAccountExtras exposes the 工蜂 account surface: the session
// login that exchanges credentials for a private token, the current user's
// detail, watched projects, and SSH-key/email management. Consumers
// type-assert:
//
//	if ax, ok := p.(tencentcode.TencentCodeAccountExtras); ok { ... }
//
// GetSession handles credentials directly and exists only for bootstrap
// flows (a consumer provisioning its first token); callers that already
// hold a token should never route passwords through it.
type TencentCodeAccountExtras interface {
	// GetSession exchanges a login (username or email) + password for the
	// account's private token. Bootstrap only — see above.
	GetSession(ctx context.Context, login, password string) (*SessionInfo, error)
	// GetCurrentUser returns the token owner's detailed profile.
	GetCurrentUser(ctx context.Context) (*UserDetailInfo, error)
	// GetUserDetail returns one user's detailed profile by numeric ID.
	GetUserDetail(ctx context.Context, userID int) (*UserDetailInfo, error)
	// ListWatchedProjects lists the projects the token's user watches
	// (0 = all pages, budget-capped), as PlatformRepo summaries.
	ListWatchedProjects(ctx context.Context, page, perPage int) ([]*provider.PlatformRepo, error)

	// ListSSHKeys lists the account's registered SSH keys.
	ListSSHKeys(ctx context.Context) ([]*SSHKeyInfo, error)
	// CreateSSHKey registers an SSH public key under a display title.
	CreateSSHKey(ctx context.Context, title, key string) (*SSHKeyInfo, error)
	// DeleteSSHKey removes an SSH key by numeric ID.
	DeleteSSHKey(ctx context.Context, keyID int) error

	// ListEmails lists the account's registered email addresses.
	ListEmails(ctx context.Context) ([]*EmailInfo, error)
	// CreateEmail registers an email address.
	CreateEmail(ctx context.Context, email string) (*EmailInfo, error)
	// GetEmail fetches one email address by numeric ID.
	GetEmail(ctx context.Context, emailID int) (*EmailInfo, error)
	// DeleteEmail removes an email address by numeric ID.
	DeleteEmail(ctx context.Context, emailID int) error

	// --- User lookup (platform-wide) ---

	// ListUsers walks the platform user directory (0 = all pages,
	// budget-capped); Search narrows by username/name.
	ListUsers(ctx context.Context, opts ListUsersOptions) ([]*provider.CRUser, error)
	// GetUserByEmail resolves one user by registered email address.
	GetUserByEmail(ctx context.Context, email string) (*provider.CRUser, error)

	// --- Projects owned by the token's user ---

	// ListOwnedProjects walks the projects the token's user owns
	// (0 = all pages, budget-capped), as PlatformRepo summaries.
	ListOwnedProjects(ctx context.Context, page, perPage int) ([]*provider.PlatformRepo, error)
}

// ListUsersOptions are the ListUsers knobs.
type ListUsersOptions struct {
	Page    int    // 0 walks every page
	PerPage int    // 0 = platform default
	Search  string // empty = no filter
}

// SessionInfo is the session payload returned by a successful login; the
// PrivateToken is the credential the rest of the API authenticates with.
type SessionInfo struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PrivateToken string `json:"private_token"`
	AvatarURL    string `json:"avatar_url"`
}

// UserDetailInfo is a user profile with the detail-only fields.
type UserDetailInfo struct {
	ID         int       `json:"id"`
	Username   string    `json:"username"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	Bio        string    `json:"bio"`
	Admin      bool      `json:"admin"`
	State      string    `json:"state"`
	WebURL     string    `json:"web_url"`
	AvatarURL  string    `json:"avatar_url"`
	CreatedAt  time.Time `json:"created_at"`
	ProjectLim int       `json:"projects_limit"`
}

// SSHKeyInfo is a registered SSH public key.
type SSHKeyInfo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailInfo is a registered email address.
type EmailInfo struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
}

// GetSession implements TencentCodeAccountExtras.
func (p *Provider) GetSession(ctx context.Context, login, password string) (*SessionInfo, error) {
	s, _, err := p.client.Session.GetSession(ctx, &gongfeng.GetSessionOptions{
		Login:    gongfeng.Ptr(login),
		Password: gongfeng.Ptr(password),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetSession", err)
	}
	return &SessionInfo{
		ID:           s.ID,
		Username:     s.Username,
		Email:        s.Email,
		Name:         s.Name,
		PrivateToken: s.PrivateToken,
		AvatarURL:    s.AvatarURL,
	}, nil
}

// GetCurrentUser implements TencentCodeAccountExtras.
func (p *Provider) GetCurrentUser(ctx context.Context) (*UserDetailInfo, error) {
	u, _, err := p.client.Users.GetCurrentUser(ctx)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetCurrentUser", err)
	}
	return convertUserDetail(u), nil
}

// GetUserDetail implements TencentCodeAccountExtras.
func (p *Provider) GetUserDetail(ctx context.Context, userID int) (*UserDetailInfo, error) {
	u, _, err := p.client.Users.GetUser(ctx, userID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetUserDetail", err)
	}
	return convertUserDetail(u), nil
}

// ListWatchedProjects implements TencentCodeAccountExtras.
func (p *Provider) ListWatchedProjects(ctx context.Context, page, perPage int) ([]*provider.PlatformRepo, error) {
	projects, err := walkOrgPages(page, func(page int) ([]*gongfeng.Project, error) {
		list, _, err := p.client.Users.ListWatchedProjects(ctx,
			&gongfeng.ListWatchedProjectsOptions{ListOptions: listOptions(page, perPage)})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListWatchedProjects", err)
	}
	result := make([]*provider.PlatformRepo, 0, len(projects))
	for _, pr := range projects {
		result = append(result, convertProject(pr))
	}
	return result, nil
}

// GetSSHKey implements TencentCodeAccountExtras.
func (p *Provider) GetSSHKey(ctx context.Context, keyID int) (*SSHKeyInfo, error) {
	k, _, err := p.client.Users.GetSSHKey(ctx, keyID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetSSHKey", err)
	}
	return &SSHKeyInfo{ID: k.ID, Title: k.Title, Key: k.Key, CreatedAt: k.CreatedAt.Time}, nil
}

// GetEmail implements TencentCodeAccountExtras.
func (p *Provider) GetEmail(ctx context.Context, emailID int) (*EmailInfo, error) {
	e, _, err := p.client.Users.GetEmail(ctx, emailID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetEmail", err)
	}
	return &EmailInfo{ID: e.ID, Email: e.Email}, nil
}

// ListUsers implements TencentCodeAccountExtras.
func (p *Provider) ListUsers(ctx context.Context, opts ListUsersOptions) ([]*provider.CRUser, error) {
	users, err := walkOrgPages(opts.Page, func(page int) ([]*gongfeng.User, error) {
		list, _, err := p.client.Users.ListUsers(ctx, &gongfeng.ListUsersOptions{
			ListOptions: listOptions(page, opts.PerPage),
			Search:      gongfeng.Ptr(opts.Search),
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListUsers", err)
	}
	result := make([]*provider.CRUser, 0, len(users))
	for _, u := range users {
		converted := convertUserBasic(u)
		if converted != nil {
			result = append(result, converted)
		}
	}
	return result, nil
}

// GetUserByEmail implements TencentCodeAccountExtras.
func (p *Provider) GetUserByEmail(ctx context.Context, email string) (*provider.CRUser, error) {
	u, _, err := p.client.Users.GetUserByEmail(ctx, &gongfeng.GetUserByEmailOptions{Email: gongfeng.Ptr(email)})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetUserByEmail", err)
	}
	if u == nil || u.ID == 0 {
		return nil, provider.New(provider.PlatformTencentCode, "GetUserByEmail", http.StatusNotFound,
			"user with email "+strconv.Quote(email)+" not found")
	}
	return convertUserBasic(u), nil
}

// convertUserBasic maps a gongfeng.User to the provider shape; nil for an
// absent user.
func convertUserBasic(u *gongfeng.User) *provider.CRUser {
	if u == nil {
		return nil
	}
	return &provider.CRUser{
		ID:        int64(u.ID),
		Username:  u.Username,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
	}
}

// ListOwnedProjects implements TencentCodeAccountExtras.
func (p *Provider) ListOwnedProjects(ctx context.Context, page, perPage int) ([]*provider.PlatformRepo, error) {
	projects, err := walkOrgPages(page, func(page int) ([]*gongfeng.Project, error) {
		list, _, err := p.client.Projects.ListOwnedProjects(ctx,
			&gongfeng.ListProjectsOptions{ListOptions: listOptions(page, perPage)})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListOwnedProjects", err)
	}
	result := make([]*provider.PlatformRepo, 0, len(projects))
	for _, pr := range projects {
		result = append(result, convertProject(pr))
	}
	return result, nil
}

// ListSSHKeys implements TencentCodeAccountExtras.
func (p *Provider) ListSSHKeys(ctx context.Context) ([]*SSHKeyInfo, error) {
	keys, _, err := p.client.Users.ListSSHKeys(ctx)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListSSHKeys", err)
	}
	result := make([]*SSHKeyInfo, 0, len(keys))
	for _, k := range keys {
		result = append(result, &SSHKeyInfo{
			ID:        k.ID,
			Title:     k.Title,
			Key:       k.Key,
			CreatedAt: k.CreatedAt.Time,
		})
	}
	return result, nil
}

// CreateSSHKey implements TencentCodeAccountExtras.
func (p *Provider) CreateSSHKey(ctx context.Context, title, key string) (*SSHKeyInfo, error) {
	k, _, err := p.client.Users.CreateSSHKey(ctx, &gongfeng.CreateSSHKeyOptions{
		Title: gongfeng.Ptr(title),
		Key:   gongfeng.Ptr(key),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateSSHKey", err)
	}
	return &SSHKeyInfo{ID: k.ID, Title: k.Title, Key: k.Key, CreatedAt: k.CreatedAt.Time}, nil
}

// DeleteSSHKey implements TencentCodeAccountExtras.
func (p *Provider) DeleteSSHKey(ctx context.Context, keyID int) error {
	if _, err := p.client.Users.DeleteSSHKey(ctx, keyID); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteSSHKey", err)
	}
	return nil
}

// ListEmails implements TencentCodeAccountExtras.
func (p *Provider) ListEmails(ctx context.Context) ([]*EmailInfo, error) {
	emails, _, err := p.client.Users.ListEmails(ctx)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListEmails", err)
	}
	result := make([]*EmailInfo, 0, len(emails))
	for _, e := range emails {
		result = append(result, &EmailInfo{ID: e.ID, Email: e.Email})
	}
	return result, nil
}

// CreateEmail implements TencentCodeAccountExtras.
func (p *Provider) CreateEmail(ctx context.Context, email string) (*EmailInfo, error) {
	e, _, err := p.client.Users.CreateEmail(ctx, &gongfeng.CreateEmailOptions{Email: gongfeng.Ptr(email)})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateEmail", err)
	}
	return &EmailInfo{ID: e.ID, Email: e.Email}, nil
}

// DeleteEmail implements TencentCodeAccountExtras.
func (p *Provider) DeleteEmail(ctx context.Context, emailID int) error {
	if _, err := p.client.Users.DeleteEmail(ctx, emailID); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteEmail", err)
	}
	return nil
}

// convertUserDetail maps a gongfeng.UserDetail (which embeds User) to the
// backend surface.
func convertUserDetail(u *gongfeng.UserDetail) *UserDetailInfo {
	return &UserDetailInfo{
		ID:         u.ID,
		Username:   u.Username,
		Name:       u.Name,
		Email:      u.Email,
		Bio:        u.Bio,
		Admin:      u.IsAdmin,
		State:      u.State,
		WebURL:     u.WebURL,
		AvatarURL:  u.AvatarURL,
		CreatedAt:  u.CreatedAt.Time,
		ProjectLim: u.ProjectLimit,
	}
}

// Compile-time guarantee that *Provider satisfies TencentCodeAccountExtras.
var _ TencentCodeAccountExtras = (*Provider)(nil)
