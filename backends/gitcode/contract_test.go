package gitcode_test

import (
	"testing"

	"github.com/yi-nology/go-git-platform/backends/contracttest"
	gitcode "github.com/yi-nology/go-git-platform/backends/gitcode"
	"github.com/yi-nology/go-git-platform/provider"
)

func TestGitCode_Contract(t *testing.T) {
	contracttest.Run(t, contracttest.Harness{
		Name:     "GitCode",
		Platform: provider.PlatformGitCode,
		NewProvider: func(t *testing.T, cfg provider.Config) provider.Provider {
			p, err := provider.NewProvider(cfg)
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			return p
		},
		EmptyListResponse:    `[]`,
		NonEmptyListResponse: `[{"id":1,"name":"repo","full_name":"owner/repo","owner":{"login":"owner"},"default_branch":"main"}]`,
		Labels: &contracttest.LabelsHarnessConfig{
			ListResponse:   `[{"id":1,"name":"bug","color":"#4cc917"}]`,
			MutateResponse: `{"id":1,"name":"bug","color":"#4cc917"}`,
			// The gitcode SDK's ListIssueLabels exposes no page/page-size
			// parameters, so the backend cannot forward pagination on the wire.
			IgnoresListPagination: true,
		},
		// GitCode's issue payloads are GitHub-shaped; milestones are
		// addressed by id, and labels carry '#'-prefixed colors.
		Issues: &contracttest.IssuesHarnessConfig{
			ListResponse:     `[{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"labels":[{"id":1,"name":"bug","color":"#4cc917"}],"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:      `{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse:   `{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			CommentsResponse: `[{"id":1,"body":"a comment","user":{"login":"dev"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			LabelsResponse:   `[{"id":1,"name":"bug","color":"#4cc917"}]`,
		},
		// GitCode review payloads are GitHub-shaped; the review author rides
		// the "user" (fallback "author") key and the timestamp is created_at.
		Reviews: &contracttest.ReviewsHarnessConfig{
			ListResponse:   `[{"id":1,"user":{"login":"dev"},"state":"APPROVED","body":"looks good","created_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:    `{"id":1,"user":{"login":"dev"},"state":"APPROVED","body":"looks good","created_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse: `{"id":1,"user":{"login":"dev"},"state":"APPROVED","body":"looks good","created_at":"2026-01-01T00:00:00Z"}`,
			// CreateReview forwards CreateReviewOptions.Event verbatim
			// (client.CreatePullRequestReview posts {"body":..,"event":..}),
			// so an APPROVE verdict hits the wire as "APPROVE".
			CreateEvent: "APPROVE",
		},
		// GitCode milestone shape: GitHub-like keys but keyed by id
		// (milestones are ID-addressed, matching MilestoneRef) with a
		// date-time due_date.
		Milestones: &contracttest.MilestonesHarnessConfig{
			ListResponse:   `[{"id":1,"title":"v1","state":"open","description":"first","due_date":"2026-01-01T00:00:00Z"}]`,
			MutateResponse: `{"id":1,"title":"v1","state":"open","description":"first","due_date":"2026-01-01T00:00:00Z"}`,
		},
		// GitCode search payloads are GitHub-shaped bare arrays; issue
		// numbers are strings on the wire.
		Search: &contracttest.SearchHarnessConfig{
			ReposResponse:  `[{"full_name":"owner/repo","description":"search hit","web_url":"https://gitcode.com/owner/repo","stargazers_count":5,"forks_count":2,"default_branch":"main","private":false}]`,
			IssuesResponse: `[{"number":"1","title":"found","state":"open","body":"b","html_url":"https://gitcode.com/owner/repo/issues/1","labels":[{"name":"bug"}],"comments":2,"created_at":"2026-01-01T00:00:00Z"}]`,
			UsersResponse:  `[{"login":"dev","name":"Dev","avatar_url":"https://gitcode.com/avatars/u/1","html_url":"https://gitcode.com/dev"}]`,
		},
		// GitCode release payloads are GitHub-shaped (id/tag_name/name/body/
		// draft/prerelease/html_url).
		Releases: &contracttest.ReleasesHarnessConfig{
			ByTagResponse:  `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0","body":"release notes","draft":false,"prerelease":false,"html_url":"https://gitcode.com/owner/repo/releases/v1.0.0","created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
			UpdateResponse: `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0-renamed","body":"updated notes","draft":false,"prerelease":false,"html_url":"https://gitcode.com/owner/repo/releases/v1.0.0","created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
		},
		// Commit status fixtures: gitcode's commit-status shape is
		// GitHub-like (GET /repos/{o}/{r}/statuses/{sha}), a bare array with
		// state/context/description/target_url.
		CommitStatus: &contracttest.CommitStatusHarnessConfig{
			ListResponse: `[{"id":1,"sha":"deadbeef","state":"success","context":"ci/lint","description":"lint passed","target_url":"https://ci.example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:10Z"},{"id":2,"sha":"deadbeef","state":"pending","context":"ci/test","description":"tests running","target_url":"https://ci.example.com/2","created_at":"2026-01-01T00:00:05Z","updated_at":"2026-01-01T00:00:05Z"}]`,
		},
		Notifications: &contracttest.NotificationsHarnessConfig{
			ListResponse: `[{"id":1,"unread":true,"reason":"subscribed","subject":{"title":"Bug report","type":"Issue","url":"https://gitcode.com/api/v5/repos/owner/repo/issues/1"},"repository":{"id":1,"full_name":"owner/repo"},"updated_at":"2026-01-01T00:00:00Z"}]`,
		},
		BranchProtections: &contracttest.BranchProtectionsHarnessConfig{
			ListResponse:   `[{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}]`,
			MutateResponse: `{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}`,
		},
		Collaborators: &contracttest.CollaboratorsHarnessConfig{
			ListResponse: `[{"id":1,"login":"dev","permission":"write"}]`,
		},
		WebhookConfig: &contracttest.WebhookConfigHarnessConfig{
			GetResponse: `{"id":42,"url":"https://hooks.example/ci","events":["push","pull_request"]}`,
		},
		DeployKeys: &contracttest.DeployKeysHarnessConfig{
			ListResponse:   `[{"id":1,"title":"CI","key":"ssh-rsa AAAA","read_only":true}]`,
			CreateResponse: `{"id":1,"title":"CI","key":"ssh-rsa AAAA","read_only":true}`,
		},
		RepoStats: &contracttest.RepoStatsHarnessConfig{
			ForksResponse:        `[{"id":1,"full_name":"fork/repo","name":"repo","owner":{"login":"fork"}}]`,
			StargazersResponse:   `[{"id":1,"login":"star"}]`,
			ContributorsResponse: `[{"login":"dev","contributions":10}]`,
		},
		Reactions: &contracttest.ReactionsHarnessConfig{
			ListResponse:   `[{"id":1,"content":"+1","user":{"id":1,"login":"dev"}}]`,
			CreateResponse: `{"id":1,"content":"heart","user":{"id":1,"login":"dev"}}`,
		},
	})
}

func TestGitCode_WebhookCorpus(t *testing.T) {
	// The gitcode parse path goes through the SDK client, so a real
	// (offline) provider instance is required rather than a zero value.
	p, err := gitcode.New(provider.Config{Token: "t"})
	if err != nil {
		t.Fatalf("gitcode.New: %v", err)
	}
	contracttest.RunWebhookCorpus(t, provider.PlatformGitCode, p, "testdata/webhooks")
}
