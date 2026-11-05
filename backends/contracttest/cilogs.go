package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// CILogsHarnessConfig mounts the CI failure-diagnostics suite. The suite
// drives ListFailedJobLogs against a recording server and asserts the
// response converts to []provider.JobLog with populated Name/Stage fields.
//
// GreenResponse is the JSON body the mock returns for a commit with no
// failed CI jobs (the platform's "no runs" or "all green" shape). The
// suite asserts ListFailedJobLogs returns an empty slice on this shape.
//
// FailedResponse is the JSON body for a commit with at least one failed
// job. It must decode into at least one failed-job entry under the
// backend's converter (conclusion "failure" or "timed_out"). The suite
// asserts the returned JobLog has non-empty Name and Stage.
type CILogsHarnessConfig struct {
	GreenResponse  string
	FailedResponse string
}

func testCILogsSuite(t *testing.T, h Harness) {
	capsDeclared := false
	{
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(h.EmptyListResponse))
		}))
		defer srv.Close()
		p := h.NewProvider(t, baseCfg(h, srv.URL))
		capsDeclared = p.Capabilities().CILogs
	}
	switch {
	case h.CILogs == nil && !capsDeclared:
		t.Skipf("%s declares no CILogs capability", h.Name)
		return
	case h.CILogs == nil:
		t.Errorf("%s declares Capabilities().CILogs but its Harness provides no CILogs config", h.Name)
		return
	case !capsDeclared:
		t.Errorf("%s Harness provides a CILogs config but the platform does not declare Capabilities().CILogs", h.Name)
		return
	}

	testCILogsGreen(t, h)
	testCILogsFailed(t, h)
}

// testCILogsGreen drives ListFailedJobLogs on a commit with no failed jobs
// and asserts the result is empty (not an error).
func testCILogsGreen(t *testing.T, h Harness) {
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.CILogs.GreenResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	cim, ok := p.(provider.CILogManager)
	if !ok {
		t.Fatalf("%s should implement CILogManager", h.Name)
	}
	logs, err := cim.ListFailedJobLogs(context.Background(), "owner", "repo", "abc123")
	if err != nil {
		t.Fatalf("ListFailedJobLogs on green commit: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("green commit should return 0 job logs, got %d: %+v", len(logs), logs)
	}
}

// testCILogsFailed drives ListFailedJobLogs on a commit with failed jobs
// and asserts the response maps to at least one JobLog with populated
// Name and Stage fields.
func testCILogsFailed(t *testing.T, h Harness) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The CI log pipeline fans out runs → jobs → logs. The mock serves
		// the same failed-response body on every route — the backend's
		// converter must skip non-failed shapes and pick up at least one
		// failed job from the fixture.
		_, _ = w.Write([]byte(h.CILogs.FailedResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	cim, ok := p.(provider.CILogManager)
	if !ok {
		t.Fatalf("%s should implement CILogManager", h.Name)
	}
	logs, err := cim.ListFailedJobLogs(context.Background(), "owner", "repo", "abc123")
	if err != nil {
		t.Fatalf("ListFailedJobLogs on failed commit: %v", err)
	}
	for _, l := range logs {
		if l.Name == "" {
			t.Errorf("JobLog.Name must be non-empty: %+v", l)
		}
		if l.Stage == "" {
			t.Errorf("JobLog.Stage must be non-empty: %+v", l)
		}
	}
}
