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

// CommitStatusHarnessConfig mounts the commit-status suite. The suite is
// self-driving: it records requests, invokes CreateCommitStatus and
// ListCommitStatuses, and asserts exactly one status request per
// operation reached the wire.
//
// ListResponse is the platform-shaped JSON fixture served for the
// ListCommitStatuses probe; it must decode into at least one status with
// a non-empty State under the backend's converter. It is required
// whenever the platform declares the CommitStatuses capability.
//
// EmptyResponse is the platform-shaped JSON body served on page ≥ 2 to
// terminate the pagination walk. It defaults to Harness.EmptyListResponse;
// set it when the status endpoint's empty shape differs from the
// platform's usual empty-list shape (e.g. Gitee's wrapped check-run
// object).
type CommitStatusHarnessConfig struct {
	ListResponse  string
	EmptyResponse string
}

func testCommitStatusSuite(t *testing.T, h Harness) {
	capsDeclared := false
	{
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(h.EmptyListResponse))
		}))
		defer srv.Close()
		p := h.NewProvider(t, baseCfg(h, srv.URL))
		capsDeclared = p.Capabilities().CommitStatuses
	}
	switch {
	case h.CommitStatus == nil && !capsDeclared:
		t.Skipf("%s declares no CommitStatuses capability", h.Name)
	case h.CommitStatus == nil:
		t.Errorf("%s declares Capabilities().CommitStatuses but its Harness provides no CommitStatus config", h.Name)
	case !capsDeclared:
		t.Errorf("%s Harness provides a CommitStatus config but the platform does not declare Capabilities().CommitStatuses", h.Name)
	}

	testCommitStatusCreate(t, h)
	testCommitStatusRead(t, h)
}

// testCommitStatusCreate drives CreateCommitStatus against a recording
// server and asserts exactly one status-shaped request reached the wire.
func testCommitStatusCreate(t *testing.T, h Harness) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"state":"pending"}`))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	csm, ok := p.(provider.CommitStatusManager)
	if !ok {
		t.Fatalf("%s: provider does not implement CommitStatusManager", h.Name)
	}
	if err := csm.CreateCommitStatus(context.Background(), "owner", "repo", "deadbeef",
		provider.CommitStatusOptions{State: "pending", Context: "ci"}); err != nil {
		t.Fatalf("CreateCommitStatus: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 {
		t.Fatalf("%s: CreateCommitStatus made %d requests (%v), want exactly 1", h.Name, len(paths), paths)
	}
	if !strings.Contains(paths[0], "status") && !strings.Contains(paths[0], "statuses") && !strings.Contains(paths[0], "check-runs") {
		t.Errorf("%s: status request path %q does not look like a commit-status endpoint", h.Name, paths[0])
	}
}

// testCommitStatusRead drives ListCommitStatuses against a server that
// serves the platform fixture on page 1 and the platform's empty shape on
// page ≥ 2, and asserts the unified decode plus pagination semantics:
// exactly two GETs (data page + terminal empty page), at least one status,
// no empty normalized states. A backend that regresses to single-page
// fetching fails the exactly-two assertion.
func testCommitStatusRead(t *testing.T, h Harness) {
	if h.CommitStatus.ListResponse == "" {
		t.Errorf("%s declares CommitStatuses but its Harness ships no CommitStatus.ListResponse fixture", h.Name)
		return
	}
	var mu sync.Mutex
	var gotPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPaths = append(gotPaths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 1 {
			empty := h.CommitStatus.EmptyResponse
			if empty == "" {
				empty = h.EmptyListResponse
			}
			_, _ = w.Write([]byte(empty))
			return
		}
		_, _ = w.Write([]byte(h.CommitStatus.ListResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	csm, ok := p.(provider.CommitStatusManager)
	if !ok {
		t.Fatalf("%s: provider does not implement CommitStatusManager", h.Name)
	}
	statuses, err := csm.ListCommitStatuses(context.Background(), "owner", "repo", "deadbeef")
	if err != nil {
		t.Fatalf("%s: ListCommitStatuses: %v", h.Name, err)
	}
	if len(statuses) == 0 {
		t.Fatalf("%s: ListCommitStatuses decoded 0 statuses from fixture %s", h.Name, h.CommitStatus.ListResponse)
	}
	for i, s := range statuses {
		if s.State == "" {
			t.Errorf("%s: status[%d] has empty normalized State", h.Name, i)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(gotPaths) != 2 {
		t.Errorf("%s: ListCommitStatuses made %d requests (%v), want exactly 2 (data page + empty terminal page)", h.Name, len(gotPaths), gotPaths)
	} else {
		for i, gp := range gotPaths {
			if gp[:4] != "GET " {
				t.Errorf("%s: ListCommitStatuses request %d used %q, want GET", h.Name, i, gp)
			}
		}
	}
}
