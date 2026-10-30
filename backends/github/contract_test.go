package github_test

import (
	"encoding/json"
	"testing"

	sdkgithub "github.com/google/go-github/v92/github"

	"github.com/yi-nology/go-git-platform/backends/contracttest"
	ghbackend "github.com/yi-nology/go-git-platform/backends/github"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitHub_Contract runs the cross-platform contract suite against the
// GitHub backend.
func TestGitHub_Contract(t *testing.T) {
	contracttest.Run(t, contracttest.Harness{
		Name:     "GitHub",
		Platform: provider.PlatformGitHub,
		NewProvider: func(t *testing.T, cfg provider.Config) provider.Provider {
			cfg.BaseURL = cfg.BaseURL + "/api/v3"
			p, err := provider.NewProvider(cfg)
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			return p
		},
		EmptyListResponse:    "[]",
		NonEmptyListResponse: githubNonEmptyList(),
		Labels: &contracttest.LabelsHarnessConfig{
			ListResponse:   `[{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}]`,
			MutateResponse: `{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}`,
		},
		Issues: &contracttest.IssuesHarnessConfig{
			ListResponse:     `[{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"number":1,"title":"v1"},"labels":[{"id":1,"name":"bug","color":"4cc917"}],"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:      `{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"number":1,"title":"v1"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse:   `{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"number":1,"title":"v1"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			CommentsResponse: `[{"id":1,"body":"a comment","user":{"login":"dev"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			LabelsResponse:   `[{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}]`,
		},
		Reviews: &contracttest.ReviewsHarnessConfig{
			ListResponse:   `[{"id":1,"user":{"login":"dev"},"state":"APPROVED","body":"looks good","submitted_at":"2026-01-01T00:00:00Z","html_url":"https://example.com/pull/1#review-1"}]`,
			GetResponse:    `{"id":1,"user":{"login":"dev"},"state":"APPROVED","body":"looks good","submitted_at":"2026-01-01T00:00:00Z","html_url":"https://example.com/pull/1#review-1"}`,
			MutateResponse: `{"id":1,"user":{"login":"dev"},"state":"APPROVED","body":"looks good","submitted_at":"2026-01-01T00:00:00Z","html_url":"https://example.com/pull/1#review-1"}`,
			// CreateReview forwards CreateReviewOptions.Event verbatim
			// (PullRequestReviewRequest.Event), so an APPROVE verdict hits
			// the wire as "APPROVE".
			CreateEvent: "APPROVE",
		},
		// GitHub milestone shape: github-like keys (number/state/due_on);
		// milestones are number-addressed.
		Milestones: &contracttest.MilestonesHarnessConfig{
			ListResponse:   `[{"number":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01T00:00:00Z"}]`,
			MutateResponse: `{"number":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01T00:00:00Z"}`,
		},
		Releases: &contracttest.ReleasesHarnessConfig{
			ByTagResponse:  `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0","body":"release notes","draft":false,"prerelease":false,"html_url":"https://example.com/releases/v1.0.0","created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
			UpdateResponse: `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0-renamed","body":"updated notes","draft":false,"prerelease":false,"html_url":"https://example.com/releases/v1.0.0","created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
		},
		// GitHub search payloads wrap items in {"total_count":..,"items":[..]}.
		Search: &contracttest.SearchHarnessConfig{
			// The repos envelope's total_count is the server-side total the
			// backend must return as SearchRepos's total.
			ReposTotalCount: 1,
			ReposResponse:   `{"total_count":1,"incomplete_results":false,"items":[{"full_name":"owner/repo","description":"search hit","html_url":"https://github.com/owner/repo","stargazers_count":5,"forks_count":2,"default_branch":"main","private":false}]}`,
			IssuesResponse:  `{"total_count":1,"items":[{"number":1,"title":"found","state":"open","body":"b","html_url":"https://github.com/owner/repo/issues/1","labels":[{"name":"bug"}],"comments":2,"created_at":"2026-01-01T00:00:00Z"}]}`,
			UsersResponse:   `{"total_count":1,"items":[{"login":"dev","name":"Dev","avatar_url":"https://github.com/avatars/u/1","html_url":"https://github.com/dev"}]}`,
		},
		// Commit status fixtures: GitHub's native commit-status shape
		// (GET /repos/{o}/{r}/commits/{ref}/statuses), a bare array with
		// state/context/description/target_url.
		CommitStatus: &contracttest.CommitStatusHarnessConfig{
			ListResponse: `[{"state":"success","context":"ci/lint","description":"lint passed","target_url":"https://ci.example.com/1"},{"state":"pending","context":"ci/test","description":"tests running","target_url":"https://ci.example.com/2"}]`,
		},
		Notifications: &contracttest.NotificationsHarnessConfig{
			ListResponse: `[{"id":"1","unread":true,"reason":"subscribed","subject":{"title":"Bug report","type":"Issue","url":"https://api.github.com/repos/owner/repo/issues/1"},"repository":{"id":1,"full_name":"owner/repo"},"updated_at":"2026-01-01T00:00:00Z"}]`,
		},
		Reactions: &contracttest.ReactionsHarnessConfig{
			ListResponse:   `[{"id":1,"content":"+1","user":{"id":1,"login":"dev"}}]`,
			CreateResponse: `{"id":1,"content":"heart","user":{"id":1,"login":"dev"}}`,
		},
		BranchProtections: &contracttest.BranchProtectionsHarnessConfig{
			ListResponse:   `[{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}]`,
			MutateResponse: `{"required_status_checks":{"strict":true,"contexts":[]},"required_pull_request_reviews":{"dismiss_stale_reviews":true,"require_code_owner_reviews":false,"required_approving_review_count":1},"enforce_admins":{"enabled":true},"allow_force_pushes":{"enabled":false},"allow_deletions":{"enabled":false}}`,
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
	})
}

func githubNonEmptyList() string {
	repos := []*sdkgithub.Repository{
		{
			ID:       new(int64(1)),
			FullName: new("owner/repo"),
			Name:     new("repo"),
			Owner:    &sdkgithub.User{Login: new("owner")},
		},
	}
	b, _ := json.Marshal(repos)
	return string(b)
}

func TestGitHub_WebhookCorpus(t *testing.T) {
	contracttest.RunWebhookCorpus(t, provider.PlatformGitHub, &ghbackend.Provider{}, "testdata/webhooks")
}
