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
func (p *Provider) ListNotifications(ctx context.Context, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	u := fmt.Sprintf("%s/notifications", p.baseURL)
	q := url.Values{}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	if opts.Page > 0 {
		q.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return p.listNotificationsURL(ctx, u)
}

// ListRepoNotifications implements provider.NotificationManager.
func (p *Provider) ListRepoNotifications(ctx context.Context, owner, repo string, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/notifications", p.baseURL, backendutil.Esc(owner), backendutil.Esc(repo))
	q := url.Values{}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	if opts.Page > 0 {
		q.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return p.listNotificationsURL(ctx, u)
}

func (p *Provider) listNotificationsURL(ctx context.Context, u string) ([]*provider.Notification, error) {
	req, err := p.client.NewRequest(ctx, "GET", u, nil)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListNotifications", err)
	}
	var nl rawNotificationList
	if _, err := p.client.Do(req, &nl); err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListNotifications", err)
	}
	result := make([]*provider.Notification, 0, len(nl.List))
	for i := range nl.List {
		result = append(result, convertRawNotification(&nl.List[i]))
	}
	return result, nil
}

// MarkNotificationRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationRead(ctx context.Context, threadID string) error {
	_, err := p.client.Activity.MarkNotificationAsRead(ctx, threadID)
	return provider.Wrap(provider.PlatformGitee, "MarkNotificationRead", err)
}

// MarkNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationsRead(ctx context.Context, opts provider.MarkNotificationsOptions) error {
	_, err := p.client.Activity.MarkNotificationsAsRead(ctx, nil)
	return provider.Wrap(provider.PlatformGitee, "MarkNotificationsRead", err)
}

// MarkRepoNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts provider.MarkNotificationsOptions) error {
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
