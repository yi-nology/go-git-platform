package gitcode_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
			// go-gitcode v0.8.0 exposes the documented page/per_page query on
			// the label list, so the backend forwards pagination on the wire
			// (IgnoresListPagination retired with the SDK fix).
		},
		// GitCode's issue payloads are GitHub-shaped; milestones are
		// addressed by id, and labels carry '#'-prefixed colors.
		Issues: &contracttest.IssuesHarnessConfig{
			ListResponse:     `[{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"labels":[{"id":1,"name":"bug","color":"#4cc917"}],"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:      `{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse:   `{"number":1,"title":"bug","state":"open","user":{"login":"dev"},"milestone":{"id":1,"title":"v1"},"html_url":"https://example.com/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			CommentsResponse: `[{"id":1,"body":"a comment","user":{"login":"dev"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			LabelsResponse:   `[{"id":1,"name":"bug","color":"#4cc917"}]`,
			// 2A 三态能力矩阵（T6）：body/milestone B 级——issue-update
			// wire 是 string/int64+omitempty，空值不可表达且无清空侧端
			// 点（patch 清空零 HTTP 返回 ErrClearNotSupported）；
			// labels/assignees A 级走侧端点。labels 的 BodyField 省略：
			// SDK ReplaceIssueLabels 发裸 JSON 数组 []（GitHub 形态），
			// harness 的 recorded body 只解 object、BodyField 断言不可
			// 达——清空落线由 Method+Path 钉住，裸数组 wire 形态另由
			// TestGitCode_TriStateSideEndpointWire 钉（body 恒为 []）。
			// assignees 侧端点按 brief 不带 BodyField（Method+Path 钉）。
			IssueClear: map[string]contracttest.ClearExpect{
				"body":      {Unsupported: true},
				"assignees": {Method: http.MethodDelete, PathContains: "assignees"}, // 侧端点
				"labels":    {Method: http.MethodPut, PathContains: "labels"},       // 侧端点，裸数组 body
				"milestone": {Unsupported: true},
			},
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
		// GitCode milestone shape: real deployments key milestones by id
		// (the addressing value), the documented v5 shape carries number and
		// a date-only due_on — the fixture carries both identifier keys and
		// the documented date form.
		Milestones: &contracttest.MilestonesHarnessConfig{
			ListResponse:   `[{"id":1,"number":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01"}]`,
			GetResponse:    `{"id":1,"number":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01"}`,
			MutateResponse: `{"id":1,"number":1,"title":"v1","state":"open","description":"first","due_on":"2026-01-01"}`,
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
		// Family suites (issue #63): GitCode wire shapes (GitHub-like).
		Branches: &contracttest.BranchHarnessConfig{
			ListResponse:   `[{"name":"main","commit":{"sha":"abc123"}}]`,
			MutateResponse: `{"name":"feature-y","commit":{"sha":"abc123"}}`,
		},
		Commits: &contracttest.CommitHarnessConfig{
			GetResponse:     `{"sha":"abc123","message":"commit msg","author":{"id":"1","login":"dev"}}`,
			ListResponse:    `[{"sha":"abc123","message":"commit msg","author":{"id":"1","login":"dev"}}]`,
			CompareResponse: `{"total_commits":1,"commits":[{"sha":"abc123","message":"commit msg"}],"files":[]}`,
		},
		Diffs: &contracttest.DiffHarnessConfig{
			FilesResponse: `[{"filename":"a.txt","status":"modified","additions":1,"deletions":0,"patch":"@@ -1,1 +1,1 @@\n+hello"}]`,
		},
		Files: &contracttest.FileHarnessConfig{
			GetResponse: `{"content":"aGVsbG8gd29ybGQ=","encoding":"base64"}`,
		},
		// CR suite fixtures (issue #63): GitCode pull shape — number-addressed,
		// head/base keys, string author id.
		CRs: &contracttest.CRHarnessConfig{
			ListResponse:   `[{"id":1,"number":1,"title":"fix bug","state":"open","body":"desc","head":{"ref":"feature-x"},"base":{"ref":"main"},"author":{"id":"1","login":"dev"},"html_url":"https://example.com/pulls/1","draft":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
			GetResponse:    `{"id":1,"number":1,"title":"fix bug","state":"open","body":"desc","head":{"ref":"feature-x"},"base":{"ref":"main"},"author":{"id":"1","login":"dev"},"html_url":"https://example.com/pulls/1","draft":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			MutateResponse: `{"id":1,"number":1,"title":"fix bug","state":"open","body":"desc","head":{"ref":"feature-x"},"base":{"ref":"main"},"author":{"id":"1","login":"dev"},"html_url":"https://example.com/pulls/1","draft":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			// 2A 三态（T6）：GitCode 的 CR 面两字段均 B 级——description
			// 的 wire 键是 body（string+omitempty 丢空串）；pull-update
			// wire 无 assignee 字段，AssigneesPatch 任意指定均哨兵
			// （legacy opts.Assignees 维持既有 ignore 静默丢弃）。
			// T2 审查 Minor 3：B 级条目也带 BodyField——unset 断言的
			// unsetKeys 词表由 canonical ∪ BodyField 构成，CR 的
			// description→body 键错位若不声明，unset 面对 "body" 的覆盖
			// 就只剩 gitcode omitempty 的运气（B 分支提前返回、
			// BodyField 仅进 unsetKeys，零行为变化）。
			CRClear: map[string]contracttest.ClearExpect{
				"description": {Unsupported: true, BodyField: "body"},
				"assignees":   {Unsupported: true, BodyField: "assignees"},
			},
		},
		Releases: &contracttest.ReleasesHarnessConfig{
			ByTagResponse:    `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0","body":"release notes","draft":false,"prerelease":false,"html_url":"https://gitcode.com/owner/repo/releases/v1.0.0","created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
			UpdateResponse:   `{"id":1,"tag_name":"v1.0.0","name":"v1.0.0-renamed","body":"updated notes","draft":false,"prerelease":false,"html_url":"https://gitcode.com/owner/repo/releases/v1.0.0","created_at":"2026-01-01T00:00:00Z","published_at":"2026-01-01T00:00:00Z"}`,
			TagsListResponse: `[{"name":"v1","commit":{"sha":"abc123def456"}}]`,
		},
		// Commit status fixtures: gitcode's commit-status shape is
		// GitHub-like (GET /repos/{o}/{r}/statuses/{sha}), a bare array with
		// state/context/description/target_url.
		// Notifications: GitCode serves only the repo-scoped half of the
		// inbox — the global routes 404 live (probe round 2026-11-03); the
		// suite pins the global methods to ErrNotImplemented and keeps the
		// repo-scoped assertions green.
		Notifications: &contracttest.NotificationsHarnessConfig{
			ListResponse:      `[{"id":1,"unread":true,"reason":"subscribed","subject":{"title":"Bug report","type":"Issue","url":"https://gitcode.com/api/v5/repos/owner/repo/issues/1"},"repository":{"id":1,"full_name":"owner/repo"},"updated_at":"2026-01-01T00:00:00Z"}]`,
			GlobalUnsupported: true,
		},
		Collaborators: &contracttest.CollaboratorsHarnessConfig{
			ListResponse: `[{"id":1,"login":"dev","permission":"write"}]`,
		},
		WebhookConfig: &contracttest.WebhookConfigHarnessConfig{
			// GitCode's hook wire expresses subscriptions as per-family
			// flags (integers in the documented response schema) — there is
			// no events array; the backend maps flags onto the neutral names.
			GetResponse: `{"id":42,"url":"https://hooks.example/ci","push_events":1,"merge_requests_events":1}`,
		},
		RepoStats: &contracttest.RepoStatsHarnessConfig{
			ForksResponse:        `[{"id":1,"full_name":"fork/repo","name":"repo","owner":{"login":"fork"}}]`,
			StargazersResponse:   `[{"id":1,"login":"star"}]`,
			ContributorsResponse: `[{"login":"dev","contributions":10}]`,
		},
		// CI log fixtures: the mock serves one body on every route, so the
		// failed-response fixture carries BOTH the runs list (workflow_runs)
		// and the jobs list (jobs) — the backend parses the runs key first
		// (FindActionRunByHeadSHA), then the jobs key (ListActionRunJobs),
		// then treats the raw bytes as the job log (download-log). The job
		// status "FAILED" follows the UPPERCASE vocabulary the SDK fixtures
		// document (COMPLETED/RUNNING).
		CILogs: &contracttest.CILogsHarnessConfig{
			GreenResponse:  `{"total_count":0,"workflow_runs":[]}`,
			FailedResponse: `{"total_count":1,"workflow_runs":[{"workflow_run_id":"r1","workflow_name":"ci","head_sha":"abc123"}],"jobs":[{"id":"j1","name":"build","status":"FAILED"}]}`,
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

// TestGitCode_TriStateSideEndpointWire 钉侧端点清空的真实 wire 形态——补
// harness recorded-body 只解 JSON object 的盲区（IssueClear.labels 因此
// 省略 BodyField）：
//
//   - ReplaceIssueLabels 发裸 JSON 数组 []（SDK 把 labels 切片本身当
//     body，GitHub 形态，不是 {"labels":[]} 对象）；
//   - RemoveIssueAssignees 发 {"assignees":[]}。
//
// 同时钉顺序裁定：主 update（PATCH，携带置值字段）先到，侧端点清空后到
// 且是本次调用的最后一条请求；B 级字段（body/milestone/CR 面）的零请求
// 形态由 contracttest dispatchClear 断言，这里不重复。
func TestGitCode_TriStateSideEndpointWire(t *testing.T) {
	type rec struct{ method, path, body string }
	var mu sync.Mutex
	var got []rec
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, rec{method: r.Method, path: r.URL.Path, body: string(raw)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent) // SDK: 204 跳过响应解码
		default:
			_, _ = w.Write([]byte(`{"number":1,"title":"bug","state":"open"}`))
		}
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitCode, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	im, ok := p.(provider.IssueManager)
	if !ok {
		t.Fatal("provider does not implement provider.IssueManager")
	}
	ctx := context.Background()
	drain := func() []rec {
		mu.Lock()
		defer mu.Unlock()
		refs := append([]rec(nil), got...)
		got = nil
		return refs
	}

	// labels 清空：PATCH（空 update）→ PUT labels（裸 []）。
	if _, err := im.UpdateIssue(ctx, "owner", "repo", "1", provider.UpdateIssueOptions{LabelsPatch: &[]string{}}); err != nil {
		t.Fatalf("labels clear: %v", err)
	}
	if reqs := drain(); len(reqs) != 2 {
		t.Fatalf("labels clear: want PATCH + PUT (main first, side clear last), got %d: %+v", len(reqs), reqs)
	} else {
		if reqs[0].method != http.MethodPatch || reqs[1].method != http.MethodPut {
			t.Errorf("labels clear order: want PATCH then PUT, got %s then %s", reqs[0].method, reqs[1].method)
		}
		if !strings.Contains(reqs[1].path, "/labels") {
			t.Errorf("labels clear: last path = %q, want .../labels", reqs[1].path)
		}
		if reqs[1].body != "[]" {
			t.Errorf("labels clear wire body = %q, want bare JSON array []", reqs[1].body)
		}
	}

	// assignees 清空：PATCH（空 update）→ DELETE {"assignees":[]}。
	if _, err := im.UpdateIssue(ctx, "owner", "repo", "1", provider.UpdateIssueOptions{AssigneesPatch: &[]string{}}); err != nil {
		t.Fatalf("assignees clear: %v", err)
	}
	if reqs := drain(); len(reqs) != 2 {
		t.Fatalf("assignees clear: want PATCH + DELETE (main first, side clear last), got %d: %+v", len(reqs), reqs)
	} else {
		if reqs[0].method != http.MethodPatch || reqs[1].method != http.MethodDelete {
			t.Errorf("assignees clear order: want PATCH then DELETE, got %s then %s", reqs[0].method, reqs[1].method)
		}
		if !strings.Contains(reqs[1].path, "/assignees") {
			t.Errorf("assignees clear: last path = %q, want .../assignees", reqs[1].path)
		}
		if reqs[1].body != `{"assignees":[]}` {
			t.Errorf("assignees clear wire body = %q, want {\"assignees\":[]}", reqs[1].body)
		}
	}
}
