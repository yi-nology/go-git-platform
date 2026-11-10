package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// MilestonesHarnessConfig carries the fixtures a backend's main Harness
// needs to auto-mount the milestone-management suite via Harness.Milestones.
type MilestonesHarnessConfig struct {
	// ListResponse is the JSON array for milestone-list GETs. First item:
	// the platform's addressing identifier ("number" on GitHub and Gitee,
	// "id" on GitLab, Gitea, Forgejo, GitCode, and Tencent Code) rendering
	// as the string "1", title "v1", state "open" (GitLab's wire state
	// "active" is also accepted — the backend normalizes it).
	ListResponse string
	// GetResponse is the JSON object for a single-milestone GET (same shape
	// as the first list item). Empty = the harness has not wired the single-
	// get endpoint yet: the Get subtest skips, the rest of the suite is
	// unaffected.
	GetResponse string
	// MutateResponse is the JSON object for POST/PATCH/PUT (same shape as
	// the first list item).
	MutateResponse string
}

// MilestonesHarness is the full harness RunMilestonesSuite consumes;
// auto-mounting builds it from the enclosing Harness plus
// MilestonesHarnessConfig.
type MilestonesHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	MilestonesHarnessConfig
}

// testMilestonesSuite auto-mounts RunMilestonesSuite from a main Harness
// with the same bidirectional drift checks as the labels, issues, and
// reviews suites.
func testMilestonesSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Milestones
	switch {
	case h.Milestones == nil && !declared:
		t.Skipf("%s declares no Milestones capability", h.Name)
	case h.Milestones == nil:
		t.Errorf("%s declares Capabilities().Milestones but its Harness provides no Milestones config — the milestones suite is not wired", h.Name)
	case !declared:
		t.Errorf("%s Harness provides a Milestones config but the platform does not declare Capabilities().Milestones", h.Name)
	default:
		RunMilestonesSuite(t, MilestonesHarness{
			Name:                    h.Name,
			Platform:                h.Platform,
			NewProvider:             h.NewProvider,
			MilestonesHarnessConfig: *h.Milestones,
		})
	}
}

// RunMilestonesSuite executes the milestone-management contract suite. The
// mock server is dedicated to milestone paths (route by "milestones" in the
// path so platform-specific prefixes don't matter) and routes by HTTP
// method: GET → ListResponse (array); POST → 201 + MutateResponse;
// PATCH/PUT → MutateResponse (GitLab updates with PUT, the rest with
// PATCH); DELETE → 204. Requests are recorded for wire assertions.
func RunMilestonesSuite(t *testing.T, h MilestonesHarness) {
	newMM := func(t *testing.T) (provider.MilestoneManager, *[]recordedRequest) {
		srv, requests := milestoneStubServer(h)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		mm, ok := p.(provider.MilestoneManager)
		if !ok {
			t.Fatalf("%s does not implement provider.MilestoneManager", h.Name)
		}
		return mm, requests
	}

	t.Run("List_ParsesAndNormalizes", func(t *testing.T) {
		mm, _ := newMM(t)
		assertMilestoneListNormalized(t, mm)
	})
	// Page==0 dual-mode contract (issue #60): milestones are an
	// exhaustive-family list — zero Page walks every page, Page>0 serves
	// exactly that page.
	runPageZeroDualModeSuite(t, h.NewProvider, PageZeroFixture{
		Platform:  h.Platform,
		ListBody:  h.ListResponse,
		EmptyBody: `[]`,
		ListZero: func(p provider.Provider) error {
			_, err := p.(provider.MilestoneManager).ListMilestones(context.Background(), "owner", "repo", provider.ListMilestonesOptions{})
			return err
		},
		ListPageOne: func(p provider.Provider) error {
			_, err := p.(provider.MilestoneManager).ListMilestones(context.Background(), "owner", "repo", provider.ListMilestonesOptions{Page: 1, PerPage: 10})
			return err
		},
	})
	// State contract (issue #61): empty State must ASK for all — the wire
	// carries either no state filter (platforms whose default is all, e.g.
	// GitLab) or the platform's explicit "all" value; it must never fall
	// through to an open-only platform default that silently drops closed
	// milestones.
	t.Run("List_ZeroState_WireAsksAll", func(t *testing.T) {
		mm, requests := newMM(t)
		assertMilestoneZeroStateWire(t, mm, requests)
	})
	// Explicit "closed" must reach the wire verbatim — except on backends
	// whose ledger registers a ListMilestones State ignore (platform filter
	// not expressible, currently Tencent Code).
	t.Run("List_ClosedState_Forwarded", func(t *testing.T) {
		assertMilestoneClosedStateWire(t, h)
	})
	t.Run("Get_ParsesAndNormalizes", func(t *testing.T) {
		if h.GetResponse == "" {
			t.Skipf("%s harness wires no GetResponse fixture", h.Name)
		}
		mm, _ := newMM(t)
		m, err := mm.GetMilestone(context.Background(), "owner", "repo", "1")
		if err != nil {
			t.Fatalf("GetMilestone: %v", err)
		}
		if m.Number != "1" {
			t.Errorf("expected number %q, got %q", "1", m.Number)
		}
		if m.Title != "v1" {
			t.Errorf("expected title %q, got %q", "v1", m.Title)
		}
	})
	t.Run("Create_PostsTitle", func(t *testing.T) {
		mm, requests := newMM(t)
		assertMilestoneCreateWire(t, mm, requests)
	})
	t.Run("Update_PatchesTitle", func(t *testing.T) {
		mm, requests := newMM(t)
		assertMilestoneUpdateWire(t, mm, requests)
	})
	t.Run("Delete_NotGet", func(t *testing.T) {
		mm, requests := newMM(t)
		assertMilestoneDeleteWire(t, mm, requests)
	})
}

