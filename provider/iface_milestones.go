package provider

import "context"

// MilestoneManager provides repository-level milestone CRUD. It is an
// optional capability interface: consumers should gate on
// Provider.Capabilities() (or type-assert) before use.
//
// Milestones are addressed by a string `number`, but what that string
// carries is platform-specific — the same identifier MilestoneRef.Number
// and Milestone.Number expose:
//
//   - GitHub: the milestone *number* (its per-repo serial number).
//   - GitLab, Gitea, Forgejo, GitCode, Tencent Code: the platform
//     milestone *ID* (the write endpoints take exactly that identifier,
//     so per-platform round-trips hold).
//   - Gitee: the milestone *serial number* (the "number" field of Gitee's
//     milestone payload — the identifier Gitee's own issue and milestone
//     write endpoints take; the SDK model exposes no id).
//
// Values obtained from MilestoneRef.Number (issue payloads) or
// Milestone.Number (list/get results) round-trip back into these methods
// on the platform they came from.
type MilestoneManager interface {
	// ListMilestones lists the repository's milestones. State contract
	// (issue #61): "open"/"closed" filter, empty State lists ALL
	// milestones — platforms whose own default is open-only send the
	// platform's explicit "all" value instead of relying on the default,
	// so an empty State never silently drops closed milestones. Tencent
	// Code's list endpoint has no state filter (registered divergence).
	//
	// Page semantics: exhaustive family (issue #60, see NormalizePageOpts)
	// — Page == 0 walks every page; Page > 0 returns exactly that page.
	ListMilestones(ctx context.Context, owner, repo string, opts ListMilestonesOptions) ([]Milestone, error)
	// GetMilestone fetches the milestone with the given number.
	GetMilestone(ctx context.Context, owner, repo, number string) (*Milestone, error)
	// CreateMilestone creates a repository milestone.
	CreateMilestone(ctx context.Context, owner, repo string, opts CreateMilestoneOptions) (*Milestone, error)
	// UpdateMilestone updates the milestone with the given number. Nil
	// fields in opts are left unchanged.
	UpdateMilestone(ctx context.Context, owner, repo, number string, opts UpdateMilestoneOptions) (*Milestone, error)
	// DeleteMilestone deletes the milestone with the given number.
	DeleteMilestone(ctx context.Context, owner, repo, number string) error
}
