package gitlab

import "github.com/yi-nology/go-git-platform/provider"

// divergences is the GitLab divergence ledger: the registered places where
// this backend's behavior departs from the unified provider semantics.
// See provider.Divergence and docs/divergence-ledger.md.
var divergences = []provider.Divergence{
	{Capability: "RepoStatsManager", Method: "ListStargazers", Kind: provider.DivergenceStub,
		Reason: "GitLab exposes no stargazers list endpoint; the stub carries ErrNotImplemented (issue #56)."},
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
	{Capability: "MilestoneManager", Method: "CreateMilestone", Field: "opts.DueOn (time of day)", Kind: provider.DivergenceMapping,
		Reason: "GitLab exchanges a date-only ISO8601 string (the SDK's ISOTime) for milestone due dates, so DueOn's time-of-day is truncated on write and reads come back at 00:00:00 — the date component round-trips, the clock does not (issue #62; pre-fix this lived only in a file comment). UpdateMilestone truncates the same way."},
	{Capability: "NotificationManager", Method: "ListNotifications", Field: "", Kind: provider.DivergenceMapping,
		Reason: "GitLab has no GitHub-style notification inbox; the method maps onto the Todos API — Notification.ID/threadID is the todo ID (mark-read is todo done), Reason is the todo action name, and opts.All/opts.Since have no todos equivalent and are dropped (issue #62; pre-fix this lived only in file comments). ListRepoNotifications maps the same way with a project filter."},
	{Capability: "NotificationManager", Method: "ListRepoNotifications", Field: "", Kind: provider.DivergenceMapping,
		Reason: "Maps onto the Todos API filtered by project ID; see ListNotifications for the data-source semantics (issue #62)."},
	{Capability: "NotificationManager", Method: "MarkRepoNotificationsRead", Kind: provider.DivergenceStub,
		Reason: "The Todos API (this backend's notification source, see the mapping note on ListNotifications) has no per-project mark-all-as-done; the method fails with ErrNotImplemented and callers fall back to MarkNotificationsRead (global) or MarkNotificationRead per thread."},
}

// Divergences returns the registered divergence ledger for the GitLab backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
