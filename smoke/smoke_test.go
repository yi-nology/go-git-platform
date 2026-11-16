// Package smoke carries the env-gated real-environment smoke test (issue
// #119, the v1.0-readiness test-matrix item): contract tests all run against
// httptest fakes, so real wire drift (route renames, vocabulary changes,
// auth-scheme shifts) has no regression net. This test runs strictly
// read-only probe families against a LIVE platform — but only when
// credentials are present; without them it skips in milliseconds, so
// `go test ./...` (and every CI job) stays hermetic.
//
// Trigger it locally:
//
//	GIT_PLATFORM_SMOKE_PLATFORM=github \
//	GIT_PLATFORM_SMOKE_TOKEN=$GITHUB_TOKEN \
//	GIT_PLATFORM_SMOKE_OWNER=go-gitea GIT_PLATFORM_SMOKE_REPO=gitea \
//	go test ./smoke/ -v -run TestSmoke
//
// Or dispatch the "smoke" GitHub Actions workflow manually (it feeds the
// variables from workflow inputs + repository secrets). The probe families
// live in probes.go (Probes registry): repo | cr | webhook | pagination |
// errors | connection — selected by GIT_PLATFORM_SMOKE_FAMILIES
// (comma-separated; empty = "repo" = the historical behavior; "all" = six).
// Every probe is strictly read-only (spec 3期 §2 red line).
package smoke_test

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/yi-nology/go-git-platform/adapter/all" // register every platform
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/smoke"
)

// TestSmoke_RealEnvironment runs the selected read-only probe families
// (default: repo — GetRepo + one page of ListRepos, the historical shape)
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
		platform = string(model.PlatformGitHub)
	}
	owner := os.Getenv("GIT_PLATFORM_SMOKE_OWNER")
	repo := os.Getenv("GIT_PLATFORM_SMOKE_REPO")
	if owner == "" || repo == "" {
		t.Skip("env-gated smoke test: set GIT_PLATFORM_SMOKE_OWNER and GIT_PLATFORM_SMOKE_REPO to name the probe target")
	}

	p, err := provider.NewProvider(provider.Config{
		Platform: model.Platform(platform),
		BaseURL:  os.Getenv("GIT_PLATFORM_SMOKE_URL"),
		Token:    token,
	})
	if err != nil {
		t.Fatalf("new provider for %s: %v", platform, err)
	}

	base := smoke.Options{
		Owner: owner,
		Repo:  repo,
		URL:   os.Getenv("GIT_PLATFORM_SMOKE_URL"),
		Token: token,
	}

	for _, fam := range smoke.ParseFamilies(os.Getenv("GIT_PLATFORM_SMOKE_FAMILIES")) {
		probe, ok := smoke.Probes[fam]
		if !ok {
			t.Fatalf("unknown smoke family %q in GIT_PLATFORM_SMOKE_FAMILIES (known: repo, cr, webhook, pagination, errors, connection, or all)", fam)
		}
		t.Run(fam, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			opts := base
			opts.Logf = t.Logf
			if err := probe(ctx, p, opts); err != nil {
				t.Fatal(err)
			}
		})
	}
}
