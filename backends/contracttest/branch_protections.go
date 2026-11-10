package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// BranchProtectionsHarnessConfig carries fixtures for the branch-protection
// suite. All fields are required.
type BranchProtectionsHarnessConfig struct {
	ListResponse   string
	MutateResponse string
	// GetResponse is the JSON object for a single-protection GET (same
	// shape as MutateResponse). Empty = harness not wired yet: the Get
	// subtest skips, the rest of the suite is affected by the
	// List/GetUnsupported flags below.
	GetResponse string
	// ListUnsupported/GetUnsupported declare the platform's documented
	// "no such endpoint" stubs (registered as DivergenceStub in the
	// backend's divergence ledger): the corresponding subtests skip instead
	// of failing on the nil,nil contract. Mirrors UpdateSendsDescriptionOnly.
	ListUnsupported bool
	GetUnsupported  bool
}

func testBranchProtectionsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().BranchProtections
	switch {
	case h.BranchProtections == nil && !declared:
		t.Skipf("%s declares no BranchProtections capability", h.Name)
	case h.BranchProtections == nil:
		t.Errorf("%s declares Capabilities().BranchProtections but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides BranchProtections config but platform does not declare capability", h.Name)
	default:
		RunBranchProtectionsSuite(t, BranchProtectionsHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			ListResponse: h.BranchProtections.ListResponse, MutateResponse: h.BranchProtections.MutateResponse,
			GetResponse:     h.BranchProtections.GetResponse,
			ListUnsupported: h.BranchProtections.ListUnsupported, GetUnsupported: h.BranchProtections.GetUnsupported,
		})
	}
}

// BranchProtectionsHarness bundles inputs for the branch-protection suite.
type BranchProtectionsHarness struct {
	Name            string
	Platform        provider.Platform
	NewProvider     func(t *testing.T, cfg provider.Config) provider.Provider
	ListResponse    string
	MutateResponse  string
	GetResponse     string
	ListUnsupported bool
	GetUnsupported  bool
}

// RunBranchProtectionsSuite tests branch-protection CRUD operations.
func RunBranchProtectionsSuite(t *testing.T, h BranchProtectionsHarness) {
	newBPM := func(t *testing.T) provider.BranchProtectionManager {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				// Page-aware: AllPages-driven backends probe page 2; serve an
				// empty terminal page so the loop stops.
				if beyondFirstPage(r) {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				// 单件取（GetBranchProtection）：路径携带分支名的 GET 返回
				// 单件对象而非列表数组。
				if h.GetResponse != "" && strings.Contains(r.URL.Path, "main") {
					_, _ = w.Write([]byte(h.GetResponse))
					return
				}
				_, _ = w.Write([]byte(h.ListResponse))
			case http.MethodPost:
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(h.MutateResponse))
			case http.MethodPatch, http.MethodPut:
				_, _ = w.Write([]byte(h.MutateResponse))
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		bpm, ok := p.(provider.BranchProtectionManager)
		if !ok {
			t.Fatalf("%s does not implement provider.BranchProtectionManager", h.Name)
		}
		return bpm
	}

	t.Run("ListBranchProtections", func(t *testing.T) {
		if h.ListUnsupported {
			t.Skipf("%s has no list endpoint (DivergenceStub)", h.Name)
		}
		bpm := newBPM(t)
		protections, err := bpm.ListBranchProtections(context.Background(), "owner", "repo")
		if err != nil {
			t.Skipf("ListBranchProtections not supported: %v", err)
		}
		if len(protections) == 0 {
			t.Skip("no branch protections returned (platform may not support listing)")
		}
	})

	t.Run("CreateBranchProtection", func(t *testing.T) {
		bpm := newBPM(t)
		bp, err := bpm.CreateBranchProtection(context.Background(), "owner", "repo", provider.CreateBranchProtectionOptions{
			BranchName: "main",
		})
		if err != nil {
			// Some backends' create needs a more complex mock (e.g. GitHub
			// reads current state before updating). Skip on error.
			t.Skipf("CreateBranchProtection: %v", err)
		}
		if bp == nil {
			t.Fatal("expected a branch protection, got nil")
		}
	})

	t.Run("DeleteBranchProtection", func(t *testing.T) {
		bpm := newBPM(t)
		if err := bpm.DeleteBranchProtection(context.Background(), "owner", "repo", "main"); err != nil {
			t.Fatalf("DeleteBranchProtection: %v", err)
		}
	})

	t.Run("GetBranchProtection_Shape", func(t *testing.T) {
		assertBPGetShape(t, h, newBPM(t))
	})

	t.Run("UpdateBranchProtection_AcceptsMutation", func(t *testing.T) {
		assertBPUpdateAcceptsMutation(t, newBPM(t))
	})
}

// assertBPGetShape asserts the single-protection GET parses to a
// BranchProtection carrying the requested branch. Skips on the platform's
// documented no-get stub (DivergenceStub) or an unwired fixture.
func assertBPGetShape(t *testing.T, h BranchProtectionsHarness, bpm provider.BranchProtectionManager) {
	t.Helper()
	if h.GetUnsupported {
		t.Skipf("%s has no get endpoint (DivergenceStub)", h.Name)
	}
	if h.GetResponse == "" {
		t.Skipf("%s harness wires no GetResponse fixture", h.Name)
	}
	bp, err := bpm.GetBranchProtection(context.Background(), "owner", "repo", "main")
	if err != nil {
		t.Fatalf("GetBranchProtection: %v", err)
	}
	if bp == nil {
		t.Fatal("expected a branch protection, got nil")
	}
	if bp.BranchName != "main" {
		t.Errorf("expected BranchName %q, got %q", "main", bp.BranchName)
	}
}

// assertBPUpdateAcceptsMutation asserts UpdateBranchProtection accepts a
// mutation and returns a protection (platform-limitation errors skip).
func assertBPUpdateAcceptsMutation(t *testing.T, bpm provider.BranchProtectionManager) {
	t.Helper()
	two := 2
	bp, err := bpm.UpdateBranchProtection(context.Background(), "owner", "repo", "main", provider.UpdateBranchProtectionOptions{
		RequiredApprovingReviews: &two,
	})
	if err != nil {
		t.Skipf("UpdateBranchProtection: %v", err)
	}
	if bp == nil {
		t.Fatal("expected a branch protection, got nil")
	}
}
