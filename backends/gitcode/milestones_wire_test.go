package gitcode_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitCode_MilestoneMutations_OmitDueOn verifies the wire form of the
// milestone mutation bodies: an unset due date must stay off the wire —
// GitCode's documented due_on carries minLength 1, and an explicit
// `"due_on": ""` clears (or errors on) the stored value, wiping the
// milestone's due date on a title-only update. Since go-gitcode v0.8.0 the
// SDK marshals due_on with omitempty and the backend rides it directly.
func TestGitCode_MilestoneMutations_OmitDueOn(t *testing.T) {
	var gotBodies = map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			raw, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			gotBodies[r.Method] = m
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"title":"v1","state":"open"}`))
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitCode, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	mm := p.(provider.MilestoneManager)

	// Title-only update must not carry a due_on key — the finding's core
	// assertion: an empty-string due_on would clear GitCode's stored date.
	newTitle := "v2-renamed"
	if _, err := mm.UpdateMilestone(context.Background(), "owner", "repo", "1", provider.UpdateMilestoneOptions{Title: &newTitle}); err != nil {
		t.Fatalf("UpdateMilestone: %v", err)
	}
	updateBody, ok := gotBodies[http.MethodPatch]
	if !ok {
		t.Fatalf("expected a PATCH request, recorded methods: create=%v", gotBodies[http.MethodPost] != nil)
	}
	if v, present := updateBody["due_on"]; present {
		t.Errorf("title-only update body carries due_on=%v; the key must be omitted so GitCode keeps the stored due date", v)
	}
	if v, _ := updateBody["title"].(string); v != newTitle {
		t.Errorf("update body title = %q, want %q", v, newTitle)
	}

	// Same omission on create without a due date.
	if _, err := mm.CreateMilestone(context.Background(), "owner", "repo", provider.CreateMilestoneOptions{Title: "v2"}); err != nil {
		t.Fatalf("CreateMilestone: %v", err)
	}
	createBody, ok := gotBodies[http.MethodPost]
	if !ok {
		t.Fatal("expected a POST request")
	}
	if v, present := createBody["due_on"]; present {
		t.Errorf("due-on-less create body carries due_on=%v; the key must be omitted", v)
	}
	if v, _ := createBody["title"].(string); v != "v2" {
		t.Errorf("create body title = %q, want %q", v, "v2")
	}

	// A set due date goes out under the due_on key (RFC3339, as the backend
	// formats it) instead of being silently dropped.
	due := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	if _, err := mm.CreateMilestone(context.Background(), "owner", "repo", provider.CreateMilestoneOptions{Title: "v3", DueOn: &due}); err != nil {
		t.Fatalf("CreateMilestone (due): %v", err)
	}
	dueBody, ok := gotBodies[http.MethodPost]
	if !ok {
		t.Fatal("expected the due-set POST to be recorded")
	}
	if v, _ := dueBody["due_on"].(string); !strings.HasPrefix(v, "2026-03-01T08:00:00") {
		t.Errorf("due-set create body due_on = %q, want the formatted timestamp", v)
	}
}

// TestGitCode_MilestoneRawPaths_EscapeSegments verifies that the raw
// milestone detours percent-encode the owner/repo path segments: the
// transport client joins BaseURL and Path by plain concatenation, so an
// unescaped '#' or '?' would corrupt or truncate the URL before it
// reaches the server.
func TestGitCode_MilestoneRawPaths_EscapeSegments(t *testing.T) {
	var mu sync.Mutex
	var paths = map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.Method] = r.URL.EscapedPath()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"title":"v1","state":"open"}`))
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitCode, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	mm := p.(provider.MilestoneManager)

	const (
		owner = "ow#ner"
		repo  = "re?po"
	)
	newTitle := "v2"
	if _, err := mm.UpdateMilestone(context.Background(), owner, repo, "1", provider.UpdateMilestoneOptions{Title: &newTitle}); err != nil {
		t.Fatalf("UpdateMilestone: %v", err)
	}
	if _, err := mm.CreateMilestone(context.Background(), owner, repo, provider.CreateMilestoneOptions{Title: "v3"}); err != nil {
		t.Fatalf("CreateMilestone: %v", err)
	}

	for method, want := range map[string]string{
		http.MethodPatch: "/repos/ow%23ner/re%3Fpo/milestones/1",
		http.MethodPost:  "/repos/ow%23ner/re%3Fpo/milestones",
	} {
		got, ok := paths[method]
		if !ok {
			t.Errorf("expected a %s request, recorded methods: %v", method, paths)
			continue
		}
		if got != want {
			t.Errorf("%s path = %q, want %q (owner/repo segments must be percent-encoded)", method, got, want)
		}
	}
}
