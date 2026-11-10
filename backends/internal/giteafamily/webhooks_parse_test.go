package giteafamily

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// signedHookRequest builds a POST carrying the family event header and a
// valid X-Gitea-Signature over the body (the family validator is an
// HMAC-SHA256 check), so ParseWebhookEvent's full validation path runs.
func signedHookRequest(t *testing.T, eventType string, payload map[string]any) *http.Request {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest("POST", "/hook", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Event", eventType)
	mac := hmac.New(sha256.New, []byte("hook-secret"))
	_, _ = mac.Write(raw)
	req.Header.Set("X-Gitea-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return req
}

func mustParse(t *testing.T, req *http.Request) *provider.NormalizedEvent {
	t.Helper()
	ev, err := ParseWebhookEvent(Family{
		Platform:             provider.PlatformGitea,
		EventIDPrefix:        "gt",
		EventHeaders:         []string{"X-Gitea-Event"},
		EventDeliveryHeaders: []string{"X-Gitea-Delivery"},
		PageSize:             20,
	}, req, "hook-secret")
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	return ev
}

// TestParseWebhookEvent_CommentActionDerived pins the action derivation
// (issue #95): the family comment hook carries action=created|edited|deleted
// and each must survive normalization — the old code hard-coded created for
// every delivery, so deletes replayed as new comments and edits as
// duplicates on gitea/forgejo.
func TestParseWebhookEvent_CommentActionDerived(t *testing.T) {
	payload := func(action string) map[string]any {
		return map[string]any{
			"action": action,
			"issue":  map[string]any{"number": 12, "title": "bug", "state": "open"},
			"comment": map[string]any{
				"id": 555, "body": "text",
				"user": map[string]any{"id": 1, "login": "dev"},
			},
			"repository": map[string]any{"id": 99, "full_name": "owner/repo"},
			"sender":     map[string]any{"id": 1, "login": "dev"},
		}
	}
	cases := []struct {
		wire string
		want string
	}{
		{wire: "created", want: provider.CommentActionCreated},
		{wire: "edited", want: provider.CommentActionEdited},
		{wire: "deleted", want: provider.CommentActionDeleted},
		{wire: "", want: provider.CommentActionCreated}, // old wire without the field
		{wire: "reacted", want: "reacted"},              // unknown passes through
	}
	for _, tc := range cases {
		t.Run("action="+tc.wire, func(t *testing.T) {
			ev := mustParse(t, signedHookRequest(t, "issue_comment", payload(tc.wire)))
			if ev.Action != tc.want {
				t.Fatalf("comment action = %q, want %q", ev.Action, tc.want)
			}
			if want := provider.EventTypeComment + tc.want; ev.Type != want {
				t.Fatalf("event type = %q, want %q", ev.Type, want)
			}
			if ev.Comment == nil || ev.Comment.ID != 555 {
				t.Fatalf("comment payload must ride along, got %+v", ev.Comment)
			}
		})
	}
}

// TestParseWebhookEvent_GlobalIDs pins the ID semantics (issue #96): event
// CR.ID/Issue.ID carry the platform's global IDs from the payload (matching
// the API stream), not the per-repo numbers — consumers correlate the two
// streams by ID.
func TestParseWebhookEvent_GlobalIDs(t *testing.T) {
	rec := signedHookRequest(t, "pull_request", map[string]any{
		"action": "opened",
		"number": 42,
		"pull_request": map[string]any{
			"id": 98123, "number": 42, "title": "t", "state": "open",
			"head": map[string]any{"ref": "f", "sha": "abc"},
			"base": map[string]any{"ref": "main", "sha": "def"},
			"user": map[string]any{"id": 1, "login": "dev"},
		},
		"repository": map[string]any{"id": 99, "full_name": "owner/repo"},
		"sender":     map[string]any{"id": 1, "login": "dev"},
	})
	ev := mustParse(t, rec)
	if ev.CR == nil {
		t.Fatal("CR must be populated")
	}
	if ev.CR.ID != 98123 {
		t.Fatalf("CR.ID = %d, want the payload's global id 98123 (old: number 42)", ev.CR.ID)
	}
	if ev.CR.Number != "42" {
		t.Fatalf("CR.Number = %q, want \"42\"", ev.CR.Number)
	}

	rec = signedHookRequest(t, "issues", map[string]any{
		"action": "opened",
		"number": 12,
		"issue": map[string]any{
			"id": 88001, "number": 12, "title": "bug", "body": "b", "state": "open",
			"user": map[string]any{"id": 1, "login": "dev"},
		},
		"repository": map[string]any{"id": 99, "full_name": "owner/repo"},
		"sender":     map[string]any{"id": 1, "login": "dev"},
	})
	ev = mustParse(t, rec)
	if ev.Issue == nil || ev.Issue.ID != 88001 {
		t.Fatalf("Issue.ID = %v, want the payload's global id 88001", ev.Issue)
	}

	// PR-shaped issue: CR rides the issue's global id too.
	rec = signedHookRequest(t, "issue_comment", map[string]any{
		"action": "created",
		"number": 12,
		"issue": map[string]any{
			"id": 88001, "number": 12, "title": "bug", "state": "open",
			"pull_request": map[string]any{"merged": false},
			"user":         map[string]any{"id": 1, "login": "dev"},
		},
		"comment":    map[string]any{"id": 555, "body": "x", "user": map[string]any{"id": 1, "login": "dev"}},
		"repository": map[string]any{"id": 99, "full_name": "owner/repo"},
		"sender":     map[string]any{"id": 1, "login": "dev"},
	})
	ev = mustParse(t, rec)
	if ev.CR == nil || ev.CR.ID != 88001 {
		t.Fatalf("PR-shaped issue CR.ID = %v, want the issue's global id 88001", ev.CR)
	}
}

// TestParseWebhookEvent_PushTagNormalized pins the tag/branch split on the
// push hook (issue #98, the #76 github face): Gitea/Forgejo deliver tag
// pushes as push hooks with a refs/tags ref — the ref prefix decides the
// event, never a (wire-nonexistent) tag_push event name. Deletion rides the
// all-zero after marker.
func TestParseWebhookEvent_PushTagNormalized(t *testing.T) {
	base := func(ref, after, before string) map[string]any {
		return map[string]any{
			"ref": ref, "after": after, "before": before,
			"repository": map[string]any{"id": 99, "full_name": "owner/repo"},
			"sender":     map[string]any{"id": 1, "login": "dev"},
		}
	}
	zero := strings.Repeat("0", 40)

	ev := mustParse(t, signedHookRequest(t, "push", base("refs/tags/v1.0.0", "abc123", zero)))
	if ev.Type != "tag.created" || ev.Tag != "v1.0.0" {
		t.Fatalf("tag push via push hook: type=%q tag=%q, want tag.created/v1.0.0", ev.Type, ev.Tag)
	}
	if ev.Branch != "" {
		t.Fatalf("Branch must stay clean of the tag ref, got %q", ev.Branch)
	}
	if ev.CommitSHA != "abc123" {
		t.Fatalf("tag.created CommitSHA = %q, want abc123", ev.CommitSHA)
	}

	ev = mustParse(t, signedHookRequest(t, "push", base("refs/tags/v1.0.0", zero, "abc123")))
	if ev.Type != "tag.deleted" || ev.Tag != "v1.0.0" {
		t.Fatalf("tag deletion via push hook: type=%q tag=%q, want tag.deleted/v1.0.0", ev.Type, ev.Tag)
	}

	ev = mustParse(t, signedHookRequest(t, "push", base("refs/heads/main", "abc123", zero)))
	if ev.Type != "push" || ev.Branch != "main" || ev.CommitSHA != "abc123" {
		t.Fatalf("branch push must stay push/main, got type=%q branch=%q sha=%q", ev.Type, ev.Branch, ev.CommitSHA)
	}
}
