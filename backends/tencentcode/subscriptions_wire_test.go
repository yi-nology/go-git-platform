package tencentcode_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/tencentcode"
)

// TestTencentCode_SubscriptionExtrasWire pins the 工蜂-native subscription
// surface behind TencentCodeExtras: issue watch state rides
// /issues/{id}/subscribe (GET=read, PUT=subscribe) with unsubscribe as its
// own PUT path (/issues/{id}/unsubscribe); MR watch rides
// /merge_request/{id}/subscribe (GET/PUT) and /unsubscribe (PUT); the MR
// mutations echo {"subscribed":b} while the issue read is a bare bool.
// Recorded pins use the escaped owner%2Frepo segment (工蜂 embeds the
// project path in the URL).
func TestTencentCode_SubscriptionExtrasWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/projects/owner/repo/issues/9/subscribe":
			writeJSON(w, true)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v3/projects/owner/repo/issues/9/subscribe":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v3/projects/owner/repo/issues/9/unsubscribe":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/projects/owner/repo/merge_request/12/subscribe",
			r.Method == http.MethodPut && r.URL.Path == "/api/v3/projects/owner/repo/merge_request/12/subscribe":
			writeJSON(w, map[string]any{"subscribed": true})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v3/projects/owner/repo/merge_request/12/unsubscribe":
			writeJSON(w, map[string]any{"subscribed": false})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv) // *tencentcode.Provider implements TencentCodeExtras directly

	subscribed, err := p.GetIssueSubscription(context.Background(), "owner", "repo", 9)
	if err != nil || !subscribed {
		t.Fatalf("GetIssueSubscription: %v %v", subscribed, err)
	}
	if err := p.SubscribeIssue(context.Background(), "owner", "repo", 9); err != nil {
		t.Fatalf("SubscribeIssue: %v", err)
	}
	if err := p.UnsubscribeIssue(context.Background(), "owner", "repo", 9); err != nil {
		t.Fatalf("UnsubscribeIssue: %v", err)
	}
	if subscribed, err = p.GetMRSubscription(context.Background(), "owner", "repo", 12); err != nil || !subscribed {
		t.Fatalf("GetMRSubscription: %v %v", subscribed, err)
	}
	if subscribed, err = p.SubscribeMR(context.Background(), "owner", "repo", 12); err != nil || !subscribed {
		t.Fatalf("SubscribeMR: %v %v", subscribed, err)
	}
	if subscribed, err = p.UnsubscribeMR(context.Background(), "owner", "repo", 12); err != nil || subscribed {
		t.Fatalf("UnsubscribeMR must echo false: %v %v", subscribed, err)
	}

	mu.Lock()
	defer mu.Unlock()
	assertCalls(t, calls, []string{
		"GET /api/v3/projects/owner%2Frepo/issues/9/subscribe",
		"PUT /api/v3/projects/owner%2Frepo/issues/9/subscribe",
		"PUT /api/v3/projects/owner%2Frepo/issues/9/unsubscribe",
		"GET /api/v3/projects/owner%2Frepo/merge_request/12/subscribe",
		"PUT /api/v3/projects/owner%2Frepo/merge_request/12/subscribe",
		"PUT /api/v3/projects/owner%2Frepo/merge_request/12/unsubscribe",
	})
}

// TestTencentCode_GlobalAndMilestoneIssuesWire pins the cross-project issue
// listing (global /issues with query filters, no project segment) and the
// milestone-scoped issue listing (projects/{pid}/milestones/{id}/issues);
// both are caller-driven single pages, so the mock serves exactly one page.
func TestTencentCode_GlobalAndMilestoneIssuesWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var globalQuery, milestoneQuery string
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/issues":
			globalQuery = r.URL.RawQuery
			writeJSON(w, []map[string]any{
				{"id": 3, "iid": 9, "title": "global issue", "state": "opened",
					"labels": []string{"bug"}, "author": map[string]any{"id": 1, "username": "dev"}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/projects/owner/repo/milestones/5/issues":
			milestoneQuery = r.URL.RawQuery
			writeJSON(w, []map[string]any{
				{"id": 4, "iid": 2, "title": "milestone issue", "state": "closed",
					"assignee": map[string]any{"id": 1, "username": "dev"}},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)

	issues, err := p.ListUserIssues(context.Background(), tencentcode.ListUserIssuesOptions{
		State: "opened", Labels: "bug", Page: 2, PerPage: 50,
	})
	if err != nil || len(issues) != 1 || issues[0].ID != 3 || issues[0].State != "open" {
		t.Fatalf("ListUserIssues: %+v %v", issues, err)
	}
	// State maps the provider vocabulary onto 工蜂's wire word.
	for _, needle := range []string{"page=2", "per_page=50", "state=opened", "labels=bug"} {
		if !strings.Contains(globalQuery, needle) {
			t.Errorf("global listing query %q misses %q", globalQuery, needle)
		}
	}

	milestoneIssues, err := p.ListMilestoneIssues(context.Background(), "owner", "repo", 5, 1, 30)
	if err != nil || len(milestoneIssues) != 1 || milestoneIssues[0].ID != 4 || milestoneIssues[0].State != "closed" {
		t.Fatalf("ListMilestoneIssues: %+v %v", milestoneIssues, err)
	}
	if milestoneIssues[0].Assignees == nil || len(milestoneIssues[0].Assignees) != 1 || milestoneIssues[0].Assignees[0] != "dev" {
		t.Errorf("milestone issue assignee: %+v", milestoneIssues[0].Assignees)
	}
	for _, needle := range []string{"page=1", "per_page=30"} {
		if !strings.Contains(milestoneQuery, needle) {
			t.Errorf("milestone listing query %q misses %q", milestoneQuery, needle)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	assertCalls(t, calls, []string{
		"GET /api/v3/issues",
		"GET /api/v3/projects/owner%2Frepo/milestones/5/issues",
	})
}

// TestTencentCode_ArchiveDownloadsWire pins the two archive variants behind
// TencentCodeExtras: MR changed files (merge_request/{id}/changed_files) and
// native code-review changed files (review/{id}/changed_files) stream into
// the caller's writer.
func TestTencentCode_ArchiveDownloadsWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/projects/owner/repo/merge_request/12/changed_files",
			"/api/v3/projects/owner/repo/review/7/changed_files":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = io.WriteString(w, "ZIPBYTES")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)

	var mrBuf, reviewBuf bytes.Buffer
	if err := p.DownloadMergeRequestChangedFiles(context.Background(), "owner", "repo", 12, &mrBuf); err != nil {
		t.Fatalf("DownloadMergeRequestChangedFiles: %v", err)
	}
	if err := p.DownloadCommitReviewChangedFiles(context.Background(), "owner", "repo", 7, &reviewBuf); err != nil {
		t.Fatalf("DownloadCommitReviewChangedFiles: %v", err)
	}
	if mrBuf.String() != "ZIPBYTES" || reviewBuf.String() != "ZIPBYTES" {
		t.Errorf("streamed payload: %q / %q", mrBuf.String(), reviewBuf.String())
	}

	mu.Lock()
	defer mu.Unlock()
	assertCalls(t, calls, []string{
		"GET /api/v3/projects/owner%2Frepo/merge_request/12/changed_files",
		"GET /api/v3/projects/owner%2Frepo/review/7/changed_files",
	})
}
