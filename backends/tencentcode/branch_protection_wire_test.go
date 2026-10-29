package tencentcode_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestTencentCode_BranchProtectionWire pins the provider-level branch
// protection surface: List walks the branch list and filters protected
// rows (no native list endpoint), Get fetches the per-branch protect
// detail, Create/Update PUT the protect upsert decoding the detail from
// the response, and Delete is a PUT to the unprotect endpoint (the
// platform has no DELETE verb there).
func TestTencentCode_BranchProtectionWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var putBodies []map[string]any
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/protect"):
			var body map[string]any
			_ = readRequestBody(r, &body)
			mu.Lock()
			putBodies = append(putBodies, body)
			mu.Unlock()
			writeJSON(w, map[string]any{"protected": true, "name": "fe#at", "approver_rule": 2})
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/unprotect"):
			writeJSON(w, map[string]any{})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/repository/branches"):
			writeListPage(w, r, []map[string]any{
				{"name": "main", "protected": true, "commit": map[string]any{"id": "abc"}},
				{"name": "dev", "protected": false},
			})
		case strings.HasSuffix(r.URL.Path, "/protect"):
			writeJSON(w, map[string]any{
				"protected": true, "name": "fe#at", "approver_rule": -1,
				"necessary_approver_rule": 1,
			})
		default:
			writeJSON(w, map[string]any{"name": "fe#at"})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	if !p.Capabilities().BranchProtections {
		t.Fatal("BranchProtectionManager implementation must be declared in Capabilities().BranchProtections")
	}
	var bpm provider.BranchProtectionManager = p

	rules, err := bpm.ListBranchProtections(context.Background(), "owner", "repo")
	if err != nil || len(rules) != 1 || rules[0].BranchName != "main" {
		t.Fatalf("ListBranchProtections: %+v err=%v", rules, err)
	}

	bp, err := bpm.GetBranchProtection(context.Background(), "owner", "repo", "fe#at")
	if err != nil || bp.BranchName != "fe#at" || bp.RequiredApprovingReviews != -1 {
		t.Fatalf("GetBranchProtection: %+v err=%v", bp, err)
	}

	created, err := bpm.CreateBranchProtection(context.Background(), "owner", "repo",
		provider.CreateBranchProtectionOptions{BranchName: "fe#at", AllowForcePushes: true})
	if err != nil || created == nil || created.RequiredApprovingReviews != 2 {
		t.Fatalf("CreateBranchProtection: %+v err=%v", created, err)
	}
	forcePushSeen := false
	for _, b := range putBodies {
		if v, ok := b["allow_force_push"].(bool); ok && v {
			forcePushSeen = true
		}
	}
	if !forcePushSeen {
		t.Errorf("create must carry allow_force_push on the wire, bodies: %v", putBodies)
	}

	updated, err := bpm.UpdateBranchProtection(context.Background(), "owner", "repo", "fe#at",
		provider.UpdateBranchProtectionOptions{})
	if err != nil || updated == nil {
		t.Fatalf("UpdateBranchProtection: %+v err=%v", updated, err)
	}
	if err := bpm.DeleteBranchProtection(context.Background(), "owner", "repo", "fe#at"); err != nil {
		t.Fatalf("DeleteBranchProtection: %v", err)
	}

	assertCalls(t, calls, []string{
		http.MethodGet + " /api/v3/projects/owner%2Frepo/repository/branches", // page 1
		http.MethodGet + " /api/v3/projects/owner%2Frepo/repository/branches", // page 2 (empty)
		http.MethodGet + " /api/v3/projects/owner%2Frepo/repository/branches/fe%23at/protect",
		http.MethodPut + " /api/v3/projects/owner%2Frepo/repository/branches/fe%23at/protect",
		http.MethodPut + " /api/v3/projects/owner%2Frepo/repository/branches/fe%23at/protect",
		http.MethodPut + " /api/v3/projects/owner%2Frepo/repository/branches/fe%23at/unprotect",
	})
}

