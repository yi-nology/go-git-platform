package tencentcode

import "github.com/yi-nology/go-git-platform/provider"

// divergences is the Tencent Code divergence ledger: the registered places
// where this backend's behavior departs from the unified provider semantics.
// See provider.Divergence and docs/divergence-ledger.md.
var divergences = []provider.Divergence{
	{Capability: "ChangeRequestManager", Method: "UpdateCRLabels", Kind: provider.DivergenceStub,
		Reason: "The Gongfeng API no longer accepts labels via the merge-request update endpoint."},
	{Capability: "ReviewManager", Method: "ListReviews", Kind: provider.DivergenceMapping,
		Reason: "Reviews are mapped from MR notes: every review is in the commented state and ordinary comments mix in."},
	{Capability: "ReviewManager", Method: "GetReview", Kind: provider.DivergenceMapping,
		Reason: "See ListReviews."},
	{Capability: "ReviewManager", Method: "CreateReview", Field: "opts.Event", Kind: provider.DivergenceMapping,
		Reason: "A review is created as a note; verdicts have no equivalent, so the state is always commented."},
	{Capability: "ReviewManager", Method: "RequestReviewers", Kind: provider.DivergenceIgnore,
		Reason: "The platform exposes no reviewer-request surface the SDK can drive; the call succeeds without effect."},
	{Capability: "ReviewManager", Method: "DismissReview", Kind: provider.DivergenceStub,
		Reason: "The platform exposes no review-dismissal surface."},
	{Capability: "IssueManager", Method: "ListIssues", Field: "opts.Assignee", Kind: provider.DivergenceIgnore,
		Reason: "The Gongfeng issue list endpoint takes no assignee filter; the option is accepted but ignored."},
	{Capability: "IssueManager", Method: "RemoveIssueLabel", Kind: provider.DivergenceMapping,
		Reason: "Label deletion routes through replace semantics; removing the last label is a no-op."},
	{Capability: "IssueManager", Method: "ListIssues", Field: "WebURL, ClosedAt", Kind: provider.DivergenceMapping,
		Reason: "The API exposes no issue web URL and no closed-at timestamp."},
	{Capability: "IssueManager", Method: "GetIssue", Field: "WebURL, ClosedAt", Kind: provider.DivergenceMapping,
		Reason: "See ListIssues."},
	{Capability: "LabelManager", Method: "ListLabels", Field: "Label.ID", Kind: provider.DivergenceMapping,
		Reason: "Label IDs are not exposed by the wire; Label.ID is always 0."},
	{Capability: "BranchProtectionManager", Method: "ListBranchProtections", Kind: provider.DivergenceDetour,
		Reason: "Gongfeng exposes no list-protected-branches endpoint; the branch list is walked and protected rows returned as summaries — approver rules surface via GetBranchProtection."},
	{Capability: "BranchProtectionManager", Method: "CreateBranchProtection", Field: "opts.RequiredApprovingReviews, opts.RequiredStatusChecks, opts.AllowDeletions", Kind: provider.DivergenceIgnore,
		Reason: "The protect PUT carries only access levels and the force-push flag."},
	{Capability: "BranchProtectionManager", Method: "CreateBranchProtection", Field: "opts.AllowForcePushes", Kind: provider.DivergenceMapping,
		Reason: "allow_force_push is write-only: the protection detail payload never reports it, so reads always show false."},
	{Capability: "BranchProtectionManager", Method: "UpdateBranchProtection", Field: "opts.RequiredApprovingReviews, opts.RequiredStatusChecks, opts.AllowDeletions", Kind: provider.DivergenceIgnore,
		Reason: "See CreateBranchProtection: the protect PUT (upsert) carries only access levels and the force-push flag."},
	{Capability: "BranchProtectionManager", Method: "UpdateBranchProtection", Field: "opts.AllowForcePushes", Kind: provider.DivergenceMapping,
		Reason: "Write-only, see CreateBranchProtection."},
	{Capability: "BranchProtectionManager", Method: "GetBranchProtection", Field: "RequiredApprovingReviews", Kind: provider.DivergenceMapping,
		Reason: "Mapped verbatim from the platform's approver_rule (-1 = all members must approve, 1 = single, N = N approvals); the provider has no all-members sentinel."},
	{Capability: "CollaboratorManager", Method: "AddCollaborator", Field: "opts.Permission", Kind: provider.DivergenceMapping,
		Reason: "The read/write/admin vocabulary maps onto 工蜂 access levels (read→reporter, write→developer, admin→master); empty takes the platform default."},
	{Capability: "ReleaseManager", Method: "GetReleaseByTag", Kind: provider.DivergenceDetour,
		Reason: "Gongfeng releases are ID-addressed and expose no by-tag fetch; the tag is resolved by scanning the release list (ErrNotFound when absent)."},
	{Capability: "ReleaseManager", Method: "UpdateRelease", Kind: provider.DivergenceDetour,
		Reason: "The update resolves the release ID by tag via the list scan, then PUTs to /releases/{id}."},
	{Capability: "ReleaseManager", Method: "DeleteRelease", Kind: provider.DivergenceDetour,
		Reason: "The delete resolves the release ID by tag via the list scan, then DELETEs /releases/{id}."},
	{Capability: "ReleaseManager", Method: "CreateRelease", Field: "opts.Target, opts.Draft, opts.Prerelease", Kind: provider.DivergenceIgnore,
		Reason: "The release create endpoint carries only tag/title/description."},
	{Capability: "ReleaseManager", Method: "UpdateRelease", Field: "opts.Draft, opts.Prerelease", Kind: provider.DivergenceIgnore,
		Reason: "The release update endpoint takes no draft or prerelease flags."},
	{Capability: "IssueManager", Method: "ListIssues", Field: "opts.Sort, opts.Direction", Kind: provider.DivergenceIgnore,
		Reason: "TencentCode's issue-list endpoint takes no sort/direction; the filters are accepted but ignored (incremental backups must fall back themselves)."},
}

// Divergences returns the registered divergence ledger for the Tencent Code backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
