package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// ForkSyncHarnessConfig carries fixtures for the fork-sync suite.
type ForkSyncHarnessConfig struct {
	// MergeResponse is the JSON POST /repos/{o}/{r}/merge-upstream answers
	// with (the RepoMergeUpstreamResult shape: message/merge_type/base_
	// branch). SyncFork discards the body, but the SDK still decodes it, so
	// it must be a valid object.
	MergeResponse string
}

func testForkSyncSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().ForkSync
	switch {
	case h.ForkSync == nil && !declared:
		t.Skipf("%s declares no ForkSync capability", h.Name)
	case h.ForkSync == nil:
		t.Errorf("%s declares Capabilities().ForkSync but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides ForkSync config but platform does not declare capability", h.Name)
	default:
		RunForkSyncSuite(t, ForkSyncHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			MergeResponse: h.ForkSync.MergeResponse,
		})
	}
}

// ForkSyncHarness bundles inputs for the fork-sync suite.
type ForkSyncHarness struct {
	Name          string
	Platform      provider.Platform
	NewProvider   func(t *testing.T, cfg provider.Config) provider.Provider
	MergeResponse string
}

// forkSyncWire records the request line the fork-sync subtests assert on:
// method, path (the merge-upstream route), and the decoded body ({branch}).
type forkSyncWire struct {
	mu     sync.Mutex
	method string
	path   string
	body   map[string]any
	n      int
}

func (w *forkSyncWire) record(r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.method = r.Method
	w.path = r.URL.Path
	w.body = decodeRecordedBody(r)
	w.n++
}

func (w *forkSyncWire) last() (method, path string, body map[string]any, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.method, w.path, w.body, w.n
}

// RunForkSyncSuite tests the fork-sync manager wire contract: a normal
// SyncFork POSTs {branch} to the merge-upstream route, and an empty branch
// is rejected pre-wire with zero requests (never a merge-upstream call with
// an empty branch).
func RunForkSyncSuite(t *testing.T, h ForkSyncHarness) {
	t.Run("SyncFork_MergeUpstream_BranchBody", func(t *testing.T) { runSyncForkCase(t, h) })
	t.Run("SyncFork_EmptyBranch_PreWire", func(t *testing.T) { runSyncForkEmptyBranchCase(t, h) })
}

// newForkSyncManager builds a provider against a mock that records every
// request and answers with resp, then type-asserts the fork-sync interface.
func newForkSyncManager(t *testing.T, h ForkSyncHarness, resp string, rec *forkSyncWire) provider.ForkSyncManager {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	fm, ok := p.(provider.ForkSyncManager)
	if !ok {
		t.Fatalf("%s does not implement provider.ForkSyncManager", h.Name)
	}
	return fm
}

// runSyncForkCase pins the normal write: exactly one POST on the
// merge-upstream route carrying {branch}, with a nil error.
func runSyncForkCase(t *testing.T, h ForkSyncHarness) {
	t.Helper()
	rec := &forkSyncWire{}
	fm := newForkSyncManager(t, h, h.MergeResponse, rec)
	if err := fm.SyncFork(context.Background(), "owner", "repo", "main"); err != nil {
		t.Fatalf("SyncFork(main): %v", err)
	}
	method, path, body, n := rec.last()
	if n != 1 {
		t.Errorf("expected exactly 1 request, got %d", n)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	// Site-verified (2C T3): go-github MergeUpstream hits
	// POST repos/{owner}/{repo}/merge-upstream (repos_merging.go, meta:
	// operation POST /repos/{owner}/{repo}/merge-upstream).
	if !strings.HasSuffix(path, "/repos/owner/repo/merge-upstream") {
		t.Errorf("path = %s, want .../repos/owner/repo/merge-upstream", path)
	}
	if body == nil {
		t.Fatal("SyncFork sent no decodable JSON body")
	}
	if body["branch"] != "main" {
		t.Errorf("wire branch = %v, want main", body["branch"])
	}
}

// runSyncForkEmptyBranchCase pins the pre-wire branch guard: an empty
// branch must fail closed BEFORE any HTTP with the unified 422 →
// ErrInvalidInput shape, never reaching merge-upstream with branch="".
func runSyncForkEmptyBranchCase(t *testing.T, h ForkSyncHarness) {
	t.Helper()
	rec := &forkSyncWire{}
	fm := newForkSyncManager(t, h, h.MergeResponse, rec)
	err := fm.SyncFork(context.Background(), "owner", "repo", "")
	_, _, _, n := rec.last()
	assertPreWireInvalidInput(t, err, n, `SyncFork(branch="")`)
}
