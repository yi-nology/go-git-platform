package gitcode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/gitcode"
)

// TestGitCode_PathEscaping mirrors the gitee backend's escape discipline
// (issue #84): go-gitcode interpolates path parameters verbatim
// (base+path string concatenation), so owner/repo/label-name/file-path
// segments must be percent-encoded on the wire — characters like '#', '?',
// '%', and spaces would otherwise truncate or corrupt the URL and hit the
// wrong resource. File paths escape per segment with '/' separators kept.
func TestGitCode_PathEscaping(t *testing.T) {
	tests := []struct {
		name string
		// resp is the canned response body (list endpoints need an array,
		// single-object endpoints an object).
		resp           string
		call           func(p *gitcode.Provider) error
		wantContains   []string
		wantRefEscaped bool
	}{
		{
			name: "ListBranches owner and repo segments survive",
			resp: `[]`,
			call: func(p *gitcode.Provider) error {
				_, err := p.ListBranches(context.Background(), "ow#ner", "re?po")
				return err
			},
			wantContains: []string{"ow#ner", "re?po", "/repos/ow#ner/re?po/branches"},
		},
		{
			name: "GetCR owner and repo segments survive",
			resp: `{}`,
			call: func(p *gitcode.Provider) error {
				_, err := p.GetCR(context.Background(), "ow#ner", "re?po", "1")
				return err
			},
			wantContains: []string{"/repos/ow#ner/re?po/pulls/1"},
		},
		{
			name: "RemoveIssueLabel label name segment survives",
			resp: `{}`,
			call: func(p *gitcode.Provider) error {
				return p.RemoveIssueLabel(context.Background(), "owner", "repo", "1", "C#")
			},
			wantContains: []string{"/repos/owner/repo/issues/1/labels/C#"},
		},
		{
			name: "GetFileContent multi-segment file path and ref survive",
			resp: `{"encoding":"","content":""}`,
			call: func(p *gitcode.Provider) error {
				_, err := p.GetFileContent(context.Background(), "owner", "repo", "dir one/file#2.txt", "bra nch")
				return err
			},
			wantContains:   []string{"/repos/owner/repo/contents/dir one/file#2.txt"},
			wantRefEscaped: true,
		},
		{
			name: "GetReleaseByTag tag segment survives",
			resp: `{}`,
			call: func(p *gitcode.Provider) error {
				_, err := p.GetReleaseByTag(context.Background(), "owner", "repo", "v1.0#beta")
				return err
			},
			wantContains: []string{"/repos/owner/repo/releases/tags/v1.0#beta"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var paths []string
			var query string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				query = r.URL.RawQuery
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.resp))
			}))
			defer srv.Close()
			if err := tt.call(newTestProvider(t, srv)); err != nil {
				t.Fatalf("call: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(paths) == 0 {
				t.Fatal("no request recorded")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(paths[0], want) {
					t.Errorf("path %q does not contain %q (segment not escaped or truncated)", paths[0], want)
				}
			}
			if tt.wantRefEscaped && !strings.Contains(query, "ref=bra%20nch") {
				t.Errorf("ref query = %q, want the ref value escaped (not truncated)", query)
			}
		})
	}
}
