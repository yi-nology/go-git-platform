package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yi-nology/go-git-platform/backends/tencentcode"
	"github.com/yi-nology/go-git-platform/provider"
)

// fakeTC embeds the nil TencentCodeExtras interface; only what the tools
// touch is real.
type fakeTC struct {
	tencentcode.TencentCodeExtras
	issueSubscribed bool
	submitEvent     tencentcode.ReviewerEvent
	created         tencentcode.CreateCodeReviewOptions
}

func (f *fakeTC) ListCodeReviews(ctx context.Context, owner, repo string, opts tencentcode.ListCodeReviewsOptions) ([]*tencentcode.CodeReview, error) {
	return []*tencentcode.CodeReview{{ID: 7, IID: 7, Title: "review one", State: "approving"}}, nil
}

func (f *fakeTC) CreateCodeReview(ctx context.Context, owner, repo string, opts tencentcode.CreateCodeReviewOptions) (*tencentcode.CodeReview, error) {
	f.created = opts
	return &tencentcode.CodeReview{ID: 8, Title: opts.Title}, nil
}

func (f *fakeTC) SubmitCodeReview(ctx context.Context, owner, repo string, reviewID int, opts tencentcode.SubmitReviewOptions) error {
	f.submitEvent = opts.Event
	return nil
}

func (f *fakeTC) GetIssueSubscription(ctx context.Context, owner, repo string, issueID int) (bool, error) {
	return f.issueSubscribed, nil
}

func (f *fakeTC) SubscribeIssue(ctx context.Context, owner, repo string, issueID int) error {
	f.issueSubscribed = true
	return nil
}

func (f *fakeTC) UnsubscribeIssue(ctx context.Context, owner, repo string, issueID int) error {
	f.issueSubscribed = false
	return nil
}

func TestTencentCodeToolsetMounting(t *testing.T) {
	p := struct {
		*fakeTC
		fakeProvider
	}{fakeTC: &fakeTC{}}
	names := listTools(t, connect(t, p, Options{}))

	for _, want := range []string{
		"list_code_reviews", "get_code_review", "create_code_review",
		"submit_code_review", "reopen_code_review",
		"list_user_issues", "list_milestone_issues",
		"get_issue_subscription", "set_issue_subscription",
		"get_mr_subscription", "set_mr_subscription",
	} {
		if !names[want] {
			t.Errorf("tool %q not mounted (have %v)", want, names)
		}
	}
}

// TestTencentCodeToolsetSkippedOnOtherPlatforms pins the gate: a provider
// that does not implement TencentCodeExtras mounts none of the tools.
func TestTencentCodeToolsetSkippedOnOtherPlatforms(t *testing.T) {
	names := listTools(t, connect(t, fakeProvider{caps: provider.CapabilitySet{}}, Options{}))
	for _, absent := range []string{"list_code_reviews", "set_issue_subscription", "list_user_issues"} {
		if names[absent] {
			t.Errorf("tool %q mounted without TencentCodeExtras", absent)
		}
	}
}

func TestTencentCodeToolCalls(t *testing.T) {
	ftc := &fakeTC{}
	p := struct {
		*fakeTC
		fakeProvider
	}{fakeTC: ftc}
	sess := connect(t, p, Options{})

	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_code_reviews",
		Arguments: map[string]any{
			"owner": "o", "repo": "r", "state": "approving", "page": 1, "per_page": 10,
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("list_code_reviews: %v %v", err, res.Content)
	}

	if _, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "submit_code_review",
		Arguments: map[string]any{
			"owner": "o", "repo": "r", "review_id": 7, "event": "approve", "summary": "LGTM",
		},
	}); err != nil {
		t.Fatalf("submit_code_review: %v", err)
	}
	if ftc.submitEvent != tencentcode.ReviewEventApprove {
		t.Errorf("submit event = %q, want approve", ftc.submitEvent)
	}

	// Invalid verdict vocabulary fails at the tool boundary with a
	// readable message (IsError outcome, not a protocol error).
	res, err = sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "submit_code_review",
		Arguments: map[string]any{"owner": "o", "repo": "r", "review_id": 7, "event": "lgtm"},
	})
	if err != nil {
		t.Fatalf("protocol error on invalid event: %v", err)
	}
	if !res.IsError {
		t.Fatalf("invalid event must be a tool error, got %v", res.Content)
	}

	if _, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "set_issue_subscription",
		Arguments: map[string]any{"owner": "o", "repo": "r", "issue_id": 9, "subscribed": true},
	}); err != nil {
		t.Fatalf("set_issue_subscription: %v", err)
	}
	if !ftc.issueSubscribed {
		t.Error("SubscribeIssue was not reached")
	}

	if _, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_issue_subscription",
		Arguments: map[string]any{"owner": "o", "repo": "r", "issue_id": 9},
	}); err != nil {
		t.Fatalf("get_issue_subscription: %v", err)
	}
}

func TestTencentCodeReadOnlyDropsMutations(t *testing.T) {
	p := struct {
		*fakeTC
		fakeProvider
	}{fakeTC: &fakeTC{}}
	names := listTools(t, connect(t, p, Options{ReadOnly: true}))
	for _, absent := range []string{"create_code_review", "submit_code_review", "reopen_code_review",
		"set_issue_subscription", "set_mr_subscription"} {
		if names[absent] {
			t.Errorf("mutating tool %q mounted in read-only mode", absent)
		}
	}
	for _, want := range []string{"list_code_reviews", "get_code_review", "list_user_issues",
		"get_issue_subscription", "get_mr_subscription"} {
		if !names[want] {
			t.Errorf("read tool %q missing in read-only mode", want)
		}
	}
}
