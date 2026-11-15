package contracttest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TeamReviewsHarnessConfig carries fixtures for the team-review suite.
type TeamReviewsHarnessConfig struct {
	// Teams is the team-slug list the suite requests on the legal path; the
	// wire body must carry every value. Key spelling differs per platform
	// (site-verified 2C T4: go-github ReviewersRequest.TeamReviewers is
	// `team_reviewers` with omitempty; gitea.dev/sdk and forgejo-sdk
	// PullReviewRequestOptions always send BOTH `reviewers` and
	// `team_reviewers`), so the suite's body assertion is a
	// platform-agnostic value check — some JSON array in the body must carry
	// every fixture team as an element — never a key-name pin.
	Teams []string
}

func testTeamReviewsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().TeamReviews
	switch {
	case h.TeamReviews == nil && !declared:
		t.Skipf("%s declares no TeamReviews capability", h.Name)
	case h.TeamReviews == nil:
		t.Errorf("%s declares Capabilities().TeamReviews but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides TeamReviews config but platform does not declare capability", h.Name)
	default:
		RunTeamReviewsSuite(t, TeamReviewsHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			Teams: h.TeamReviews.Teams,
		})
	}
}

// TeamReviewsHarness bundles inputs for the team-review suite.
type TeamReviewsHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	Teams       []string
}

// teamReviewsWire records the request line the team-review subtests assert
// on: method, path (the requested_reviewers route), and the decoded body.
type teamReviewsWire struct {
	mu     sync.Mutex
	method string
	path   string
	body   map[string]any
	n      int
}

func (w *teamReviewsWire) record(r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.method = r.Method
	w.path = r.URL.Path
	w.body = decodeRecordedBody(r)
	w.n++
}

func (w *teamReviewsWire) last() (method, path string, body map[string]any, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.method, w.path, w.body, w.n
}

// RunTeamReviewsSuite tests the team-review-manager wire contract (issue
// #79): a legal request posts every team slug to the requested_reviewers
// route (value assertion, platform-agnostic across the three declaring
// backends), and an invalid PR number is rejected pre-wire with zero
// requests.
func RunTeamReviewsSuite(t *testing.T, h TeamReviewsHarness) {
	t.Run("RequestTeamReviewers_PostsAllTeams", func(t *testing.T) { runRequestTeamReviewersCase(t, h) })
	t.Run("RequestTeamReviewers_InvalidNumber_PreWire", func(t *testing.T) { runRequestTeamReviewersInvalidNumberCase(t, h) })
}

// newTeamReviewManager builds a provider against a mock that records every
// request and answers with resp, then type-asserts the team-review
// interface.
func newTeamReviewManager(t *testing.T, h TeamReviewsHarness, resp string, rec *teamReviewsWire) provider.TeamReviewManager {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	tm, ok := p.(provider.TeamReviewManager)
	if !ok {
		t.Fatalf("%s does not implement provider.TeamReviewManager", h.Name)
	}
	return tm
}

