package provider

import "context"

// NotificationManager provides access to a user's notification inbox and
// per-repository notification streams. It is an optional capability interface:
// consumers should gate on Provider.Capabilities().Notifications (or
// type-assert) before use.
//
// Platform support (aligned with DeriveCapabilities and the divergence
// ledger, issue #124): GitHub, Gitea, Forgejo and Gitee serve the full
// surface. GitLab implements the interface over its Todos API — thread IDs
// are todo IDs and MarkRepoNotificationsRead is a registered
// ErrNotImplemented stub (the Todos API has no per-project mark-all), see
// the GitLab divergence ledger. GitCode serves only the repo-scoped half:
// the global routes 404 live (probe round 2026-11-03), so the three global
// methods return ErrNotImplemented and only ListRepoNotifications /
// MarkRepoNotificationsRead are usable — see the README matrix's ⚠² note.
// TencentCode has no notification API at all and reports
// Notifications=false.
type NotificationManager interface {
	// ListNotifications returns the authenticated user's notifications,
	// exhausting the platform's pagination.
	ListNotifications(ctx context.Context, opts ListNotificationsOptions) ([]*Notification, error)
	// ListRepoNotifications returns notifications for a specific repository.
	ListRepoNotifications(ctx context.Context, owner, repo string, opts ListNotificationsOptions) ([]*Notification, error)
	// MarkNotificationRead marks a single notification thread as read.
	MarkNotificationRead(ctx context.Context, threadID string) error
	// MarkNotificationsRead marks all (or filtered) notifications as read.
	MarkNotificationsRead(ctx context.Context, opts MarkNotificationsOptions) error
	// MarkRepoNotificationsRead marks all notifications for a repository as read.
	MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts MarkNotificationsOptions) error
}
