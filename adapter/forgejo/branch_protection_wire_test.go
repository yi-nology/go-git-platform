package forgejo_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestForgejo_BranchProtectionWire pins the branch-protection option mapping
// at the real wire level (issue #130): the forgejo SDK's protection model
// (mvdkleijn v3) has no force-push field, so AllowForcePushes/AllowDeletions
// are dropped — the request bodies must never carry a force-push or
// block_on_rejected_reviews write (the pre-#130 mapping rode enable_push /
// inverted block_on_rejected_reviews, silently flipping unrelated switches).
func TestForgejo_BranchProtectionWire(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	record := func(r *http.Request) {
		var raw []byte
		if r.Body != nil {
			raw, _ = io.ReadAll(r.Body)
		}
		m := map[string]any{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &m)
		}
		mu.Lock()
		bodies = append(bodies, m)
		mu.Unlock()
	}

	mux := http.NewServeMux()
	// create:POST /api/v1/repos/{o}/{r}/branch_protections
	mux.HandleFunc("/api/v1/repos/owner/repo/branch_protections", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		record(r)
		writeJSON(w, map[string]any{"branch_name": "main", "required_approvals": 1,
			"enable_status_check": true, "enable_push": false, "block_on_rejected_reviews": false})
	})
	// edit:PATCH .../branch_protections/{branch}
	mux.HandleFunc("/api/v1/repos/owner/repo/branch_protections/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		record(r)
		writeJSON(w, map[string]any{"branch_name": "main", "required_approvals": 1,
			"enable_status_check": true, "enable_push": false, "block_on_rejected_reviews": false})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestProvider(t, srv)
	var bpm provider.BranchProtectionManager = p

	t.Run("create_drops_unsupported_switches", func(t *testing.T) {
		mu.Lock()
		bodies = nil
		mu.Unlock()
		if _, err := bpm.CreateBranchProtection(context.Background(), "owner", "repo",
			provider.CreateBranchProtectionOptions{BranchName: "main", AllowForcePushes: true, AllowDeletions: true}); err != nil {
			t.Fatalf("CreateBranchProtection: %v", err)
		}
		mu.Lock()
		body := bodies[0]
		mu.Unlock()
		for _, forbidden := range []string{"enable_force_push", "allow_force_pushes", "allow_deletions"} {
			if _, ok := body[forbidden]; ok {
				t.Errorf("create body must not carry %q (forgejo wire has no such face; issue #130), got %v", forbidden, body)
			}
		}
		if v, ok := body["block_on_rejected_reviews"].(bool); ok && v {
			t.Errorf("create body must not open block_on_rejected_reviews, got %v", body)
		}
	})

	t.Run("update_never_writes_block_on_rejected_reviews", func(t *testing.T) {
		mu.Lock()
		bodies = nil
		mu.Unlock()
		off := false
		if _, err := bpm.UpdateBranchProtection(context.Background(), "owner", "repo", "main",
			provider.UpdateBranchProtectionOptions{AllowDeletions: &off}); err != nil {
			t.Fatalf("UpdateBranchProtection: %v", err)
		}
		mu.Lock()
		body := bodies[0]
		mu.Unlock()
		if v, ok := body["block_on_rejected_reviews"].(bool); ok && v {
			t.Errorf("AllowDeletions update must not flip block_on_rejected_reviews (the #130 defect), got %v", body)
		}
		if v, ok := body["enable_push"].(bool); ok && v {
			t.Errorf("AllowForcePushes must not ride enable_push (unrelated switch; issue #130), got %v", body)
		}
	})
}
