package tencentcode_test

import (
	"testing"

	"github.com/yi-nology/go-git-platform/backends/contracttest"
	tencentcode "github.com/yi-nology/go-git-platform/backends/tencentcode"
	"github.com/yi-nology/go-git-platform/provider"
)

func TestTencentCode_Contract(t *testing.T) {
	contracttest.Run(t, contracttest.Harness{
		Name:     "Tencent Code",
		Platform: provider.PlatformTencentCode,
		NewProvider: func(t *testing.T, cfg provider.Config) provider.Provider {
			p, err := provider.NewProvider(cfg)
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			return p
		},
		EmptyListResponse:    `[]`,
		NonEmptyListResponse: `[{"id":1,"name":"repo","path_with_namespace":"owner/repo","http_url_to_repo":"https://example.com/owner/repo.git","default_branch":"main","visibility_level":20}]`,
		// Gongfeng milestone shape (GitLab-shaped): id-addressed, wire state
		// "active", date-only due_date.
		Milestones: &contracttest.MilestonesHarnessConfig{
			ListResponse:   `[{"id":1,"iid":1,"project_id":1,"title":"v1","state":"active","description":"first","due_date":"2026-01-01"}]`,
			MutateResponse: `{"id":1,"iid":1,"project_id":1,"title":"v1","state":"active","description":"first","due_date":"2026-01-01"}`,
		},
		// Gongfeng label shape (GitLab-shaped): name-addressed, colors carry
		// a leading '#'.
		Labels: &contracttest.LabelsHarnessConfig{
			ListResponse:   `[{"name":"bug","color":"#4cc917","description":"something broke"}]`,
			MutateResponse: `{"name":"bug","color":"#4cc917","description":"something broke"}`,
		},
		// Gongfeng issue shape (GitLab-shaped): iid/description, state
		// "opened", plain-string labels, milestone keyed by id, no web_url
		// (registered). GetResponse carries two labels so label removal has
		// a survivor — removing an issue's last label is a registered no-op
		// (the empty csv cannot travel on the update body).
		Issues: &contracttest.IssuesHarnessConfig{
			// Assignees resolve through 工蜂's /users/{username} lookup and
			// land on the wire as the assignee_ids csv.
			CreateIssueAssigneesByID: true,
			// Note edits route through the issue (issues/{iid}/notes/{id}).
			UpdateCommentViaIssue: true,
			ListResponse:          `[{"id":1,"iid":1,"title":"bug","description":"broke","state":"opened","author":{"id":1,"username":"dev","name":"Developer"},"labels":["bug","enhancement"],"milestone":{"id":1,"title":"v1"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:           `{"id":1,"iid":1,"title":"bug","description":"broke","state":"opened","author":{"id":1,"username":"dev","name":"Developer"},"labels":["bug","enhancement"],"milestone":{"id":1,"title":"v1"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse:        `{"id":1,"iid":1,"title":"bug","description":"broke","state":"opened","author":{"id":1,"username":"dev","name":"Developer"},"labels":["bug","enhancement"],"milestone":{"id":1,"title":"v1"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			CommentsResponse:      `[{"id":1,"body":"a comment","author":{"id":1,"username":"dev","name":"Developer"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			LabelsResponse:        `[{"name":"bug","color":"#4cc917","description":"something broke"}]`,
		},
		// Gongfeng review-note shape (GitLab-shaped notes carrying the
		// review): id/body/author/created_at with system bookkeeping notes
		// mixed into lists (filtered out by ListReviews). The note model
		// carries no state field, so reads normalize to commented
		// (registered) and the List subtest asserts that; create verdicts
		// travel as reviewer_state — not under the suite's "event" key — so
		// CreateEvent stays empty. RequestReviewers is a registered ignore
		// and DismissReview a registered stub, both flagged.
		Reviews: &contracttest.ReviewsHarnessConfig{
			ListResponse:            `[{"id":1,"body":"looks good","author":{"id":1,"username":"dev","name":"Developer"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false},{"id":2,"body":"milestone removed","author":{"id":1,"username":"dev","name":"Developer"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":true}]`,
			GetResponse:             `{"id":1,"body":"looks good","author":{"id":1,"username":"dev","name":"Developer"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false}`,
			MutateResponse:          `{"id":1,"body":"looks good","author":{"id":1,"username":"dev","name":"Developer"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false}`,
			IgnoresRequestReviewers: true,
			IgnoresDismissal:        true,
			ListStateIsCommented:    true,
		},
		// Gongfeng release shape: id/tag/title/description — the platform's
		// release object has no tag_name/name/url fields (the GitHub-style
		// keys in the old fixture encoded a wrong belief about this wire),
		// the endpoints are ID-addressed, and the update surface cannot
		// carry a name (registered limitation), so the suite asserts the
		// description key. By-tag lookups resolve through the list scan the
		// suite's collection route already serves.
		Releases: &contracttest.ReleasesHarnessConfig{
			ByTagResponse:              `{"id":1,"tag":"v1.0.0","title":"v1.0.0","description":"release notes","created_at":"2026-01-01T00:00:00Z"}`,
			UpdateResponse:             `{"id":1,"tag":"v1.0.0","title":"v1.0.0","description":"updated notes","created_at":"2026-01-01T00:00:00Z"}`,
			UpdateSendsDescriptionOnly: true,
		},
		// Project members double as collaborators: the list serves the
		// member shape, add/remove resolve the username through the users
		// lookup (ID-addressed member API, registered mapping for the
		// permission vocabulary).
		Collaborators: &contracttest.CollaboratorsHarnessConfig{
			ListResponse:       `[{"id":7,"username":"dev","name":"Dev","access_level":30,"state":"active"}]`,
			UserLookupResponse: `{"id":7,"username":"dev","name":"Dev"}`,
		},
		// Branch protection: List walks the branch list and filters the
		// protected rows (branch-row shape); create/update PUT the protect
		// endpoint and decode the protection detail straight from the PUT
		// response (registered ignores for the contract fields the protect
		// PUT cannot carry).
		BranchProtections: &contracttest.BranchProtectionsHarnessConfig{
			ListResponse:   `[{"name":"main","protected":true,"commit":{"id":"abc"}}]`,
			MutateResponse: `{"protected":true,"name":"main","approver_rule":1}`,
		},
		// Commit status fixtures: gongfeng's commit-status shape is
		// GitLab-like (GET /projects/:id/commits/:sha/statuses — the
		// repository/ prefix in studyzy v0.6.0 was a doc error the
		// vendored SDK corrected),
		// a bare array where the state verb rides the "status" key and the
		// context "name".
		CommitStatus: &contracttest.CommitStatusHarnessConfig{
			ListResponse: `[{"id":1,"sha":"deadbeef","ref":"main","status":"success","name":"ci/lint","description":"lint passed","target_url":"https://ci.example.com/1","created_at":"2026-01-01T00:00:00Z"},{"id":2,"sha":"deadbeef","ref":"main","status":"pending","name":"ci/test","description":"tests running","target_url":"https://ci.example.com/2","created_at":"2026-01-01T00:00:05Z"}]`,
		},
		RepoStats: &contracttest.RepoStatsHarnessConfig{
			ForksResponse:      `[{"id":1,"full_name":"fork/repo","name":"repo","owner":{"login":"fork"}}]`,
			StargazersResponse: `[{"id":1,"login":"star"}]`,
		},
		Users: &contracttest.UsersHarnessConfig{
			GetUserResponse: `{"id":1,"username":"dev","name":"Dev","avatar_url":"https://tencentcode.com/avatars/1"}`,
		},
	})
}

func TestTencentCode_WebhookCorpus(t *testing.T) {
	contracttest.RunWebhookCorpus(t, provider.PlatformTencentCode, &tencentcode.Provider{}, "testdata/webhooks")
}
