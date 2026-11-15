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

// TestGitee_CreateDiscussion_InlineAnchor pins issue #86's gitee side: an
// anchored discussion (FilePath set) must ride the PR-comment wire's
// path/position/commit_id fields; an unanchored one sends body only. The
// anchors used to be silently dropped and every discussion landed as a
// top-level note.
func TestGitee_CreateDiscussion_InlineAnchor(t *testing.T) {
	tests := []struct {
		name     string
		opts     provider.DiscussionOptions
		wantBody map[string]any
	}{
		{
			name:     "unanchored sends body only",
			opts:     provider.DiscussionOptions{Body: "top level"},
			wantBody: map[string]any{"body": "top level"},
		},
		{
			name:     "anchor lands path/position and head sha as commit_id",
			opts:     provider.DiscussionOptions{Body: "inline", FilePath: "a.go", NewLine: 42, HeadSHA: "cafebabe"},
			wantBody: map[string]any{"body": "inline", "path": "a.go", "position": float64(42), "commit_id": "cafebabe"},
		},
		{
			name:     "old-line-only anchor rides position",
			opts:     provider.DiscussionOptions{Body: "inline", FilePath: "a.go", OldLine: 7},
			wantBody: map[string]any{"body": "inline", "path": "a.go", "position": float64(7)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var gotBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					mu.Lock()
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					mu.Unlock()
				}
				writeJSON(w, map[string]any{"id": 321})
			}))
			defer srv.Close()
			p := newTestProvider(t, srv)
			id, err := p.CreateDiscussion(context.Background(), "owner", "repo", "7", tt.opts)
			if err != nil {
				t.Fatalf("CreateDiscussion: %v", err)
			}
			if id != "321" {
				t.Errorf("discussion id = %q, want 321", id)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(gotBody) != len(tt.wantBody) {
				t.Fatalf("wire body = %v, want exactly %v", gotBody, tt.wantBody)
			}
			for k, want := range tt.wantBody {
				if gotBody[k] != want {
					t.Errorf("wire body %q = %v, want %v", k, gotBody[k], want)
				}
			}
		})
	}
}
