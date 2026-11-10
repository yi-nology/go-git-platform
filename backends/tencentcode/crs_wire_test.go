package tencentcode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestMergeCR_MergedIsIdempotentSuccess pins the family-aligned merged
// recovery (issue #94): retrying MergeCR on an already-merged MR must
// return the CR (idempotent convergence), not a hard "not opened" failure —
// gitea/forgejo recover the same shape via the 405-merged path, so a retry
// storm against one backend must not poison the other.
func TestMergeCR_MergedIsIdempotentSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"iid": 7, "state": "merged", "title": "t"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	cr, err := p.MergeCR(context.Background(), "owner", "repo", "7", provider.MergeCROptions{})
	if err != nil {
		t.Fatalf("merged MR retry must be an idempotent success, got %v", err)
	}
	if cr == nil || cr.State != provider.CRStateMerged {
		t.Fatalf("want the merged CR, got %+v", cr)
	}
}

// TestMergeCR_ClosedStillFails keeps the genuine failure sharp: merging a
// closed (not merged) MR is a caller error and must stay an error.
func TestMergeCR_ClosedStillFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"iid": 7, "state": "closed", "title": "t"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	_, err := p.MergeCR(context.Background(), "owner", "repo", "7", provider.MergeCROptions{})
	if err == nil {
		t.Fatal("merging a closed MR must fail")
	}
}

// TestMergeCR_AcceptRaceRecovers pins the race recovery: the accept call
// fails (another writer completed the merge between the pre-flight read
// and the accept), the re-read reports merged — the outcome is the merged
// CR, not an error.
func TestMergeCR_AcceptRaceRecovers(t *testing.T) {
	var mergedState atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge_request/7/merge") {
			mergedState.Store(true) // the merge completes server-side but the accept errors
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"409 Conflict"}`))
			return
		}
		state := "opened"
		if mergedState.Load() {
			state = "merged"
		}
		writeJSON(w, map[string]any{"iid": 7, "state": state, "title": "t"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	cr, err := p.MergeCR(context.Background(), "owner", "repo", "7", provider.MergeCROptions{})
	if err != nil {
		t.Fatalf("accept-failed-but-merged must recover, got %v", err)
	}
	if cr == nil || cr.State != provider.CRStateMerged {
		t.Fatalf("want the merged CR, got %+v", cr)
	}
}

// TestMergeCR_IgnoredOptionsAreRegistered documents the option face (issue
// #94): Squash/RemoveSourceBranch have no fields on the gongfeng accept
// wire, so the drop is registered as DivergenceIgnore — no silent,
// undocumented divergence.
func TestMergeCR_IgnoredOptionsAreRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"version": "1.0"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	found := false
	for _, d := range p.Divergences() {
		if d.Method == "MergeCR" && d.Kind == provider.DivergenceIgnore &&
			strings.Contains(d.Field, "opts.Squash") && strings.Contains(d.Field, "opts.RemoveSourceBranch") {
			found = true
		}
	}
	if !found {
		t.Fatalf("MergeCR squash/remove-source-branch drop must be registered, ledger: %+v", p.Divergences())
	}
}
