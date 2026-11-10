package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// testDivergenceSuite locks each backend's divergence ledger to its
// behavior. For every method-scoped stub or ignore entry the dispatch table
// invokes the method against a recording server: a stub must fail with
// provider.ErrNotImplemented and stay off the wire; an ignore must succeed
// and stay off the wire. Every entry's method must exist on the concrete
// provider, and method-scoped stub/ignore entries must have a dispatch case
// (the suite fails on unknown pairs so the table cannot rot).
func testDivergenceSuite(t *testing.T, h Harness) {
	var mu sync.Mutex
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.EmptyListResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))

	rv := reflect.ValueOf(p)
	for _, d := range p.Divergences() {
		if m := rv.MethodByName(d.Method); !m.IsValid() {
			t.Errorf("%s: ledger entry (Capability %q) references method %q that does not exist on the provider", h.Name, d.Capability, d.Method)
			continue
		}
		if d.Kind != provider.DivergenceStub && d.Kind != provider.DivergenceIgnore {
			continue // behavioral checks apply to stub/ignore only
		}
		if d.Field != "" {
			continue // field-scoped ignores are documentation; the call itself works
		}
		mu.Lock()
		before := len(requests)
		mu.Unlock()
		invoked, err := dispatchDivergenceCall(p, d.Capability, d.Method)
		if !invoked {
			t.Errorf("%s: no dispatch case for %s.%s — extend dispatchDivergenceCall in contracttest/divergence.go", h.Name, d.Capability, d.Method)
			continue
		}
		mu.Lock()
		after := len(requests)
		mu.Unlock()
		switch d.Kind {
		case provider.DivergenceStub:
			if err == nil {
				t.Errorf("%s: registered stub %s.%s returned nil error, want ErrNotImplemented", h.Name, d.Capability, d.Method)
			} else if !provider.IsNotImplemented(err) {
				t.Errorf("%s: registered stub %s.%s = %v, want an error wrapping ErrNotImplemented", h.Name, d.Capability, d.Method, err)
			}
		case provider.DivergenceIgnore:
			if err != nil {
				t.Errorf("%s: registered ignore %s.%s = %v, want nil (succeeds without effect)", h.Name, d.Capability, d.Method, err)
			}
		}
		if after != before {
			t.Errorf("%s: %s.%s (%s) touched the wire (%d requests), want zero", h.Name, d.Capability, d.Method, d.Kind, after-before)
		}
	}
}

// dispatchDivergenceCall invokes a ledger-declared stub/ignore with dummy
// arguments. The table is closed on purpose: adding a new method-scoped
// stub or ignore without a case here fails the suite with instructions.
func dispatchDivergenceCall(p provider.Provider, capability, method string) (bool, error) {
	ctx := context.Background()
	switch capability + "." + method {
	case "ReviewManager.RequestReviewers":
		rm, ok := p.(provider.ReviewManager)
		if !ok {
			return true, nil
		}
		return true, rm.RequestReviewers(ctx, "owner", "repo", "1", []string{"dev"})
	case "ReviewManager.DismissReview":
		rm, ok := p.(provider.ReviewManager)
		if !ok {
			return true, nil
		}
		return true, rm.DismissReview(ctx, "owner", "repo", "1", 1, "stale review")
	case "ChangeRequestManager.UpdateCRLabels":
		return true, p.UpdateCRLabels(ctx, "owner", "repo", "1", []string{"bug"})
	case "BranchProtectionManager.ListBranchProtections":
		bpm, ok := p.(provider.BranchProtectionManager)
		if !ok {
			return true, nil
		}
		_, err := bpm.ListBranchProtections(ctx, "owner", "repo")
		return true, err
	case "BranchProtectionManager.GetBranchProtection":
		bpm, ok := p.(provider.BranchProtectionManager)
		if !ok {
			return true, nil
		}
		_, err := bpm.GetBranchProtection(ctx, "owner", "repo", "main")
		return true, err
	case "CommitStatusManager.CreateCommitStatus":
		csm, ok := p.(provider.CommitStatusManager)
		if !ok {
			return true, nil
		}
		return true, csm.CreateCommitStatus(ctx, "owner", "repo", "deadbeef", provider.CommitStatusOptions{State: "pending"})
	case "ReleaseManager.GetArchive":
		rm, ok := p.(provider.ReleaseManager)
		if !ok {
			return true, nil
		}
		_, err := rm.GetArchive(ctx, "owner", "repo", "v1.0.0", "zip")
		return true, err
	case "NotificationManager.ListNotifications",
		"NotificationManager.MarkNotificationRead",
		"NotificationManager.MarkNotificationsRead":
		return dispatchNotificationDivergenceCall(p, capability, method, ctx)
	default:
		return false, nil
	}
}

// dispatchNotificationDivergenceCall serves the NotificationManager stub
// cases; split out to keep dispatchDivergenceCall's branch budget flat.
func dispatchNotificationDivergenceCall(p provider.Provider, capability, method string, ctx context.Context) (bool, error) {
	nm, ok := p.(provider.NotificationManager)
	if !ok {
		return true, nil
	}
	switch capability + "." + method {
	case "NotificationManager.ListNotifications":
		_, err := nm.ListNotifications(ctx, provider.ListNotificationsOptions{})
		return true, err
	case "NotificationManager.MarkNotificationRead":
		return true, nm.MarkNotificationRead(ctx, "1")
	case "NotificationManager.MarkNotificationsRead":
		return true, nm.MarkNotificationsRead(ctx, provider.MarkNotificationsOptions{})
	}
	return false, nil
}
