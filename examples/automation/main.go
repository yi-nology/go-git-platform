// Example: the automation/agent primitives beyond plain CRUD — page-walking
// iterators, idempotent ensure-helpers, conditional requests, and
// rate-limit recovery.
//
// Set PLATFORM_TOKEN (and optionally PLATFORM=github|gitlab|... to pick
// another platform). Runs three read-only demos:
//
//  1. Each: walk every page of ListRepos lazily (no hand-rolled page loop)
//  2. ConditionalRequests + polling: ETag revalidation keeps repeated reads
//     inside the rate-limit budget
//  3. RateLimitRecovery: how to back off by the server's own clock
//
// Set ENSURE_WEBHOOK_URL as well to additionally run EnsureWebhook (the one
// write demo): it creates or repairs a webhook and is safe to re-run.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/yi-nology/go-git-platform/backends/all" // register every platform
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/runtime"
)

func main() {
	token := os.Getenv("PLATFORM_TOKEN")
	if token == "" {
		log.Fatal("set PLATFORM_TOKEN to run this example")
	}
	platform := model.Platform(os.Getenv("PLATFORM"))
	if platform == "" {
		platform = model.PlatformGitHub
	}
	owner := os.Getenv("PLATFORM_OWNER")
	if owner == "" {
		owner = "yi-nology"
	}

	ctx := context.Background()
	p, err := provider.NewProvider(provider.Config{
		Platform:            platform,
		Token:               token,
		ConditionalRequests: true, // GETs revalidate via If-None-Match; GitHub 304s are quota-free
		RetryConfig:         &provider.RetryConfig{MaxRetries: 2, BaseDelay: 500 * time.Millisecond},
	})
	if err != nil {
		log.Fatalf("new provider: %v", err)
	}

	// 1) Each walks all pages lazily: fetch is called page by page, the
	// callback sees items as they arrive, and a platform that ignores the
	// page parameter trips ErrPageBudgetExceeded instead of spinning.
	var count, pages int
	err = runtime.Each(ctx, func(ctx context.Context, page int) ([]*model.PlatformRepo, error) {
		pages++
		return p.ListRepos(ctx, provider.ListRepoOptions{Owner: owner, Page: page, PerPage: 100})
	}, func(r *model.PlatformRepo) error {
		count++
		if count <= 3 {
			fmt.Printf("  %s (stars=%d archived=%v)\n", r.FullName, r.Stars, r.Archived)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Each: %v", err)
	}
	fmt.Printf("[each] %d repos across %d page(s)\n", count, pages)

	// 2) The same read again: with ConditionalRequests on, identical GETs
	// answer 304 and the transport replays the cached page.
	repos, err := p.ListRepos(ctx, provider.ListRepoOptions{Owner: owner, Page: 1, PerPage: 100})
	if err != nil {
		log.Fatalf("ListRepos: %v", err)
	}
	fmt.Printf("[conditional] re-read returned %d repos from the first page\n", len(repos))

	demoRateLimitRecovery(ctx, p, owner)
	demoEnsureWebhook(ctx, p, owner)
}

// demoRateLimitRecovery reads a deliberately absent repo and shows how to
// branch on rate-limit errors using the server's own recovery hints.
func demoRateLimitRecovery(ctx context.Context, p provider.Provider, owner string) {
	_, err := p.GetRepo(ctx, owner, "no-such-repo-404")
	switch {
	case err == nil:
		fmt.Println("[ratelimit] demo repo unexpectedly exists")
	case provider.IsRateLimited(err):
		after, reset, ok := provider.RateLimitRecovery(err)
		if !ok {
			fmt.Println("[ratelimit] rate limited without server hints; back off locally")
			return
		}
		wait := after
		if reset.After(time.Now()) && (wait == 0 || time.Until(reset) > wait) {
			wait = time.Until(reset)
		}
		fmt.Printf("[ratelimit] server advises waiting %s\n", wait.Round(time.Second))
	case provider.IsNotFound(err):
		fmt.Println("[ratelimit] demo repo absent — not rate limited, recovery hints would apply here")
	default:
		log.Fatalf("GetRepo: %v", err)
	}
}

// demoEnsureWebhook runs the one write demo: create-or-repair a webhook,
// idempotent by construction. Requires ENSURE_WEBHOOK_URL so the default
// run stays read-only.
func demoEnsureWebhook(ctx context.Context, p provider.Provider, owner string) {
	hookURL := os.Getenv("ENSURE_WEBHOOK_URL")
	if hookURL == "" {
		return
	}
	action, hook, err := runtime.EnsureWebhook(ctx, p, provider.CreateWebhookOptions{
		Owner: owner, Repo: "go-git-platform", URL: hookURL,
		Events: []string{"push", "pull_request"},
	})
	if err != nil {
		log.Fatalf("EnsureWebhook: %v", err)
	}
	fmt.Printf("[ensure] action=%s webhookID=%d\n", action, hook.ID)
}
