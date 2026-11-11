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

// TestGitCode_PathEscaping verifies special characters in owner/repo/
// label-name/file-path segments survive to the right resource. Since
// go-gitcode v0.16.0 the SDK escapes path segments itself (its issue #6,
// pathf) and QueryEscapes the ref query parameter — this backend's #84
// pre-escaping is retired (issue #116), so the decoded path must carry the
// caller's exact value (one escaping level, applied by the SDK).
// File paths escape per segment with '/' separators kept; the ref query
// arrives in QueryEscape form ('+' for space — decodes to the same value).
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
			if tt.wantRefEscaped && !strings.Contains(query, "ref=bra+nch") {
				// SDK v0.16.0 QueryEscapes ref itself (issue #116): space
				// rides as '+' on the wire and decodes back to "bra nch" —
				// any truncation or double-escape ("%2520") fails this.
				t.Errorf("ref query = %q, want the ref value escaped (not truncated)", query)
			}
		})
	}
}

// TestGitCode_NoDoubleEscapingOnWire pins the post-#116 contract: this
// backend passes caller values through raw and go-gitcode v0.16.0 applies
// exactly one escaping level (pathf's pathSeg / pathSegs, QueryEscape for
// ref). The wire must therefore carry '%23' for a '#' — never '%2523',
// the double-escape signature that shipped when the #84 pre-escaping and
// the SDK's built-in escaping stacked (v0.125.0 had to pin the SDK back to
// v0.15.0 for it). EscapedPath/RawQuery see the wire bytes directly.
func TestGitCode_NoDoubleEscapingOnWire(t *testing.T) {
	tests := []struct {
		name string
		resp string
		call func(p *gitcode.Provider) error
		// wantWire is the exact single-escaped wire form of the resource
		// path; neverWant lists double-escape signatures that must not
		// appear anywhere on the wire.
		wantWire string
	}{
		{
			name: "owner with # is %23 not %2523",
			resp: `[]`,
			call: func(p *gitcode.Provider) error {
				_, err := p.ListBranches(context.Background(), "ow#ner", "re?po")
				return err
			},
			wantWire: "/repos/ow%23ner/re%3Fpo/branches",
		},
		{
			name: "release tag with # is %23 not %2523",
			resp: `{}`,
			call: func(p *gitcode.Provider) error {
				_, err := p.GetReleaseByTag(context.Background(), "owner", "repo", "v1.0#beta")
				return err
			},
			wantWire: "/repos/owner/repo/releases/tags/v1.0%23beta",
		},
		{
			name: "multi-segment file path with # and space escapes per segment",
			resp: `{"encoding":"","content":""}`,
			call: func(p *gitcode.Provider) error {
				_, err := p.GetFileContent(context.Background(), "owner", "repo", "dir one/file#2.txt", "bra nch")
				return err
			},
			wantWire: "/repos/owner/repo/contents/dir%20one/file%232.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var wirePath, rawQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				wirePath = r.URL.EscapedPath()
				rawQuery = r.URL.RawQuery
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
			if !strings.Contains(wirePath, tt.wantWire) {
				t.Errorf("wire path %q does not contain single-escaped form %q", wirePath, tt.wantWire)
			}
			for _, bad := range []string{"%2523", "%252F", "%253F", "%2520"} {
				if strings.Contains(wirePath, bad) || strings.Contains(rawQuery, bad) {
					t.Errorf("double-escape signature %s found on wire (path %q, query %q) — escaping is applied twice", bad, wirePath, rawQuery)
				}
			}
		})
	}
}
