package gitlab_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// usersRecordingStub is a recording mock for the username→ID resolution
// path: GET /users answers [{"id":101,"username":"<query.username>"}] (an
// empty array when emptyUsers is set), and the issue create/update verbs
// answer with a minimal issue fixture. Every request is recorded.
type usersRecordingStub struct {
	mu        sync.Mutex
	methods   []string
	paths     []string
	bodies    []map[string]any
	userGets  int
	issuePut  int
	issuePost int
}

func (s *usersRecordingStub) handler(emptyUsers bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		s.mu.Lock()
		s.methods = append(s.methods, r.Method)
		s.paths = append(s.paths, r.URL.RequestURI())
		s.bodies = append(s.bodies, body)
		isUsersGet := r.Method == http.MethodGet && r.URL.Path == "/api/v4/users"
		if isUsersGet {
			s.userGets++
		}
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case isUsersGet:
			if emptyUsers {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			// Echo the exact-match username filter back as the resolved user.
			resp, _ := json.Marshal([]map[string]any{
				{"id": 101, "username": r.URL.Query().Get("username")},
			})
			_, _ = w.Write(resp)
		case r.Method == http.MethodPost:
			s.mu.Lock()
			s.issuePost++
			s.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened"}`))
		case r.Method == http.MethodPut:
			s.mu.Lock()
			s.issuePut++
			s.mu.Unlock()
			_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}
}

// assigneeIDsOf extracts the assignee_ids array from a recorded body.
func assigneeIDsOf(t *testing.T, body map[string]any) []any {
	t.Helper()
	v, ok := body["assignee_ids"]
	if !ok {
		t.Fatalf("expected body to carry assignee_ids, got %v", body)
	}
	ids, ok := v.([]any)
	if !ok {
		t.Fatalf("expected assignee_ids to be a JSON array, got %T (%v)", v, v)
	}
	return ids
}

// TestGitLab_CreateIssue_ResolvesAssignees verifies the fixed behavior:
// CreateIssue's Assignees resolve through the Users API and land on the
// wire as assignee_ids (they were silently ignored before).
func TestGitLab_CreateIssue_ResolvesAssignees(t *testing.T) {
	stub := &usersRecordingStub{}
	srv := httptest.NewServer(stub.handler(false))
	defer srv.Close()
	p := newTestProvider(t, srv)

	if _, err := p.CreateIssue(context.Background(), provider.CreateIssueOptions{
		Owner: "owner", Repo: "repo", Title: "bug", Assignees: []string{"dev"},
	}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.userGets != 1 {
		t.Fatalf("expected exactly 1 GET /users, got %d (paths %v)", stub.userGets, stub.paths)
	}
	if stub.issuePost != 1 {
		t.Fatalf("expected exactly 1 issue POST, got %d (paths %v)", stub.issuePost, stub.paths)
	}
	for i, body := range stub.bodies {
		if stub.methods[i] != http.MethodPost {
			continue
		}
		ids := assigneeIDsOf(t, body)
		if len(ids) != 1 || ids[0].(float64) != 101 {
			t.Errorf("expected POST body assignee_ids [101], got %v", ids)
		}
	}
}

// TestGitLab_UpdateIssue_ResolvesAssignees verifies UpdateIssue carries
// assignee_ids, and that a second assignee write reuses the cached
// username→ID resolution instead of re-hitting the Users API.
func TestGitLab_UpdateIssue_ResolvesAssignees(t *testing.T) {
	stub := &usersRecordingStub{}
	srv := httptest.NewServer(stub.handler(false))
	defer srv.Close()
	p := newTestProvider(t, srv)

	if _, err := p.UpdateIssue(context.Background(), "owner", "repo", "1", provider.UpdateIssueOptions{
		Assignees: []string{"dev"},
	}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	// Second write with the same assignee must be served from the cache.
	if _, err := p.UpdateIssue(context.Background(), "owner", "repo", "1", provider.UpdateIssueOptions{
		Assignees: []string{"dev"},
	}); err != nil {
		t.Fatalf("UpdateIssue (cached): %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.userGets != 1 {
		t.Errorf("expected the cached resolution to keep GET /users at 1, got %d (paths %v)", stub.userGets, stub.paths)
	}
	if stub.issuePut != 2 {
		t.Fatalf("expected 2 issue PUTs, got %d (paths %v)", stub.issuePut, stub.paths)
	}
	for i, body := range stub.bodies {
		if stub.methods[i] != http.MethodPut {
			continue
		}
		ids := assigneeIDsOf(t, body)
		if len(ids) != 1 || ids[0].(float64) != 101 {
			t.Errorf("expected PUT body assignee_ids [101], got %v", ids)
		}
	}
}

// TestGitLab_ResolveUserIDs_UnknownUserNotFound verifies an unknown
// assignee surfaces as a NotFound under the calling op and stops the
// write before it reaches the wire.
func TestGitLab_ResolveUserIDs_UnknownUserNotFound(t *testing.T) {
	stub := &usersRecordingStub{}
	srv := httptest.NewServer(stub.handler(true)) // /users answers []
	defer srv.Close()
	p := newTestProvider(t, srv)

	_, err := p.CreateIssue(context.Background(), provider.CreateIssueOptions{
		Owner: "owner", Repo: "repo", Title: "bug", Assignees: []string{"ghost"},
	})
	if err == nil {
		t.Fatal("expected a NotFound error for the unknown user, got nil")
	}
	if !provider.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.issuePost != 0 {
		t.Errorf("expected no issue POST after a failed resolution, got %d", stub.issuePost)
	}
}
