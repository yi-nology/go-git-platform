package gitee_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGitee_CR_DraftMapped pins the ChangeRequest.Draft mapping at the wire
// level (issue #126): the go-gitee SDK's PullRequest model HAS the draft
// field (`Draft *bool json:"draft,omitempty"`, next-bin/go-gitee
// types.go) and convertPR maps it — the old README/readiness limitation
// ("SDK 模型缺该字段,恒为 false") described the pre-v0.54.0 swagger SDK and
// is stale. This test proves the mapping: a wire payload carrying
// draft:true surfaces as ChangeRequest.Draft=true.
func TestGitee_CR_DraftMapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id": 1, "number": 1, "title": "draft pr", "state": "open", "body": "desc",
			"head":  map[string]any{"ref": "feature-x"},
			"base":  map[string]any{"ref": "main"},
			"user":  map[string]any{"id": 1, "login": "dev"},
			"draft": true,
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	cr, err := p.GetCR(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatalf("GetCR: %v", err)
	}
	if !cr.Draft {
		t.Errorf("ChangeRequest.Draft must map the wire's draft:true, got false (mapping regression)")
	}
}
