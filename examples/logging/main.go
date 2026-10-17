// Example: attach the oekit/logx logging stack to the SDK.
//
// The SDK's Logger is injected per provider. provider.NewLogxLogger wraps
// oekit/logx's Delegating logger, which resolves the global default lazily —
// so the consumer can install (or replace) the log backend AFTER the provider
// is built and every subsequent call logs through it.
//
// Set PLATFORM_TOKEN to run. LOG_LEVEL=DEBUG additionally shows the
// transport layer's per-request debug events; the default INFO level only
// surfaces warnings and errors.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/oekit/logx"

	_ "github.com/yi-nology/go-git-platform/backends/all" // register every platform
	"github.com/yi-nology/go-git-platform/provider"
)

func main() {
	token := os.Getenv("PLATFORM_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "set PLATFORM_TOKEN to run this example")
		os.Exit(1)
	}

	// Attach the oekit/logx facade. No backend is installed yet, so this
	// resolves to logx's default until the consumer replaces it.
	logger := provider.NewLogxLogger("git-platform")

	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitHub,
		Token:    token,
		Logger:   logger,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "new provider:", err)
		os.Exit(1)
	}

	// Consumer-side backend choice, made AFTER NewProvider — the provider
	// still picks it up, because Delegating resolves the global default on
	// every call. Swap for logx.SetDefault(logx.NewStdLogger("my-service",
	// logx.LevelDebug)) or an hlog-backed logger to route this same provider
	// somewhere else without rebuilding it.
	logx.UseSlog("my-service")

	ctx := context.Background()
	repos, err := p.ListRepos(ctx, provider.ListRepoOptions{Page: 1, PerPage: 5})
	if err != nil {
		fmt.Fprintln(os.Stderr, "list repos:", err)
		os.Exit(1)
	}
	for _, r := range repos {
		fmt.Println(r.FullName)
	}

	fmt.Println()
	fmt.Println("the logs above came through oekit/logx (service=my-service).")
	fmt.Println("re-run with LOG_LEVEL=DEBUG to see per-request transport events.")
}
