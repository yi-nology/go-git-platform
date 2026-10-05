package forgejo_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/contracttest"
	"github.com/yi-nology/go-git-platform/backends/forgejo"
	"github.com/yi-nology/go-git-platform/provider"
)

func TestForgejo_Contract(t *testing.T) {
	contracttest.Run(t, contracttest.Harness{
		Name:     "Forgejo",
		Platform: provider.PlatformForgejo,
		NewProvider: func(t *testing.T, cfg provider.Config) provider.Provider {
			wrapper := contracttest.VersionProxy(cfg.BaseURL, `{"version":"8.0.0"}`)
			t.Cleanup(wrapper.Close)
			cfg.BaseURL = wrapper.URL
			p, err := forgejo.New(cfg)
			if err != nil {
				t.Fatalf("forgejo.New: %v", err)
			}
			return p
		},
		EmptyListResponse:    "[]",
		NonEmptyListResponse: `[{"id":1,"full_name":"owner/repo","name":"repo","owner":{"username":"owner"},"default_branch":"main"}]`,
		Labels: &contracttest.LabelsHarnessConfig{
			ListResponse:   `[{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}]`,
			MutateResponse: `{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}`,
		},
		Issues: &contracttest.IssuesHarnessConfig{
			ListResponse:     `[{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"labels":[{"id":1,"name":"bug","color":"#4cc917"}],"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:      `{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse:   `{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			CommentsResponse: `[{"id":1,"body":"a comment","user":{"login":"dev"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			LabelsResponse:   `[{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}]`,
		},
		// Forgejo PullReview shape mirrors gitea: github-like keys with
		// UPPERCASE wire states ("APPROVED"/"REQUEST_CHANGES"/"COMMENT"/"PENDING").
		Reviews: &contracttest.ReviewsHarnessConfig{
			ListResponse:   `[{"id":1,"user":{"id":7,"login":"dev"},"state":"APPROVED","body":"looks good","submitted_at":"2026-01-01T00:00:00Z","html_url":"https://example.com/pull/1#review-1"}]`,
			GetResponse:    `{"id":1,"user":{"id":7,"login":"dev"},"state":"APPROVED","body":"looks good","submitted_at":"2026-01-01T00:00:00Z","html_url":"https://example.com/pull/1#review-1"}`,
			MutateResponse: `{"id":1,"user":{"id":7,"login":"dev"},"state":"APPROVED","body":"looks good","submitted_at":"2026-01-01T00:00:00Z","html_url":"https://example.com/pull/1#review-1"}`,
			// CreateReview maps the APPROVE verdict to the forgejo SDK's
			// ReviewStateApproved ("APPROVED"), posted under the "event" key
			// (CreatePullReviewOptions.State json tag).
			CreateEvent: "APPROVED",
		},
		// Forgejo milestone shape mirrors gitea: keyed by id (ID-addressed),
		// github-like state/due_on keys.
		Milestones: &contracttest.MilestonesHarnessConfig{
			ListResponse:   `[{"id":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01T00:00:00Z"}]`,
			MutateResponse: `{"id":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01T00:00:00Z"}`,
		},
		// Forgejo release shape mirrors gitea: github-like keys with url.
		Releases: &contracttest.ReleasesHarnessConfig{
			ByTagResponse:  `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0","body":"release notes","draft":false,"prerelease":false,"url":"https://forgejo.example.com/owner/repo/releases/tag/v1.0.0","created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
			UpdateResponse: `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0-renamed","body":"updated notes","draft":false,"prerelease":false,"url":"https://forgejo.example.com/owner/repo/releases/tag/v1.0.0","created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
		},
		// Forgejo search mirrors gitea: repo/user results wrapped in
		// {"data":[..]}, issue search a bare array keyed by number.
		Search: &contracttest.SearchHarnessConfig{
			ReposResponse:  `{"ok":true,"data":[{"id":1,"full_name":"owner/repo","description":"search hit","html_url":"https://forgejo.example.com/owner/repo","stars_count":5,"forks_count":2,"default_branch":"main","private":false,"owner":{"login":"owner"}}]}`,
			IssuesResponse: `[{"number":1,"title":"found","state":"open","body":"b","html_url":"https://forgejo.example.com/owner/repo/issues/1","labels":[{"name":"bug"}],"comments":2,"created_at":"2026-01-01T00:00:00Z","repository":{"full_name":"owner/repo"}}]`,
			UsersResponse:  `{"ok":true,"data":[{"login":"dev","full_name":"Dev","avatar_url":"https://forgejo.example.com/avatars/1","html_url":"https://forgejo.example.com/dev"}]}`,
		},
		// Commit status fixtures: forgejo's commit-status shape mirrors
		// gitea (GET /repos/{o}/{r}/commits/{ref}/statuses), a bare array
		// where the state verb rides the "status" key.
		CommitStatus: &contracttest.CommitStatusHarnessConfig{
			ListResponse: `[{"id":1,"status":"success","context":"ci/lint","description":"lint passed","target_url":"https://ci.example.com/1","url":"https://forgejo.example.com/api/v1/repos/owner/repo/statuses/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:10Z"},{"id":2,"status":"pending","context":"ci/test","description":"tests running","target_url":"https://ci.example.com/2","url":"https://forgejo.example.com/api/v1/repos/owner/repo/statuses/2","created_at":"2026-01-01T00:00:05Z","updated_at":"2026-01-01T00:00:05Z"}]`,
		},
		Notifications: &contracttest.NotificationsHarnessConfig{
			ListResponse: `[{"id":1,"unread":true,"subject":{"title":"Bug report","type":"Issue","url":"https://forgejo.com/api/v1/repos/owner/repo/issues/1"},"repository":{"id":1,"full_name":"owner/repo"},"updated_at":"2026-01-01T00:00:00Z"}]`,
		},
		Reactions: &contracttest.ReactionsHarnessConfig{
			ListResponse:   `[{"content":"+1","user":{"id":1,"login":"dev"}}]`,
			CreateResponse: `{"content":"heart","user":{"id":1,"login":"dev"}}`,
		},
		BranchProtections: &contracttest.BranchProtectionsHarnessConfig{
			ListResponse:   `[{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}]`,
			MutateResponse: `{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}`,
		},
		Collaborators: &contracttest.CollaboratorsHarnessConfig{
			ListResponse: `[{"id":1,"login":"dev","permission":"write"}]`,
		},
		DeployKeys: &contracttest.DeployKeysHarnessConfig{
			ListResponse:   `[{"id":1,"title":"CI","key":"ssh-rsa AAAA","read_only":true}]`,
			CreateResponse: `{"id":1,"title":"CI","key":"ssh-rsa AAAA","read_only":true}`,
		},
		RepoStats: &contracttest.RepoStatsHarnessConfig{
			ForksResponse:      `[{"id":1,"full_name":"fork/repo","name":"repo","owner":{"login":"fork"}}]`,
			StargazersResponse: `[{"id":1,"login":"star"}]`,
		},
	})
}

func TestForgejo_WebhookCorpus(t *testing.T) {
	// The webhook parser now reads the family descriptor set at construction
	// (platform, event header), so the corpus runs against a real provider
	// rather than a zero-value struct.
	p := newTestProvider(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	contracttest.RunWebhookCorpus(t, provider.PlatformForgejo, p, "testdata/webhooks")
}
