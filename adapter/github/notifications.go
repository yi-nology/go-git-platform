package github

import (
	"context"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/runtime"
)

// notificationBaseOpts builds the shared (pagination-free) notification
// filter options. Since 解析失败必须报错（issue #52）：静默丢弃会把一个
// 格式笔误升级成不过滤的全量通知流。
func notificationBaseOpts(opts provider.ListNotificationsOptions) (github.NotificationListOptions, error) {
	base := github.NotificationListOptions{All: opts.All}
	since, err := runtime.ParseTimestamp("Since", opts.Since)
	if err != nil {
		return base, err
	}
	if !since.IsZero() {
		base.Since = since
	}
	return base, nil
}

// ListNotifications implements provider.NotificationManager.
//
// Dual pagination mode: opts.Page > 0 means the caller manages paging
// explicitly and receives exactly that one page (opts.Page/opts.PerPage
// honored via NormalizePageOpts); opts.Page == 0 (the default) exhausts
// pagination via backendutil.AllPages — fetching 100 per page until an
// empty page — so every matching notification is returned.
func (p *Provider) ListNotifications(ctx context.Context, opts provider.ListNotificationsOptions) ([]*model.Notification, error) {
	base, err := notificationBaseOpts(opts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "ListNotifications", err)
	}
	var threads []*github.Notification
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		base.ListOptions = github.ListOptions{Page: page, PerPage: perPage}
		list, _, err := p.client.Activity.ListNotifications(ctx, &base)
		if err != nil {
			return nil, provider.Wrap(model.PlatformGitHub, "ListNotifications", err)
		}
		threads = list
	} else {
		var err error
		threads, err = backendutil.AllPages(func(page int) ([]*github.Notification, error) {
			o := base
			o.ListOptions = github.ListOptions{Page: page, PerPage: 100}
			list, _, err := p.client.Activity.ListNotifications(ctx, &o)
			return list, err
		})
		if err != nil {
			return nil, provider.Wrap(model.PlatformGitHub, "ListNotifications", err)
		}
	}
	result := make([]*model.Notification, 0, len(threads))
	for _, t := range threads {
		result = append(result, convertNotification(t))
	}
	return result, nil
}

// ListRepoNotifications implements provider.NotificationManager.
//
// Dual pagination mode, mirroring ListNotifications: opts.Page > 0 returns
// exactly that one caller-managed page; opts.Page == 0 exhausts pagination
// via backendutil.AllPages.
func (p *Provider) ListRepoNotifications(ctx context.Context, owner, repo string, opts provider.ListNotificationsOptions) ([]*model.Notification, error) {
	base, err := notificationBaseOpts(opts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "ListRepoNotifications", err)
	}
	var threads []*github.Notification
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		base.ListOptions = github.ListOptions{Page: page, PerPage: perPage}
		list, _, err := p.client.Activity.ListRepositoryNotifications(ctx, owner, repo, &base)
		if err != nil {
			return nil, provider.Wrap(model.PlatformGitHub, "ListRepoNotifications", err)
		}
		threads = list
	} else {
		var err error
		threads, err = backendutil.AllPages(func(page int) ([]*github.Notification, error) {
			o := base
			o.ListOptions = github.ListOptions{Page: page, PerPage: 100}
			list, _, err := p.client.Activity.ListRepositoryNotifications(ctx, owner, repo, &o)
			return list, err
		})
		if err != nil {
			return nil, provider.Wrap(model.PlatformGitHub, "ListRepoNotifications", err)
		}
	}
	result := make([]*model.Notification, 0, len(threads))
	for _, t := range threads {
		result = append(result, convertNotification(t))
	}
	return result, nil
}

// MarkNotificationRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationRead(ctx context.Context, threadID string) error {
	_, err := p.client.Activity.MarkThreadRead(ctx, threadID)
	return provider.Wrap(model.PlatformGitHub, "MarkNotificationRead", err)
}

// parseLastReadAt resolves opts.LastReadAt per the contract ("RFC3339;
// empty = mark all") into the go-github timestamp.
//
// 解析统一委托 runtime.ParseTimestamp（issue #129：#75 手写实现未走共享
// 解析器，双实现漂移风险——timeparse.go 的族契约把 LastReadAt 列入强制
// 校验面；%v 丢 *time.ParseError cause 链也一并改为多 %w，规则一归位）。
// 非空但解析失败**必须报错**(issue #75,#52 的写操作面同源缺陷):
// go-github 对零值 lastRead 的语义是「把**全部**通知标为已读」
// (activity_notifications.go:127)——旧的静默丢弃(err==nil 才赋值)会把
// 一个格式笔误(如 "2026/11/11")升级成不可逆的全量副作用。拒绝语义与
// GitLab 非法 threadID 同型:ErrInvalidInput 哨兵经 Wrap 保留可判定性。
func parseLastReadAt(op, raw string) (github.Timestamp, error) {
	t, err := runtime.ParseTimestamp("LastReadAt", raw)
	if err != nil {
		return github.Timestamp{}, provider.Wrap(model.PlatformGitHub, op, err)
	}
	return github.Timestamp{Time: t}, nil
}

// MarkNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkNotificationsRead(ctx context.Context, opts provider.MarkNotificationsOptions) error {
	lastRead, err := parseLastReadAt("MarkNotificationsRead", opts.LastReadAt)
	if err != nil {
		return err
	}
	_, err = p.client.Activity.MarkNotificationsRead(ctx, lastRead)
	return provider.Wrap(model.PlatformGitHub, "MarkNotificationsRead", err)
}

// MarkRepoNotificationsRead implements provider.NotificationManager.
func (p *Provider) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts provider.MarkNotificationsOptions) error {
	lastRead, err := parseLastReadAt("MarkRepoNotificationsRead", opts.LastReadAt)
	if err != nil {
		return err
	}
	_, err = p.client.Activity.MarkRepositoryNotificationsRead(ctx, owner, repo, lastRead)
	return provider.Wrap(model.PlatformGitHub, "MarkRepoNotificationsRead", err)
}

func convertNotification(n *github.Notification) *model.Notification {
	out := &model.Notification{
		ID:     n.GetID(),
		Unread: n.GetUnread(),
		Reason: n.GetReason(),
	}
	if n.Subject != nil {
		out.Subject = model.NotificationSubject{
			Title: n.Subject.GetTitle(),
			Type:  n.Subject.GetType(),
			URL:   n.Subject.GetURL(),
		}
	}
	if n.Repository != nil {
		out.Repo = &model.EventRepo{
			ID:       n.Repository.GetID(),
			FullName: n.Repository.GetFullName(),
		}
		owner, name := runtime.SplitFullName(n.Repository.GetFullName())
		out.Repo.Owner = owner
		out.Repo.Name = name
	}
	if n.UpdatedAt != nil {
		out.UpdatedAt = n.UpdatedAt.Time
	}
	return out
}

var _ provider.NotificationManager = (*Provider)(nil)
