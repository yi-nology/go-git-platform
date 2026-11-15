package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// RepoTemplatesHarnessConfig mounts the template-repo suite. The suite
// drives CreateRepoFromTemplate against a recording server and asserts the
// response converts to a PlatformRepo with a non-empty Name.
//
// GenerateResponse is the platform-shaped JSON body served on the
// generate endpoint (POST /repos/{o}/{r}/generate). It must decode into a
// repository object with a non-empty Name under the backend's converter.
type RepoTemplatesHarnessConfig struct {
	GenerateResponse string
}

func testRepoTemplatesSuite(t *testing.T, h Harness) {
	capsDeclared := false
	{
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(h.EmptyListResponse))
		}))
		defer srv.Close()
		p := h.NewProvider(t, baseCfg(h, srv.URL))
		capsDeclared = p.Capabilities().RepoTemplates
	}
	switch {
	case h.RepoTemplates == nil && !capsDeclared:
		t.Skipf("%s declares no RepoTemplates capability", h.Name)
		return
	case h.RepoTemplates == nil:
		t.Errorf("%s declares Capabilities().RepoTemplates but its Harness provides no RepoTemplates config", h.Name)
		return
	case !capsDeclared:
		t.Errorf("%s Harness provides a RepoTemplates config but the platform does not declare Capabilities().RepoTemplates", h.Name)
		return
	}

	testRepoTemplatesGenerate(t, h)
}

// testRepoTemplatesGenerate drives CreateRepoFromTemplate and asserts the
// response maps to a PlatformRepo with a non-empty Name.
func testRepoTemplatesGenerate(t *testing.T, h Harness) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.RepoTemplates.GenerateResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	tm, ok := p.(provider.TemplateRepoManager)
	if !ok {
		t.Fatalf("%s should implement TemplateRepoManager", h.Name)
	}
	repo, err := tm.CreateRepoFromTemplate(context.Background(), "tpl", "base",
		provider.CreateRepoFromTemplateOptions{Owner: "owner", Name: "new-repo"})
	if err != nil {
		t.Fatalf("CreateRepoFromTemplate: %v", err)
	}
	if repo == nil {
		t.Fatal("CreateRepoFromTemplate returned nil repo")
	}
	if repo.Name == "" {
		t.Errorf("PlatformRepo.Name must be non-empty: %+v", repo)
	}
}
