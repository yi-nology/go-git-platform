package gitee_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestWebhooks_UpdateWire pins Gitee's edit shape: PATCH verb, the secret
// rides the password key, and the boolean-flag endpoint requires every
// derived flag explicitly on a non-nil Events slice (omission = unchanged,
// so removals must go over the wire as false).
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
		writeJSON(w, map[string]any{"id": 42, "url": "https://example.com/hook", "push_events": true})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	url := "https://example.com/hook"
	secret := "s3cret"
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{URL: &url, Secret: &secret, Events: []string{"push", "note"}}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("expected 1 recorded body, got %d", len(bodies))
	}
	body := bodies[0]
	if body["url"] != url || body["password"] != secret {
		t.Errorf("url/password keys: got %v", body)
	}
	if _, has := body["secret"]; has {
		t.Errorf("Gitee has no secret key (it is password): %v", body)
	}
	if body["push_events"] != true || body["note_events"] != true ||
		body["issues_events"] != false || body["tag_push_events"] != false || body["merge_requests_events"] != false {
		t.Errorf("all five flags must be explicit: got %v", body)
	}
}
