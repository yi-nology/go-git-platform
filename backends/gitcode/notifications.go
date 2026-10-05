package gitcode

import (
	"context"
	"strconv"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	gitcode "github.com/yi-nology/go-gitcode"
)

// ListNotifications implements provider.NotificationManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListNotifications(ctx context.Context, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	buildOpts := func(page, perPage int) gitcode.ListNotificationsOptions {
		return gitcode.ListNotificationsOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
			All:         opts.All,
			Since:       opts.Since,
		}
	}
	var threads []*gitcode.NotificationThread
	threads, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitcode.NotificationThread, error) {
			return p.client.ListNotificationsWithOptions(ctx, buildOpts(page, perPage))
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListNotifications", err)
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
	buildOpts := func(page, perPage int) gitcode.ListNotificationsOptions {
		return gitcode.ListNotificationsOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
			All:         opts.All,
			Since:       opts.Since,
		}
	}
	var threads []*gitcode.NotificationThread
	threads, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitcode.NotificationThread, error) {
			return p.client.ListRepoNotifications(ctx, owner, repo, buildOpts(page, perPage))
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListRepoNotifications", err)
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
		return provider.Wrapf(provider.PlatformGitCode, "MarkNotificationRead", "invalid thread ID %q", threadID)
	}
	return provider.Wrap(provider.PlatformGitCode, "MarkNotificationRead", p.client.MarkNotificationThreadAsRead(ctx, id))
}

// MarkNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationsRead(ctx context.Context, opts provider.MarkNotificationsOptions) error {
	return provider.Wrap(provider.PlatformGitCode, "MarkNotificationsRead", p.client.MarkNotificationsAsRead(ctx, gitcode.MarkNotificationsOptions{
		LastReadAt: opts.LastReadAt,
	}))
}

// MarkRepoNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts provider.MarkNotificationsOptions) error {
	return provider.Wrap(provider.PlatformGitCode, "MarkRepoNotificationsRead", p.client.MarkRepoNotificationsAsRead(ctx, owner, repo, gitcode.MarkNotificationsOptions{
		LastReadAt: opts.LastReadAt,
	}))
}

var _ provider.NotificationManager = (*Provider)(nil)