// assertMilestoneListNormalized checks that ListMilestones returns parsed,
// normalized milestones: the addressing identifier as string "1", title
// "v1", and state MilestoneStateOpen (GitLab's wire state "active"
// normalizes to open).
func assertMilestoneListNormalized(t *testing.T, mm provider.MilestoneManager) {
	t.Helper()
	milestones, err := mm.ListMilestones(context.Background(), "owner", "repo", provider.ListMilestonesOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("ListMilestones: %v", err)
	}
	if len(milestones) == 0 {
		t.Fatal("expected at least one milestone")
	}
	if milestones[0].Number != "1" {
		t.Errorf("expected first milestone number %q, got %q", "1", milestones[0].Number)
	}
	if milestones[0].Title != "v1" {
		t.Errorf("expected milestone title %q, got %q", "v1", milestones[0].Title)
	}
	if milestones[0].State != provider.MilestoneStateOpen {
		t.Errorf("expected normalized state %q, got %q", provider.MilestoneStateOpen, milestones[0].State)
	}
}

// assertMilestoneCreateWire checks that CreateMilestone succeeds, returns
// the created milestone, and posts the title under the "title" key (the key
// every current platform's create endpoint reads).
func assertMilestoneCreateWire(t *testing.T, mm provider.MilestoneManager, requests *[]recordedRequest) {
	t.Helper()
	m, err := mm.CreateMilestone(context.Background(), "owner", "repo", provider.CreateMilestoneOptions{Title: "v2"})
	if err != nil || m == nil {
		t.Fatalf("CreateMilestone: milestone=%v err=%v", m, err)
	}
	assertBodyHas(t, requests, http.MethodPost, "title", "v2")
}

// assertMilestoneUpdateWire checks that UpdateMilestone succeeds and
// mutates via PATCH or PUT (GitLab uses PUT; the other platforms PATCH — a
// POST would mean the update was routed to the create endpoint) carrying
// the new title under the "title" key.
func assertMilestoneUpdateWire(t *testing.T, mm provider.MilestoneManager, requests *[]recordedRequest) {
	t.Helper()
	newTitle := "v2-renamed"
	m, err := mm.UpdateMilestone(context.Background(), "owner", "repo", "1", provider.UpdateMilestoneOptions{Title: &newTitle})
	if err != nil || m == nil {
		t.Fatalf("UpdateMilestone: milestone=%v err=%v", m, err)
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, "title", newTitle)
}

