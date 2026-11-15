package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// SearchHarnessConfig carries the fixtures a backend's main Harness needs to
// auto-mount the search suite via Harness.Search. Every fixture's first item
// must render as: repos — full_name "owner/repo"; issues — number "1" (the
// platform's wire encoding of the identifier, parsed to the string "1"),
// title "found"; users — login "dev".
type SearchHarnessConfig struct {
	// ReposResponse is the JSON body for repository-search GETs, in the
	// platform's wire shape (GitHub wraps items in {"total_count":..,
	// "items":[..]}, Gitea/Forgejo in {"data":[..]}, the rest are bare
	// arrays).
	ReposResponse string
	// ReposTotalCount is the server-side total the repos fixture reports
	// (GitHub's envelope carries total_count; the suite then asserts
	// SearchRepos returns exactly this total). Zero means the platform
	// reports no server-side total — the backend returns nil for total and
	// the suite keeps the weaker total == nil || *total >= len(results)
	// assertion.
	ReposTotalCount int
	// IssuesResponse is the JSON body for issue-search GETs, same per-
	// platform wrapping rules as ReposResponse.
	IssuesResponse string
	// UsersResponse is the JSON body for user-search GETs, same per-
	// platform wrapping rules as ReposResponse.
	UsersResponse string
}

// SearchHarness is the full harness RunSearchSuite consumes; auto-mounting
// builds it from the enclosing Harness plus SearchHarnessConfig.
type SearchHarness struct {
	Name        string
	Platform    model.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	SearchHarnessConfig
}

// testSearchSuite auto-mounts RunSearchSuite from a main Harness with the
// same bidirectional drift checks as the labels, issues, reviews, and
// milestones suites.
func testSearchSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Search
	switch {
	case h.Search == nil && !declared:
		t.Skipf("%s declares no Search capability", h.Name)
	case h.Search == nil:
		t.Errorf("%s declares Capabilities().Search but its Harness provides no Search config — the search suite is not wired", h.Name)
	case !declared:
		t.Errorf("%s Harness provides a Search config but the platform does not declare Capabilities().Search", h.Name)
	default:
		RunSearchSuite(t, SearchHarness{
			Name:                h.Name,
			Platform:            h.Platform,
			NewProvider:         h.NewProvider,
			SearchHarnessConfig: *h.Search,
		})
	}
}

// RunSearchSuite executes the search contract suite. The mock server is
// dedicated to search requests and routes pragmatically, because the
// platforms disagree on both paths and encoding:
//
//   - GitLab sends every scope to the same path (/search) with a scope query
//     param, so a scope param wins when present: projects -> repos,
//     issues -> issues, users -> users.
//   - Otherwise the path is matched: "repositories" (GitHub/Gitee/GitCode
//     /search/repositories) or "projects" -> repos; "issues" (including
//     Gitea/Forgejo's /repos/issues/search) -> issues; "users" -> users.
//   - Anything else falls back to the repos fixture, which covers
//     Gitea/Forgejo's bare /repos/search path.
//
// Subtests: each search parses its fixture (repos by full_name, issues by
// title + string number, users by login) and the query keyword reaches the
// wire under some query parameter; the repo-scoped subtest additionally
// verifies SearchIssuesOptions.Repo takes a wire route that reflects the
// repo (path-embedded or query-carried, per platform).
func RunSearchSuite(t *testing.T, h SearchHarness) {
	newSM := func(t *testing.T) (provider.SearchManager, *[]recordedRequest) {
		srv, requests := searchStubServer(h)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		sm, ok := p.(provider.SearchManager)
		if !ok {
			t.Fatalf("%s does not implement provider.SearchManager", h.Name)
		}
		return sm, requests
	}

	t.Run("SearchRepos_Parses", func(t *testing.T) {
		sm, requests := newSM(t)
		assertSearchRepos(t, sm, requests, h.ReposTotalCount)
	})
	t.Run("SearchIssues_Parses", func(t *testing.T) {
		sm, requests := newSM(t)
		assertSearchIssues(t, sm, requests)
	})
	t.Run("SearchIssues_RepoScoped_Wire", func(t *testing.T) {
		sm, requests := newSM(t)
		assertSearchIssuesRepoScoped(t, sm, requests)
	})
	t.Run("SearchUsers_Parses", func(t *testing.T) {
		sm, requests := newSM(t)
		assertSearchUsers(t, sm, requests)
	})
}

// assertSearchRepos checks that SearchRepos returns parsed repo results —
// first hit full_name "owner/repo" — and that the keyword travelled to the
// server under some query parameter (q/search/keyword encodings differ per
// platform). The total is checked tightly when wantTotal is positive — the
// platform's fixture reports a server-side total (GitHub's total_count) and
// the returned total must equal it — and weakly (total == nil or
// *total >= len(results)) otherwise.
func assertSearchRepos(t *testing.T, sm provider.SearchManager, requests *[]recordedRequest, wantTotal int) {
	t.Helper()
	repos, total, err := sm.SearchRepos(context.Background(), provider.SearchReposOptions{Query: "gopher"})
	if err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("expected at least one repo result")
	}
	if repos[0].FullName != "owner/repo" {
		t.Errorf("expected first repo full_name %q, got %q", "owner/repo", repos[0].FullName)
	}
	if wantTotal > 0 {
		if total == nil || *total != wantTotal {
			t.Errorf("SearchRepos total = %v, want %d", total, wantTotal)
		}
	} else if total != nil && *total < len(repos) {
		t.Errorf("SearchRepos total = %d, want nil or >= len(results) (%d)", *total, len(repos))
	}
	assertQueryCarriesKeyword(t, requests, "gopher")
}