// TestTencentCode_NoteManagerWire pins the CR-note update/list surface
// (NoteManager): MR notes live in their own iid namespace on this
// GitLab-family platform, so edits go through the merge_request notes API.
func TestTencentCode_NoteManagerWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var updateBody map[string]any
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_ = readRequestBody(r, &updateBody)
			writeJSON(w, map[string]any{
				"id": 7, "body": "rewritten", "author": map[string]any{"id": 1, "username": "dev", "name": "Dev"},
				"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
			})
			return
		}
		writeListPage(w, r, []map[string]any{
			{"id": 7, "body": "first", "author": map[string]any{"id": 1, "username": "dev", "name": "Dev"}},
			{"id": 8, "body": "second", "author": map[string]any{"id": 1, "username": "dev", "name": "Dev"}},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	var nm provider.NoteManager = p

	comment, err := nm.UpdateNote(context.Background(), "owner", "repo", "1", "7", "rewritten")
	if err != nil || comment.Body != "rewritten" || comment.Author.Username != "dev" {
		t.Fatalf("UpdateNote: %+v err=%v", comment, err)
	}
	if updateBody["body"] != "rewritten" {
		t.Errorf("update must PUT the new body, got %v", updateBody)
	}

	notes, err := nm.ListNotes(context.Background(), "owner", "repo", "1")
	if err != nil || len(notes) != 2 || notes[0].ID != 7 || notes[1].ID != 8 {
		t.Fatalf("ListNotes: %+v err=%v", notes, err)
	}

	assertCalls(t, calls, []string{
		http.MethodPut + " /api/v3/projects/owner%2Frepo/merge_requests/1/notes/7",
		http.MethodGet + " /api/v3/projects/owner%2Frepo/merge_requests/1/notes", // page 1
		http.MethodGet + " /api/v3/projects/owner%2Frepo/merge_requests/1/notes", // page 2 (empty)
	})
}

// TestTencentCode_RepositoryReadsWire pins the extras repository reads:
// single-branch fetch, raw file stream (blob endpoint + filepath query),
// and the compare changed-files zip download (from/to travel as query).
func TestTencentCode_RepositoryReadsWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var compareQuery string
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/repository/branches/main"):
			writeJSON(w, map[string]any{"name": "main", "protected": true, "commit": map[string]any{"id": "abc"}})
		case strings.HasSuffix(r.URL.Path, "/repository/compare/changed_files"):
			mu.Lock()
			compareQuery = r.URL.RawQuery
			mu.Unlock()
			_, _ = w.Write([]byte("zip-bytes"))
		case strings.HasSuffix(r.URL.Path, "/repository/commits/abc/blob"):
			_, _ = w.Write([]byte("commit-raw"))
		default: // raw blob
			_, _ = w.Write([]byte("raw-content"))
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)

	branch, err := p.GetBranch(context.Background(), "owner", "repo", "main")
	if err != nil || branch.CommitSHA != "abc" || !branch.Protected {
		t.Fatalf("GetBranch: %+v err=%v", branch, err)
	}
	raw, err := p.GetRawFile(context.Background(), "owner", "repo", "main", "docs/read me.md")
	if err != nil || string(raw) != "raw-content" {
		t.Fatalf("GetRawFile: %q err=%v", raw, err)
	}
	commitRaw, err := p.GetCommitRawFile(context.Background(), "owner", "repo", "abc", "docs/read me.md")
	if err != nil || string(commitRaw) != "commit-raw" {
		t.Fatalf("GetCommitRawFile: %q err=%v", commitRaw, err)
	}
	var zipBuf strings.Builder
	if err := p.DownloadCompareChangedFiles(context.Background(), "owner", "repo", "main", "feature", io.Writer(&zipBuf)); err != nil {
		t.Fatalf("DownloadCompareChangedFiles: %v", err)
	}
	if zipBuf.String() != "zip-bytes" {
		t.Errorf("compare download must stream the body, got %q", zipBuf.String())
	}
	if !strings.Contains(compareQuery, "from=main") || !strings.Contains(compareQuery, "to=feature") {
		t.Errorf("compare download must carry from/to, got %q", compareQuery)
	}

	assertCalls(t, calls, []string{
		http.MethodGet + " /api/v3/projects/owner%2Frepo/repository/branches/main",
		http.MethodGet + " /api/v3/projects/owner%2Frepo/repository/blobs/main", // raw file; filepath rides the query
		http.MethodGet + " /api/v3/projects/owner%2Frepo/repository/commits/abc/blob",
		http.MethodGet + " /api/v3/projects/owner%2Frepo/repository/compare/changed_files",
	})
}
