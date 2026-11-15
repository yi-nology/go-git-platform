package gitea_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	giteasdk "gitea.dev/sdk"

	"github.com/yi-nology/go-git-platform/backends/gitea"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// Regression: Forgejo/Gitea list endpoints only recognize state=open|closed.
// The provider-uniform value "opened" was passed through verbatim, and the
// server responded by returning pull requests of EVERY state — so pollers
// re-reviewed closed PRs. The gitea backend must normalize the filter.
func TestListCRs_NormalizesOpenStateFilter(t *testing.T) {
	var gotState string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotState = r.URL.Query().Get("state")
		writeJSON(w, []*giteasdk.PullRequest{})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	if _, _, err := p.ListCRs(context.Background(), provider.ListCROptions{
		Owner: "owner", Repo: "repo", State: model.CRStateOpened,
	}); err != nil {
		t.Fatal(err)
	}
	if gotState != "open" {
		t.Errorf("expected normalized state filter %q, got %q", "open", gotState)
	}
}

// Regression: the gitea backend dropped pr.Draft when converting, so
// ChangeRequest.Draft was always false and draft-PR skip logic never fired.
func TestGetCR_DraftMapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, &giteasdk.PullRequest{
			ID: 9, Index: 3, Title: "wip", State: giteasdk.StateOpen, Draft: true,
			Head: &giteasdk.PRBranchInfo{Ref: "wip", Sha: "abc"},
			Base: &giteasdk.PRBranchInfo{Ref: "main", Sha: "def"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	cr, err := p.GetCR(context.Background(), "owner", "repo", "3")
	if err != nil {
		t.Fatal(err)
	}
	if !cr.Draft {
		t.Error("expected ChangeRequest.Draft=true for a draft PR, got false")
	}
}

// GetCR is exercised through the exported provider surface; keep the compile
// guard that the concrete type still satisfies the interface.
var _ provider.Provider = (*gitea.Provider)(nil)

// Regression: the gitea SDK reports missing files as a plain sentinel error
// without status info, so provider.IsNotFound stayed false and optional-file
// callers (repo review config, review guidelines) logged spurious failures
// instead of falling back silently.
func TestGetFileContent_MissingFileIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"The target couldn't be found."}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, err := p.GetFileContent(context.Background(), "owner", "repo", ".reviewagents.yaml", "main")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !provider.IsNotFound(err) {
		t.Fatalf("expected IsNotFound=true, got %v", err)
	}
}
