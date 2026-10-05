package forgejo

import (
	"context"
	"strconv"
	"time"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// buildListNotificationOptions maps provider options onto the forgejo SDK's
// ListNotificationOptions (filters only; pagination is applied by the
// callers, which drive it in one of two modes).
func buildListNotificationOptions(opts provider.ListNotificationsOptions) forgejo.ListNotificationOptions {
	listOpts := forgejo.ListNotificationOptions{}
	if opts.Since != "" {
		listOpts.Since, _ = time.Parse(time.RFC3339, opts.Since)
	}
	if opts.All {
		listOpts.Status = []forgejo.NotifyStatus{forgejo.NotifyStatusRead, forgejo.NotifyStatusUnread, forgejo.NotifyStatusPinned}
	}
	return listOpts
}

// ListNotifications implements provider.NotificationManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListNotifications(ctx context.Context, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	listOpts := buildListNotificationOptions(opts)
	var threads []*forgejo.NotificationThread
	threads, err := backendutil.PageList(opts.Page, opts.PerPage, listPageSize,
		func(page, perPage int) ([]*forgejo.NotificationThread, error) {
			listOpts.ListOptions = forgejo.ListOptions{Page: page, PageSize: perPage}
			list, _, err := p.client.ListNotifications(listOpts)
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "ListNotifications", err)
	}
	result := make([]*provider.Notification, 0, len(threads))
	for _, t := range threads {
		result = append(result, convertNotification(t))
	}
	return result, nil
}

// ListRepoNotifications implements provider.NotificationManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListRepoNotifications(ctx context.Context, owner, repo string, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	listOpts := buildListNotificationOptions(opts)
	var threads []*forgejo.NotificationThread
	threads, err := backendutil.PageList(opts.Page, opts.PerPage, listPageSize,
		func(page, perPage int) ([]*forgejo.NotificationThread, error) {
			listOpts.ListOptions = forgejo.ListOptions{Page: page, PageSize: perPage}
			list, _, err := p.client.ListRepoNotifications(owner, repo, listOpts)
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "ListRepoNotifications", err)
	}
	result := make([]*provider.Notification, 0, len(threads))
	for _, t := range threads {
		result = append(result, convertNotification(t))
	}
	return result, nil
}

// MarkNotificationRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationRead(ctx context.Context, threadID string) error {
	id, err := strconv.ParseInt(threadID, 10, 64)
	if err != nil {
		return provider.Wrapf(provider.PlatformForgejo, "MarkNotificationRead", "invalid thread ID %q", threadID)
	}
	_, _, err = p.client.ReadNotification(id)
	return provider.Wrap(provider.PlatformForgejo, "MarkNotificationRead", err)
}

// MarkNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationsRead(ctx context.Context, opts provider.MarkNotificationsOptions) error {
	markOpts := forgejo.MarkNotificationOptions{}
	if opts.LastReadAt != "" {
		markOpts.LastReadAt, _ = time.Parse(time.RFC3339, opts.LastReadAt)
	}
	_, _, err := p.client.ReadNotifications(markOpts)
	return provider.Wrap(provider.PlatformForgejo, "MarkNotificationsRead", err)
}

// MarkRepoNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts provider.MarkNotificationsOptions) error {
	markOpts := forgejo.MarkNotificationOptions{}
	if opts.LastReadAt != "" {
		markOpts.LastReadAt, _ = time.Parse(time.RFC3339, opts.LastReadAt)
	}
	_, _, err := p.client.ReadRepoNotifications(owner, repo, markOpts)
	return provider.Wrap(provider.PlatformForgejo, "MarkRepoNotificationsRead", err)
}

func convertNotification(t *forgejo.NotificationThread) *provider.Notification {
	n := &provider.Notification{
		ID:     strconv.FormatInt(t.ID, 10),
		Unread: t.Unread,
	}
	if t.Subject != nil {
		n.Subject = provider.NotificationSubject{
			Title: t.Subject.Title,
			Type:  string(t.Subject.Type),
			URL:   t.Subject.URL,
		}
	}
	if t.Repository != nil {
		n.Repo = &provider.EventRepo{
			ID:       t.Repository.ID,
			FullName: t.Repository.FullName,
		}
		owner, name := provider.SplitFullName(t.Repository.FullName)
		n.Repo.Owner = owner
		n.Repo.Name = name
	}
	n.UpdatedAt = t.UpdatedAt
	return n
}

var _ provider.NotificationManager = (*Provider)(nil)