// assertSearchIssues checks that SearchIssues returns parsed issue results —
// first hit title "found" and the addressing identifier as string "1" (so
// results feed GetIssue(number string) directly) — and that the keyword
// reached the wire.
func assertSearchIssues(t *testing.T, sm provider.SearchManager, requests *[]recordedRequest) {
	t.Helper()
	issues, _, err := sm.SearchIssues(context.Background(), provider.SearchIssuesOptions{Query: "gopher"})
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected at least one issue result")
	}
	if issues[0].Title != "found" {
		t.Errorf("expected first issue title %q, got %q", "found", issues[0].Title)
	}
	if issues[0].Number != "1" {
		t.Errorf("expected first issue number %q, got %q", "1", issues[0].Number)
	}
	assertQueryCarriesKeyword(t, requests, "gopher")
}

// assertSearchIssuesRepoScoped checks that repo-scoped issue search takes a
// wire route that reflects the repo: the platform either addresses the repo
// in the request path (Gitea/Forgejo /repos/{owner}/{repo}/issues; GitLab
// /projects/{owner%2Frepo}/search) or carries it in a query parameter
// (Gitee/GitCode repo=; GitHub's repo: qualifier inside q). The assertion
// is platform-agnostic and encoding-tolerant ("/" vs "%2F").
func assertSearchIssuesRepoScoped(t *testing.T, sm provider.SearchManager, requests *[]recordedRequest) {
	t.Helper()
	const repo = "owner/repo"
	issues, _, err := sm.SearchIssues(context.Background(), provider.SearchIssuesOptions{Query: "gopher", Repo: repo})
	if err != nil {
		t.Fatalf("SearchIssues(repo-scoped): %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected at least one repo-scoped issue result")
	}
	forms := []string{repo, url.PathEscape(repo)}
	for i := range *requests {
		for _, form := range forms {
			if strings.Contains((*requests)[i].Path, form) || queryValuesContain(*requests, i, form) {
				return
			}
		}
	}
	t.Errorf("expected a request reflecting repo %q in its path or query parameters, recorded %s", repo, methodsOf(*requests))
}

// queryValuesContain checks whether any query parameter value of the i-th
// recorded request contains s.
func queryValuesContain(requests []recordedRequest, i int, s string) bool {
	for _, values := range requests[i].Query() {
		for _, v := range values {
			if strings.Contains(v, s) {
				return true
			}
		}
	}
	return false
}

// assertSearchUsers checks that SearchUsers returns parsed user results —
// first hit login "dev" — and that the keyword reached the wire.
func assertSearchUsers(t *testing.T, sm provider.SearchManager, requests *[]recordedRequest) {
	t.Helper()
	users, _, err := sm.SearchUsers(context.Background(), provider.SearchUsersOptions{Query: "gopher"})
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(users) == 0 {
		t.Fatal("expected at least one user result")
	}
	if users[0].Login != "dev" {
		t.Errorf("expected first user login %q, got %q", "dev", users[0].Login)
	}
	assertQueryCarriesKeyword(t, requests, "gopher")
}

// assertQueryCarriesKeyword checks that at least one recorded request has
// the keyword in some query parameter value. Platforms encode the search
// term under different keys (q on GitHub/Gitee/GitCode, search on GitLab,
// q on Gitea/Forgejo), so the assertion is key-agnostic.
func assertQueryCarriesKeyword(t *testing.T, requests *[]recordedRequest, keyword string) {
	t.Helper()
	for i := range *requests {
		for _, values := range (*requests)[i].Query() {
			for _, v := range values {
				if strings.Contains(v, keyword) {
					return
				}
			}
		}
	}
	t.Errorf("expected the keyword %q to reach the wire in a query parameter, recorded %s", keyword, methodsOf(*requests))
}

// searchStubServer returns the fixture-routed recording mock for the search
// suite; see RunSearchSuite for the routing rules.
func searchStubServer(h SearchHarness) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeRecordedBody(r)
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.RequestURI(), Body: body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		// Page-aware: AllPages-driven backends probe page 2; serve an empty
		// terminal page (shaped like the fixture) so the loop stops.
		fixture := routeSearchResponse(r, h)
		if beyondFirstPage(r) {
			fixture = emptySearchPageOf(fixture)
		}
		_, _ = w.Write([]byte(fixture))
	}))
	return srv, &requests
}

// emptySearchPageOf returns an empty result page shaped like fixture so the
// AllPages loop terminates: envelope responses (Gitea/Forgejo wrap search
// hits in {"data":[...]}) need the envelope preserved or the SDK decode
// fails, bare-array ones take a bare empty array.
func emptySearchPageOf(fixture string) string {
	if strings.Contains(fixture, `"data"`) {
		return `{"ok":true,"data":[]}`
	}
	return `[]`
}

// routeSearchResponse picks the fixture for a search request: the GitLab
// scope param first, then path keywords, defaulting to the repos fixture.
func routeSearchResponse(r *http.Request, h SearchHarness) string {
	if scope := r.URL.Query().Get("scope"); scope != "" {
		switch scope {
		case "projects":
			return h.ReposResponse
		case "issues":
			return h.IssuesResponse
		case "users":
			return h.UsersResponse
		}
	}
	switch {
	case strings.Contains(r.URL.Path, "repositories"), strings.Contains(r.URL.Path, "projects"):
		return h.ReposResponse
	case strings.Contains(r.URL.Path, "issues"):
		return h.IssuesResponse
	case strings.Contains(r.URL.Path, "users"):
		return h.UsersResponse
	default:
		// Gitea/Forgejo's repository search lives at a bare /repos/search
		// path with no distinguishing keyword.
		return h.ReposResponse
	}
}
