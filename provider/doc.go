// Package provider is the contract seam of go-git-platform: one Go interface
// for GitHub, GitLab, Gitea, Forgejo, Gitee, GitCode, and Tencent Code
// (工蜂) — the layer every platform backend implements and every consumer
// programs against.
//
// This package holds the contract surface: the Provider interface and its
// eight core sub-interface families, the optional capability interfaces
// (iface_*.go), all XxxOptions parameter structs, sentinel errors and
// ProviderError, patch three-state resolution, CapabilitySet, the
// Divergence ledger types and predicates, the factory/registry
// (NewProvider/Register), Logger/Hooks/TokenSource, shared pure helpers
// (state mapping, pagination, file decoding, diff stats), and Version().
//
// Entity types (repositories, change requests, issues, reviews, events,
// commit statuses, deployments, …) live in package model; the machines
// that drive these contracts (platform detection, Ensure* convergence,
// CI gating, batch reads, page-walking iterators, webhook validation,
// the token cache Manager) live in package runtime. This package imports
// only model — never runtime, adapter or anything below it (spec §4.1
// edge table).
//
// # Overview
//
// Start with NewProvider and the blank import that registers every shipped
// backend:
//
//	import (
//	    _ "github.com/yi-nology/go-git-platform/adapter/all"
//
//	    "github.com/yi-nology/go-git-platform/model"
//	    "github.com/yi-nology/go-git-platform/provider"
//	)
//
//	p, err := provider.NewProvider(provider.Config{
//	    Platform: model.PlatformGitHub,
//	    Token:    "ghp_...",
//	})
//
// The returned Provider composes eight core sub-interface families —
// repositories, change requests (PRs/MRs), webhooks, branches, diffs,
// commits, files, and releases — so consumer code can depend on the narrow
// slice it needs. A further 23 capability families are optional: backends
// declare them via Capabilities() and consumers reach them with a type
// assertion (the README capability matrix lists all of them per platform).
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
//     like runtime.WaitForCommitStatus.
//
// # Cross-platform consistency
//
// Behavior parity is enforced, not aspirational: contracttest runs the
// same suite against every platform, and every deliberate departure from
// unified semantics is registered in the backend's Divergence ledger
// (rendered to docs/divergence-ledger.md). The webhook event corpus
// (testdata/webhooks under each adapter) pins the normalized event
// vocabulary — cr./push/tag./branch./issue./comment. — across platforms
// with golden files.
//
// # Automation helpers
//
// The agent- and CI-facing machines live one layer up in package runtime:
// DetectPlatform, WaitForCommitStatus (CI gating), GetFileContents
// (bounded-concurrency batch reads), Each/Collect (page-walking
// iterators), EnsureWebhook/EnsureBranchProtection/EnsureDeployKey
// (idempotent desired-state convergence), webhook signature validation,
// and the Manager token cache. RateLimitRecovery stays here: it reads
// the server-advised backoff window off this package's rate-limit errors.
package provider
