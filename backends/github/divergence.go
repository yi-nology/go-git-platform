package github

import "github.com/yi-nology/go-git-platform/provider"

// divergences is the GitHub divergence ledger: the registered places where
// this backend's behavior departs from the unified provider semantics.
// See provider.Divergence and docs/divergence-ledger.md.
var divergences = []provider.Divergence{
	{Capability: "BranchProtectionManager", Method: "ListBranchProtections", Kind: provider.DivergenceStub,
		Reason: "GitHub exposes no list-all-protections endpoint; per-branch protection reads are served by GetBranchProtection."},
	{Capability: "ChangeRequestManager", Method: "GetCR", Field: "BaseSHA", Kind: provider.DivergenceMapping,
		Reason: "GitHub payloads expose no merge base; BaseSHA carries the target-branch tip instead (StartSHA equals BaseSHA), as does every other method returning a change request."},
	{Capability: "ChangeRequestManager", Method: "ListCRs", Field: "opts.SourceBranch (pure branch name)", Kind: provider.DivergenceMapping,
		Reason: "GitHub's head filter is namespace-pinned (owner:branch), so a pure branch-name filter is applied client-side on the unified SourceBranch (head.ref) instead - fork-sourced PRs match like on GitLab. Pagination semantics differ from GitLab's server-side filter: totals stay the pre-filter upper bound and a page can return fewer rows than PerPage while more pages remain (issue #78)."},
	{Capability: "ChangeRequestManager", Method: "ListCRs", Field: "BaseSHA", Kind: provider.DivergenceMapping,
		Reason: "See GetCR."},
	{Capability: "ReleaseManager", Method: "GetArchive", Kind: provider.DivergenceDetour,
		Reason: "Archive downloads two-hop: the go-github link endpoint resolves the signed URL, the byte download streams through the token-less followClient (no auth wrapper, no per-request timeout - large archives must not be truncated; issue #72), behind a status gate so error pages never return as archive bytes."},
	{Capability: "WebhookConfigManager", Method: "UpdateWebhook", Field: "opts.Events (empty)", Kind: provider.DivergenceIgnore,
		Reason: "go-github marshals Hook.Events with omitempty, so an empty slice is indistinguishable from \"unchanged\" on the wire — clearing every event is not expressible here (other platforms send explicit empty event sets)."},
	{Capability: "CILogManager", Method: "ListFailedJobLogs", Field: "", Kind: provider.DivergenceMapping,
		Reason: "A commit can fan out to multiple workflow runs (one per workflow file); failed jobs are collected from the latest run per workflow (runs return newest-first, first sighting wins) instead of a single pipeline."},
	{Capability: "RepoManager", Method: "ListRepos", Field: "opts.Owner", Kind: provider.DivergenceMapping,
		Reason: "GitHub serves org repos from /orgs/{owner}/repos and user repos from /users/{owner}/repos with no unified endpoint: an Owner listing tries the org endpoint first and falls back to the user endpoint on 404, so both namespace kinds list without the caller knowing which one the login is (mirrors the search-based fallback the gitea family registers)."},
}

// Divergences returns the registered divergence ledger for the GitHub backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
