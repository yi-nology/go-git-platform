package gitlab

import "github.com/yi-nology/go-git-platform/provider"

// divergences is the GitLab divergence ledger: the registered places where
// this backend's behavior departs from the unified provider semantics.
// See provider.Divergence and docs/divergence-ledger.md.
var divergences = []provider.Divergence{
	{Capability: "CollaboratorManager", Method: "AddCollaborator", Field: "opts.Permission", Kind: provider.DivergenceMapping,
		Reason: "The read/write/admin vocabulary maps onto GitLab access levels (read→reporter, write→developer, admin→maintainer); GitLab's add-member endpoint has no platform default and rejects a missing access_level, so empty maps to developer."},
	{Capability: "ReviewManager", Method: "ListReviews", Kind: provider.DivergenceMapping,
		Reason: "GitLab has no per-review objects: approvals are summarized into one review per approver, each keyed by the MR IID because no per-approval IDs exist."},
	{Capability: "ReviewManager", Method: "GetReview", Kind: provider.DivergenceMapping,
		Reason: "Backed by the approval state; returns the first approver's synthesized review and reports NotFound when nobody has approved yet."},
	{Capability: "ReviewManager", Method: "CreateReview", Field: "opts.Event, opts.CommitID, opts.Comments", Kind: provider.DivergenceMapping,
		Reason: "A review is created as a merge-request note; verdicts and inline comments have no GitLab mapping, so every created review is in the commented state."},
	{Capability: "ReviewManager", Method: "DismissReview", Field: "reviewID, message", Kind: provider.DivergenceMapping,
		Reason: "Maps to UnapproveMergeRequest: approvals hang off the merge request as a whole, so per-review IDs are not addressable and the message has no equivalent."},
	{Capability: "ReleaseManager", Method: "UpdateRelease", Field: "opts.Draft, opts.Prerelease", Kind: provider.DivergenceIgnore,
		Reason: "GitLab releases expose no draft or prerelease flags."},
	{Capability: "SearchManager", Method: "SearchRepos", Field: "sort, order, state", Kind: provider.DivergenceIgnore,
		Reason: "GitLab's search endpoints take no sort, order, or state; the filters are accepted but ignored."},
	{Capability: "SearchManager", Method: "SearchIssues", Field: "sort, order, state", Kind: provider.DivergenceIgnore,
		Reason: "GitLab's search endpoints take no sort, order, or state; the filters are accepted but ignored."},
	{Capability: "SearchManager", Method: "SearchUsers", Field: "sort, order, state", Kind: provider.DivergenceIgnore,
		Reason: "GitLab's search endpoints take no sort, order, or state; the filters are accepted but ignored."},
}

// Divergences returns the registered divergence ledger for the GitLab backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
