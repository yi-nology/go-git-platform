package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// ReactionsHarnessConfig carries the fixtures a backend's main Harness needs
// to auto-mount the reaction suite via Harness.Reactions.
type ReactionsHarnessConfig struct {
	// ListResponse is the JSON array the mock returns for reaction list
	// requests. It must contain at least one reaction with content "+1".
	ListResponse string
	// CreateResponse is the JSON object the mock returns for reaction create
	// requests.
	CreateResponse string
}

// testReactionsSuite auto-mounts RunReactionsSuite from a main Harness,
// enforcing bidirectional drift checks.
func testReactionsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Reactions
	switch {
	case h.Reactions == nil && !declared:
		t.Skipf("%s declares no Reactions capability", h.Name)
	case h.Reactions == nil:
		t.Errorf("%s declares Capabilities().Reactions but its Harness provides no Reactions config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides a Reactions config but the platform does not declare Capabilities().Reactions", h.Name)
	default:
		RunReactionsSuite(t, ReactionsHarness{
			Name:           h.Name,
			Platform:       h.Platform,
			NewProvider:    h.NewProvider,
			ListResponse:   h.Reactions.ListResponse,
			CreateResponse: h.Reactions.CreateResponse,
		})
	}
}

// ReactionsHarness bundles the inputs needed to run the reaction contract
// suite against a backend that implements provider.ReactionManager.
type ReactionsHarness struct {
	Name           string
	Platform       provider.Platform
	NewProvider    func(t *testing.T, cfg provider.Config) provider.Provider
	ListResponse   string
	CreateResponse string
}

// RunReactionsSuite executes the reaction contract suite.
func RunReactionsSuite(t *testing.T, h ReactionsHarness) {
	newRM := func(t *testing.T) provider.ReactionManager {
		srv := reactionStubServer(h.ListResponse, h.CreateResponse)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		rm, ok := p.(provider.ReactionManager)
		if !ok {
			t.Fatalf("%s does not implement provider.ReactionManager", h.Name)
		}
		return rm
	}

	t.Run("ListIssueReactions_ReturnsResults", func(t *testing.T) {
		rm := newRM(t)
		reactions, err := rm.ListIssueReactions(context.Background(), "owner", "repo", "1")
		if err != nil {
			t.Fatalf("ListIssueReactions: %v", err)
		}
		if len(reactions) == 0 {
			t.Fatal("expected at least one reaction")
		}
		if reactions[0].Emoji == "" {
			t.Error("expected reaction emoji to be non-empty")
		}
	})

	t.Run("AddIssueReaction_ReturnsReaction", func(t *testing.T) {
		rm := newRM(t)
		r, err := rm.AddIssueReaction(context.Background(), "owner", "repo", "1", provider.ReactionHeart)
		if err != nil {
			t.Fatalf("AddIssueReaction: %v", err)
		}
		if r == nil {
			t.Fatal("expected a reaction, got nil")
		}
		if r.Emoji != provider.ReactionHeart {
			t.Errorf("expected emoji %q, got %q", provider.ReactionHeart, r.Emoji)
		}
	})

	t.Run("RemoveIssueReaction_Succeeds", func(t *testing.T) {
		rm := newRM(t)
		if err := rm.RemoveIssueReaction(context.Background(), "owner", "repo", "1", 1); err != nil {
			t.Fatalf("RemoveIssueReaction: %v", err)
		}
	})

	t.Run("CommentAndCRLevel_OneShapePerMethod", func(t *testing.T) {
		assertReactionFamilyShapes(t, h, newFamilyShapeProvider(t, h))
	})
}

// newFamilyShapeProvider builds one provider against a fresh reaction stub
// for the family-shape subtest.
func newFamilyShapeProvider(t *testing.T, h ReactionsHarness) provider.Provider {
	t.Helper()
	srv := reactionStubServer(h.ListResponse, h.CreateResponse)
	t.Cleanup(srv.Close)
	return h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
}

