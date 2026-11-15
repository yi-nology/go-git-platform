package tencentcode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/adapter/tencentcode"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestTencentCode_RawPaths_EscapeSegments verifies that the raw diff and
// extras detours percent-encode the variable path segments: the gongfeng
// client's NewRequest concatenates the base URL and the path verbatim into
// RawPath, so an unescaped '#', '?', '%', or space would corrupt or
// truncate the URL before it reaches the server. The escaped pid form
// ("owner%2Frepo") matches what the SDK's typed methods already produce
// via parseID, keeping raw and typed traffic wire-identical.
func TestTencentCode_RawPaths_EscapeSegments(t *testing.T) {
	var mu sync.Mutex
	paths := map[string]string{}
	decoded := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.Method] = stripAPIPrefix(r.URL.EscapedPath())
		decoded[r.Method] = stripAPIPrefix(r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost: // CreateDiscussion decodes an id.
			_, _ = w.Write([]byte(`{"id":11}`))
		case http.MethodGet: // GetCodeReviewChangedFiles decodes a slice.
			_, _ = w.Write([]byte(`[]`))
		default: // DELETE note and PUT protect read no body.
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)

	const (
		owner  = "ow#ner"
		repo   = "re?po"
		noteID = "5 6%"
		branch = "fe#at"
	)
	if err := p.DeleteNote(context.Background(), owner, repo, "1", noteID); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	if _, err := p.CreateDiscussion(context.Background(), owner, repo, "1", provider.DiscussionOptions{Body: "hi"}); err != nil {
		t.Fatalf("CreateDiscussion: %v", err)
	}
	if _, err := p.GetCodeReviewChangedFiles(context.Background(), owner, repo, 7); err != nil {
		t.Fatalf("GetCodeReviewChangedFiles: %v", err)
	}
	if err := p.ProtectBranch(context.Background(), owner, repo, branch, tencentcode.ProtectBranchOptions{MergeAccessLevel: 40}); err != nil {
		t.Fatalf("ProtectBranch: %v", err)
	}

	const escapedPID = "ow%23ner%2Fre%3Fpo"
	for method, want := range map[string]string{
		http.MethodDelete: "/projects/" + escapedPID + "/merge_requests/1/notes/5%206%25",
		http.MethodPost:   "/projects/" + escapedPID + "/merge_requests/1/discussions",
		http.MethodGet:    "/projects/" + escapedPID + "/review/7/changed_files",
		http.MethodPut:    "/projects/" + escapedPID + "/repository/branches/fe%23at/protect",
	} {
		got, ok := paths[method]
		if !ok {
			t.Errorf("expected a %s request, recorded methods: %v", method, paths)
			continue
		}
		if got != want {
			t.Errorf("%s escaped path = %q, want %q (variable segments must be percent-encoded)", method, got, want)
		}
	}
	// The server-side decoded path must round-trip the originals verbatim.
	if want := "/projects/ow#ner/re?po/merge_requests/1/notes/5 6%"; decoded[http.MethodDelete] != want {
		t.Errorf("DELETE decoded path = %q, want %q", decoded[http.MethodDelete], want)
	}
}
