package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// RepoStatsHarnessConfig carries fixtures for the repo-stats suite.
type RepoStatsHarnessConfig struct {
	ForksResponse        string
	StargazersResponse   string
	ContributorsResponse string
}

func testRepoStatsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().RepoStats
	switch {
	case h.RepoStats == nil && !declared:
		t.Skipf("%s declares no RepoStats capability", h.Name)
	case h.RepoStats == nil:
		t.Errorf("%s declares Capabilities().RepoStats but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides RepoStats config but platform does not declare capability", h.Name)
	default:
		RunRepoStatsSuite(t, RepoStatsHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			ForksResponse: h.RepoStats.ForksResponse, StargazersResponse: h.RepoStats.StargazersResponse,
			ContributorsResponse: h.RepoStats.ContributorsResponse,
		})
	}
}

// RepoStatsHarness bundles inputs for the repo-stats suite.
type RepoStatsHarness struct {
	Name                 string
	Platform             provider.Platform
	NewProvider          func(t *testing.T, cfg provider.Config) provider.Provider
	ForksResponse        string
	StargazersResponse   string
	ContributorsResponse string
}

// RunRepoStatsSuite tests repo-stats read operations, one subtest per
// method with its own fixture. The outcome contract (issue #56) is exact,
// not skip-shaped:
//
//   - implemented: the fixture must decode into a non-empty result;
//   - stub: the error must wrap provider.ErrNotImplemented AND be locked by
//     a matching DivergenceStub ledger entry (the pairing direction the
//     DivergenceSuite cannot see — it only walks the ledger forward);
//   - anything else: a failure, not a skip (the old skipf swallowed every
//     error, so the suite was green while asserting nothing).
func RunRepoStatsSuite(t *testing.T, h RepoStatsHarness) {
	var serve atomic.Value // string: the body the next first-page read gets
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Page-aware: AllPages-driven backends probe page 2; serve an
		// empty terminal page so the loop stops.
		if r.Method == http.MethodGet && beyondFirstPage(r) {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		body, _ := serve.Load().(string)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	rsm, ok := p.(provider.RepoStatsManager)
	if !ok {
		t.Fatalf("%s does not implement provider.RepoStatsManager", h.Name)
	}
	hasStubEntry := func(method string) bool {
		for _, d := range p.Divergences() {
			if d.Capability == "RepoStatsManager" && d.Method == method && d.Kind == provider.DivergenceStub {
				return true
			}
		}
		return false
	}
	expect := func(t *testing.T, method, fixture string, call func() (int, error)) {
		t.Helper()
		serve.Store(fixture)
		n, err := call()
		switch {
		case err == nil:
			if n == 0 {
				t.Errorf("%s: implemented method decoded zero rows from its fixture", method)
			}
		case provider.IsNotImplemented(err):
			if !hasStubEntry(method) {
				t.Errorf("%s: returns ErrNotImplemented but no RepoStatsManager.%s DivergenceStub entry is registered in the ledger — register it so the stub is machine-readable", method, method)
			}
		default:
			t.Errorf("%s: unexpected error %v (want nil result or ErrNotImplemented)", method, err)
		}
	}

	t.Run("ListForks", func(t *testing.T) {
		expect(t, "ListForks", h.ForksResponse, func() (int, error) {
			forks, err := rsm.ListForks(context.Background(), "owner", "repo")
			return len(forks), err
		})
	})

	t.Run("ListStargazers", func(t *testing.T) {
		expect(t, "ListStargazers", h.StargazersResponse, func() (int, error) {
			stars, err := rsm.ListStargazers(context.Background(), "owner", "repo")
			return len(stars), err
		})
	})

	t.Run("ListContributors", func(t *testing.T) {
		expect(t, "ListContributors", h.ContributorsResponse, func() (int, error) {
			contributors, err := rsm.ListContributors(context.Background(), "owner", "repo")
			return len(contributors), err
		})
	})
}
