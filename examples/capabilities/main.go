// Example: probe every optional capability the connected platform declares
// and run one small READ-ONLY operation per capability.
//
// This is the tour of the SDK's core idiom: check Capabilities() (a static,
// machine-readable declaration), type-assert to the capability interface,
// then use it. Run it against any of the seven platforms.
//
// Set PLATFORM, PLATFORM_TOKEN (and optionally PLATFORM_URL, OWNER, REPO)
// to run:
//
//	PLATFORM=gitea PLATFORM_TOKEN=xxx PLATFORM_URL=https://gitea.example.com \
//	OWNER=my-org REPO=my-repo go run ./examples/capabilities
//
// Every probe is isolated: one failing capability never aborts the rest.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/yi-nology/go-git-platform/provider"

	// register every shipped backend
	_ "github.com/yi-nology/go-git-platform/backends/all"
)

// probe is one capability tour step: a display name, whether the connected
// platform declares the capability, and a small read-only operation.
type probe struct {
	name      string
	declared  bool
	predicate func(ctx context.Context) error
}

func main() {
	platform := os.Getenv("PLATFORM")
	token := os.Getenv("PLATFORM_TOKEN")
	if platform == "" || token == "" {
		log.Fatal("set PLATFORM and PLATFORM_TOKEN to run this example")
	}
	owner, repo := os.Getenv("OWNER"), os.Getenv("REPO")

	p, err := provider.NewProvider(provider.Config{
		Platform: provider.Platform(platform),
		BaseURL:  os.Getenv("PLATFORM_URL"),
		Token:    token,
	})
	if err != nil {
		log.Fatalf("new provider: %v", err)
	}

	ctx := context.Background()
	caps := p.Capabilities()
	fmt.Printf("connected to %s\n\n", p.Platform())
	printDeclarations(caps)

	runProbes(ctx, p, caps, owner, repo)
}

// printDeclarations renders the static CapabilitySet — the machine-readable
// declaration whose consistency with the actual implementations is locked
// by the contract suites.
func printDeclarations(caps provider.CapabilitySet) {
	fmt.Println("declared capabilities:")
	for name, on := range map[string]bool{
		"Issues": caps.Issues, "Search": caps.Search,
		"Labels": caps.Labels, "Milestones": caps.Milestones,
		"Reviews": caps.Reviews, "CommitStatuses": caps.CommitStatuses,
		"Notifications": caps.Notifications, "Reactions": caps.Reactions,
		"BranchProtections": caps.BranchProtections, "Collaborators": caps.Collaborators,
		"DeployKeys": caps.DeployKeys, "RepoStats": caps.RepoStats, "Users": caps.Users,
		"WebhookConfig": caps.WebhookConfig, "Gists": caps.Gists,
		"Starred": caps.Starred, "Migrations": caps.Migrations,
		"ReleaseAssets": caps.ReleaseAssets,
		"CILogs":        caps.CILogs,
	} {
		mark := "  -"
		if on {
			mark = "  ✓"
		}
		fmt.Printf("%s %s\n", mark, name)
	}
	fmt.Println()
}

// runProbes executes one read-only operation per declared capability.
// Entity-scoped probes (Reviews, CommitStatuses, Reactions, WebhookConfig,
// Migrations, ReleaseAssets) only run when their entity is supplied via
// CR_NUMBER / COMMIT_SHA / ISSUE_NUMBER / WEBHOOK_ID / MIGRATION_ID
// (+ optional MIGRATION_ORG for org-level migrations) / ASSET_ID.
func runProbes(ctx context.Context, p provider.Provider, caps provider.CapabilitySet, owner, repo string) {
	probes := baseProbes(p, caps, owner, repo)
	probes = append(probes, accountProbes(p, caps)...)
	probes = append(probes, entityProbes(p, caps, owner, repo)...)
	probes = append(probes, idScopedProbes(p, caps, owner, repo)...)

	failures := 0
	for _, pr := range probes {
		if !pr.declared {
			fmt.Printf("✗ %-18s not declared on %s\n", pr.name, p.Platform())
			continue
		}
		if err := pr.predicate(ctx); err != nil {
			failures++
			fmt.Printf("⚠ %-18s %v\n", pr.name, err)
			continue
		}
		fmt.Printf("✓ %-18s ok\n", pr.name)
	}
	if failures > 0 {
		fmt.Printf("\n%d probe(s) errored (permissions/scope are the usual cause)\n", failures)
	}
}

