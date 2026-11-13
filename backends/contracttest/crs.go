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

// CRHarnessConfig carries the fixtures a backend's main Harness needs to
// auto-mount the change-request suite via Harness.CRs (issue #63: the CR
// family had zero behavior assertions and hosted the highest-yield historical
// bugs — #44 silent truncation, #78 filter semantics).
//
// ChangeRequestManager is a core interface composed into provider.Provider
// and implemented by every backend, so the config is mandatory for all seven
// platforms; individual fixture fields may be empty, which skips their
// subtest without failing the run.
type CRHarnessConfig struct {
	// ListResponse is the JSON array for CR-list GETs. First item: the
	// platform's addressing identifier rendering as "1", title "fix bug",
	// open state, source branch "feature-x", target branch "main".
	ListResponse string
	// GetResponse is the JSON object for a single-CR GET (same shape as the
	// first list item).
	GetResponse string
	// MutateResponse is the JSON object for create/update/merge
	// POST/PATCH/PUT responses (same shape).
	MutateResponse string
	// PageZeroWalks declares that Page==0 walks every page (the registered
	// caller-paged-family departure, currently Tencent Code — cross-checked
	// against the ledger's ListCRs Page divergence, so a stale flag fails
	// the run).
	PageZeroWalks bool
	// CRClear 声明各字段清空请求的期望（键：description/assignees）。
	// nil = 两字段全部必须可清空（全 A 级平台可不设）。
	CRClear map[string]ClearExpect
}

// CRHarness is the full harness RunCRSuite consumes.
type CRHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	CRHarnessConfig
}

// testCRSuite auto-mounts RunCRSuite from a main Harness. ChangeRequestManager
// is core: a missing config is a wiring error, there is no opt-out.
func testCRSuite(t *testing.T, h Harness) {
	if h.CRs == nil {
		t.Fatalf("%s provides no CRs config — ChangeRequestManager is a core interface, every backend must wire the CR suite", h.Name)
	}
	RunCRSuite(t, CRHarness{
		Name:            h.Name,
		Platform:        h.Platform,
		NewProvider:     h.NewProvider,
		CRHarnessConfig: *h.CRs,
	})
}

// RunCRSuite executes the change-request contract suite. The mock records
// every request; GETs ending at a CR collection return ListResponse on the
// first list call and an empty array on later ones (walk terminator), every
// other GET returns GetResponse, POST returns 201 + MutateResponse,
// PATCH/PUT return MutateResponse, DELETE returns 204.
func RunCRSuite(t *testing.T, h CRHarness) {
	newCM := func(t *testing.T) (provider.ChangeRequestManager, *[]recordedRequest) {
		srv, requests := crStubServer(h)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		cm, ok := p.(provider.ChangeRequestManager)
		if !ok {
			t.Fatalf("%s does not implement provider.ChangeRequestManager", h.Name)
		}
		return cm, requests
	}

	t.Run("Get_Parses", func(t *testing.T) {
		if h.GetResponse == "" {
			t.Skipf("%s harness wires no CR GetResponse fixture", h.Name)
		}
		cm, _ := newCM(t)
		assertCRGetParses(t, cm)
	})

	t.Run("List_Parses", func(t *testing.T) {
		cm, _ := newCM(t)
		assertCRListParses(t, cm)
	})

	// Page==0 contract, caller-paged family (issue #60): one request, no
	// walk — request volume must not scale with the repository's CR count.
	// Backends whose ledger registers the walk departure (Tencent Code) must
	// walk and stop on the first empty page (exactly two GETs); the flag and
	// the ledger are cross-checked so neither can rot silently.
	t.Run("List_PageZero_FirstPageOnly", func(t *testing.T) {
		assertCRPageZeroContract(t, h)
	})

	// Create wire: title plus the source/target branch pair under whichever
	// key vocabulary the platform speaks (source_branch/target_branch on
	// GitLab/Gongfeng; head/base on the GitHub family).
	t.Run("Create_PostsLifecycleWire", func(t *testing.T) {
		cm, requests := newCM(t)
		assertCRCreateWire(t, cm, requests)
	})

	// Close/Reopen wire: a state-changing verb carrying the state verb under
	// the platform's key (state=closed/open, or state_event=close/reopen on
	// the GitLab family).
	t.Run("CloseReopen_StateVerbWire", func(t *testing.T) {
		cm, requests := newCM(t)
		assertCRCloseReopenWire(t, cm, requests)
	})

	t.Run("Update_TitleWire", func(t *testing.T) {
		cm, requests := newCM(t)
		assertCRUpdateWire(t, cm, requests)
		assertCRUpdateTriState(t, cm, requests, h.CRClear)
	})

	t.Run("Merge_Mutates", func(t *testing.T) {
		cm, requests := newCM(t)
		assertCRMergeWire(t, cm, requests)
	})
}

