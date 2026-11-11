// Package smoke carries the env-gated real-environment smoke test (issue
// #119, the v1.0-readiness test-matrix item): contract tests all run against
// httptest fakes, so real wire drift (route renames, vocabulary changes,
// auth-scheme shifts) has no regression net. This test runs the smallest
// read-only round trip against a LIVE platform — but only when credentials
// are present; without them it skips in milliseconds, so `go test ./...`
// (and every CI job) stays hermetic.
//
// Trigger it locally:
//
//	GIT_PLATFORM_SMOKE_PLATFORM=github \
//	GIT_PLATFORM_SMOKE_TOKEN=$GITHUB_TOKEN \
//	GIT_PLATFORM_SMOKE_OWNER=go-gitea GIT_PLATFORM_SMOKE_REPO=gitea \
//	go test ./smoke/ -v -run TestSmoke
//
// Or dispatch the "smoke" GitHub Actions workflow manually (it feeds the
// variables from workflow inputs + repository secrets). The probes are
// strictly read-only: one GetRepo and one page of ListRepos.
package smoke_test

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/yi-nology/go-git-platform/backends/all" // register every platform
	"github.com/yi-nology/go-git-platform/provider"
)

// TestSmoke_RealEnvironment runs two read-only probes (GetRepo, ListRepos)
// against the live platform the env vars name. Env-gated: without
// GIT_PLATFORM_SMOKE_TOKEN the test skips — a missing secret must look like
// a skip, never like a failure (CI stays green and hermetic).
func TestSmoke_RealEnvironment(t *testing.T) {
	token := os.Getenv("GIT_PLATFORM_SMOKE_TOKEN")
	if token == "" {
		t.Skip("env-gated smoke test: set GIT_PLATFORM_SMOKE_TOKEN (plus GIT_PLATFORM_SMOKE_PLATFORM/OWNER/REPO, " +
			"optional GIT_PLATFORM_SMOKE_URL for self-hosted) to run the real-environment probes; " +
			"or dispatch the smoke workflow manually")
	}

	platform := os.Getenv("GIT_PLATFORM_SMOKE_PLATFORM")
	if platform == "" {
		platform = string(provider.PlatformGitHub)
	}
	owner := os.Getenv("GIT_PLATFORM_SMOKE_OWNER")
	repo := os.Getenv("GIT_PLATFORM_SMOKE_REPO")
	if owner == "" || repo == "" {
		t.Skip("env-gated smoke test: set GIT_PLATFORM_SMOKE_OWNER and GIT_PLATFORM_SMOKE_REPO to name the probe target")
	}

	p, err := provider.NewProvider(provider.Config{
		Platform: provider.Platform(platform),
		BaseURL:  os.Getenv("GIT_PLATFORM_SMOKE_URL"),
		Token:    token,
	})
	if err != nil {
		t.Fatalf("new provider for %s: %v", platform, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 探针 1:GetRepo——单仓读路径、鉴权、错误分类全走一遍。
	got, err := p.GetRepo(ctx, owner, repo)
	if err != nil {
		t.Fatalf("GetRepo(%s/%s) against live %s: %v (wire drift? route/auth/vocabulary changes surface here first)", owner, repo, platform, err)
	}
	if got == nil || got.Name == "" {
		t.Fatalf("GetRepo returned an empty repo payload: %+v", got)
	}
	t.Logf("GetRepo ok: %s/%s (default branch %q)", got.Owner, got.Name, got.DefaultBranch)

	// 探针 2:ListRepos 一页——列表路径与分页词表。
	repos, err := p.ListRepos(ctx, provider.ListRepoOptions{Owner: owner, Page: 1, PerPage: 5})
	if err != nil {
		t.Fatalf("ListRepos(%s) against live %s: %v", owner, platform, err)
	}
	if len(repos) == 0 {
		t.Fatalf("ListRepos returned zero repos for %s — wire shape drift?", owner)
	}
	t.Logf("ListRepos ok: %d repo(s) on page 1", len(repos))
}
