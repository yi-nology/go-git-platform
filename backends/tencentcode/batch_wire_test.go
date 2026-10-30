package tencentcode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestTencentCode_BatchHelpers pins the provider-level batch helpers
// (provider.GetFileContents / GetCRs / SetCommitStatuses — the oekit
// syncx.FanInLimitCtx-driven bounded-concurrency engine) over the 工蜂
// wire: every item lands as its own BatchResult in input order, per-item
// failures do not abort the batch, and the engine's per-item panics are
// contained by the engine itself.
func TestTencentCode_BatchHelpers(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/repository/files"):
			// 工蜂's file endpoint addresses the file via the file_path
			// query parameter, not the path.
			if r.URL.Query().Get("file_path") == "README.md" {
				writeJSON(w, map[string]any{
					"file_name": "README.md", "file_path": "README.md",
					"encoding": "base64", "content": "aGVsbG8=",
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/merge_request/2"):
			writeJSON(w, map[string]any{
				"id": 2, "iid": 2, "title": "second", "description": "d",
				"state": "opened", "source_branch": "f", "target_branch": "main",
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/merge_request/1"):
			writeJSON(w, map[string]any{
				"id": 1, "iid": 1, "title": "first", "description": "d",
				"state": "opened", "source_branch": "f", "target_branch": "main",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/statuses"):
			writeJSON(w, map[string]any{})
		default:
			writeJSON(w, map[string]any{})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)

	// GetFileContents: one hit, one miss — the miss must stay a per-item Err.
	fileResults := provider.GetFileContents(context.Background(), p, "owner", "repo", "main",
		[]string{"README.md", "missing.txt"})
	if len(fileResults) != 2 {
		t.Fatalf("expected 2 file results, got %d", len(fileResults))
	}
	if fileResults[0].Err != nil || fileResults[0].Value != "hello" {
		t.Errorf("file[0]: value=%q err=%v", fileResults[0].Value, fileResults[0].Err)
	}
	if fileResults[1].Err == nil {
		t.Error("missing.txt must surface as a per-item Err")
	}
	if fileResults[1].Key != "missing.txt" {
		t.Errorf("file[1].Key = %q, want missing.txt", fileResults[1].Key)
	}

	// GetCRs: input order preserved.
	crResults := provider.GetCRs(context.Background(), p, "owner", "repo", []string{"2", "1"})
	for i, want := range []string{"2", "1"} {
		if crResults[i].Err != nil {
			t.Fatalf("cr[%d]: %v", i, crResults[i].Err)
		}
		if crResults[i].Value.Number != want {
			t.Errorf("cr[%d].Number = %q, want %q (input order)", i, crResults[i].Value.Number, want)
		}
	}

	// SetCommitStatuses: both statuses POST through.
	statusResults := provider.SetCommitStatuses(context.Background(), p, "owner", "repo", "abc",
		[]provider.CommitStatusOptions{
			{State: "success", Context: "ci/lint"},
			{State: "pending", Context: "ci/test"},
		})
	for i, r := range statusResults {
		if r.Err != nil {
			t.Errorf("status[%d]: %v", i, r.Err)
		}
	}

	posts := 0
	for _, c := range calls {
		if strings.HasPrefix(c, http.MethodPost+" ") && strings.Contains(c, "/statuses") {
			posts++
		}
	}
	if posts != 2 {
		t.Errorf("expected 2 status POSTs, recorded: %v", calls)
	}
}