// assertMilestoneDeleteWire checks that DeleteMilestone reached the server
// with a state-changing verb. Every current platform deletes milestones
// with DELETE; a GET-only recording means nothing was mutated.
func assertMilestoneDeleteWire(t *testing.T, mm provider.MilestoneManager, requests *[]recordedRequest) {
	t.Helper()
	if err := mm.DeleteMilestone(context.Background(), "owner", "repo", "1"); err != nil {
		t.Fatalf("DeleteMilestone: %v", err)
	}
	for i := range *requests {
		if (*requests)[i].Method != http.MethodGet {
			return
		}
	}
	t.Errorf("expected a non-GET delete request, recorded %s", methodsOf(*requests))
}

// pathsOf summarizes the recorded request paths for failure messages.
func pathsOf(requests []recordedRequest) []string {
	ps := make([]string, len(requests))
	for i, r := range requests {
		ps[i] = r.Path
	}
	return ps
}

// milestoneStubServer returns the method-routed recording mock for the
// milestones suite. Every milestone-path request is served; the GET branch
// answers list and single fetches alike with ListResponse since the suite's
// assertions only consume the array shape.
func milestoneStubServer(h MilestonesHarness) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "milestones") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body := decodeRecordedBody(r)
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.RequestURI(), Body: body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(h.MutateResponse))
		case http.MethodPatch, http.MethodPut:
			_, _ = w.Write([]byte(h.MutateResponse))
		default:
			// GET：路径以 /milestones/<id> 段结尾的是单件取（fixture 缺省
			// 时返回空体跳过语义不成立——Get 步骤自会跳过，这里仍给列表
			// 以免其它 GET 误伤）。
			if h.GetResponse != "" && strings.HasSuffix(r.URL.Path, "/1") && strings.Contains(r.URL.Path, "milestones") {
				_, _ = w.Write([]byte(h.GetResponse))
				return
			}
			_, _ = w.Write([]byte(h.ListResponse))
		}
	}))
	return srv, &requests
}

// assertMilestoneZeroStateWire pins the empty-State asks-all contract
// (issue #61): every list GET carries either no state filter or the
// explicit "all" value.
func assertMilestoneZeroStateWire(t *testing.T, mm provider.MilestoneManager, requests *[]recordedRequest) {
	t.Helper()
	if _, err := mm.ListMilestones(context.Background(), "owner", "repo", provider.ListMilestonesOptions{Page: 1, PerPage: 10}); err != nil {
		t.Fatalf("ListMilestones (empty State): %v", err)
	}
	asserted := false
	for i := range *requests {
		r := (*requests)[i]
		if r.Method != http.MethodGet {
			continue
		}
		asserted = true
		if s := r.Query().Get("state"); s != "" && s != "all" {
			t.Errorf("empty State must not send an open-only filter, got state=%q on %s", s, r.Path)
		}
	}
	if !asserted {
		t.Fatal("expected at least one GET to assert the state wire")
	}
}

// assertMilestoneClosedStateWire pins the explicit-State forwarding contract
// (issue #61): "closed" reaches the wire verbatim, unless the ledger
// registers a ListMilestones State ignore.
func assertMilestoneClosedStateWire(t *testing.T, h MilestonesHarness) {
	t.Helper()
	srv, requests := milestoneStubServer(h)
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	if provider.Ignores(p.Divergences(), "ListMilestones", "State") {
		t.Skipf("%s registers a ListMilestones State ignore", h.Name)
	}
	mm := p.(provider.MilestoneManager)
	if _, err := mm.ListMilestones(context.Background(), "owner", "repo", provider.ListMilestonesOptions{State: "closed", Page: 1, PerPage: 10}); err != nil {
		t.Fatalf("ListMilestones (closed): %v", err)
	}
	for i := range *requests {
		r := (*requests)[i]
		if r.Method != http.MethodGet {
			continue
		}
		if s := r.Query().Get("state"); s == "closed" {
			return // forwarded verbatim
		}
	}
	t.Fatalf("State=%q never reached the wire as state=closed; requests: %v", "closed", pathsOf(*requests))
}