// runRequestTeamReviewersCase pins the legal write: exactly one POST on the
// requested_reviewers route whose body carries every fixture team slug in a
// JSON array.
func runRequestTeamReviewersCase(t *testing.T, h TeamReviewsHarness) {
	t.Helper()
	if len(h.Teams) == 0 {
		t.Skipf("%s harness wires no TeamReviews Teams fixture", h.Name)
	}
	rec := &teamReviewsWire{}
	tm := newTeamReviewManager(t, h, `{}`, rec)
	if err := tm.RequestTeamReviewers(context.Background(), "owner", "repo", "1", h.Teams); err != nil {
		t.Fatalf("RequestTeamReviewers: %v", err)
	}
	method, path, body, n := rec.last()
	if n != 1 {
		t.Errorf("expected exactly 1 request, got %d", n)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	// Site-verified (2C T4): go-github v93 pulls_reviewers.go POSTs
	// repos/{o}/{r}/pulls/{n}/requested_reviewers (no /api prefix in the
	// SDK path — the contract harness's /api/v3 base rides in front);
	// gitea.dev/sdk v1.5.1 pull_review.go and forgejo-sdk v3 pull_review.go
	// POST the same suffix under their clients' /api/v1 prefix. The suffix
	// assertion holds for all three.
	if !strings.HasSuffix(path, "/repos/owner/repo/pulls/1/requested_reviewers") {
		t.Errorf("path = %s, want .../repos/owner/repo/pulls/1/requested_reviewers", path)
	}
	if body == nil {
		t.Fatal("RequestTeamReviewers sent no decodable JSON body")
	}
	if !bodyCarriesAllTeams(body, h.Teams) {
		t.Errorf("wire body %v must carry all team values %v in a JSON array (platform-agnostic value assertion — key spellings differ per platform)",
			body, h.Teams)
	}
}

// runRequestTeamReviewersInvalidNumberCase pins the pre-wire number guard:
// a non-numeric PR number must fail closed BEFORE any HTTP, at the unified
// ProviderError shape with the ErrInvalidInput sentinel in the chain.
//
// #165 fixed the sentinel gap found by the 2C T4 probe: ParsePRNumber /
// ParsePRNumber64 used to reject via provider.Wrapf without any
// classification sentinel — the surfaced *provider.ProviderError had
// StatusCode 0 and errors.Is(err, provider.ErrInvalidInput) was FALSE,
// so this pin (and every CR/PR negative case) could only hold the
// structural face. Parse rejections now wrap ErrInvalidInput (matching the
// 422 write-side rejections pinned by assertPreWireInvalidInput); the pin
// asserts both faces: sentinel matchability + structured shape (unified
// error at the right Op, status 0) + the load-bearing zero-requests
// contract. 规则二 still forbids matching error message text.
func runRequestTeamReviewersInvalidNumberCase(t *testing.T, h TeamReviewsHarness) {
	t.Helper()
	rec := &teamReviewsWire{}
	tm := newTeamReviewManager(t, h, `{}`, rec)
	err := tm.RequestTeamReviewers(context.Background(), "owner", "repo", "not-a-number", h.Teams)
	_, _, _, n := rec.last()
	if err == nil {
		t.Fatal("RequestTeamReviewers(not-a-number): expected a pre-wire rejection, got nil")
	}
	if !errors.Is(err, provider.ErrInvalidInput) {
		t.Errorf("errors.Is(err, provider.ErrInvalidInput) = false (#165: parse rejection must carry the sentinel), got %v", err)
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Errorf("err type %T, want *provider.ProviderError (unified shape)", err)
	} else {
		if pe.Platform != h.Platform {
			t.Errorf("ProviderError.Platform = %s, want %s", pe.Platform, h.Platform)
		}
		if pe.Op != "RequestTeamReviewers" {
			t.Errorf("ProviderError.Op = %s, want RequestTeamReviewers", pe.Op)
		}
		if pe.StatusCode != 0 {
			t.Errorf("ProviderError.StatusCode = %d, want 0 (pre-wire: never HTTP-classified)", pe.StatusCode)
		}
	}
	if n != 0 {
		t.Errorf("invalid number must be rejected before the wire, recorded %d requests", n)
	}
}

// bodyCarriesAllTeams reports whether some JSON array inside the decoded
// body carries every team slug as a string element. Walking for the array
// (rather than pinning a key name) keeps the assertion platform-agnostic
// across the three declaring backends' wire spellings.
func bodyCarriesAllTeams(v any, teams []string) bool {
	switch x := v.(type) {
	case []any:
		if allTeamsInArray(x, teams) {
			return true
		}
		for _, e := range x {
			if bodyCarriesAllTeams(e, teams) {
				return true
			}
		}
	case map[string]any:
		for _, e := range x {
			if bodyCarriesAllTeams(e, teams) {
				return true
			}
		}
	}
	return false
}

// allTeamsInArray reports whether arr contains every team slug as a direct
// string element.
func allTeamsInArray(arr []any, teams []string) bool {
	for _, team := range teams {
		found := false
		for _, e := range arr {
			if s, ok := e.(string); ok && s == team {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
