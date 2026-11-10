package gitlab

import (
	"context"
	"strconv"

	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

// ListNotifications implements provider.NotificationManager.
// GitLab has no GitHub-style notification inbox; the SDK maps this to the
// Todos API, which provides the same "unread items that need attention"
// semantics (registered mapping divergence: thread IDs are todo IDs, and
// All/Since have no todos equivalent).
//
// Dual-mode pagination is handled by backendutil.PageList (issue #60 — the
// exhaustive family): opts.Page == 0 walks every page (budget-capped, fails
// loud); opts.Page > 0 returns exactly that single caller-driven page. The
// pre-fix shape issued one ListTodos call with empty options — both
// ignoring Page/PerPage entirely and truncating at GitLab's default 20
// todos, violating the interface's "exhausting the platform's pagination"
// contract.
func (p *Provider) ListNotifications(ctx context.Context, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	todos, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitlab.Todo, error) {
			list, _, err := p.client.Todos.ListTodos(&gitlab.ListTodosOptions{
				ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
			}, gitlab.WithContext(ctx))
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListNotifications", err)
	}
	result := make([]*provider.Notification, 0, len(todos))
	for _, t := range todos {
		result = append(result, convertTodo(t))
	}
	return result, nil
}

// ListRepoNotifications implements provider.NotificationManager.
// Filters todos by project ID (resolved from owner/repo via the project
// API) with the same dual-mode pagination as ListNotifications (issue #60).
func (p *Provider) ListRepoNotifications(ctx context.Context, owner, repo string, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	pid, _, err := p.client.Projects.GetProject(pidOf(owner, repo), nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListRepoNotifications", err)
	}
	projectID := pid.ID
	todos, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitlab.Todo, error) {
			list, _, err := p.client.Todos.ListTodos(&gitlab.ListTodosOptions{
				ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
				ProjectID:   &projectID,
			}, gitlab.WithContext(ctx))
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListRepoNotifications", err)
	}
	result := make([]*provider.Notification, 0, len(todos))
	for _, t := range todos {
		result = append(result, convertTodo(t))
	}
	return result, nil
}

// MarkNotificationRead implements provider.NotificationManager.
// Maps to marking a single GitLab todo as done.
func (p *Provider) MarkNotificationRead(ctx context.Context, threadID string) error {
	id, err := strconv.ParseInt(threadID, 10, 64)
	if err != nil {
		return provider.Wrapf(provider.PlatformGitLab, "MarkNotificationRead", "invalid thread ID %q", threadID)
	}
	_, err = p.client.Todos.MarkTodoAsDone(id, gitlab.WithContext(ctx))
	return provider.Wrap(provider.PlatformGitLab, "MarkNotificationRead", err)
}

// MarkNotificationsRead implements provider.NotificationManager.
// Maps to marking all GitLab todos as done.
func (p *Provider) MarkNotificationsRead(ctx context.Context, opts provider.MarkNotificationsOptions) error {
	_, err := p.client.Todos.MarkAllTodosAsDone(gitlab.WithContext(ctx))
	return provider.Wrap(provider.PlatformGitLab, "MarkNotificationsRead", err)
}

// MarkRepoNotificationsRead implements provider.NotificationManager.
// GitLab's Todos API has no per-project mark-all-as-done; this fails with
// ErrNotImplemented (registered divergence). Callers should use
// MarkNotificationsRead (global) or MarkNotificationRead per thread.
func (p *Provider) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opts provider.MarkNotificationsOptions) error {
	return provider.Wrap(provider.PlatformGitLab, "MarkRepoNotificationsRead",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab todos API has no per-project mark-all-as-done; use MarkNotificationsRead or MarkNotificationRead"))
}

func convertTodo(t *gitlab.Todo) *provider.Notification {
	if t == nil {
		return nil
	}
	n := &provider.Notification{
		ID:     strconv.FormatInt(t.ID, 10),
		Unread: t.State == "pending",
		Reason: string(t.ActionName),
	}
	if t.Target != nil {
		n.Subject = provider.NotificationSubject{
			Title: t.Target.Title,
			Type:  string(t.TargetType),
			URL:   t.TargetURL,
		}
	}
	if t.Project != nil {
		n.Repo = &provider.EventRepo{
			ID:       t.Project.ID,
			FullName: t.Project.PathWithNamespace,
		}
		owner, name := provider.SplitFullName(t.Project.PathWithNamespace)
		n.Repo.Owner = owner
		n.Repo.Name = name
	}
	if t.CreatedAt != nil {
		n.UpdatedAt = *t.CreatedAt
	}
	return n
}

var _ provider.NotificationManager = (*Provider)(nil)
