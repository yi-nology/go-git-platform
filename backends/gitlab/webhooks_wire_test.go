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

// TestWebhooks_UpdateWire pins GitLab's edit shape: the endpoint is
// boolean-flag based (omitted flag = unchanged), so a non-nil Events slice
// must set every derived flag explicitly — including false for the events
// being removed — while nil fields stay off the wire entirely. The note
// family (issue #32) joins the explicit-replacement set: it was previously
// unreachable, silently dropping "note"/"comment" from Events.
func TestWebhooks_UpdateWire(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("update verb: got %s, want PUT", r.Method)
		}
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		writeJSON(w, map[string]any{"id": 42, "url": "https://example.com/hook", "push_events": true})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	url := "https://example.com/hook"
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{URL: &url, Events: []string{"push"}}); err != nil {
		t.Fatal(err)
	}
	// Removing push: the flag must go over the wire as an explicit false,
	// not be omitted (omission would mean "unchanged" to GitLab).
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{Events: []string{"pull_request"}}); err != nil {
		t.Fatal(err)
	}
	// Note family: "note" and its aliases must reach NoteEvents...
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{Events: []string{"push", "note"}}); err != nil {
		t.Fatal(err)
	}
	// ...and a set without note must turn it off explicitly (removal).
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{Events: []string{"push"}}); err != nil {
		t.Fatal(err)
	}
	// Nil-everything patch: an empty body, nothing silently reset.
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 5 {
		t.Fatalf("expected 5 recorded bodies, got %d", len(bodies))
	}
	if bodies[0]["url"] != url || bodies[0]["push_events"] != true || bodies[0]["merge_requests_events"] != false || bodies[0]["tag_push_events"] != false {
		t.Errorf("URL+events body: got %v", bodies[0])
	}
	if bodies[0]["note_events"] != false {
		t.Errorf("note family must join the explicit-replacement set (false when absent): got %v", bodies[0])
	}
	if bodies[1]["push_events"] != false || bodies[1]["merge_requests_events"] != true {
		t.Errorf("event-removal body must send explicit false: got %v", bodies[1])
	}
	if bodies[2]["note_events"] != true {
		t.Errorf("note family must be expressible: got %v", bodies[2])
	}
	if bodies[3]["note_events"] != false {
		t.Errorf("events without note must turn the family off explicitly: got %v", bodies[3])
	}
	if len(bodies[4]) != 0 {
		t.Errorf("nil patch must send an empty body: got %v", bodies[4])
	}
}

// TestWebhooks_CreateWire_NoteFamily pins the create side of issue #32:
// "note"/"comment" aliases map onto NoteEvents, and a non-nil Events slice
// without note leaves the family explicitly off (the flag set is a full
// replacement of the derived families).
func TestWebhooks_CreateWire_NoteFamily(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("create verb: got %s, want POST", r.Method)
		}
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		writeJSON(w, map[string]any{"id": 42, "url": "https://example.com/hook"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	// note via alias "comment".
	if _, err := p.CreateWebhook(context.Background(), provider.CreateWebhookOptions{
		Owner: "owner", Repo: "repo", URL: "https://example.com/hook",
		Events: []string{"push", "comment"},
	}); err != nil {
		t.Fatal(err)
	}
	// No note anywhere: explicit false.
	if _, err := p.CreateWebhook(context.Background(), provider.CreateWebhookOptions{
		Owner: "owner", Repo: "repo", URL: "https://example.com/hook",
		Events: []string{"push", "merge_request"},
	}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 recorded bodies, got %d", len(bodies))
	}
	if bodies[0]["note_events"] != true {
		t.Errorf("comment alias must map onto note_events: got %v", bodies[0])
	}
	if bodies[1]["note_events"] != false {
		t.Errorf("create without note must state the family explicitly off: got %v", bodies[1])
	}
}
