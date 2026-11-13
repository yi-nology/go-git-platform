package gitlab_test

import (
	"testing"

	"github.com/yi-nology/go-git-platform/backends/contracttest"
	glbackend "github.com/yi-nology/go-git-platform/backends/gitlab"
	"github.com/yi-nology/go-git-platform/provider"
)

func TestGitLab_Contract(t *testing.T) {
	contracttest.Run(t, contracttest.Harness{
		Name:     "GitLab",
		Platform: provider.PlatformGitLab,
		NewProvider: func(t *testing.T, cfg provider.Config) provider.Provider {
			p, err := provider.NewProvider(cfg)
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			return p
		},
		EmptyListResponse:    "[]",
		NonEmptyListResponse: `[{"id":1,"name":"repo","path_with_namespace":"owner/repo","http_url_to_repo":"https://gitlab.com/owner/repo.git","default_branch":"main","visibility":"public"}]`,
		Labels: &contracttest.LabelsHarnessConfig{
			ListResponse:   `[{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}]`,
			MutateResponse: `{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}`,
		},
		Issues: &contracttest.IssuesHarnessConfig{
			// GitLab-shaped fixtures: iid/author/description/web_url instead of
			// github's number/user/body/html_url, plain-string labels, milestone
			// keyed by id, and state "opened" (the backend maps opened→open).
			// Assignees resolve through ListUsers' exact-match filter and land
			// on the wire as assignee_ids.
			CreateIssueAssigneesByID: true,
			// Note edits route through the issue (issues/{iid}/notes/{id}).
			UpdateCommentViaIssue: true,
			ListResponse:          `[{"id":1,"iid":1,"title":"bug","state":"opened","author":{"id":1,"username":"dev","name":"dev"},"milestone":{"id":1,"title":"v1"},"labels":["bug"],"web_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:           `{"id":1,"iid":1,"title":"bug","state":"opened","author":{"id":1,"username":"dev","name":"dev"},"milestone":{"id":1,"title":"v1"},"labels":["bug"],"web_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse:        `{"id":1,"iid":1,"title":"bug","state":"opened","author":{"id":1,"username":"dev","name":"dev"},"milestone":{"id":1,"title":"v1"},"labels":["bug"],"web_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			CommentsResponse:      `[{"id":1,"body":"a comment","author":{"id":1,"username":"dev","name":"dev"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			LabelsResponse:        `[{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}]`,
			// 2A 三态能力矩阵：GitLab 四字段全 A 级——正文/指派人/标签/里程碑
			// 清空全部落 wire（里程碑走 ResetMilestoneID 字面 null）。
			IssueClear: map[string]contracttest.ClearExpect{
				"body":      {BodyField: "description"}, // GitLab wire 字段名
				"assignees": {BodyField: "assignee_ids"},
				"labels":    {BodyField: "labels"},
				"milestone": {BodyField: "milestone_id"}, // null 在场
			},
		},
		Reviews: &contracttest.ReviewsHarnessConfig{
			// GitLab approval-state shape: ListReviews/GetReview hit the
			// approval_state endpoint whose rules[].approved_by entries
			// (BasicUser) become synthesized single "approved" reviews keyed
			// by the MR IID. There is no per-review GET on GitLab, so
			// GetResponse mirrors ListResponse (the suite's /reviews/{id}
			// route is never hit).
			ListResponse: `{"approval_rules_overwritten":false,"rules":[{"id":1,"name":"All Eligible Users","rule_type":"any_approver","approvals_required":1,"approved_by":[{"id":1,"username":"dev","name":"dev","state":"active"}],"approved":true}]}`,
			GetResponse:  `{"approval_rules_overwritten":false,"rules":[{"id":1,"name":"All Eligible Users","rule_type":"any_approver","approvals_required":1,"approved_by":[{"id":1,"username":"dev","name":"dev","state":"active"}],"approved":true}]}`,
			// CreateReview posts a merge-request note; MutateResponse is the
			// created Note (also served, and ignored, to the unapprove POST).
			MutateResponse: `{"id":1,"body":"looks good","author":{"id":1,"username":"dev","name":"dev"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			// The note-based create carries no verdict on the wire (a note is
			// neither an approval nor a change request), so CreateEvent stays
			// empty and the suite's event-key assertion is skipped.
			// RequestReviewers resolves reviewer usernames to user IDs through
			// the Users API (GET /users?username=<name>) and writes them via
			// UpdateMergeRequest's reviewer_ids, so the wire subtest asserts
			// the /users lookup plus the ID-carrying update body.
			RequestReviewersByID: true,
		},
		// GitLab milestone shape: keyed by id (milestones are ID-addressed),
		// date-only due_date, and wire state "active" (the backend maps
		// active→open and open→activate on writes).
		Milestones: &contracttest.MilestonesHarnessConfig{
			ListResponse:   `[{"id":1,"iid":1,"title":"v1","state":"active","description":"first","due_date":"2026-01-01"}]`,
			GetResponse:    `{"id":1,"iid":1,"title":"v1","state":"active","description":"first","due_date":"2026-01-01"}`,
			MutateResponse: `{"id":1,"iid":1,"title":"v1","state":"active","description":"first","due_date":"2026-01-01"}`,
		},
		// GitLab release shape: name/description/_links.self (released_at is
		// the publish timestamp). Update is a tag-addressed PUT carrying
		// name/description; Draft/Prerelease have no GitLab counterpart
		// (registered ignore in the backend).
		// Family suites (issue #63): GitLab wire shapes (id-keyed commits,
		// old_path/new_path/diff keyed files and diffs).
		Branches: &contracttest.BranchHarnessConfig{
			ListResponse:   `[{"name":"main","commit":{"id":"abc123"}}]`,
			MutateResponse: `{"name":"feature-y","commit":{"id":"abc123"}}`,
		},
		Commits: &contracttest.CommitHarnessConfig{
			GetResponse:     `{"id":"abc123","short_id":"abc123","message":"commit msg","author_name":"dev","authored_date":"2026-01-01T00:00:00Z"}`,
			ListResponse:    `[{"id":"abc123","short_id":"abc123","message":"commit msg","author_name":"dev","authored_date":"2026-01-01T00:00:00Z"}]`,
			CompareResponse: `{"commits":[{"id":"abc123","short_id":"abc123","message":"commit msg","author_name":"dev","authored_date":"2026-01-01T00:00:00Z"}],"diffs":[{"old_path":"a.txt","new_path":"a.txt","diff":"@@ -1,1 +1,1 @@\n+hello"}]}`,
		},
		Diffs: &contracttest.DiffHarnessConfig{
			FilesResponse: `[{"old_path":"a.txt","new_path":"a.txt","diff":"@@ -1,1 +1,1 @@\n+hello","new_file":false,"deleted_file":false,"renamed_file":false}]`,
		},
		Files: &contracttest.FileHarnessConfig{
			// GetRawFile returns the raw bytes, not an envelope.
			GetResponse:  "hello world",
			ContentIsRaw: true,
		},
		// CR suite fixtures (issue #63): GitLab MR shape — iid-addressed,
		// source_branch/target_branch keys, state_event verbs, "opened" state.
		CRs: &contracttest.CRHarnessConfig{
			ListResponse:   `[{"iid":1,"title":"fix bug","state":"opened","description":"desc","source_branch":"feature-x","target_branch":"main","sha":"abc123","detailed_merge_status":"mergeable","author":{"id":1,"username":"dev"},"draft":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:    `{"iid":1,"title":"fix bug","state":"opened","description":"desc","source_branch":"feature-x","target_branch":"main","sha":"abc123","detailed_merge_status":"mergeable","author":{"id":1,"username":"dev"},"draft":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse: `{"iid":1,"title":"fix bug","state":"opened","description":"desc","source_branch":"feature-x","target_branch":"main","sha":"abc123","detailed_merge_status":"mergeable","author":{"id":1,"username":"dev"},"draft":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			// 2A 三态：GitLab MR wire 的描述字段名是 description（非 GitHub
			// 的 body）；assignees 走 assignee_ids（清空发 []）。
			CRClear: map[string]contracttest.ClearExpect{
				"description": {BodyField: "description"},
				"assignees":   {BodyField: "assignee_ids"},
			},
		},
		Releases: &contracttest.ReleasesHarnessConfig{
			ByTagResponse:    `{"tag_name":"v1.0.0","name":"v1.0.0","description":"release notes","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","_links":{"self":"https://gitlab.example.com/owner/repo/-/releases/v1.0.0"}}`,
			UpdateResponse:   `{"tag_name":"v1.0.0","name":"v1.0.0-renamed","description":"updated notes","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","_links":{"self":"https://gitlab.example.com/owner/repo/-/releases/v1.0.0"}}`,
			TagsListResponse: `[{"name":"v1","target":"abc123def456","commit":{"id":"abc123def456","created_at":"2026-01-01T00:00:00Z"}}]`,
		},
		// GitLab search payloads are bare arrays keyed by iid/path_with_
		// namespace/username; the scope rides the query string, not the path.
		Search: &contracttest.SearchHarnessConfig{
			ReposResponse:  `[{"path_with_namespace":"owner/repo","description":"search hit","web_url":"https://gitlab.example.com/owner/repo","star_count":5,"forks_count":2,"default_branch":"main","visibility":"private"}]`,
			IssuesResponse: `[{"id":1,"iid":1,"title":"found","state":"opened","description":"b","web_url":"https://gitlab.example.com/owner/repo/-/issues/1","labels":["bug"],"user_notes_count":2,"created_at":"2026-01-01T00:00:00Z"}]`,
			UsersResponse:  `[{"username":"dev","name":"Dev","avatar_url":"https://gitlab.example.com/uploads/-/system/user/avatar/1/avatar.png","web_url":"https://gitlab.example.com/dev"}]`,
		},
		// CR notes ride the MergeRequest Notes API (MR notes are a separate
		// iid namespace from issues on GitLab — the reason this backend
		// implements NoteManager). Shape: id/body/author.
		Notes: &contracttest.NotesHarnessConfig{
			ListResponse:   `[{"id":7,"body":"first note","author":{"id":1,"username":"dev","name":"Dev"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			MutateResponse: `{"id":7,"body":"rewritten","author":{"id":1,"username":"dev","name":"Dev"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
		},
		// Commit status fixtures: GitLab's commit-status shape
		// (GET /projects/:id/repository/commits/:sha/statuses), a bare array
		// where the state verb rides the "status" key and the context "name".
		// Project members double as collaborators (GitLab family): the list
		// serves the member shape, add/remove resolve the username through
		// the users lookup (ID-addressed member API, registered mapping for
		// the permission vocabulary). GitLab's lookup filter rides the
		// query string on /users, so the member fixture doubles as the
		// user payload — both decode as arrays.
		Collaborators: &contracttest.CollaboratorsHarnessConfig{
			ListResponse:       `[{"id":7,"username":"dev","name":"Dev","access_level":30,"state":"active"}]`,
			UserLookupResponse: `[{"id":7,"username":"dev","name":"Dev"}]`,
		},
		WebhookConfig: &contracttest.WebhookConfigHarnessConfig{
			GetResponse: `{"id":42,"url":"https://hooks.example/ci","push_events":true,"merge_requests_events":true}`,
		},
		CommitStatus: &contracttest.CommitStatusHarnessConfig{
			ListResponse: `[{"id":1,"sha":"deadbeef","ref":"main","status":"success","name":"ci/lint","description":"lint passed","target_url":"https://ci.example.com/1","created_at":"2026-01-01T00:00:00Z","finished_at":"2026-01-01T00:00:10Z","allow_failure":false,"coverage":87.5,"pipeline_id":10,"author":{"id":1,"username":"dev"}},{"id":2,"sha":"deadbeef","ref":"main","status":"pending","name":"ci/test","description":"tests running","target_url":"https://ci.example.com/2","created_at":"2026-01-01T00:00:05Z","allow_failure":false,"coverage":0,"pipeline_id":10,"author":{"id":1,"username":"dev"}}]`,
		},
		// GitLab's notification surface is the Todos API. The wire shape
		// differs from GitHub/Gitea-style notification threads: todos carry
		// action_name, target_type, state, and a nested target object.
		Notifications: &contracttest.NotificationsHarnessConfig{
			ListResponse: `[{"id":1,"project":{"id":1,"path_with_namespace":"owner/repo"},"author":{"id":1,"username":"dev"},"action_name":"mentioned","target_type":"Issue","target":{"iid":1,"title":"Bug report","state":"opened"},"target_url":"https://gitlab.com/owner/repo/-/issues/1","body":"you were mentioned","state":"pending","created_at":"2026-01-01T00:00:00Z"}]`,
		},
		Reactions: &contracttest.ReactionsHarnessConfig{
			ListResponse:   `[{"id":1,"name":"+1","user":{"id":1,"username":"dev"}}]`,
			CreateResponse: `{"id":1,"name":"heart","user":{"id":1,"username":"dev"}}`,
		},
		BranchProtections: &contracttest.BranchProtectionsHarnessConfig{
			ListResponse:   `[{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}]`,
			MutateResponse: `{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}`,
			GetResponse:    `{"name":"main","allow_force_push":false}`,
		},
		DeployKeys: &contracttest.DeployKeysHarnessConfig{
			ListResponse:   `[{"id":1,"title":"CI","key":"ssh-rsa AAAA","read_only":true}]`,
			CreateResponse: `{"id":1,"title":"CI","key":"ssh-rsa AAAA","read_only":true}`,
		},
		RepoStats: &contracttest.RepoStatsHarnessConfig{
			ForksResponse:        `[{"id":1,"full_name":"fork/repo","name":"repo","owner":{"login":"fork"}}]`,
			ContributorsResponse: `[{"login":"dev","contributions":10}]`,
		},
		CILogs: &contracttest.CILogsHarnessConfig{
			GreenResponse:  `[]`,
			FailedResponse: `[{"id":1,"name":"build","stage":"test","status":"failed","web_url":"https://x/1"}]`,
		},
		Users: &contracttest.UsersHarnessConfig{
			GetUserResponse: `[{"id":1,"username":"dev","name":"Dev","avatar_url":"https://gitlab.com/avatars/1"}]`,
		},
	})
}

func TestGitLab_WebhookCorpus(t *testing.T) {
	contracttest.RunWebhookCorpus(t, provider.PlatformGitLab, &glbackend.Provider{}, "testdata/webhooks")
}
