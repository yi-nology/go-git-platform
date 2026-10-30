package gitee_test

import (
	"testing"

	"github.com/yi-nology/go-git-platform/backends/contracttest"
	gitee "github.com/yi-nology/go-git-platform/backends/gitee"
	"github.com/yi-nology/go-git-platform/provider"
)

func TestGitee_Contract(t *testing.T) {
	contracttest.Run(t, contracttest.Harness{
		Name:     "Gitee",
		Platform: provider.PlatformGitee,
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
			ListResponse:   `[{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}]`,
			MutateResponse: `{"id":1,"name":"bug","color":"#4cc917","description":"something broke"}`,
		},
		// Gitee wire shapes: issue number is a string (alphanumeric
		// identifiers), the milestone ref carries the milestone serial
		// number, the assignee is a single user object, and comments'
		// created_at is a timestamp string (the SDK Note model has no
		// updated_at).
		Issues: &contracttest.IssuesHarnessConfig{
			ListResponse: `[{"id":1,"number":"1","title":"bug","body":"broke","state":"open","user":{"id":1,"login":"dev","name":"Dev"},"labels":[],"assignee":{"id":1,"login":"dev","name":"Dev"},"milestone":{"id":1,"number":1,"title":"v1"},"html_url":"https://gitee.com/owner/repo/issues/I1AB2C","created_at":"2026-08-15T10:00:00+08:00","updated_at":"2026-08-15T11:00:00+08:00"}]`,
			GetResponse:  `{"id":1,"number":"1","title":"bug","body":"broke","state":"open","user":{"id":1,"login":"dev","name":"Dev"},"labels":[],"assignee":{"id":1,"login":"dev","name":"Dev"},"milestone":{"id":1,"number":1,"title":"v1"},"html_url":"https://gitee.com/owner/repo/issues/I1AB2C","created_at":"2026-08-15T10:00:00+08:00","updated_at":"2026-08-15T11:00:00+08:00"}`,
			MutateResponse: `{"id":1,"number":"1","title":"bug","body":"broke","state":"open","user":{"id":1,"login":"dev","name":"Dev"},"labels":[],"assignee":{"id":1,"login":"dev","name":"Dev"},` +
				`"milestone":{"id":1,"number":1,"title":"v1"},"html_url":"https://gitee.com/owner/repo/issues/I1AB2C","created_at":"2026-08-15T10:00:00+08:00","updated_at":"2026-08-15T11:00:00+08:00"}`,
			CommentsResponse: `[{"id":1,"body":"a comment","user":{"id":1,"login":"dev","name":"Dev"},"created_at":"2026-08-15T10:30:00+08:00","updated_at":"2026-08-15T10:35:00+08:00"}]`,
			LabelsResponse:   `[{"id":1,"name":"bug","color":"#4cc917"}]`,
		},
		// Gitee release shape (raw transport; the SDK model mis-types the
		// payload — see backends/gitee/releases.go): github-like keys with
		// html_url, served by the same mock for by-tag fetches and the
		// tag→id resolution that precedes update/delete.
		// Gitee milestone shape: keyed by the serial "number" (the write
		// endpoints address milestones by it), date-only due_on. Create and
		// update ride the raw transport (SDK multipart bug), the rest the SDK.
		Milestones: &contracttest.MilestonesHarnessConfig{
			ListResponse:   `[{"number":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01"}]`,
			MutateResponse: `{"number":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01"}`,
		},
		Releases: &contracttest.ReleasesHarnessConfig{
			ByTagResponse:  `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0","body":"release notes","html_url":"https://gitee.com/owner/repo/releases/v1.0.0","draft":false,"prerelease":false,"created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
			UpdateResponse: `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0-renamed","body":"updated notes","html_url":"https://gitee.com/owner/repo/releases/v1.0.0","draft":false,"prerelease":false,"created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
		},
		// Gitee search payloads are bare arrays; issue numbers are
		// alphanumeric strings on the wire.
		Search: &contracttest.SearchHarnessConfig{
			ReposResponse:  `[{"full_name":"owner/repo","description":"search hit","html_url":"https://gitee.com/owner/repo","stargazers_count":5,"forks_count":2,"default_branch":"main","private":false}]`,
			IssuesResponse: `[{"number":"1","title":"found","state":"open","body":"b","html_url":"https://gitee.com/owner/repo/issues/1","labels":[{"name":"bug"}],"comments":2,"created_at":"2026-01-01T00:00:00Z"}]`,
			UsersResponse:  `[{"login":"dev","name":"Dev","avatar_url":"https://gitee.com/avatars/1","html_url":"https://gitee.com/dev"}]`,
		},
		Notifications: &contracttest.NotificationsHarnessConfig{
			ListResponse: `{"total_count":1,"list":[{"id":1,"unread":"true","type":"Issue","subject":{"title":"Bug report","type":"Issue","url":"https://gitee.com/api/v5/repos/owner/repo/issues/1"},"repository":{"id":1,"full_name":"owner/repo"},"updated_at":"2026-01-01T00:00:00+08:00"}]}`,
		},
		BranchProtections: &contracttest.BranchProtectionsHarnessConfig{
			ListResponse:   `[{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}]`,
			MutateResponse: `{"branch_name":"main","required_approving_reviews":1,"required_status_checks":true,"allow_force_pushes":false,"allow_deletions":false}`,
		},
		Collaborators: &contracttest.CollaboratorsHarnessConfig{
			ListResponse: `[{"id":1,"login":"dev","permission":"write"}]`,
		},
		WebhookConfig: &contracttest.WebhookConfigHarnessConfig{
			GetResponse: `{"id":42,"url":"https://hooks.example/ci","push_events":true,"merge_requests_events":true}`,
		},
		RepoStats: &contracttest.RepoStatsHarnessConfig{
			ForksResponse:        `[{"id":1,"full_name":"fork/repo","name":"repo","owner":{"login":"fork"}}]`,
			ContributorsResponse: `[{"login":"dev","contributions":10}]`,
		},
		DeployKeys: &contracttest.DeployKeysHarnessConfig{
			ListResponse:   `[{"id":1,"title":"CI","key":"ssh-rsa AAAA"}]`,
			CreateResponse: `{"id":1,"title":"CI","key":"ssh-rsa AAAA"}`,
		},
		// Gitee commit-status fixtures: the public API has no commit-status
		// endpoint, so reads go through the Checks API
		// (GET /repos/{o}/{r}/commits/{ref}/check-runs) — the response wraps
		// the runs in {"total_count":..,"check_runs":[..]}; a completed run
		// carries its outcome in "conclusion", an in-flight one in "status".
		CommitStatus: &contracttest.CommitStatusHarnessConfig{
			ListResponse: `{"total_count":2,"check_runs":[{"id":1,"name":"ci/lint","head_sha":"deadbeef","status":"completed","conclusion":"success","details_url":"https://ci.example.com/1"},{"id":2,"name":"ci/test","head_sha":"deadbeef","status":"in_progress","details_url":"https://ci.example.com/2"}]}`,
			// the check-run list decodes into a wrapped object, not a bare array
			EmptyResponse: `{"total_count":0,"check_runs":[]}`,
		},
	})
}

func TestGitee_WebhookCorpus(t *testing.T) {
	contracttest.RunWebhookCorpus(t, provider.PlatformGitee, &gitee.Provider{}, "testdata/webhooks")
}
