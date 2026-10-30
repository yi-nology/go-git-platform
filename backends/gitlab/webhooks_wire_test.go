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
// being removed — while nil fields stay off the wire entirely.
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
	// Nil-everything patch: an empty body, nothing silently reset.
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 3 {
		t.Fatalf("expected 3 recorded bodies, got %d", len(bodies))
	}
	if bodies[0]["url"] != url || bodies[0]["push_events"] != true || bodies[0]["merge_requests_events"] != false || bodies[0]["tag_push_events"] != false {
		t.Errorf("URL+events body: got %v", bodies[0])
	}
	if bodies[1]["push_events"] != false || bodies[1]["merge_requests_events"] != true {
		t.Errorf("event-removal body must send explicit false: got %v", bodies[1])
	}
	if len(bodies[2]) != 0 {
		t.Errorf("nil patch must send an empty body: got %v", bodies[2])
	}
}
