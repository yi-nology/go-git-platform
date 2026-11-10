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

// TestWebhooks_UpdateWire pins GitCode's edit shape: the documented v5 wire
// expresses subscriptions as per-family boolean flags (push_events /
// tag_push_events / issues_events / note_events / merge_requests_events) —
// there is no events array and the secret field is "password". A non-nil
// Events slice sets every flag explicitly (true for members, false for the
// rest) so removal is expressible; nil fields stay off the wire.
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
		writeJSON(w, map[string]any{"id": 42, "url": "https://example.com/hook", "push_events": 1, "merge_requests_events": 1})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	url := "https://example.com/hook"
	secret := "s3cret"
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{URL: &url, Secret: &secret, Events: []string{"push", "pull_request"}}); err != nil {
		t.Fatal(err)
	}
	// Events-only patch: url/password must stay off the wire, and the full
	// flag set flips to the requested family (only issues here).
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{Events: []string{"issues"}}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 recorded bodies, got %d", len(bodies))
	}
	if bodies[0]["url"] != url || bodies[0]["password"] != secret {
		t.Errorf("url/password keys: got %v", bodies[0])
	}
	if _, has := bodies[0]["secret"]; has {
		t.Errorf("wire secret key must be password, not secret: %v", bodies[0])
	}
	if _, has := bodies[0]["events"]; has {
		t.Errorf("GitCode's hook wire has no events array: %v", bodies[0])
	}
	// Non-nil Events → every family explicit: push+pull_request on, rest off.
	for flag, want := range map[string]bool{
		"push_events": true, "merge_requests_events": true,
		"tag_push_events": false, "issues_events": false, "note_events": false,
	} {
		if got, _ := bodies[0][flag].(bool); got != want {
			t.Errorf("bodies[0][%s] = %v, want %v", flag, got, want)
		}
	}

	if _, has := bodies[1]["url"]; has {
		t.Errorf("events-only update must omit url: %v", bodies[1])
	}
	if _, has := bodies[1]["password"]; has {
		t.Errorf("events-only update must omit password: %v", bodies[1])
	}
	if got, _ := bodies[1]["issues_events"].(bool); !got {
		t.Errorf("bodies[1][issues_events] = %v, want true", bodies[1]["issues_events"])
	}
	if got, _ := bodies[1]["push_events"].(bool); got {
		t.Errorf("bodies[1][push_events] = %v, want false (family removed)", bodies[1]["push_events"])
	}
}

// TestWebhooks_GetWire_MapsFlagsToEvents pins the response mapping: the
// per-family flags (documented as integers) render as the neutral event
// names on provider.PlatformWebhook.Events.
func TestWebhooks_GetWire_MapsFlagsToEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id": 42, "url": "https://example.com/hook",
			"push_events": 1, "tag_push_events": 0, "issues_events": true,
			"note_events": "false", "merge_requests_events": 0,
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	hook, err := p.GetWebhook(context.Background(), "owner", "repo", 42)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"issues", "push"}
	if len(hook.Events) != len(want) {
		t.Fatalf("Events = %v, want %v", hook.Events, want)
	}
	for i, name := range want {
		if hook.Events[i] != name {
			t.Errorf("Events[%d] = %q, want %q", i, hook.Events[i], name)
		}
	}
}

// TestWebhooks_CreateWire pins the create body: password key, explicit
// flags for the default event set (push + pull_request), no events array.
func TestWebhooks_CreateWire(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("create verb: got %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(w, map[string]any{"id": 42, "url": "https://example.com/hook", "push_events": 1})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	if _, err := p.CreateWebhook(context.Background(), provider.CreateWebhookOptions{
		Owner: "owner", Repo: "repo", URL: "https://example.com/hook", Secret: "s3cret",
	}); err != nil {
		t.Fatal(err)
	}
	if body["url"] != "https://example.com/hook" || body["password"] != "s3cret" {
		t.Errorf("create body url/password: got %v", body)
	}
	if _, has := body["events"]; has {
		t.Errorf("create body must not carry an events array: %v", body)
	}
	for flag, want := range map[string]bool{
		"push_events": true, "merge_requests_events": true,
		"tag_push_events": false, "issues_events": false, "note_events": false,
	} {
		if got, _ := body[flag].(bool); got != want {
			t.Errorf("body[%s] = %v, want %v", flag, got, want)
		}
	}
}

// TestWebhooks_ReadBackOneNamePerFlag pins issue #85's convertHook half:
// each flag family reads back as exactly one canonical event name. The
// note_events flag used to emit both "comment" and "note", which made
// provider.EnsureWebhook's event-set comparison fail and every ensure call
// delete+recreate the hook.
func TestWebhooks_ReadBackOneNamePerFlag(t *testing.T) {
	tests := []struct {
		name string
		hook map[string]any
		want []string
	}{
		{
			name: "note flag reads back as note only",
			hook: map[string]any{"id": 1, "url": "u", "note_events": true},
			want: []string{"note"},
		},
		{
			name: "all flags read back as five distinct names",
			hook: map[string]any{"id": 2, "url": "u", "push_events": 1, "tag_push_events": 1, "issues_events": 1, "note_events": 1, "merge_requests_events": 1},
			want: []string{"issues", "note", "pull_request", "push", "tag_push"},
		},
		{
			name: "no flags read back empty",
			hook: map[string]any{"id": 3, "url": "u"},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, tt.hook)
			}))
			defer srv.Close()
			p := newTestProvider(t, srv)
			hook, err := p.GetWebhook(context.Background(), "owner", "repo", 1)
			if err != nil {
				t.Fatalf("GetWebhook: %v", err)
			}
			if len(hook.Events) != len(tt.want) {
				t.Fatalf("Events = %v, want %v (one canonical name per flag)", hook.Events, tt.want)
			}
			for i, name := range tt.want {
				if hook.Events[i] != name {
					t.Errorf("Events[%d] = %q, want %q", i, hook.Events[i], name)
				}
			}
		})
	}
}
