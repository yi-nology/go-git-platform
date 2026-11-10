package gitcode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestGitCode_ListCRComments_Paginates pins issue #81: CR comments ride the
// issue-comment endpoint, whose plain SDK helper delegates with empty
// options pinned to page=1/per_page=20 — a thread longer than 20 comments
// was silently truncated. The walk must go through
// ListIssueCommentsWithOptions and exhaust the platform's pagination (the
// same contract ListIssueComments satisfies).
func TestGitCode_ListCRComments_Paginates(t *testing.T) {
	var mu sync.Mutex
	var pages []string
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pages = append(pages, r.URL.Query().Get("page"))
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "", "1":
			_, _ = w.Write([]byte(`[{"id":1,"body":"c1"},{"id":2,"body":"c2"}]`))
		case "2":
			_, _ = w.Write([]byte(`[{"id":3,"body":"a comment"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	comments, err := p.ListCRComments(context.Background(), "owner", "repo", "7")
	if err != nil {
		t.Fatalf("ListCRComments: %v", err)
	}
	if len(comments) != 3 {
		t.Fatalf("expected the paginated merge (2 + 1 comments), got %d", len(comments))
	}
	found := false
	for _, c := range comments {
		if c.Body == "a comment" {
			found = true
		}
	}
	if !found {
		t.Error(`expected the second-page fixture comment ("a comment") in the merged list`)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(pages) < 2 || pages[0] != "1" && pages[0] != "" || pages[len(pages)-1] != "3" {
		t.Errorf("expected page GETs walking 1..3, got %v", pages)
	}
	for _, path := range paths {
		if !strings.Contains(path, "/repos/owner/repo/issues/7/comments") {
			t.Errorf("CR comments must route through the issue-comment endpoint, got %s", path)
		}
	}
}
