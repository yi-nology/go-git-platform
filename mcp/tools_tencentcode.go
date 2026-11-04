package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/yi-nology/go-git-platform/backends/tencentcode"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- tencentcode (toolset "tencentcode") ---
//
// 工蜂-native workflow surface behind TencentCodeExtras: native code
// reviews, the cross-project issue listing, milestone issue listings and
// issue/MR subscriptions. The toolset mounts only when the concrete
// provider implements the extras interface — every other platform never
// sees these tools.

func registerTencentCode(s *mcp.Server, st *state) {
	tc, ok := st.p.(tencentcode.TencentCodeExtras)
	if !ok {
		return
	}

	type listReviewsIn struct {
		Owner   string `json:"owner"`
		Repo    string `json:"repo"`
		State   string `json:"state,omitempty" jsonschema:"approving|change_required|closed; empty = all"`
		Page    int    `json:"page,omitempty"`
		PerPage int    `json:"per_page,omitempty"`
	}
	add(s, st, "list_code_reviews", "List native code reviews", "List the repository's 工蜂-native code reviews (the /review workflow, distinct from merge requests).", false, func(ctx context.Context, in listReviewsIn) ([]*tencentcode.CodeReview, error) {
		return tc.ListCodeReviews(ctx, in.Owner, in.Repo, tencentcode.ListCodeReviewsOptions{
			State: in.State, Page: in.Page, PerPage: in.PerPage,
		})
	})

	add(s, st, "get_code_review", "Get code review", "Fetch one 工蜂-native code review by ID.", false, func(ctx context.Context, in struct {
		Owner    string `json:"owner"`
		Repo     string `json:"repo"`
		ReviewID int    `json:"review_id"`
	}) (*tencentcode.CodeReview, error) {
		return tc.GetCodeReview(ctx, in.Owner, in.Repo, in.ReviewID)
	})

	type createReviewIn struct {
		Owner        string `json:"owner"`
		Repo         string `json:"repo"`
		Title        string `json:"title"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		Description  string `json:"description,omitempty"`
		ReviewerIDs  string `json:"reviewer_ids,omitempty" jsonschema:"comma-separated numeric user IDs"`
		ApproverRule int    `json:"approver_rule,omitempty" jsonschema:"-1 = all must approve, 1 = single, N = N approvers; 0 = platform default"`
	}
	add(s, st, "create_code_review", "Create code review", "Create a 工蜂-native code review between two branches.", true, func(ctx context.Context, in createReviewIn) (*tencentcode.CodeReview, error) {
		return tc.CreateCodeReview(ctx, in.Owner, in.Repo, tencentcode.CreateCodeReviewOptions{
			Title:        in.Title,
			SourceBranch: in.SourceBranch,
			TargetBranch: in.TargetBranch,
			Description:  in.Description,
			ReviewerIDs:  in.ReviewerIDs,
			ApproverRule: in.ApproverRule,
		})
	})

	type submitReviewIn struct {
		Owner    string `json:"owner"`
		Repo     string `json:"repo"`
		ReviewID int    `json:"review_id"`
		Event    string `json:"event" jsonschema:"approve|comment|require_change|deny"`
		Summary  string `json:"summary,omitempty"`
	}
	add(s, st, "submit_code_review", "Submit code review verdict", "Submit a reviewer verdict on a 工蜂-native code review.", true, func(ctx context.Context, in submitReviewIn) (map[string]any, error) {
		event, err := parseReviewerEvent(in.Event)
		if err != nil {
			return nil, err
		}
		if err := tc.SubmitCodeReview(ctx, in.Owner, in.Repo, in.ReviewID,
			tencentcode.SubmitReviewOptions{Event: event, Summary: in.Summary}); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	add(s, st, "reopen_code_review", "Reopen code review", "Reopen a closed 工蜂-native code review.", true, func(ctx context.Context, in struct {
		Owner    string `json:"owner"`
		Repo     string `json:"repo"`
		ReviewID int    `json:"review_id"`
	}) (map[string]any, error) {
		if err := tc.ReopenCodeReview(ctx, in.Owner, in.Repo, in.ReviewID); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	type listUserIssuesIn struct {
		State   string `json:"state,omitempty" jsonschema:"open|closed; empty = all"`
		Page    int    `json:"page,omitempty"`
		PerPage int    `json:"per_page,omitempty"`
	}
	add(s, st, "list_user_issues", "List issues across projects", "List issues across all projects visible to the authenticated user (工蜂's global issue listing).", false, func(ctx context.Context, in listUserIssuesIn) ([]*provider.Issue, error) {
		return tc.ListUserIssues(ctx, tencentcode.ListUserIssuesOptions{
			State: normalizeTCIssueState(in.State), Page: in.Page, PerPage: in.PerPage,
		})
	})

	add(s, st, "list_milestone_issues", "List milestone issues", "List the issues belonging to one milestone.", false, func(ctx context.Context, in struct {
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		MilestoneID int    `json:"milestone_id"`
		Page        int    `json:"page,omitempty"`
		PerPage     int    `json:"per_page,omitempty"`
	}) ([]*provider.Issue, error) {
		return tc.ListMilestoneIssues(ctx, in.Owner, in.Repo, in.MilestoneID, in.Page, in.PerPage)
	})

	add(s, st, "get_issue_subscription", "Get issue subscription", "Report whether the authenticated user watches an issue.", false, func(ctx context.Context, in struct {
		Owner   string `json:"owner"`
		Repo    string `json:"repo"`
		IssueID int    `json:"issue_id" jsonschema:"platform-global issue ID"`
	}) (map[string]any, error) {
		subscribed, err := tc.GetIssueSubscription(ctx, in.Owner, in.Repo, in.IssueID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"subscribed": subscribed}, nil
	})

	type setSubscriptionIn struct {
		Owner      string `json:"owner"`
		Repo       string `json:"repo"`
		IssueID    int    `json:"issue_id" jsonschema:"platform-global issue ID"`
		Subscribed bool   `json:"subscribed" jsonschema:"true to watch, false to stop watching"`
	}
	add(s, st, "set_issue_subscription", "Set issue subscription", "Watch or stop watching an issue.", true, func(ctx context.Context, in setSubscriptionIn) (map[string]any, error) {
		var err error
		if in.Subscribed {
			err = tc.SubscribeIssue(ctx, in.Owner, in.Repo, in.IssueID)
		} else {
			err = tc.UnsubscribeIssue(ctx, in.Owner, in.Repo, in.IssueID)
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "subscribed": in.Subscribed}, nil
	})

	add(s, st, "get_mr_subscription", "Get MR subscription", "Report whether the authenticated user watches a merge request.", false, func(ctx context.Context, in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		MRID  int    `json:"mr_id" jsonschema:"merge request number"`
	}) (map[string]any, error) {
		subscribed, err := tc.GetMRSubscription(ctx, in.Owner, in.Repo, in.MRID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"subscribed": subscribed}, nil
	})

	type setMRSubscriptionIn struct {
		Owner      string `json:"owner"`
		Repo       string `json:"repo"`
		MRID       int    `json:"mr_id" jsonschema:"merge request number"`
		Subscribed bool   `json:"subscribed" jsonschema:"true to watch, false to stop watching"`
	}
	add(s, st, "set_mr_subscription", "Set MR subscription", "Watch or stop watching a merge request.", true, func(ctx context.Context, in setMRSubscriptionIn) (map[string]any, error) {
		var subscribed bool
		var err error
		if in.Subscribed {
			subscribed, err = tc.SubscribeMR(ctx, in.Owner, in.Repo, in.MRID)
		} else {
			subscribed, err = tc.UnsubscribeMR(ctx, in.Owner, in.Repo, in.MRID)
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "subscribed": subscribed}, nil
	})
}

// parseReviewerEvent validates the verdict vocabulary at the tool boundary
// so invalid values fail with a readable message instead of a wire error.
func parseReviewerEvent(event string) (tencentcode.ReviewerEvent, error) {
	switch tencentcode.ReviewerEvent(event) {
	case tencentcode.ReviewEventApprove, tencentcode.ReviewEventComment,
		tencentcode.ReviewEventRequireChange, tencentcode.ReviewEventDeny:
		return tencentcode.ReviewerEvent(event), nil
	case "":
		return "", oekiterrors.Errorf("event is required (approve|comment|require_change|deny)")
	default:
		return "", oekiterrors.Errorf("invalid event %q (approve|comment|require_change|deny)", event)
	}
}

// normalizeTCIssueState maps the tool's open/closed vocabulary onto 工蜂's
// wire words (empty passes through as "all").
func normalizeTCIssueState(state string) string {
	if state == "open" {
		return "opened"
	}
	return state
}
