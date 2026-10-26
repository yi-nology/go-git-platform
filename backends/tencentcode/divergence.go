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
	{Capability: "ReleaseManager", Method: "UpdateRelease", Field: "opts.Draft, opts.Prerelease", Kind: provider.DivergenceIgnore,
		Reason: "The release update endpoint takes no draft or prerelease flags."},
	{Capability: "IssueManager", Method: "ListIssues", Field: "opts.Sort, opts.Direction", Kind: provider.DivergenceIgnore,
		Reason: "TencentCode's issue-list endpoint takes no sort/direction; the filters are accepted but ignored (incremental backups must fall back themselves)."},
}

// Divergences returns the registered divergence ledger for the Tencent Code backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
