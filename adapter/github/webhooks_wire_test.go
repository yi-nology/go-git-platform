package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestWebhooks_UpdatePatchWire pins GitHub's PATCH shape: unprovided
// settings are omitted — the config object is only attached when URL/Secret
// change, so the server keeps its remaining config keys (the secret is
// write-only and unreadable) — and Events replaces the set wholesale.
func TestWebhooks_UpdatePatchWire(t *testing.T) {
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"url":"https://example.com/hook","events":["push"]}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL+"/api/v3")

	url := "https://example.com/hook"
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{URL: &url, Events: []string{"push", "pull_request"}}); err != nil {
		t.Fatal(err)
	}
	// URL/Secret untouched: no config key may hit the wire at all.
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{Events: []string{"issues"}}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 recorded bodies, got %d", len(bodies))
	}
	cfg1, ok := bodies[0]["config"].(map[string]any)
	if !ok || cfg1["url"] != url {
		t.Errorf("update-with-URL body config: got %v", bodies[0]["config"])
	}
	if _, has := cfg1["secret"]; has {
		t.Errorf("update-with-URL body must not carry an empty secret: %v", cfg1)
	}
	events1, _ := bodies[0]["events"].([]any)
	if len(events1) != 2 {
		t.Errorf("update events replace the set: got %v", bodies[0]["events"])
	}
	if _, has := bodies[1]["config"]; has {
		t.Errorf("events-only update must not attach a config object: %v", bodies[1])
	}
	if v, has := bodies[1]["events"]; !has || len(v.([]any)) != 1 {
		t.Errorf("events-only update body events: got %v", bodies[1]["events"])
	}
}
