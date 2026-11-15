package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// GistsHarnessConfig carries fixtures for the gist-list suite.
type GistsHarnessConfig struct {
	// ListResponse is the JSON array the mock returns for GET /gists. Its
	// first element must have a non-empty id and description — the suite
	// asserts both parse into model.Gist.
	ListResponse string
}

func testGistsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Gists
	switch {
	case h.Gists == nil && !declared:
		t.Skipf("%s declares no Gists capability", h.Name)
	case h.Gists == nil:
		t.Errorf("%s declares Capabilities().Gists but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides Gists config but platform does not declare capability", h.Name)
	default:
		RunGistsSuite(t, GistsHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			ListResponse: h.Gists.ListResponse,
		})
	}
}

// GistsHarness bundles inputs for the gist-list suite.
type GistsHarness struct {
	Name         string
	Platform     model.Platform
	NewProvider  func(t *testing.T, cfg provider.Config) provider.Provider
	ListResponse string
}

// gistsWire records the request line the gists subtests assert on: the path
// (route shape) and the normalized page/per_page query parameters.
type gistsWire struct {
	mu      sync.Mutex
	path    string
	page    string
	perPage string
	n       int
}

func (w *gistsWire) record(r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.path = r.URL.Path
	w.page = r.URL.Query().Get("page")
	w.perPage = r.URL.Query().Get("per_page")
	w.n++
}

func (w *gistsWire) last() (path, page, perPage string, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path, w.page, w.perPage, w.n
}

// RunGistsSuite tests the gist-manager wire contract: GET /gists (the token
// user's own list, never the users/{login}/gists public route) parses into
// model.Gist, and page/perPage land in the query after normalization.
func RunGistsSuite(t *testing.T, h GistsHarness) {
	newGM := func(t *testing.T, rec *gistsWire) provider.GistManager {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec.record(r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(h.ListResponse))
		}))
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		gm, ok := p.(provider.GistManager)
		if !ok {
			t.Fatalf("%s does not implement provider.GistManager", h.Name)
		}
		return gm
	}

	t.Run("ListMyGists_ParsesAndPages", func(t *testing.T) {
		rec := &gistsWire{}
		gm := newGM(t, rec)
		gists, err := gm.ListMyGists(context.Background(), 2, 5)
		if err != nil {
			t.Fatalf("ListMyGists: %v", err)
		}
		path, page, perPage, n := rec.last()
		if n != 1 {
			t.Errorf("expected exactly 1 request, got %d", n)
		}
		if !strings.HasSuffix(path, "/gists") || strings.Contains(path, "/users/") {
			t.Errorf("expected GET /gists (token 用户列表), got path %s", path)
		}
		if page != "2" || perPage != "5" {
			t.Errorf("expected page=2&per_page=5 on the wire, got page=%s per_page=%s", page, perPage)
		}
		if len(gists) == 0 {
			t.Fatal("expected at least 1 gist")
		}
		g := gists[0]
		if g.ID == "" {
			t.Error("model.Gist.ID must be non-empty")
		}
		if g.Description == "" {
			t.Error("model.Gist.Description must be non-empty")
		}
	})

	t.Run("ListMyGists_NormalizedPageDefaults", func(t *testing.T) {
		rec := &gistsWire{}
		gm := newGM(t, rec)
		if _, err := gm.ListMyGists(context.Background(), 0, 0); err != nil {
			t.Fatalf("ListMyGists(0,0): %v", err)
		}
		_, page, perPage, _ := rec.last()
		wantPage := strconv.Itoa(provider.DefaultPage)
		wantPerPage := strconv.Itoa(provider.DefaultPerPage)
		if page != wantPage || perPage != wantPerPage {
			t.Errorf("expected normalized page=%s&per_page=%s on the wire, got page=%s per_page=%s",
				wantPage, wantPerPage, page, perPage)
		}
	})
}
