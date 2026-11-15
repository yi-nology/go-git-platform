package gitea_test

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

// TestGitea_BranchProtectionWire pins the branch-protection option mapping at
// the real wire level (issue #130): the create/update request bodies must
// carry the platform's REAL keys — enable_force_push for AllowForcePushes
// (gitea.dev/sdk models it) — and must never touch block_on_rejected_reviews
// on AllowDeletions' behalf (the pre-#130 inverse mapping silently flipped
// that unrelated merge-blocking switch). Reads map enable_force_push back.
func TestGitea_BranchProtectionWire(t *testing.T) {
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

	writeBP := func(w http.ResponseWriter, forcePush bool) {
		writeJSON(w, map[string]any{"branch_name": "main", "required_approvals": 2,
			"enable_status_check": true, "enable_push": false, "enable_force_push": forcePush,
			"block_on_rejected_reviews": false})
	}

	mux := http.NewServeMux()
	// create:POST /api/v1/repos/{o}/{r}/branch_protections
	mux.HandleFunc("/api/v1/repos/owner/repo/branch_protections", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		record(r)
		writeBP(w, true)
	})
	// edit/get:.../branch_protections/{branch}
	mux.HandleFunc("/api/v1/repos/owner/repo/branch_protections/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			record(r)
			writeBP(w, false)
		case http.MethodGet:
			// 读侧夹具:enable_force_push=true ⇒ AllowForcePushes 应映射回 true。
			writeJSON(w, map[string]any{"branch_name": "main", "required_approvals": 2,
				"enable_status_check": true, "enable_push": false, "enable_force_push": true,
				"block_on_rejected_reviews": false})
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestProvider(t, srv)
	var bpm provider.BranchProtectionManager = p

	t.Run("create_maps_real_force_push_key", func(t *testing.T) {
		mu.Lock()
		bodies = nil
		mu.Unlock()
		if _, err := bpm.CreateBranchProtection(context.Background(), "owner", "repo",
			provider.CreateBranchProtectionOptions{BranchName: "main", AllowForcePushes: true}); err != nil {
			t.Fatalf("CreateBranchProtection: %v", err)
		}
		mu.Lock()
		body := bodies[0]
		mu.Unlock()
		if v, ok := body["enable_force_push"].(bool); !ok || !v {
			t.Errorf("create body must carry enable_force_push=true (the real gitea wire key), got %v", body)
		}
		if v, ok := body["block_on_rejected_reviews"].(bool); ok && v {
			t.Errorf("create body must not open block_on_rejected_reviews, got %v", body)
		}
		if _, ok := body["allow_deletions"]; ok {
			t.Errorf("create body must not carry a GitHub-shaped allow_deletions key, got %v", body)
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
		// SDK 的 edit 模型对 nil *bool 字段编出显式 null(平台语义=不动);
		// 断言口径只禁布尔 true——force-push 开关不得被这条更新打开。
		if v, ok := body["enable_force_push"].(bool); ok && v {
			t.Errorf("AllowDeletions-only update must not open the force-push toggle, got %v", body)
		}
	})

	t.Run("read_maps_enable_force_push_back", func(t *testing.T) {
		bp, err := bpm.GetBranchProtection(context.Background(), "owner", "repo", "main")
		if err != nil {
			t.Fatalf("GetBranchProtection: %v", err)
		}
		if !bp.AllowForcePushes {
			t.Errorf("read must map the wire's enable_force_push=true back to AllowForcePushes=true, got %+v", bp)
		}
		if bp.AllowDeletions {
			t.Errorf("read must report AllowDeletions=false (no wire face, issue #130), got true")
		}
		if bp.RequiredApprovingReviews != 2 || bp.RequiredStatusChecks != true {
			t.Errorf("read conversion drifted: %+v", bp)
		}
	})
}
