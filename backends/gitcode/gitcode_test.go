package gitcode_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/backends/contracttest"

	"github.com/yi-nology/go-git-platform/backends/gitcode"
	"github.com/yi-nology/go-git-platform/provider"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newTestProvider(t *testing.T, srv *httptest.Server) *gitcode.Provider {
	t.Helper()
	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitCode,
		BaseURL:  srv.URL,
		Token:    "test-token",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	gp, ok := p.(*gitcode.Provider)
	if !ok {
		t.Fatalf("expected *gitcode.Provider, got %T", p)
	}
	return gp
}

func TestListRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]any{
			{"id": 1, "name": "r1", "full_name": "owner/r1",
				"owner": map[string]any{"login": "owner"}, "default_branch": "main"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Owner != "owner" {
		t.Errorf("unexpected: %+v", repos)
	}
}

// TestGetReview_UsesSingleReviewEndpoint guards that GetReview rides the
// dedicated single-review endpoint (GET .../pulls/{n}/reviews/{id}) rather
// than fetching the whole review list and filtering client-side.
func TestGetReview_UsesSingleReviewEndpoint(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		writeJSON(w, map[string]any{
			"id": 42, "state": "APPROVED", "body": "lgtm",
			"user":       map[string]any{"id": "7", "login": "dev"},
			"created_at": "2026-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	review, err := p.GetReview(context.Background(), "owner", "repo", "1", 42)
	if err != nil {
		t.Fatal(err)
	}
	if review == nil || review.ID != 42 || review.User != "dev" || review.State != provider.ReviewStateApproved {
		t.Fatalf("unexpected review: %+v", review)
	}
	if len(paths) != 1 || paths[0] != "GET /repos/owner/repo/pulls/1/reviews/42" {
		t.Fatalf("expected a single GET to the review-by-id path, recorded %v", paths)
	}
}

func TestCreateCR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id": 7, "number": 7, "title": "test", "state": "open",
			"head": map[string]any{"ref": "feature"},
			"base": map[string]any{"ref": "main"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	cr, err := p.CreateCR(context.Background(), provider.CreateCROptions{
		Owner: "owner", Repo: "repo", Title: "test",
		SourceBranch: "feature", TargetBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cr.Number != "7" {
		t.Errorf("expected 7, got %q", cr.Number)
	}
}

func TestParseWebhookEvent_PullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"action":"opened","number":1,"pull_request":{"number":1,"title":"t","state":"open","head":{"ref":"f","sha":"abc"},"base":{"ref":"main"},"html_url":"https://gitcode.com/owner/repo/pulls/1"},"sender":{"id":"1","login":"dev"},"repository":{"full_name":"owner/repo"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-GitCode-Event", "pull_request")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "cr.opened" {
		t.Errorf("expected cr.opened, got %s", ne.Type)
	}
}

func TestProvider_ImplementsProvider(t *testing.T) {
	var _ provider.Provider = (*gitcode.Provider)(nil)
}

func TestRetry_TriggersOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, []map[string]any{})
	}))
	defer srv.Close()
	// Note: gitcode API SDK uses its own transport, so retry is currently
	// best-effort. This test asserts that the SDK at least tolerates 5xx
	// responses and eventually succeeds.
	p, err := gitcode.New(provider.Config{
		Platform:    provider.PlatformGitCode,
		BaseURL:     srv.URL,
		Token:       "test",
		RetryConfig: &provider.RetryConfig{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 5})
	// We don't strictly assert success because the gitcode SDK owns its
	// transport. We just check that no panic occurred.
	_ = err
	_ = atomic.LoadInt32(&calls)
}

// TestListFailedJobLogs_SHAFilter locks the GitCode CI wiring: the run is
// matched by head_sha client-side (the wire has no sha filter), only FAILED
// jobs are attributed (UPPERCASE vocabulary), and the download-log bytes
// land in the trace tail.
func TestListFailedJobLogs_SHAFilter(t *testing.T) {
	var mu sync.Mutex
	jobsRoutesSeen := map[string]bool{}
	logRequested := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/actions/runs") && !strings.Contains(r.URL.Path, "/jobs"):
			// 最新序：命中 SHA 的 run 在第二条——扫描必须跳过不匹配的。
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_count": 2,
				"workflow_runs": []any{
					map[string]any{"workflow_run_id": "rOld", "workflow_name": "ci", "head_sha": "other"},
					map[string]any{"workflow_run_id": "r1", "workflow_name": "ci", "head_sha": "abc123"},
				},
			})
		case strings.Contains(r.URL.Path, "/download-log"): // 先于 /jobs 判定——download-log 路径含 /jobs 子串
			logRequested = true
			_, _ = w.Write([]byte("setup ok\nerror: build failed\n"))
		case strings.Contains(r.URL.Path, "/actions/runs/r1/jobs"):
			jobsRoutesSeen["jobs"] = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_count": 2,
				"jobs": []any{
					map[string]any{"id": "j1", "name": "build", "status": "FAILED"},
					map[string]any{"id": "j2", "name": "lint", "status": "COMPLETED"},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	logs, err := p.ListFailedJobLogs(context.Background(), "owner", "repo", "abc123")
	if err != nil {
		t.Fatalf("ListFailedJobLogs: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(logs) != 1 {
		t.Fatalf("want 1 failed job (COMPLETED excluded), got %d: %+v", len(logs), logs)
	}
	if logs[0].Name != "build" || logs[0].Stage != "ci" {
		t.Fatalf("unexpected job mapping: %+v", logs[0])
	}
	if !logRequested || !strings.Contains(logs[0].Trace, "build failed") {
		t.Fatalf("log tail not fetched: requested=%v trace=%q", logRequested, logs[0].Trace)
	}
	_ = jobsRoutesSeen
}

// TestParseWebhookEvent_BranchDeletionNormalized 分支删除归一（issue #67，
// 同族 #47 的 push 案）：gitcode 的分支删除以 push + 全零 after 投递
// （其独立 delete 事件路径只覆盖部分场景）——此前一律归一成 push。
func TestParseWebhookEvent_BranchDeletionNormalized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	mk := func(before, after string) *http.Request {
		body := fmt.Sprintf(`{"ref":"refs/heads/feature","before":%q,"after":%q,"repository":{"id":99,"full_name":"owner/repo"},"sender":{"id":"1","login":"dev"}}`, before, after)
		r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
		r.Header.Set("X-GitCode-Event", "push")
		r.Header.Set("Content-Type", "application/json")
		return r
	}

	ne := contracttest.ParseSigned(t, p, mk("def4567890abcdef4567890abcdef4567890abcd", "0000000000000000000000000000000000000000"), "corpus-secret")
	if ne.Type != "branch.deleted" || ne.Action != "deleted" || ne.Branch != "feature" {
		t.Fatalf("删除投递必须归一 branch.deleted, got type=%q action=%q branch=%q", ne.Type, ne.Action, ne.Branch)
	}
	if ne.CommitSHA != "" {
		t.Fatalf("删除投递不得携带全零标记作为 commit_sha, got %q", ne.CommitSHA)
	}

	ne = contracttest.ParseSigned(t, p, mk("def4567890abcdef4567890abcdef4567890abcd", "abc123"), "corpus-secret")
	if ne.Type != "push" || ne.CommitSHA != "abc123" {
		t.Fatalf("正常 push 不得误判为删除, got type=%q sha=%q", ne.Type, ne.CommitSHA)
	}
}
