package gitee

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// rawNotification is a local type that handles Gitee's non-standard
// notification wire format (unread is a string "true"/"false" instead
// of a boolean).
type rawNotification struct {
	ID         int              `json:"id"`
	Unread     string           `json:"unread"`
	Type       string           `json:"type"`
	UpdatedAt  string           `json:"updated_at"`
	URL        string           `json:"url"`
	HTMLURL    string           `json:"html_url"`
	Subject    *rawNotifSubject `json:"subject,omitempty"`
	Repository *rawNotifRepo    `json:"repository,omitempty"`
}

type rawNotifSubject struct {
	Title            string `json:"title"`
	URL              string `json:"url"`
	LatestCommentURL string `json:"latest_comment_url"`
	Type             string `json:"type"`
}

type rawNotifRepo struct {
	ID       int    `json:"id"`
	FullName string `json:"full_name"`
}

type rawNotificationList struct {
	TotalCount int               `json:"total_count"`
	List       []rawNotification `json:"list"`
}

// ListNotifications implements provider.NotificationManager.
//
// Dual-mode pagination is handled by backendutil.PageList (issue #60 — the
// exhaustive family): opts.Page == 0 walks every page (budget-capped, fails
// loud); opts.Page > 0 returns exactly that single caller-driven page. The
// pre-fix shape attached pagination query parameters only when opts.Page > 0
// — a zero-value call sent none at all and silently returned just the
// platform's default page, violating the interface's "exhausting the
// platform's pagination" contract.
func (p *Provider) ListNotifications(ctx context.Context, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	// Since 裸串透传平台前先经共享解析器校验（issue #52）：时间戳选项的
	// 接受/拒绝面必须全后端一致（见 gitee/commits.go 同批注）。
	if _, err := provider.ParseTimestamp("Since", opts.Since); err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListNotifications", err)
	}
	u := fmt.Sprintf("%s/notifications", p.baseURL)
	nots, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]rawNotification, error) {
			return p.fetchNotificationPage(ctx, u, opts.Since, page, perPage)
		})
	if err != nil {
		return nil, err
	}
	result := make([]*provider.Notification, 0, len(nots))
	for i := range nots {
		result = append(result, convertRawNotification(&nots[i]))
	}
	return result, nil
}

// ListRepoNotifications implements provider.NotificationManager.
// Same dual-mode pagination as ListNotifications (issue #60).
func (p *Provider) ListRepoNotifications(ctx context.Context, owner, repo string, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	if _, err := provider.ParseTimestamp("Since", opts.Since); err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListRepoNotifications", err)
	}
	u := fmt.Sprintf("%s/repos/%s/%s/notifications", p.baseURL, backendutil.Esc(owner), backendutil.Esc(repo))
	nots, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]rawNotification, error) {
			return p.fetchNotificationPage(ctx, u, opts.Since, page, perPage)
		})
	if err != nil {
		return nil, err
	}
	result := make([]*provider.Notification, 0, len(nots))
	for i := range nots {
		result = append(result, convertRawNotification(&nots[i]))
	}
	return result, nil
}

// fetchNotificationPage fetches one raw notification page (backendutil's
// walkPageSize/caller sizes already normalized). Kept on the raw transport
// detour because go-gitee's typed notifications surface does not model this
// endpoint's non-standard payload shape (unread as a string; see the
// registered detour divergence).
func (p *Provider) fetchNotificationPage(ctx context.Context, u, since string, page, perPage int) ([]rawNotification, error) {
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(perPage))
	req, err := p.client.NewRequest(ctx, "GET", u+"?"+q.Encode(), nil)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListNotifications", err)
	}
	var nl rawNotificationList
	if _, err := p.client.Do(req, &nl); err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListNotifications", err)
	}
	return nl.List, nil
}

// MarkNotificationRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationRead(ctx context.Context, threadID string) error {
	_, err := p.client.Activity.MarkNotificationAsRead(ctx, threadID)
	return provider.Wrap(provider.PlatformGitee, "MarkNotificationRead", err)
}

// MarkNotificationsRead implements provider.NotificationManager.
//
// LastReadAt 走 provider.ParseTimestamp（issue #129，#52/#75/#97 同族收口）：
// 合法值被丢弃（go-gitee 的 mark 面只收 ids，wire 无 last_read_at 参数，
// 见 divergence 台账的登记），但**畸形值必须报错**——mark-all 是不可逆的
// 全量副作用，格式笔误静默退化成「全部标已读」正是该族要消灭的失败模式。
func (p *Provider) MarkNotificationsRead(ctx context.Context, opts provider.MarkNotificationsOptions) error {
	if _, err := provider.ParseTimestamp("LastReadAt", opts.LastReadAt); err != nil {
		return provider.Wrap(provider.PlatformGitee, "MarkNotificationsRead", err)
	}
	_, err := p.client.Activity.MarkNotificationsAsRead(ctx, nil)
	return provider.Wrap(provider.PlatformGitee, "MarkNotificationsRead", err)
}

// MarkRepoNotificationsRead implements provider.NotificationManager.
// LastReadAt 校验语义同 MarkNotificationsRead（issue #129）。
func (p *Provider) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts provider.MarkNotificationsOptions) error {
	if _, err := provider.ParseTimestamp("LastReadAt", opts.LastReadAt); err != nil {
		return provider.Wrap(provider.PlatformGitee, "MarkRepoNotificationsRead", err)
	}
	_, err := p.client.Activity.MarkRepoNotificationsAsRead(ctx, backendutil.Esc(owner), backendutil.Esc(repo), nil)
	return provider.Wrap(provider.PlatformGitee, "MarkRepoNotificationsRead", err)
}

func convertRawNotification(n *rawNotification) *provider.Notification {
	if n == nil {
		return nil
	}
	out := &provider.Notification{
		ID:     strconv.Itoa(n.ID),
		Unread: n.Unread == "true",
	}
	if n.Subject != nil {
		out.Subject = provider.NotificationSubject{
			Title: n.Subject.Title,
			Type:  n.Subject.Type,
			URL:   n.Subject.URL,
		}
	}
	if n.Repository != nil {
		out.Repo = &provider.EventRepo{
			ID:       int64(n.Repository.ID),
			FullName: n.Repository.FullName,
		}
		owner, name := provider.SplitFullName(n.Repository.FullName)
		out.Repo.Owner = owner
		out.Repo.Name = name
	}
	out.UpdatedAt = parseGiteeTime(&n.UpdatedAt)
	return out
}

var _ provider.NotificationManager = (*Provider)(nil)