// assertVerb checks that at least one recorded request used one of the
// given verbs.
func assertVerb(t *testing.T, requests *[]recordedRequest, verbs ...string) {
	t.Helper()
	for i := range *requests {
		for _, v := range verbs {
			if (*requests)[i].Method == v {
				return
			}
		}
	}
	t.Errorf("expected one of the verbs %v on the wire, recorded %s", verbs, methodsOf(*requests))
}

// providerIgnoresPageZero reports whether the ledger registers the
// ListCRs Page==0 walk departure (the caller-paged family's registered
// exception).
func providerIgnoresPageZero(divs []provider.Divergence) bool {
	for _, d := range provider.FindByMethod(divs, "ListCRs") {
		if d.Kind == provider.DivergenceMapping && strings.Contains(d.Field, "Page") {
			return true
		}
	}
	return false
}

// assertStateVerb checks that one recorded PATCH/PUT carries want under
// "state" or alt under "state_event".
func assertStateVerb(t *testing.T, requests *[]recordedRequest, want, alt string) {
	t.Helper()
	for i := range *requests {
		r := (*requests)[i]
		if r.Method != http.MethodPatch && r.Method != http.MethodPut {
			continue
		}
		if s, _ := r.Body["state"].(string); s == want {
			return
		}
		if s, _ := r.Body["state_event"].(string); s == alt {
			return
		}
	}
	t.Errorf("expected a PATCH/PUT carrying state=%q or state_event=%q, recorded %s", want, alt, methodsOf(*requests))
}

// isCRCollectionPath reports whether the request path addresses a CR
// collection (list) rather than a single CR — path vocabulary across the
// seven platforms: pulls (GitHub family), merge_requests (GitLab family),
// pulls under gongfeng's mr prefix.
func isCRCollectionPath(path string) bool {
	lower := strings.ToLower(path)
	for _, kw := range []string{"pulls", "merge_requests"} {
		if strings.Contains(lower, kw) && !strings.HasSuffix(lower, "/"+kw+"/1") {
			return true
		}
	}
	return false
}

// crStubServer returns the recording mock for the CR suite.
func crStubServer(h CRHarness) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	var lists int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeRecordedBody(r)
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.RequestURI(), Body: body})
		isList := r.Method == http.MethodGet && isCRCollectionPath(r.URL.Path)
		if isList {
			lists++
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case isList:
			if lists > 1 {
				_, _ = w.Write([]byte(`[]`)) // walk terminator
				return
			}
			_, _ = w.Write([]byte(h.ListResponse))
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(h.MutateResponse))
		case r.Method == http.MethodPatch || r.Method == http.MethodPut:
			_, _ = w.Write([]byte(h.MutateResponse))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(h.GetResponse))
		}
	}))
	return srv, &requests
}

// assertCRGetParses checks GetCR parses the fixture and normalizes number,
// title, state, and the branch pair.
func assertCRGetParses(t *testing.T, cm provider.ChangeRequestManager) {
	t.Helper()
	cr, err := cm.GetCR(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatalf("GetCR: %v", err)
	}
	if cr == nil || cr.Number != "1" {
		t.Fatalf("expected CR number \"1\", got %+v", cr)
	}
	if cr.Title != "fix bug" {
		t.Errorf("expected title \"fix bug\", got %q", cr.Title)
	}
	if cr.State != provider.CRStateOpened {
		t.Errorf("expected normalized state %q, got %q", provider.CRStateOpened, cr.State)
	}
	if cr.SourceBranch != "feature-x" {
		t.Errorf("expected source branch %q, got %q", "feature-x", cr.SourceBranch)
	}
	if cr.TargetBranch != "main" {
		t.Errorf("expected target branch %q, got %q", "main", cr.TargetBranch)
	}
}

