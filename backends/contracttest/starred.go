package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// StarredHarnessConfig carries fixtures for the starred-repo list suite.
type StarredHarnessConfig struct {
	// WrappedResponse is the star+json envelope shape
	// ([{"starred_at":…,"repo":{…}}]) GitHub serves when the request carries
	// Accept: application/vnd.github.star+json — the shape ListStarred asks
	// for. Its first element must map to a repo with non-empty full_name,
	// owner, and description, and a non-zero stargazers_count.
	WrappedResponse string
	// BareResponse is the bare repository array shape ([{…}]) a server
	// returns when it ignores the Accept header (proxy rewrites, GHES
	// drift). ListStarred must fall back to it per element via
	// decodeStarredEntry; same first-element requirements as
	// WrappedResponse.
	BareResponse string
}

func testStarredSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Starred
	switch {
	case h.Starred == nil && !declared:
		t.Skipf("%s declares no Starred capability", h.Name)
	case h.Starred == nil:
		t.Errorf("%s declares Capabilities().Starred but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides Starred config but platform does not declare capability", h.Name)
	default:
		RunStarredSuite(t, StarredHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			WrappedResponse: h.Starred.WrappedResponse,
			BareResponse:    h.Starred.BareResponse,
		})
	}
}

// StarredHarness bundles inputs for the starred-repo list suite.
type StarredHarness struct {
	Name            string
	Platform        provider.Platform
	NewProvider     func(t *testing.T, cfg provider.Config) provider.Provider
	WrappedResponse string
	BareResponse    string
}

// starredWire records the request line the starred subtests assert on: the
// path, the normalized page/per_page query parameters, and the Accept
// header (ListStarred must request the star+json envelope).
type starredWire struct {
	mu      sync.Mutex
	path    string
	page    string
	perPage string
	accept  string
	n       int
}

func (w *starredWire) record(r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.path = r.URL.Path
	w.page = r.URL.Query().Get("page")
	w.perPage = r.URL.Query().Get("per_page")
	w.accept = r.Header.Get("Accept")
	w.n++
}

func (w *starredWire) last() (path, page, perPage, accept string, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path, w.page, w.perPage, w.accept, w.n
}

// RunStarredSuite tests the starred-manager wire contract: GET user/starred
// requests the star+json Accept header, carries normalized paging, and
// decodes both the wrapped ({repo:…}) and bare repository shapes into
// provider.PlatformRepo.
func RunStarredSuite(t *testing.T, h StarredHarness) {
	newSM := func(t *testing.T, body string, rec *starredWire) provider.StarredManager {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec.record(r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		sm, ok := p.(provider.StarredManager)
		if !ok {
			t.Fatalf("%s does not implement provider.StarredManager", h.Name)
		}
		return sm
	}

	t.Run("ListStarred_WrappedShape", func(t *testing.T) {
		rec := &starredWire{}
		sm := newSM(t, h.WrappedResponse, rec)
		repos, err := sm.ListStarred(context.Background(), 2, 5)
		if err != nil {
			t.Fatalf("ListStarred: %v", err)
		}
		path, page, perPage, accept, n := rec.last()
		if n != 1 {
			t.Errorf("expected exactly 1 request, got %d", n)
		}
		if !strings.HasSuffix(path, "/user/starred") {
			t.Errorf("expected GET .../user/starred, got path %s", path)
		}
		if accept != "application/vnd.github.star+json" {
			t.Errorf("Accept = %q, want application/vnd.github.star+json", accept)
		}
		if page != "2" || perPage != "5" {
			t.Errorf("expected page=2&per_page=5 on the wire, got page=%s per_page=%s", page, perPage)
		}
		assertStarredRepo(t, repos, h.Platform)
	})

	t.Run("ListStarred_BareShape", func(t *testing.T) {
		rec := &starredWire{}
		sm := newSM(t, h.BareResponse, rec)
		repos, err := sm.ListStarred(context.Background(), 0, 0)
		if err != nil {
			t.Fatalf("ListStarred: %v", err)
		}
		_, page, perPage, _, _ := rec.last()
		wantPage := strconv.Itoa(provider.DefaultPage)
		wantPerPage := strconv.Itoa(provider.DefaultPerPage)
		if page != wantPage || perPage != wantPerPage {
			t.Errorf("expected normalized page=%s&per_page=%s on the wire, got page=%s per_page=%s",
				wantPage, wantPerPage, page, perPage)
		}
		assertStarredRepo(t, repos, h.Platform)
	})
}

// assertStarredRepo pins the parsed provider.PlatformRepo fields the starred
// fixtures guarantee: identity, description, star count, and platform.
func assertStarredRepo(t *testing.T, repos []*provider.PlatformRepo, platform provider.Platform) {
	t.Helper()
	if len(repos) == 0 {
		t.Fatal("expected at least 1 starred repo")
	}
	r := repos[0]
	if r.FullName == "" {
		t.Error("PlatformRepo.FullName must be non-empty")
	}
	if r.Owner == "" {
		t.Error("PlatformRepo.Owner must be non-empty")
	}
	if r.Description == "" {
		t.Error("PlatformRepo.Description must be non-empty")
	}
	if r.Stars == 0 {
		t.Error("PlatformRepo.Stars must be non-zero")
	}
	if r.Platform != platform {
		t.Errorf("PlatformRepo.Platform = %s, want %s", r.Platform, platform)
	}
}
