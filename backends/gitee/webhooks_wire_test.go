package gitee_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/contracttest"
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

// TestParseWebhookEvent_RealHeaderVocabulary 钉住 Gitee 真实投递头的分发
// （issue #41）：平台下发的是 "Push Hook" 形态（带空格、首字母大写），
// 此前 switch 只认小写键，四族投递全部返回空 Type + nil err。
// 对照表：真实 wire 值 → 期望的 canonical Type。
func TestParseWebhookEvent_RealHeaderVocabulary(t *testing.T) {
	cases := []struct {
		wireHeader string
		payload    string
		wantType   string
	}{
		{"Push Hook", `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"o/r"}}`, "push"},
		{"Pull Request Hook", `{"action":"open","number":1,"state":"open","repository":{"full_name":"o/r"}}`, "cr.opened"},
		{"Tag Push Hook", `{"ref":"refs/tags/v1.0","after":"abc123","repository":{"full_name":"o/r"}}`, "tag.created"},
		{"Note Hook", `{"note":"hi","noteable_type":"PullRequest","number":2,"repository":{"full_name":"o/r"}}`, "comment.created"}, //nolint:misspell // 平台 webhook 原始字段名
		{"Issue Hook", `{"action":"open","issue":{"number":"I-1","state":"open"},"repository":{"full_name":"o/r"}}`, "issue.opened"},
		// 大小写/空白变体兜底（防御未来平台改形态时静默回归）
		{"push hook", `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"o/r"}}`, "push"},
		{"  Tag Push Hook  ", `{"ref":"refs/tags/v1.0","after":"abc123","repository":{"full_name":"o/r"}}`, "tag.created"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		p := newTestProvider(t, srv)
		r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(tc.payload))
		r.Header.Set("X-Gitee-Event", tc.wireHeader)
		r.Header.Set("Content-Type", "application/json")
		ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
		if ne.Type != tc.wantType {
			t.Errorf("header %q: type = %q, want %q", tc.wireHeader, ne.Type, tc.wantType)
		}
		srv.Close()
	}
}

// 历史小写键（真实平台从不下发）保留兼容：直接喂入的旧调用方不破。
func TestParseWebhookEvent_LegacyLowercaseKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(`{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"o/r"}}`))
	r.Header.Set("X-Gitee-Event", "push")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "push" {
		t.Fatalf("legacy key: type = %q, want push", ne.Type)
	}
}

// TestParseWebhookEvent_BranchDeletionNormalized 分支删除归一（issue #67，
// 同族 #47 的 push 案）：Gitee 家族没有独立 delete 事件，分支删除以
// push + 全零 after 投递——此前一律归一成 push。
func TestParseWebhookEvent_BranchDeletionNormalized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	mk := func(before, after string) *http.Request {
		body := fmt.Sprintf(`{"ref":"refs/heads/feature","before":%q,"after":%q,"user":{"id":1,"login":"d"},"repository":{"full_name":"o/r"}}`, before, after)
		r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
		r.Header.Set("X-Gitee-Event", "Push Hook")
		r.Header.Set("Content-Type", "application/json")
		return r
	}

	ne := contracttest.ParseSigned(t, p, mk("def4567890abcdef4567890abcdef4567890abcd", "0000000000000000000000000000000000000000"), "corpus-secret")
	if ne.Type != "branch.deleted" || ne.Action != "deleted" || ne.Branch != "feature" {
		t.Fatalf("删除投递必须归一 branch.deleted, got type=%q action=%q branch=%q", ne.Type, ne.Action, ne.Branch)
	}
	if ne.CommitSHA != "" {
		t.Fatalf("删除投递不得携带全零标记作为 commit_sha, got %q", ne.CommitSHA)
	}

	ne = contracttest.ParseSigned(t, p, mk("def4567890abcdef4567890abcdef4567890abcd", "abc123"), "corpus-secret")
	if ne.Type != "push" || ne.CommitSHA != "abc123" {
		t.Fatalf("正常 push 不得误判为删除, got type=%q sha=%q", ne.Type, ne.CommitSHA)
	}
}
