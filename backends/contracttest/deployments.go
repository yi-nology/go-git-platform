package contracttest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/provider"
)

// DeploymentsHarnessConfig carries fixtures for the deployment suite.
type DeploymentsHarnessConfig struct {
	// CreateResponse is the JSON POST /repos/{o}/{r}/deployments answers
	// with. It must decode to a non-zero id with non-empty ref/environment:
	// the suite compares the provider's parse against a canonical stdlib
	// decode of this exact fixture (assertDeployment), so a wire-key
	// mapping regression fails here.
	CreateResponse string
	// ListResponse is the JSON array GET /repos/{o}/{r}/deployments answers
	// with — at least one element carrying a non-zero id (the parse check
	// would pass vacuously against an empty or typo'd fixture otherwise).
	ListResponse string
	// StatusResponse is the JSON POST .../deployments/{id}/statuses answers
	// with. The provider discards the body, but the SDK still decodes it,
	// so it must be a valid status object.
	StatusResponse string
}

func testDeploymentsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Deployments
	switch {
	case h.Deployments == nil && !declared:
		t.Skipf("%s declares no Deployments capability", h.Name)
	case h.Deployments == nil:
		t.Errorf("%s declares Capabilities().Deployments but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides Deployments config but platform does not declare capability", h.Name)
	default:
		RunDeploymentsSuite(t, DeploymentsHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			CreateResponse: h.Deployments.CreateResponse,
			ListResponse:   h.Deployments.ListResponse,
			StatusResponse: h.Deployments.StatusResponse,
		})
	}
}

// DeploymentsHarness bundles inputs for the deployment suite.
type DeploymentsHarness struct {
	Name           string
	Platform       provider.Platform
	NewProvider    func(t *testing.T, cfg provider.Config) provider.Provider
	CreateResponse string
	ListResponse   string
	StatusResponse string
}

// deploymentsWire records the request line the deployment subtests assert
// on: method, path (the create/list/status routes), the query parameters
// (environment filter + paging), and the decoded body.
type deploymentsWire struct {
	mu     sync.Mutex
	method string
	path   string
	query  map[string]string
	body   map[string]any
	n      int
}

func (w *deploymentsWire) record(r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.method = r.Method
	w.path = r.URL.Path
	q := r.URL.Query()
	envPresent := "0"
	if q.Has("environment") {
		envPresent = "1"
	}
	w.query = map[string]string{
		"environment":        q.Get("environment"),
		"environmentPresent": envPresent, // presence, not just value: q.Get is "" for both absent and empty
		"page":               q.Get("page"),
		"per_page":           q.Get("per_page"),
	}
	w.body = decodeRecordedBody(r)
	w.n++
}

func (w *deploymentsWire) last() (method, path string, query map[string]string, body map[string]any, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.method, w.path, w.query, w.body, w.n
}

// RunDeploymentsSuite tests the deployment-manager wire contract: the create
// POST carries ref/environment (plus the option fields) and parses, the list
// GET carries the environment filter + paging and parses with the page-item
// count convention, a legal status state POSTs its state, and an illegal
// state is rejected pre-wire with zero requests.
func RunDeploymentsSuite(t *testing.T, h DeploymentsHarness) {
	t.Run("CreateDeployment_PostBody_Parses", func(t *testing.T) { runCreateDeploymentCase(t, h) })
	t.Run("ListDeployments_EnvironmentQuery_Parses", func(t *testing.T) { runListDeploymentsCase(t, h) })
	t.Run("ListDeployments_EmptyEnvironment_Omitted", func(t *testing.T) { runListDeploymentsEmptyEnvCase(t, h) })
	t.Run("CreateDeploymentStatus_LegalState", func(t *testing.T) { runCreateDeploymentStatusCase(t, h) })
	t.Run("CreateDeploymentStatus_InvalidState_PreWire", func(t *testing.T) { runCreateDeploymentStatusInvalidCase(t, h) })
}

