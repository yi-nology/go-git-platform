// Package provider is the public API of go-git-platform: one Go interface
// for GitHub, GitLab, Gitea, Forgejo, Gitee, GitCode, and Tencent Code
// (工蜂), plus the local-git entry points that hang off the same unified
// models.
//
// Entity types (repositories, change requests, issues, reviews, events,
// commit statuses, deployments, …) live in package model; this package is
// the contract seam — interfaces, options, errors and shared helpers.
//
// # Overview
//
// Start with NewProvider and the blank import that registers every shipped
// backend:
//
//	import _ "github.com/yi-nology/go-git-platform/adapter/all"
//
//	p, err := provider.NewProvider(provider.Config{
//	    Platform: model.PlatformGitHub,
//	    Token:    "ghp_...",
//	})
//
// The returned Provider composes eight core sub-interface families —
// repositories, change requests (PRs/MRs), webhooks, branches, diffs,
// commits, files, and releases — so consumer code can depend on the narrow
// slice it needs. Thirteen further capability families (issues, search,
// labels, milestones, reviews, commit statuses, notifications, reactions,
// branch protections, collaborators, deploy keys, repo stats, users) are
// optional: backends declare them via Capabilities() and consumers reach
// them with a type assertion.
//
// # Configuration surface
//
// Config carries everything a backend needs: platform, base URL (for
// self-hosted instances), credentials, retry, hooks, and logging. Two
// newer knobs are worth knowing:
//
//   - Config.TokenSource replaces the static Token with a per-request
//     source, so expiring credentials (GitHub App installation tokens,
//     OAuth flows) rotate without rebuilding the provider.
//   - Config.ConditionalRequests enables ETag-based conditional GETs: the
//     transport replays cached 304 answers, which on GitHub do not count
//     against the rate-limit budget — a large win for polling workloads
//     like WaitForCommitStatus.
//
// # Cross-platform consistency
//
// Behavior parity is enforced, not aspirational: backends/contracttest
// runs the same suite against every platform, and every deliberate
// departure from unified semantics is registered in the backend's
// Divergence ledger (rendered to docs/divergence-ledger.md). The webhook
// event corpus (testdata/webhooks under each backend) pins the normalized
// event vocabulary — cr./push/tag./branch./issue./comment. — across
// platforms with golden files.
//
// # Automation helpers
//
// Beyond the raw managers, the package ships primitives for agent and CI
// use: WaitForCommitStatus (CI gating), GetFileContents (bounded
// concurrency batch reads), Each/Collect (page-walking iterators),
// EnsureWebhook/EnsureBranchProtection/EnsureDeployKey (idempotent
// desired-state convergence), and RateLimitRecovery (server-advised
// backoff hints from rate-limit errors).
package provider