// baseProbes covers every capability probeable from OWNER/REPO alone.
func baseProbes(p provider.Provider, caps provider.CapabilitySet, owner, repo string) []probe {
	return []probe{
		{"Issues", caps.Issues, func(ctx context.Context) error {
			issues, total, err := p.(provider.IssueManager).ListIssues(ctx,
				provider.ListIssuesOptions{Owner: owner, Repo: repo, PerPage: 5})
			if err != nil {
				return err
			}
			return printf("    %d issues (total %d)\n", len(issues), total)
		}},
		{"Search", caps.Search, func(ctx context.Context) error {
			repos, _, err := p.(provider.SearchManager).SearchRepos(ctx,
				provider.SearchReposOptions{Query: owner, PerPage: 5})
			if err != nil {
				return err
			}
			return printf("    %d repo hits for %q\n", len(repos), owner)
		}},
		{"Labels", caps.Labels, func(ctx context.Context) error {
			labels, err := p.(provider.LabelManager).ListLabels(ctx, owner, repo, provider.ListLabelsOptions{})
			if err != nil {
				return err
			}
			return printf("    %d labels\n", len(labels))
		}},
		{"Milestones", caps.Milestones, func(ctx context.Context) error {
			ms, err := p.(provider.MilestoneManager).ListMilestones(ctx, owner, repo, provider.ListMilestonesOptions{})
			if err != nil {
				return err
			}
			return printf("    %d milestones\n", len(ms))
		}},
		{"Notifications", caps.Notifications, func(ctx context.Context) error {
			notifications, err := p.(provider.NotificationManager).ListNotifications(ctx,
				provider.ListNotificationsOptions{PerPage: 5})
			if err != nil {
				return err
			}
			return printf("    %d notifications\n", len(notifications))
		}},
		{"BranchProtections", caps.BranchProtections, func(ctx context.Context) error {
			rules, err := p.(provider.BranchProtectionManager).ListBranchProtections(ctx, owner, repo)
			if err != nil {
				return err
			}
			return printf("    %d protected branches\n", len(rules))
		}},
		{"Collaborators", caps.Collaborators, func(ctx context.Context) error {
			collaborators, err := p.(provider.CollaboratorManager).ListCollaborators(ctx, owner, repo)
			if err != nil {
				return err
			}
			return printf("    %d collaborators\n", len(collaborators))
		}},
		{"DeployKeys", caps.DeployKeys, func(ctx context.Context) error {
			keys, err := p.(provider.DeploymentKeyManager).ListDeployKeys(ctx, owner, repo)
			if err != nil {
				return err
			}
			return printf("    %d deploy keys\n", len(keys))
		}},
		{"RepoStats", caps.RepoStats, func(ctx context.Context) error {
			forks, err := p.(provider.RepoStatsManager).ListForks(ctx, owner, repo)
			if err != nil {
				return err
			}
			return printf("    %d forks\n", len(forks))
		}},
		{"Users", caps.Users, func(ctx context.Context) error {
			login := os.Getenv("PLATFORM_USER")
			if login == "" {
				return printf("    (set PLATFORM_USER to resolve a username)\n")
			}
			users, err := p.(provider.UserManager).ResolveUsernames(ctx, []string{login})
			if err != nil {
				return err
			}
			if len(users) == 0 || users[0] == nil {
				return fmt.Errorf("no profile returned for %q", login)
			}
			return printf("    %s → ID %d\n", login, users[0].ID)
		}},
	}
}

// accountProbes covers the account-scoped capabilities: they read the token
// user's own data (gists, starred repos) and need neither OWNER nor REPO.
func accountProbes(p provider.Provider, caps provider.CapabilitySet) []probe {
	return []probe{
		{"Gists", caps.Gists, func(ctx context.Context) error {
			gists, err := p.(provider.GistManager).ListMyGists(ctx, 1, 5)
			if err != nil {
				return err
			}
			return printf("    %d gists (page 1)\n", len(gists))
		}},
		{"Starred", caps.Starred, func(ctx context.Context) error {
			repos, err := p.(provider.StarredManager).ListStarred(ctx, 1, 5)
			if err != nil {
				return err
			}
			return printf("    %d starred repos (page 1)\n", len(repos))
		}},
	}
}

