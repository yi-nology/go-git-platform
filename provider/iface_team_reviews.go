package provider

import "context"

// TeamReviewManager requests team review requests on change requests. It is
// an optional capability (issue #79): the unified ReviewManager only carries
// user logins, while GitHub/Gitea/Forgejo expose a separate team_reviewers
// wire channel — organizations that gate on CODEOWNERS-driven team review
// had to bypass the SDK for it. Consumers should check
// Capabilities().TeamReviews (or type-assert) before use.
//
// Backends whose platform has no team-review wire channel simply do not
// implement the interface (TeamReviews stays false); there is deliberately
// no stub: the capability gate is the discovery mechanism, mirroring the
// other optional-manager families.
type TeamReviewManager interface {
	// RequestTeamReviewers requests review from the given team slugs. Team
	// slugs are namespace-free ("platform-team", not "org/platform-team")
	// on every platform that serves this capability. The call is additive:
	// existing user review requests are left untouched, mirroring
	// ReviewManager.RequestReviewers.
	RequestTeamReviewers(ctx context.Context, owner, repo, number string, teams []string) error
}
