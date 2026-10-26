package gitee

import "github.com/yi-nology/go-git-platform/provider"

// divergences is the Gitee divergence ledger: the registered places where
// this backend's behavior departs from the unified provider semantics.
// See provider.Divergence and docs/divergence-ledger.md.
var divergences = []provider.Divergence{
	{Capability: "ChangeRequestManager", Method: "UpdateCR", Field: "opts.TargetBranch", Kind: provider.DivergenceIgnore,
		Reason: "Gitee's pull-update endpoint has no base field; retargeting a pull request is not possible."},
	{Capability: "LabelManager", Method: "CreateLabel", Field: "opts.Description", Kind: provider.DivergenceIgnore,
		Reason: "Gitee's label wire has no description field."},
	{Capability: "LabelManager", Method: "UpdateLabel", Field: "opts.Description", Kind: provider.DivergenceIgnore,
		Reason: "Gitee's label wire has no description field."},
	{Capability: "ReleaseManager", Method: "CreateRelease", Field: "opts.Draft", Kind: provider.DivergenceIgnore,
		Reason: "Gitee's release create wire takes no draft flag."},
	{Capability: "CommitStatusManager", Method: "CreateCommitStatus", Field: "", Kind: provider.DivergenceMapping,
		Reason: "Gitee's public API has no commit-status endpoint; statuses are published as check runs (Checks API). Context maps to the check-run name, state to the conclusion (pending→neutral)."},
	{Capability: "CommitStatusManager", Method: "ListCommitStatuses", Field: "", Kind: provider.DivergenceMapping,
		Reason: "Reads map to the check runs of the commit (Checks.List). Context is the check-run name; state is concluded from status/conclusion. This surface has no string description, so CommitStatus.Description stays empty."},
	{Capability: "IssueManager", Method: "ListIssues", Field: "opts.Sort, opts.Direction", Kind: provider.DivergenceIgnore,
		Reason: "Gitee's issue-list endpoint takes no sort/direction; the filters are accepted but ignored (incremental backups must fall back themselves)."},
}

// Divergences returns the registered divergence ledger for the Gitee backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
