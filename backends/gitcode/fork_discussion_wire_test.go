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

// TestGitCode_ForkRepo_MapsName pins issue #87: ForkRepoOptions.Name maps
// onto the fork body's name field (renaming the fork at creation). It used
// to be swallowed whole, so a caller asking for a renamed fork silently
// got the default name. opts.Organization has no wire (registered
// divergence) and must stay off the body.
func TestGitCode_ForkRepo_MapsName(t *testing.T) {
	tests := []struct {
		name     string
		opts     provider.ForkRepoOptions
		wantBody map[string]any
	}{
		{
			name:     "no options send no name",
			opts:     provider.ForkRepoOptions{},
			wantBody: map[string]any{},
		},
		{
			name:     "name rides the fork body",
			opts:     provider.ForkRepoOptions{Name: "renamed"},
			wantBody: map[string]any{"name": "renamed"},
		},
		{
			name:     "organization has no wire and stays off the body",
			opts:     provider.ForkRepoOptions{Organization: "some-org", Name: "renamed"},
			wantBody: map[string]any{"name": "renamed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var gotBody map[string]any
			var path string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				path = r.URL.Path
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				mu.Unlock()
				writeJSON(w, map[string]any{"id": 9, "name": "forked", "full_name": "me/forked"})
			}))
			defer srv.Close()
			if _, err := newTestProvider(t, srv).ForkRepo(context.Background(), "owner", "repo", tt.opts); err != nil {
				t.Fatalf("ForkRepo: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if path != "/repos/owner/repo/forks" {
				t.Errorf("fork path = %q, want /repos/owner/repo/forks", path)
			}
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

// TestGitCode_CreateDiscussion_InlineAnchor pins issue #86: an anchored
// discussion (FilePath set) must ride the inline-comment wire with
// path/line/side/commit_id; an unanchored one stays a plain top-level
// comment. The anchors used to be silently dropped.
func TestGitCode_CreateDiscussion_InlineAnchor(t *testing.T) {
	tests := []struct {
		name     string
		opts     provider.DiscussionOptions
		wantBody map[string]any
	}{
		{
			name:     "unanchored stays a plain comment",
			opts:     provider.DiscussionOptions{Body: "top level"},
			wantBody: map[string]any{"body": "top level", "path": "", "position": "", "commit_id": ""},
		},
		{
			name:     "new-line anchor lands path/line/side and head sha",
			opts:     provider.DiscussionOptions{Body: "inline", FilePath: "a.go", NewLine: 42, HeadSHA: "cafebabe"},
			wantBody: map[string]any{"body": "inline", "path": "a.go", "line": float64(42), "side": "RIGHT", "commit_id": "cafebabe"},
		},
		{
			name:     "old-line-only anchor flips to the LEFT side",
			opts:     provider.DiscussionOptions{Body: "inline", FilePath: "a.go", OldLine: 7},
			wantBody: map[string]any{"body": "inline", "path": "a.go", "line": float64(7), "side": "LEFT"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var gotBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				mu.Unlock()
				writeJSON(w, map[string]any{"id": 77})
			}))
			defer srv.Close()
			id, err := newTestProvider(t, srv).CreateDiscussion(context.Background(), "owner", "repo", "7", tt.opts)
			if err != nil {
				t.Fatalf("CreateDiscussion: %v", err)
			}
			if id != "77" {
				t.Errorf("discussion id = %q, want 77", id)
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
