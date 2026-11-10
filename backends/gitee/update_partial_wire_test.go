package gitee_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/gitee"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitee_PartialUpdateOmitsEmptyFields pins the "empty string = leave
// unchanged" convention on the wire (issue #88): go-gitee's update-option
// fields are *string with omitempty, which only omits nil pointers — a
// gitee.String("") used to go out as an explicit empty string and wipe the
// untouched field (or 422 the whole request). Only the fields the caller
// set may appear in the PATCH body.
func TestGitee_PartialUpdateOmitsEmptyFields(t *testing.T) {
	tests := []struct {
		name     string
		call     func(p *gitee.Provider) error
		wantBody map[string]any
	}{
		{
			name: "UpdateCR description only keeps title off the wire",
			call: func(p *gitee.Provider) error {
				_, err := p.UpdateCR(context.Background(), "owner", "repo", "7", provider.UpdateCROptions{Description: "new body"})
				return err
			},
			wantBody: map[string]any{"body": "new body"},
		},
		{
			name: "UpdateCR title only keeps body off the wire",
			call: func(p *gitee.Provider) error {
				_, err := p.UpdateCR(context.Background(), "owner", "repo", "7", provider.UpdateCROptions{Title: "new title"})
				return err
			},
			wantBody: map[string]any{"title": "new title"},
		},
		{
			name: "UpdateRepo name only keeps description and default_branch off the wire",
			call: func(p *gitee.Provider) error {
				_, err := p.UpdateRepo(context.Background(), "owner", "repo", provider.UpdateRepoOptions{Name: "renamed"})
				return err
			},
			wantBody: map[string]any{"name": "renamed"},
		},
		{
			name: "UpdateRepo default_branch only keeps name off the wire",
			call: func(p *gitee.Provider) error {
				_, err := p.UpdateRepo(context.Background(), "owner", "repo", provider.UpdateRepoOptions{DefaultBranch: "trunk"})
				return err
			},
			wantBody: map[string]any{"default_branch": "trunk"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var gotBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch {
					mu.Lock()
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					mu.Unlock()
				}
				writeJSON(w, map[string]any{"id": 7, "number": 7, "name": "repo", "title": "t", "state": "open", "path": "owner/repo"})
			}))
			defer srv.Close()
			if err := tt.call(newTestProvider(t, srv)); err != nil {
				t.Fatalf("update call: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(gotBody) != len(tt.wantBody) {
				t.Fatalf("wire body = %v, want exactly %v (unset fields must stay off the wire)", gotBody, tt.wantBody)
			}
			for k, want := range tt.wantBody {
				if gotBody[k] != want {
					t.Errorf("wire body %q = %v, want %v", k, gotBody[k], want)
				}
			}
		})
	}
}
