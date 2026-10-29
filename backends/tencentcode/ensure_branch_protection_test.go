package tencentcode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestTencentCode_EnsureBranchProtection verifies that the cross-platform
// expect-state helper provider.EnsureBranchProtection converges over the
// tencentcode backend now that it implements BranchProtectionManager:
// a missing rule is Created, a matching rule is left Unchanged, and a
// diverging force-push flag is Updated.
//
// Known-shape caveat (registered): RequiredApprovingReviews maps from the
// platform's approver_rule and is not writable via the protect PUT, so a
// desired state carrying a nonzero reviews count never reads back equal —
// 工蜂 callers should express desired state with the platform-reported
// rule (or 0) to reach EnsureUnchanged.
func TestTencentCode_EnsureBranchProtection(t *testing.T) {
	var mu sync.Mutex
	protected := true
	rule := map[string]any{"protected": true, "name": "main", "approver_rule": 1}
	forcePushOnWire := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/protect"):
			var body map[string]any
			_ = readRequestBody(r, &body)
			if v, ok := body["allow_force_push"].(bool); ok {
				forcePushOnWire = v
			}
			writeJSON(w, rule)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/protect"):
			if !protected {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
				return
			}
			writeJSON(w, rule)
		default:
			writeJSON(w, map[string]any{})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)

	desired := provider.CreateBranchProtectionOptions{
		BranchName:               "main",
		RequiredApprovingReviews: 1, // must mirror the platform's approver_rule to converge
	}

	// 1. Unprotected branch → the helper Creates.
	protected = false
	action, _, err := provider.EnsureBranchProtection(context.Background(), p, "owner", "repo", desired)
	if err != nil || action != provider.EnsureCreated {
		t.Fatalf("want EnsureCreated, got %q err=%v", action, err)
	}

	// 2. Protected with the same rule → Unchanged (no PUT fired).
	protected = true
	action, got, err := provider.EnsureBranchProtection(context.Background(), p, "owner", "repo", desired)
	if err != nil || action != provider.EnsureUnchanged {
		t.Fatalf("want EnsureUnchanged, got %q err=%v (rule %+v)", action, err, got)
	}

	// 3. Force-push desired true (platform reports false) → Updated, and
	// the PUT carries allow_force_push=true.
	action, _, err = provider.EnsureBranchProtection(context.Background(), p, "owner", "repo",
		provider.CreateBranchProtectionOptions{BranchName: "main", RequiredApprovingReviews: 1, AllowForcePushes: true})
	if err != nil || action != provider.EnsureUpdated {
		t.Fatalf("want EnsureUpdated, got %q err=%v", action, err)
	}
	if !forcePushOnWire {
		t.Errorf("update must PUT allow_force_push=true")
	}
}