// newDeploymentManager builds a provider against a mock that records every
// request and answers with resp, then type-asserts the deployment interface.
func newDeploymentManager(t *testing.T, h DeploymentsHarness, resp string, rec *deploymentsWire) provider.DeploymentManager {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	dm, ok := p.(provider.DeploymentManager)
	if !ok {
		t.Fatalf("%s does not implement provider.DeploymentManager", h.Name)
	}
	return dm
}

// runCreateDeploymentCase pins the create wire: exactly one POST on
// .../deployments carrying ref + environment (the positional args) and the
// option fields, and a faithful response parse (non-zero id).
func runCreateDeploymentCase(t *testing.T, h DeploymentsHarness) {
	t.Helper()
	rec := &deploymentsWire{}
	dm := newDeploymentManager(t, h, h.CreateResponse, rec)
	autoMerge := true
	dep, err := dm.CreateDeployment(context.Background(), "owner", "repo", "main", "production",
		provider.CreateDeploymentOptions{
			Task:             "deploy",
			Description:      "ship it",
			AutoMerge:        &autoMerge,
			RequiredContexts: []string{"ci/build"},
			Payload:          map[string]string{"version": "1.2.3"},
		})
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	method, path, _, body, n := rec.last()
	if n != 1 {
		t.Errorf("expected exactly 1 request, got %d", n)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if !strings.HasSuffix(path, "/repos/owner/repo/deployments") {
		t.Errorf("path = %s, want .../repos/owner/repo/deployments", path)
	}
	assertCreateDeploymentBody(t, body)
	assertDeployment(t, dep, decodeDeploymentFixture(t, h.CreateResponse, "CreateResponse"), "CreateDeployment")
}

// assertCreateDeploymentBody pins the create body: the positional ref and
// environment must ride the wire, and so must every option field the caller
// set (a dropped key here is a silently ignored option).
func assertCreateDeploymentBody(t *testing.T, body map[string]any) {
	t.Helper()
	if body == nil {
		t.Fatal("CreateDeployment sent no decodable JSON body")
	}
	if body["ref"] != "main" {
		t.Errorf("wire ref = %v, want main", body["ref"])
	}
	if body["environment"] != "production" {
		t.Errorf("wire environment = %v, want production", body["environment"])
	}
	if body["task"] != "deploy" {
		t.Errorf("wire task = %v, want deploy", body["task"])
	}
	if body["description"] != "ship it" {
		t.Errorf("wire description = %v, want \"ship it\"", body["description"])
	}
	if body["auto_merge"] != true {
		t.Errorf("wire auto_merge = %v, want true", body["auto_merge"])
	}
	contexts, ok := body["required_contexts"].([]any)
	if !ok || len(contexts) != 1 || contexts[0] != "ci/build" {
		t.Errorf("wire required_contexts = %v, want [ci/build]", body["required_contexts"])
	}
	payload, ok := body["payload"].(map[string]any)
	if !ok || payload["version"] != "1.2.3" {
		t.Errorf("wire payload = %v, want {version:1.2.3}", body["payload"])
	}
}

// runListDeploymentsCase pins the list wire: exactly one GET on
// .../deployments with the environment filter and paging in the query, a
// faithful parse, and the page-item count convention (total == len, never a
// page index — issue #89).
func runListDeploymentsCase(t *testing.T, h DeploymentsHarness) {
	t.Helper()
	rec := &deploymentsWire{}
	dm := newDeploymentManager(t, h, h.ListResponse, rec)
	// page=3 (≠ the 2-element fixture length) so a hypothetical
	// `return page` regression yields total=3 ≠ len(deps)=2 and fails the
	// issue #89 pin below — page=2 would collide with the fixture length.
	deps, total, err := dm.ListDeployments(context.Background(), "owner", "repo", "production", 3, 5)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	method, path, query, _, n := rec.last()
	if n != 1 {
		t.Errorf("expected exactly 1 request, got %d", n)
	}
	if method != http.MethodGet {
		t.Errorf("method = %s, want GET", method)
	}
	if !strings.HasSuffix(path, "/repos/owner/repo/deployments") {
		t.Errorf("path = %s, want .../repos/owner/repo/deployments", path)
	}
	if query["environment"] != "production" || query["environmentPresent"] != "1" {
		t.Errorf("query environment = %q (present=%s), want production on the wire",
			query["environment"], query["environmentPresent"])
	}
	if query["page"] != "3" || query["per_page"] != "5" {
		t.Errorf("query page=%s per_page=%s, want page=3 per_page=5", query["page"], query["per_page"])
	}
	want := decodeDeploymentListFixture(t, h.ListResponse, "ListResponse")
	if len(deps) != len(want) {
		t.Errorf("parsed %d deployments, fixture carries %d — wire keys must map onto provider.Deployment tags", len(deps), len(want))
	}
	if total != len(deps) {
		t.Errorf("total = %d, want %d (page item count, never a page index — issue #89)", total, len(deps))
	}
	if len(deps) == 0 {
		t.Fatal("ListResponse fixture must carry at least one deployment (vacuous parse check)")
	}
	assertDeployment(t, deps[0], want[0], "ListDeployments[0]")
}

// runListDeploymentsEmptyEnvCase pins the filter's other face: an empty
// environment must omit the query key entirely (the URL tag's omitempty),
// not send environment=.
func runListDeploymentsEmptyEnvCase(t *testing.T, h DeploymentsHarness) {
	t.Helper()
	rec := &deploymentsWire{}
	dm := newDeploymentManager(t, h, h.ListResponse, rec)
	if _, _, err := dm.ListDeployments(context.Background(), "owner", "repo", "", 1, 10); err != nil {
		t.Fatalf("ListDeployments(empty environment): %v", err)
	}
	_, _, query, _, n := rec.last()
	if n != 1 {
		t.Errorf("expected exactly 1 request, got %d", n)
	}
	if query["environmentPresent"] != "0" {
		t.Errorf("empty environment must omit the query key entirely, got present=%s value=%q",
			query["environmentPresent"], query["environment"])
	}
	if query["page"] != "1" || query["per_page"] != "10" {
		t.Errorf("query page=%s per_page=%s, want page=1 per_page=10", query["page"], query["per_page"])
	}
}

// runCreateDeploymentStatusCase pins the legal-state write: one POST on
// .../deployments/{id}/statuses carrying the state vocabulary entry.
func runCreateDeploymentStatusCase(t *testing.T, h DeploymentsHarness) {
	t.Helper()
	rec := &deploymentsWire{}
	dm := newDeploymentManager(t, h, h.StatusResponse, rec)
	err := dm.CreateDeploymentStatus(context.Background(), "owner", "repo", 42,
		provider.DeploymentStatusOptions{State: "success", Description: "done"})
	if err != nil {
		t.Fatalf("CreateDeploymentStatus(success): %v", err)
	}
	method, path, _, body, n := rec.last()
	if n != 1 {
		t.Errorf("expected exactly 1 request, got %d", n)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if !strings.HasSuffix(path, "/repos/owner/repo/deployments/42/statuses") {
		t.Errorf("path = %s, want .../deployments/42/statuses (id must ride the route)", path)
	}
	if body == nil {
		t.Fatal("CreateDeploymentStatus sent no decodable JSON body")
	}
	if body["state"] != "success" {
		t.Errorf("wire state = %v, want success", body["state"])
	}
	if body["description"] != "done" {
		t.Errorf("wire description = %v, want done", body["description"])
	}
}

// runCreateDeploymentStatusInvalidCase pins the pre-wire state whitelist
// (isValidDeploymentState): an illegal state must fail closed BEFORE any
// HTTP with the unified 422 → ErrInvalidInput shape, never reaching the
// platform with an off-vocabulary state.
func runCreateDeploymentStatusInvalidCase(t *testing.T, h DeploymentsHarness) {
	t.Helper()
	rec := &deploymentsWire{}
	dm := newDeploymentManager(t, h, h.StatusResponse, rec)
	err := dm.CreateDeploymentStatus(context.Background(), "owner", "repo", 42,
		provider.DeploymentStatusOptions{State: "bogus"})
	_, _, _, _, n := rec.last()
	assertPreWireInvalidInput(t, err, n, `CreateDeploymentStatus(state="bogus")`)
}

// assertPreWireInvalidInput pins the pre-wire rejection contract shared by
// the write-side negative cases (illegal deployment state, empty fork-sync
// branch): provider.New(…, 422, …) must surface as ErrInvalidInput through
// the ProviderError cause chain, with zero requests reaching the wire.
func assertPreWireInvalidInput(t *testing.T, err error, requests int, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected a pre-wire rejection, got nil", label)
	}
	if !provider.IsInvalidInput(err) {
		t.Errorf("%s: err = %v, want errors.Is(err, provider.ErrInvalidInput)", label, err)
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Errorf("%s: err type %T, want *provider.ProviderError (unified shape)", label, err)
	} else if !pe.IsStatus(http.StatusUnprocessableEntity) {
		t.Errorf("%s: status = %d, want 422", label, pe.StatusCode)
	}
	if requests != 0 {
		t.Errorf("%s: rejection must happen before the wire, recorded %d requests", label, requests)
	}
}

// deploymentWire mirrors the GitHub deployments response shape for a
// canonical stdlib decode of the fixture. provider.Deployment carries the
// creator LOGIN as a string while the wire sends the user object, so the
// fixture cannot be decoded straight into the provider type the way
// MigrationInfo can — hence this wire-side mirror plus a field-by-field
// comparison (assertDeployment).
type deploymentWire struct {
	ID          int64  `json:"id"`
	SHA         string `json:"sha"`
	Ref         string `json:"ref"`
	Environment string `json:"environment"`
	Description string `json:"description"`
	Creator     struct {
		Login string `json:"login"`
	} `json:"creator"`
	CreatedAt time.Time `json:"created_at"`
	URL       string    `json:"url"`
}

// decodeDeploymentFixture decodes a single-deployment fixture and refuses a
// zero id, so the parse assertion can never pass vacuously.
func decodeDeploymentFixture(t *testing.T, fixture, field string) deploymentWire {
	t.Helper()
	var w deploymentWire
	if err := json.Unmarshal([]byte(fixture), &w); err != nil {
		t.Fatalf("%s fixture does not decode: %v", field, err)
	}
	if w.ID == 0 {
		t.Fatalf("%s fixture must carry a non-zero id (vacuous parse check)", field)
	}
	return w
}

// decodeDeploymentListFixture decodes a deployment-array fixture and
// refuses an empty or zero-id first element (vacuous parse check).
func decodeDeploymentListFixture(t *testing.T, fixture, field string) []deploymentWire {
	t.Helper()
	var ws []deploymentWire
	if err := json.Unmarshal([]byte(fixture), &ws); err != nil {
		t.Fatalf("%s fixture does not decode: %v", field, err)
	}
	if len(ws) == 0 || ws[0].ID == 0 {
		t.Fatalf("%s fixture must carry at least one deployment with a non-zero id (vacuous parse check)", field)
	}
	return ws
}

// assertDeployment pins the response parse: every provider.Deployment field
// must equal the canonical stdlib decode of the fixture — the same wire keys
// the implementation maps — so a swapped or dropped field breaks it.
func assertDeployment(t *testing.T, got *provider.Deployment, want deploymentWire, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: Deployment is nil", label)
	}
	if got.ID != want.ID || got.SHA != want.SHA || got.Ref != want.Ref ||
		got.Environment != want.Environment || got.Description != want.Description ||
		got.Creator != want.Creator.Login || got.URL != want.URL {
		t.Errorf("%s: parsed %+v, want id=%d sha=%s ref=%s env=%s desc=%s creator=%s url=%s — wire keys must map onto provider.Deployment fields",
			label, *got, want.ID, want.SHA, want.Ref, want.Environment, want.Description, want.Creator.Login, want.URL)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("%s: created_at = %v, want %v", label, got.CreatedAt, want.CreatedAt)
	}
}