// assertReactionFamilyShapes pins issue #57: the remaining seven reaction
// methods must each carry exactly one shape — implemented (real data off the
// stub) or a registered ErrNotImplemented stub. The pre-fix mixed forms are
// pinned out: (nil, nil) silent-empty reads are indistinguishable from "no
// reactions", and sentinel-less text errors leave both IsNotImplemented and
// IsNotFound false at once. The ledger pairing is machine-checked: an
// ErrNotImplemented verdict must have a DivergenceStub ledger entry, and a
// working method must not.
func assertReactionFamilyShapes(t *testing.T, h ReactionsHarness, p provider.Provider) {
	t.Helper()
	rm, ok := p.(provider.ReactionManager)
	if !ok {
		t.Fatalf("%s does not implement provider.ReactionManager", h.Name)
	}
	stubs := map[string]bool{}
	for _, d := range p.Divergences() {
		if d.Kind == provider.DivergenceStub {
			stubs[d.Method] = true
		}
	}
	ctx := context.Background()
	rl, err := rm.ListIssueCommentReactions(ctx, "owner", "repo", 1)
	checkReactionRead(t, stubs, "ListIssueCommentReactions", rl, err)
	r, err := rm.AddIssueCommentReaction(ctx, "owner", "repo", 1, provider.ReactionHeart)
	if err == nil && r == nil {
		t.Error("AddIssueCommentReaction returned (nil, nil)")
	}
	checkReactionWrite(t, stubs, "AddIssueCommentReaction", err)
	checkReactionWrite(t, stubs, "RemoveIssueCommentReaction", rm.RemoveIssueCommentReaction(ctx, "owner", "repo", 1, 1))
	crl, err := rm.ListCRReactions(ctx, "owner", "repo", "1")
	checkReactionRead(t, stubs, "ListCRReactions", crl, err)
	ccl, err := rm.ListCRCommentReactions(ctx, "owner", "repo", 1)
	checkReactionRead(t, stubs, "ListCRCommentReactions", ccl, err)
	r2, err := rm.AddCRCommentReaction(ctx, "owner", "repo", 1, provider.ReactionHeart)
	if err == nil && r2 == nil {
		t.Error("AddCRCommentReaction returned (nil, nil)")
	}
	checkReactionWrite(t, stubs, "AddCRCommentReaction", err)
	checkReactionWrite(t, stubs, "RemoveCRCommentReaction", rm.RemoveCRCommentReaction(ctx, "owner", "repo", 1, 1))
}

// checkReactionRead asserts one reaction READ method's shape: implemented
// (non-nil results) or a registered ErrNotImplemented stub — the (nil, nil)
// silent-empty and sentinel-less text-error forms of issue #57 are pinned
// out, and the stub verdict must be paired with a DivergenceStub entry.
func checkReactionRead(t *testing.T, stubs map[string]bool, name string, results []*provider.Reaction, err error) {
	t.Helper()
	switch {
	case err == nil:
		if results == nil {
			t.Errorf("%s returned (nil, nil) — silent-empty cannot be told apart from \"no reactions\" (issue #57)", name)
		}
		if stubs[name] {
			t.Errorf("%s returned results but the ledger registers it as a stub — fix the ledger or the method", name)
		}
	case provider.IsNotImplemented(err):
		if !stubs[name] {
			t.Errorf("%s fails with ErrNotImplemented but no DivergenceStub ledger entry registers it (issue #57)", name)
		}
	default:
		t.Errorf("%s err = %v, want implemented or a registered ErrNotImplemented stub — sentinel-less text errors leave both escape hatches false (issue #57)", name, err)
	}
}

// checkReactionWrite asserts one reaction WRITE method's shape, same pairing
// rules as checkReactionRead.
func checkReactionWrite(t *testing.T, stubs map[string]bool, name string, err error) {
	t.Helper()
	switch {
	case err == nil:
		if stubs[name] {
			t.Errorf("%s succeeded but the ledger registers it as a stub — fix the ledger or the method", name)
		}
	case provider.IsNotImplemented(err):
		if !stubs[name] {
			t.Errorf("%s fails with ErrNotImplemented but no DivergenceStub ledger entry registers it (issue #57)", name)
		}
	default:
		t.Errorf("%s err = %v, want implemented or a registered ErrNotImplemented stub (issue #57)", name, err)
	}
}

// reactionStubServer returns a mock server for the reaction suite.
func reactionStubServer(listResponse, createResponse string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			// Page-aware: AllPages-driven backends probe page 2; serve an
			// empty terminal page so the loop stops.
			if beyondFirstPage(r) {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(listResponse))
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(createResponse))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}