// assertCRListParses checks the caller-driven list returns parsed CRs.
func assertCRListParses(t *testing.T, cm provider.ChangeRequestManager) {
	t.Helper()
	crs, _, err := cm.ListCRs(context.Background(), provider.ListCROptions{Owner: "owner", Repo: "repo", Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("ListCRs: %v", err)
	}
	if len(crs) == 0 {
		t.Fatal("expected at least one CR")
	}
	if crs[0].Number != "1" {
		t.Errorf("expected first CR number \"1\", got %q", crs[0].Number)
	}
}

// assertCRPageZeroContract pins the caller-paged-family Page==0 contract
// (issue #60): one request, no walk — unless the backend's ledger registers
// the walk departure, which the PageZeroWalks flag must then mirror.
func assertCRPageZeroContract(t *testing.T, h CRHarness) {
	t.Helper()
	srv, requests := crStubServer(CRHarness{
		Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider, CRHarnessConfig: h.CRHarnessConfig,
	})
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	cm := p.(provider.ChangeRequestManager)
	ledgerWalk := providerIgnoresPageZero(p.Divergences())
	if ledgerWalk != h.PageZeroWalks {
		t.Fatalf("PageZeroWalks=%v disagrees with the divergence ledger (%v) — update the CR harness flag and the ledger together", h.PageZeroWalks, ledgerWalk)
	}
	if _, _, err := cm.ListCRs(context.Background(), provider.ListCROptions{Owner: "owner", Repo: "repo"}); err != nil {
		t.Fatalf("ListCRs(Page==0): %v", err)
	}
	want := 1
	if h.PageZeroWalks {
		want = 2 // fixture page + empty terminal page
	}
	if got := countGETs(requests); got != want {
		t.Fatalf("ListCRs(Page==0): expected exactly %d GETs, got %d (issue #60 caller-paged family)", want, got)
	}
}

// assertCRCreateWire checks the create POST carries title and the branch
// pair under whichever key vocabulary the platform speaks
// (source_branch/target_branch on the GitLab family; head/base on GitHub).
func assertCRCreateWire(t *testing.T, cm provider.ChangeRequestManager, requests *[]recordedRequest) {
	t.Helper()
	cr, err := cm.CreateCR(context.Background(), provider.CreateCROptions{
		Owner: "owner", Repo: "repo",
		Title:        "new CR",
		SourceBranch: "feature-x",
		TargetBranch: "main",
	})
	if err != nil || cr == nil {
		t.Fatalf("CreateCR: cr=%v err=%v", cr, err)
	}
	var post *recordedRequest
	for i := range *requests {
		if (*requests)[i].Method == http.MethodPost {
			post = &(*requests)[i]
		}
	}
	if post == nil {
		t.Fatalf("expected a POST on the wire, recorded %s", methodsOf(*requests))
	}
	if post.Body["title"] != "new CR" {
		t.Errorf("create body must carry title, got %v", post.Body)
	}
	if post.Body["source_branch"] == nil && post.Body["head"] == nil {
		t.Errorf("create body must carry source_branch or head, got %v", post.Body)
	}
	if post.Body["target_branch"] == nil && post.Body["base"] == nil {
		t.Errorf("create body must carry target_branch or base, got %v", post.Body)
	}
}

// assertCRCloseReopenWire checks close/reopen mutate via PATCH/PUT carrying
// the state verb under the platform's key.
func assertCRCloseReopenWire(t *testing.T, cm provider.ChangeRequestManager, requests *[]recordedRequest) {
	t.Helper()
	if _, err := cm.CloseCR(context.Background(), "owner", "repo", "1"); err != nil {
		t.Fatalf("CloseCR: %v", err)
	}
	if _, err := cm.ReopenCR(context.Background(), "owner", "repo", "1"); err != nil {
		t.Fatalf("ReopenCR: %v", err)
	}
	assertStateVerb(t, requests, "closed", "close")
	assertStateVerb(t, requests, "open", "reopen")
}

// assertCRUpdateWire checks the update mutates via PATCH/PUT carrying the
// new title.
func assertCRUpdateWire(t *testing.T, cm provider.ChangeRequestManager, requests *[]recordedRequest) {
	t.Helper()
	if _, err := cm.UpdateCR(context.Background(), "owner", "repo", "1", provider.UpdateCROptions{Title: "renamed CR"}); err != nil {
		t.Fatalf("UpdateCR: %v", err)
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, "title", "renamed CR")
}

// assertCRUpdateTriState 钉 UpdateCR 三态（2A spec，issues 侧同构）：unset
// 不得携带字段、legacy 置值回归、patch 置值与 legacy 同形、
// description/assignees 清空按 ClearExpect 分派（B 级哨兵 + 零请求 /
// A 级 wire 在场空值）。
//
// CR 描述的 wire 键两套词表：GitHub 家族 body、GitLab/工蜂 description——
// 平台以 BodyField 声明，未声明回落 body（gitcode 的 B 级条目
// description 也声明 BodyField:"body"，其 wire 正是 body）。
func assertCRUpdateTriState(t *testing.T, cm provider.ChangeRequestManager, requests *[]recordedRequest, clears map[string]ClearExpect) {
	t.Helper()
	ctx := context.Background()

	descKey := clearKey(clears, "description", "body")

	// 1) unset：全零 options——若产生写请求，body 不得携带两字段
	//    （外加平台声明的 wire 键）。
	before := len(*requests)
	if _, err := cm.UpdateCR(ctx, "owner", "repo", "1", provider.UpdateCROptions{}); err != nil {
		t.Fatalf("unset UpdateCR: %v", err)
	}
	assertBodyLacks(t, requests, before, unsetKeys(clears, "description", "assignees")...)

	// 2) legacy 置值回归（旧行为不许破）。
	if _, err := cm.UpdateCR(ctx, "owner", "repo", "1", provider.UpdateCROptions{Description: "legacy-desc"}); err != nil {
		t.Fatalf("legacy set: %v", err)
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, descKey, "legacy-desc")

	// 3) patch 置值与 legacy 同形。
	if _, err := cm.UpdateCR(ctx, "owner", "repo", "1", provider.UpdateCROptions{DescriptionPatch: new("patch-desc")}); err != nil {
		t.Fatalf("patch set: %v", err)
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, descKey, "patch-desc")

	// 4) 逐字段清空（B/A 分派见 dispatchClear）。
	clearSteps := []struct {
		field string
		opts  provider.UpdateCROptions
	}{
		{"description", provider.UpdateCROptions{DescriptionPatch: new("")}},
		{"assignees", provider.UpdateCROptions{AssigneesPatch: &[]string{}}},
	}
	for i := range clearSteps {
		step := &clearSteps[i]
		stepBefore := len(*requests)
		dispatchClear(t, clears, step.field, requests, stepBefore, func() error {
			_, err := cm.UpdateCR(ctx, "owner", "repo", "1", step.opts)
			return err
		})
	}
}

// assertCRMergeWire checks the merge reached a merge endpoint with a
// state-changing verb.
func assertCRMergeWire(t *testing.T, cm provider.ChangeRequestManager, requests *[]recordedRequest) {
	t.Helper()
	if _, err := cm.MergeCR(context.Background(), "owner", "repo", "1", provider.MergeCROptions{MergeCommitMessage: "merged"}); err != nil {
		t.Fatalf("MergeCR: %v", err)
	}
	for i := range *requests {
		r := (*requests)[i]
		if (r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.Contains(r.Path, "merge") {
			return
		}
	}
	t.Errorf("expected a POST/PUT to a merge endpoint, recorded %v", pathsOf(*requests))
}
