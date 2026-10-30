package gitcode_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestWebhooks_UpdateWire pins GitCode's edit shape: GitHub-style body
// (events array, secret key), PATCH verb, and omitempty semantics — nil
// provider fields stay off the wire.
func TestWebhooks_UpdateWire(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("update verb: got %s, want PATCH", r.Method)
		}
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		writeJSON(w, map[string]any{"id": 42, "url": "https://example.com/hook", "events": []string{"push"}})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	url := "https://example.com/hook"
	secret := "s3cret"
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{URL: &url, Secret: &secret, Events: []string{"push", "pull_request"}}); err != nil {
		t.Fatal(err)
	}
	// Events-only patch: url/secret must stay off the wire.
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{Events: []string{"issues"}}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 recorded bodies, got %d", len(bodies))
	}
	if bodies[0]["url"] != url || bodies[0]["secret"] != secret {
		t.Errorf("url/secret keys: got %v", bodies[0])
	}
	if events, _ := bodies[0]["events"].([]any); len(events) != 2 {
		t.Errorf("events replace the set: got %v", bodies[0]["events"])
	}
	if _, has := bodies[1]["url"]; has {
		t.Errorf("events-only update must omit url: %v", bodies[1])
	}
	if _, has := bodies[1]["secret"]; has {
		t.Errorf("events-only update must omit secret: %v", bodies[1])
	}
}