// entityProbes covers the entity-scoped capabilities: they run only when
// the entity is provided via CR_NUMBER / COMMIT_SHA / ISSUE_NUMBER.
func entityProbes(p provider.Provider, caps provider.CapabilitySet, owner, repo string) []probe {
	var probes []probe
	if num := os.Getenv("CR_NUMBER"); num != "" && caps.Reviews {
		probes = append(probes, probe{"Reviews", true, func(ctx context.Context) error {
			reviews, err := p.(provider.ReviewManager).ListReviews(ctx, owner, repo, num)
			if err != nil {
				return err
			}
			return printf("    %d reviews on CR %s\n", len(reviews), num)
		}})
	}
	if sha := os.Getenv("COMMIT_SHA"); sha != "" && caps.CommitStatuses {
		probes = append(probes, probe{"CommitStatuses", true, func(ctx context.Context) error {
			statuses, err := p.(provider.CommitStatusManager).ListCommitStatuses(ctx, owner, repo, sha)
			if err != nil {
				return err
			}
			return printf("    %d statuses on %s\n", len(statuses), sha[:7])
		}})
	}
	if sha := os.Getenv("COMMIT_SHA"); sha != "" && caps.CILogs {
		probes = append(probes, probe{"CILogs", true, func(ctx context.Context) error {
			logs, err := p.(provider.CILogManager).ListFailedJobLogs(ctx, owner, repo, sha)
			if err != nil {
				return err
			}
			if len(logs) == 0 {
				return printf("    no failed jobs on %s (green or no CI run)\n", sha[:7])
			}
			return printf("    %d failed job(s) on %s\n", len(logs), sha[:7])
		}})
	}
	if num := os.Getenv("ISSUE_NUMBER"); num != "" && caps.Reactions {
		probes = append(probes, probe{"Reactions", true, func(ctx context.Context) error {
			reactions, err := p.(provider.ReactionManager).ListIssueReactions(ctx, owner, repo, num)
			if err != nil {
				return err
			}
			return printf("    %d reactions on issue %s\n", len(reactions), num)
		}})
	}

	return probes
}

// idScopedProbes covers the ID-scoped capabilities: they run only when the
// target ID is supplied via WEBHOOK_ID / MIGRATION_ID (+ optional
// MIGRATION_ORG for org-level migrations) / ASSET_ID.
func idScopedProbes(p provider.Provider, caps provider.CapabilitySet, owner, repo string) []probe {
	var probes []probe
	if id := os.Getenv("WEBHOOK_ID"); id != "" && caps.WebhookConfig {
		probes = append(probes, probe{"WebhookConfig", true, func(ctx context.Context) error {
			whID, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				return fmt.Errorf("bad WEBHOOK_ID %q: %w", id, err)
			}
			hook, err := p.(provider.WebhookConfigManager).GetWebhook(ctx, owner, repo, whID)
			if err != nil {
				return err
			}
			return printf("    webhook %d → %d events\n", whID, len(hook.Events))
		}})
	}
	if id := os.Getenv("MIGRATION_ID"); id != "" && caps.Migrations {
		probes = append(probes, probe{"Migrations", true, func(ctx context.Context) error {
			mID, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				return fmt.Errorf("bad MIGRATION_ID %q: %w", id, err)
			}
			org := os.Getenv("MIGRATION_ORG") // 空 = 用户级迁移
			m, err := p.(provider.MigrationManager).GetMigration(ctx, org, mID)
			if err != nil {
				return err
			}
			return printf("    migration %d → %s\n", mID, m.State)
		}})
	}
	if id := os.Getenv("ASSET_ID"); id != "" && caps.ReleaseAssets {
		probes = append(probes, probe{"ReleaseAssets", true, func(ctx context.Context) error {
			aID, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				return fmt.Errorf("bad ASSET_ID %q: %w", id, err)
			}
			// 只读语义但会流式拉全量附件字节——落到 io.Discard，显式 opt-in。
			start := time.Now()
			if err := p.(provider.ReleaseAssetManager).DownloadReleaseAsset(ctx, owner, repo, aID, io.Discard); err != nil {
				return err
			}
			return printf("    asset %d downloaded in %s\n", aID, time.Since(start).Round(time.Millisecond))
		}})
	}

	return probes
}

// printf is fmt.Printf that doubles as an error-free probe step.
func printf(format string, args ...any) error {
	fmt.Printf(format, args...)
	return nil
}
