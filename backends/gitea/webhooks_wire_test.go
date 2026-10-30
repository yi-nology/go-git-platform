package gitea_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestWebhooks_UpdateWire pins the family edit shape: PATCH verb, config
// keys merged server-side (only the provided keys go over the wire so the
// server keeps the rest, e.g. the secret), Events replacing the set, and —
// the family SDKs' edit returning no body — a follow-up GET hydrating the
// returned hook.
func TestWebhooks_UpdateWire(t *testing.T) {
	var mu sync.Mutex
	var calls []struct {
		method string
		path   string
		body   map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		mu.Lock()
		calls = append(calls, struct {
			method string
			path   string
			body   map[string]any
		}{r.Method, r.URL.Path, body})
		mu.Unlock()
		writeJSON(w, map[string]any{
			"id": 42, "config": map[string]any{"url": "https://example.com/hook"},
			"events": []string{"push"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	url := "https://example.com/hook"
	hook, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{URL: &url, Events: []string{"push", "pull_request"}})
	if err != nil {
		t.Fatal(err)
	}
	if hook.ID != 42 || hook.URL != url {
		t.Errorf("hydrated hook: %+v", hook)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("expected PATCH + read-back GET, got %+v", calls)
	}
	if calls[0].method != http.MethodPatch || calls[0].path != "/api/v1/repos/owner/repo/hooks/42" {
		t.Errorf("update request: %s %s", calls[0].method, calls[0].path)
	}
	cfg, ok := calls[0].body["config"].(map[string]any)
	if !ok || cfg["url"] != url {
		t.Errorf("update body config: got %v", calls[0].body["config"])
	}
	if _, has := cfg["secret"]; has {
		t.Errorf("URL-only patch must not carry an empty secret key: %v", cfg)
	}
	events, _ := calls[0].body["events"].([]any)
	if len(events) != 2 {
		t.Errorf("events replace the set: got %v", calls[0].body["events"])
	}
	if calls[1].method != http.MethodGet {
		t.Errorf("read-back request: %s %s", calls[1].method, calls[1].path)
	}
}
