package gitcode

import (
	"context"

	oekiterrors "github.com/oekit/pkg/errors"
	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListNotifications implements provider.NotificationManager. GitCode
// serves no global notification inbox — the global routes 404 live
// (probe round 2026-11-03, see gitcode_api docs/wire-provenance.md); only
// the repo-scoped endpoints exist. The interface couples the two, so the
// global half fails with ErrNotImplemented; use ListRepoNotifications.
func (p *Provider) ListNotifications(ctx context.Context, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	return nil, provider.Wrap(provider.PlatformGitCode, "ListNotifications",
		oekiterrors.Wrap(provider.ErrNotImplemented, "GitCode serves no global notification inbox (live-verified 2026-11-03); use ListRepoNotifications"))
}

// ListRepoNotifications implements provider.NotificationManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListRepoNotifications(ctx context.Context, owner, repo string, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	// Since 裸串透传平台前先经共享解析器校验（issue #52）：时间戳选项的
	// 接受/拒绝面必须全后端一致（见 gitee/commits.go 同批注）。
	if _, err := provider.ParseTimestamp("Since", opts.Since); err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListRepoNotifications", err)
	}
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

// MarkNotificationRead implements provider.NotificationManager. The
// thread-read route hangs off the global inbox GitCode does not serve
// (live-verified 2026-11-03); it fails with ErrNotImplemented.
func (p *Provider) MarkNotificationRead(ctx context.Context, threadID string) error {
	return provider.Wrap(provider.PlatformGitCode, "MarkNotificationRead",
		oekiterrors.Wrap(provider.ErrNotImplemented, "GitCode serves no global notification inbox (live-verified 2026-11-03); use MarkRepoNotificationsRead"))
}

// MarkNotificationsRead implements provider.NotificationManager. The
// global mark-all route 404s live (probe round 2026-11-03); it fails with
// ErrNotImplemented.
func (p *Provider) MarkNotificationsRead(ctx context.Context, opts provider.MarkNotificationsOptions) error {
	return provider.Wrap(provider.PlatformGitCode, "MarkNotificationsRead",
		oekiterrors.Wrap(provider.ErrNotImplemented, "GitCode serves no global notification inbox (live-verified 2026-11-03); use MarkRepoNotificationsRead"))
}

// MarkRepoNotificationsRead implements provider.NotificationManager.
//
// LastReadAt 裸传平台前先经共享解析器校验（issue #129，#52 同族收口的漏网
// 处）：畸形值此前原样触线，而 mark-all 是不可逆的全量副作用——拒绝面必须
// 与其他后端一致（ErrInvalidInput），合法值照旧透传（GitCode wire 支持
// last_read_at）。
func (p *Provider) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts provider.MarkNotificationsOptions) error {
	if _, err := provider.ParseTimestamp("LastReadAt", opts.LastReadAt); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "MarkRepoNotificationsRead", err)
	}
	return provider.Wrap(provider.PlatformGitCode, "MarkRepoNotificationsRead", p.client.MarkRepoNotificationsAsRead(ctx, owner, repo, gitcode.MarkNotificationsOptions{
		LastReadAt: opts.LastReadAt,
	}))
}

var _ provider.NotificationManager = (*Provider)(nil)
