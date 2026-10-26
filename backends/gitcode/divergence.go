package gitcode

import "github.com/yi-nology/go-git-platform/provider"

// divergences is the GitCode divergence ledger: the registered places where
// this backend's behavior departs from the unified provider semantics.
// See provider.Divergence and docs/divergence-ledger.md.
var divergences = []provider.Divergence{
	{Capability: "LabelManager", Method: "ListLabels", Field: "opts.Page, opts.PerPage", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's label list endpoint does not paginate; paging options are accepted but ignored."},
	{Capability: "LabelManager", Method: "CreateLabel", Field: "opts.Description", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's label API has no description field."},
	{Capability: "LabelManager", Method: "UpdateLabel", Field: "opts.Description", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's label API has no description field."},
	{Capability: "MilestoneManager", Method: "CreateMilestone", Kind: provider.DivergenceDetour,
		Reason: "go-gitcode marshals due_on without omitempty, which would clear due dates on the GitHub-shaped API; create goes through the raw client with exactly the fields the caller set."},
	{Capability: "MilestoneManager", Method: "UpdateMilestone", Kind: provider.DivergenceDetour,
		Reason: "See CreateMilestone."},
	{Capability: "IssueManager", Method: "ListIssueComments", Kind: provider.DivergenceDetour,
		Reason: "go-gitcode's ListIssueComment surface takes no pagination parameters (a bare GET returns the server-default first page only); the raw client drives page/per_page until an empty page."},
	{Capability: "IssueManager", Method: "ListIssueLabels", Kind: provider.DivergenceDetour,
		Reason: "go-gitcode's ListIssueLabels surface takes no pagination parameters; the raw client drives page/per_page until an empty page."},
	{Capability: "IssueManager", Method: "ListIssues", Field: "opts.Sort, opts.Direction", Kind: provider.DivergenceIgnore,
		Reason: "GitCode's issue-list endpoint takes no sort/direction; the filters are accepted but ignored (incremental backups must fall back themselves)."},
}

// Divergences returns the registered divergence ledger for the GitCode backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
