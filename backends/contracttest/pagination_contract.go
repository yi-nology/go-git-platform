package contracttest

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// pagination_contract.go pins the cross-backend Page==0 dual-mode contract
// (issue #60) and the ListMilestones State contract (issue #61) on the
// exhaustive-family managers (Labels, Milestones, Notifications).
//
// Wire-level assertions are deliberately count- and state-param-based so
// they hold for every backend regardless of SDK query vocabulary: the walk
// subtest counts GETs (fixture page + one empty terminal page = exactly 2),
// the caller-paged subtest counts GETs (exactly 1), and the State subtests
// read the "state" query parameter (absent or "all" for the zero value;
// forwarded verbatim for "closed" unless the backend's ledger registers a
// ListMilestones State ignore).

// paginatedListStub returns a recording mock that serves body on the first
// GET and emptyBody on every later GET — the walk terminator. Requests are
// recorded for count/param assertions; pathFilter (nil = all paths) lets a
// suite restrict counting to its own endpoint when the backend issues other
// GETs around the list (project lookups, version probes).
func paginatedListStub(body, emptyBody string, pathFilter func(*http.Request) bool) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	var lists int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pathFilter != nil && !pathFilter(r) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.RequestURI()})
		isList := r.Method == http.MethodGet
		if isList {
			lists++
		}
		serve := body
		if lists > 1 {
			serve = emptyBody
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !isList {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(serve))
	}))
	return srv, &requests
}

// countGETs counts the recorded GET requests.
func countGETs(requests *[]recordedRequest) int {
	n := 0
	for i := range *requests {
		if (*requests)[i].Method == http.MethodGet {
			n++
		}
	}
	return n
}

// newProviderFn builds a backend provider the same way the enclosing
// suite's harness does (including VersionProxy wrapping where wired).
type newProviderFn func(t *testing.T, cfg provider.Config) provider.Provider

// assertPageZeroWalksAllPages pins the exhaustive-family Page==0 contract:
// the list walks past the first page and stops on the first empty page —
// exactly two GETs for a one-page fixture (issue #60). A single GET means
// the backend still truncates at the platform's default page (the pre-fix
// GitLab/Gitee shape); a grind to the 50-page budget cap means the empty
// terminal page did not stop the walk.
func assertPageZeroWalksAllPages(t *testing.T, h newProviderFn, platform provider.Platform, body, emptyBody string, pathFilter func(*http.Request) bool, list func(p provider.Provider) error) {
	t.Helper()
	srv, requests := paginatedListStub(body, emptyBody, pathFilter)
	defer srv.Close()
	p := h(t, provider.Config{Platform: platform, BaseURL: srv.URL, Token: "test"})
	if err := list(p); err != nil {
		t.Fatalf("Page==0 list: %v", err)
	}
	if got := countGETs(requests); got != 2 {
		t.Fatalf("Page==0 walk: expected exactly 2 GETs (fixture page + empty terminal page), got %d — dual-mode exhaust contract violated (issue #60)", got)
	}
}

// assertPageOneSingleRequest pins the Page>0 half of the dual-mode
// contract: a caller-driven page issues exactly one GET (issue #60).
func assertPageOneSingleRequest(t *testing.T, h newProviderFn, platform provider.Platform, body string, pathFilter func(*http.Request) bool, list func(p provider.Provider) error) {
	t.Helper()
	srv, requests := paginatedListStub(body, body, pathFilter)
	defer srv.Close()
	p := h(t, provider.Config{Platform: platform, BaseURL: srv.URL, Token: "test"})
	if err := list(p); err != nil {
		t.Fatalf("Page==1 list: %v", err)
	}
	if got := countGETs(requests); got != 1 {
		t.Fatalf("Page==1: expected exactly 1 GET, got %d — caller-driven paging must not walk (issue #60)", got)
	}
}

// PageZeroFixture carries the manager-specific pieces the exhaustive-family
// suites hand to runPageZeroDualModeSuite.
type PageZeroFixture struct {
	// Platform is the provider.Platform constant the suite's Harness carries.
	Platform provider.Platform
	// ListBody is the fixture served on the first list GET.
	ListBody string
	// EmptyBody is the terminal empty page in the same top-level JSON shape
	// as ListBody (array `[]` or wrapped object).
	EmptyBody string
	// PathFilter restricts which paths count as list requests (nil = all).
	PathFilter func(*http.Request) bool
	// ListZero issues the manager's list call with zero-valued options.
	ListZero func(p provider.Provider) error
	// ListPageOne issues the manager's list call with Page:1.
	ListPageOne func(p provider.Provider) error
}

func runPageZeroDualModeSuite(t *testing.T, newProvider newProviderFn, f PageZeroFixture) {
	t.Run("List_PageZero_WalksAllPages", func(t *testing.T) {
		assertPageZeroWalksAllPages(t, newProvider, f.Platform, f.ListBody, f.EmptyBody, f.PathFilter, f.ListZero)
	})
	t.Run("List_PageOne_SingleRequest", func(t *testing.T) {
		assertPageOneSingleRequest(t, newProvider, f.Platform, f.ListBody, f.PathFilter, f.ListPageOne)
	})
}
