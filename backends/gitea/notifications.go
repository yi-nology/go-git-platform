package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListNotifications implements provider.NotificationManager.
func (p *Provider) ListNotifications(ctx context.Context, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	return giteafamily.ListNotifications(ctx, p.family, p.ports, opts)
}

// ListRepoNotifications implements provider.NotificationManager.
func (p *Provider) ListRepoNotifications(ctx context.Context, owner, repo string, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	return giteafamily.ListRepoNotifications(ctx, p.family, p.ports, owner, repo, opts)
}

// MarkNotificationRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationRead(ctx context.Context, threadID string) error {
	return giteafamily.MarkNotificationRead(ctx, p.family, p.ports, threadID)
}

// MarkNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationsRead(ctx context.Context, opts provider.MarkNotificationsOptions) error {
	return giteafamily.MarkNotificationsRead(ctx, p.family, p.ports, opts)
}

// MarkRepoNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts provider.MarkNotificationsOptions) error {
	return giteafamily.MarkRepoNotificationsRead(ctx, p.family, p.ports, owner, repo, opts)
}

var _ provider.NotificationManager = (*Provider)(nil)
