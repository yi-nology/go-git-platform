package tencentcode_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestWebhooks_UpdateWire pins 工蜂's edit shape: PUT verb, the secret rides
// the token key, and the boolean-flag endpoint requires every derived flag
// explicitly on a non-nil Events slice (omission = unchanged, so removals
// must go over the wire as false). The mapping scope mirrors create:
// push / merge-request / tag-push families.
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
	secret := "s3cret"
	if _, err := p.UpdateWebhook(context.Background(), "owner", "repo", 42,
		provider.UpdateWebhookOptions{URL: &url, Secret: &secret, Events: []string{"push", "cr"}}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("expected 1 recorded body, got %d", len(bodies))
	}
	body := bodies[0]
	if body["url"] != url || body["token"] != secret {
		t.Errorf("url/token keys: got %v", body)
	}
	if body["push_events"] != true || body["merge_requests_events"] != true || body["tag_push_events"] != false {
		t.Errorf("all three derived flags must be explicit: got %v", body)
	}
	if _, has := body["secret"]; has {
		t.Errorf("工蜂 has no secret key (it is token): %v", body)
	}
}

// TestWebhooks_EventVocabulary pins the write-side event vocabulary against
// the read side (issue #91): the old create path defaulted push_events=true
// and never mapped issues/note, so Events=["merge_request"] produced a
// push-only leak subscription and Events=["issues"] dropped the family
// entirely — EnsureWebhook then diverged forever (listed "issues" vs a
// listed "push"-only hook). Every flag must be explicit: true for requested
// families, false for the rest.
func TestWebhooks_EventVocabulary(t *testing.T) {
	cases := []struct {
		name   string
		events []string
		want   map[string]any // flags that must be exactly true; the rest must be false
	}{
		{
			name:   "merge_request only: push leak forbidden",
			events: []string{"merge_request"},
			want:   map[string]any{"merge_requests_events": true},
		},
		{
			name:   "issues only lands on the wire",
			events: []string{"issues"},
			want:   map[string]any{"issues_events": true},
		},
		{
			name:   "note family aliases",
			events: []string{"note"},
			want:   map[string]any{"note_events": true},
		},
		{
			name:   "issue_comment rides the note family",
			events: []string{"issue_comment"},
			want:   map[string]any{"note_events": true},
		},
		{
			name:   "tag only",
			events: []string{"tag"},
			want:   map[string]any{"tag_push_events": true},
		},
		{
			name:   "push stays expressible",
			events: []string{"push", "issues"},
			want:   map[string]any{"push_events": true, "issues_events": true},
		},
	}
	const flags = 5
	for _, tc := range cases {
		for _, op := range []string{"create", "update"} {
			t.Run(tc.name+"/"+op, func(t *testing.T) {
				var mu sync.Mutex
				var body map[string]any
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var b map[string]any
					_ = json.NewDecoder(r.Body).Decode(&b)
					mu.Lock()
					body = b
					mu.Unlock()
					writeJSON(w, map[string]any{"id": 42, "url": "https://example.com/hook"})
				}))
				defer srv.Close()
				p := newTestProvider(t, srv)

				var err error
				if op == "create" {
					_, err = p.CreateWebhook(context.Background(), provider.CreateWebhookOptions{
						Owner: "owner", Repo: "repo", URL: "https://example.com/hook", Events: tc.events,
					})
				} else {
					_, err = p.UpdateWebhook(context.Background(), "owner", "repo", 42,
						provider.UpdateWebhookOptions{Events: tc.events})
				}
				if err != nil {
					t.Fatal(err)
				}

				mu.Lock()
				defer mu.Unlock()
				// every derived flag must be explicit (bool on the body)
				for _, f := range []string{"push_events", "issues_events", "merge_requests_events", "tag_push_events", "note_events"} {
					if _, ok := body[f]; !ok {
						t.Errorf("flag %s must be explicit (omission = unchanged on edit): %v", f, body)
					}
				}
				for f, want := range tc.want {
					if body[f] != want {
						t.Errorf("flag %s = %v, want %v", f, body[f], want)
					}
				}
				explicit := 0
				for _, f := range []string{"push_events", "issues_events", "merge_requests_events", "tag_push_events", "note_events"} {
					if _, ok := body[f]; ok {
						explicit++
					}
				}
				if explicit != flags {
					t.Errorf("want all %d flags explicit, got %d: %v", flags, explicit, body)
				}
				// non-requested families must be false, not leaked true
				for f := range tc.want {
					delete(body, f)
				}
				for _, f := range []string{"push_events", "issues_events", "merge_requests_events", "tag_push_events", "note_events"} {
					if v, ok := body[f]; ok && v != false {
						t.Errorf("unrequested flag %s must be false, got %v", f, v)
					}
				}
			})
		}
	}
}

// TestWebhooks_EmptyEventsDefaultsToPush pins the historical default: no
// Events means a push subscription (explicit true, not a flag-less body).
func TestWebhooks_EmptyEventsDefaultsToPush(t *testing.T) {
	var mu sync.Mutex
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		body = b
		mu.Unlock()
		writeJSON(w, map[string]any{"id": 42, "url": "https://example.com/hook"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	if _, err := p.CreateWebhook(context.Background(), provider.CreateWebhookOptions{
		Owner: "owner", Repo: "repo", URL: "https://example.com/hook",
	}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if body["push_events"] != true {
		t.Errorf("default subscription must be push-only, got %v", body)
	}
	if body["issues_events"] != false || body["note_events"] != false {
		t.Errorf("non-push flags must be explicit false, got %v", body)
	}
}
