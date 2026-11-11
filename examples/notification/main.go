// Example: list notifications and mark them as read.
//
// Set PLATFORM_TOKEN to run. Works with GitHub, GitLab, Gitea, Forgejo, and
// Gitee (issue #125: GitCode serves only the repo-scoped half — the global
// ListNotifications this example starts with is ErrNotImplemented there).
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	_ "github.com/yi-nology/go-git-platform/backends/all" // register every platform
	"github.com/yi-nology/go-git-platform/provider"
)

func main() {
	token := os.Getenv("PLATFORM_TOKEN")
	if token == "" {
		log.Fatal("set PLATFORM_TOKEN to run this example")
	}

	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitHub,
		Token:    token,
	})
	if err != nil {
		log.Fatalf("new provider: %v", err)
	}

	// NotificationManager is an optional capability.
	nm, ok := p.(provider.NotificationManager)
	if !ok {
		fmt.Println("notifications not supported on this platform")
		return
	}

	ctx := context.Background()

	notifications, err := nm.ListNotifications(ctx, provider.ListNotificationsOptions{})
	if err != nil {
		log.Fatalf("list notifications: %v", err)
	}

	unread := 0
	for _, n := range notifications {
		if n.Unread {
			unread++
		}
		// Notification.Repo 是可空指针(gitlab 的 todo 无 project 时为 nil,
		// issue #125)——判空取仓库名,别解引用。
		repo := ""
		if n.Repo != nil {
			repo = n.Repo.FullName
		}
		fmt.Printf("[%s] %s: %s\n", repo, n.Subject.Type, n.Subject.Title)
	}
	fmt.Printf("\n%d total, %d unread\n", len(notifications), unread)

	// Mark the first unread notification as read.
	if len(notifications) > 0 && notifications[0].Unread {
		if err := nm.MarkNotificationRead(ctx, notifications[0].ID); err != nil {
			log.Printf("mark read: %v", err)
		} else {
			fmt.Printf("marked notification %s as read\n", notifications[0].ID)
		}
	}
}
