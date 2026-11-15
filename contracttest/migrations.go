package contracttest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// MigrationsHarnessConfig carries fixtures for the migration suite.
type MigrationsHarnessConfig struct {
	// CreateResponse is the JSON both POST user/migrations and POST
	// orgs/{org}/migrations answer with. It must decode to a non-zero id and
	// a non-empty state: the suite compares the provider's parse against a
	// canonical stdlib decode of this exact fixture (assertMigrationInfo),
	// so a wire-key mapping regression fails here.
	CreateResponse string
	// GetResponse is the JSON GET .../migrations/{id} answers with. Include a
	// non-empty archive_url so the exported-terminal field mapping is pinned
	// as well.
	GetResponse string
}

func testMigrationsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Migrations
	switch {
	case h.Migrations == nil && !declared:
		t.Skipf("%s declares no Migrations capability", h.Name)
	case h.Migrations == nil:
		t.Errorf("%s declares Capabilities().Migrations but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides Migrations config but platform does not declare capability", h.Name)
	default:
		RunMigrationsSuite(t, MigrationsHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			CreateResponse: h.Migrations.CreateResponse,
			GetResponse:    h.Migrations.GetResponse,
		})
	}
}

// MigrationsHarness bundles inputs for the migration suite.
type MigrationsHarness struct {
	Name           string
	Platform       model.Platform
	NewProvider    func(t *testing.T, cfg provider.Config) provider.Provider
	CreateResponse string
	GetResponse    string
}

// migrationsWire records the request line the migration subtests assert on:
// method, path (the user/org dual route), Accept header, and the decoded
// body CreateMigration must serialize.
type migrationsWire struct {
	mu     sync.Mutex
	method string
	path   string
	accept string
	body   map[string]any
	n      int
}

func (w *migrationsWire) record(r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.method = r.Method
	w.path = r.URL.Path
	w.accept = r.Header.Get("Accept")
	w.body = decodeRecordedBody(r)
	w.n++
}

func (w *migrationsWire) last() (method, path, accept string, body map[string]any, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.method, w.path, w.accept, w.body, w.n
}

// RunMigrationsSuite tests the migration-manager wire contract: org
// emptiness picks the Create route (user vs org), the org Get route carries
// the id, both calls send the GA Accept header, both Create routes share the
// lock/exclude request body, and responses parse into model.MigrationInfo.
func RunMigrationsSuite(t *testing.T, h MigrationsHarness) {
	t.Run("CreateMigration_UserRoute", func(t *testing.T) {
		runCreateMigrationCase(t, h, "", "/user/migrations")
	})
	t.Run("CreateMigration_OrgRoute", func(t *testing.T) {
		runCreateMigrationCase(t, h, "acme", "/orgs/acme/migrations")
	})
	t.Run("GetMigration_OrgRoute", func(t *testing.T) { runGetMigrationCase(t, h) })
}

// newMigrationManager builds a provider against a mock that records every
// request and answers with resp, then type-asserts the migration interface.
func newMigrationManager(t *testing.T, h MigrationsHarness, resp string, rec *migrationsWire) provider.MigrationManager {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	mm, ok := p.(provider.MigrationManager)
	if !ok {
		t.Fatalf("%s does not implement provider.MigrationManager", h.Name)
	}
	return mm
}

// runCreateMigrationCase pins one CreateMigration route: org=="" must POST
// .../user/migrations, a non-empty org must POST .../orgs/{org}/migrations;
// either way exactly one POST with the GA Accept header, the shared
// lock/exclude body, and a faithful response parse.
func runCreateMigrationCase(t *testing.T, h MigrationsHarness, org, wantSuffix string) {
	t.Helper()
	rec := &migrationsWire{}
	mm := newMigrationManager(t, h, h.CreateResponse, rec)
	info, err := mm.CreateMigration(context.Background(), org, provider.CreateMigrationOptions{
		LockRepositories: true,
		ExcludeMetadata:  true,
	})
	if err != nil {
		t.Fatalf("CreateMigration(org=%q): %v", org, err)
	}
	method, path, accept, body, n := rec.last()
	if n != 1 {
		t.Errorf("expected exactly 1 request, got %d", n)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if !strings.HasSuffix(path, wantSuffix) {
		t.Errorf("org=%q: path = %s, want suffix %s (org emptiness must pick the route)", org, path, wantSuffix)
	}
	if accept != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want application/vnd.github+json", accept)
	}
	assertCreateMigrationBody(t, body)
	assertMigrationInfo(t, info, h.CreateResponse, "CreateMigration(org="+org+")")
}

// runGetMigrationCase pins the org-level GetMigration route: GET
// .../orgs/{org}/migrations/{id} with the id in the path, the GA Accept
// header, and a faithful response parse (archive_url included).
func runGetMigrationCase(t *testing.T, h MigrationsHarness) {
	t.Helper()
	rec := &migrationsWire{}
	mm := newMigrationManager(t, h, h.GetResponse, rec)
	const org = "acme"
	const id = int64(42)
	info, err := mm.GetMigration(context.Background(), org, id)
	if err != nil {
		t.Fatalf("GetMigration(org=%q, id=%d): %v", org, id, err)
	}
	method, path, accept, _, n := rec.last()
	if n != 1 {
		t.Errorf("expected exactly 1 request, got %d", n)
	}
	if method != http.MethodGet {
		t.Errorf("method = %s, want GET", method)
	}
	if !strings.HasSuffix(path, "/orgs/acme/migrations/42") {
		t.Errorf("path = %s, want .../orgs/acme/migrations/42 (org route must carry the id)", path)
	}
	if accept != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want application/vnd.github+json", accept)
	}
	assertMigrationInfo(t, info, h.GetResponse, "GetMigration(org=acme, id=42)")
	if info.ArchiveURL == "" {
		t.Error("GetResponse fixture must carry a non-empty archive_url — the exported-terminal mapping would pass vacuously otherwise")
	}
}

// assertCreateMigrationBody pins the body both Create routes share: the two
// keys must ride the wire explicitly (the reason CreateMigration is
// hand-rolled instead of delegating to the SDK's org-only endpoint, whose
// request body lacks exclude_metadata).
func assertCreateMigrationBody(t *testing.T, body map[string]any) {
	t.Helper()
	if body == nil {
		t.Fatal("CreateMigration sent no decodable JSON body")
	}
	if body["lock_repositories"] != true {
		t.Errorf("wire lock_repositories = %v, want true", body["lock_repositories"])
	}
	if body["exclude_metadata"] != true {
		t.Errorf("wire exclude_metadata = %v, want true", body["exclude_metadata"])
	}
}

// assertMigrationInfo pins the response parse: the provider's MigrationInfo
// must equal a canonical stdlib decode of the fixture — the same struct the
// implementation decodes into, so equality proves the wire keys land on the
// model.MigrationInfo tags (a swapped or dropped field breaks it). The
// fixture must itself carry a non-zero id, or the check could pass vacuously
// against an empty or typo'd fixture.
func assertMigrationInfo(t *testing.T, got *model.MigrationInfo, fixture, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: MigrationInfo is nil", label)
	}
	var want model.MigrationInfo
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("%s: fixture does not decode: %v", label, err)
	}
	if want.ID == 0 {
		t.Fatalf("%s: fixture must carry a non-zero id (vacuous parse check)", label)
	}
	if *got != want {
		t.Errorf("%s: parsed %+v, want %+v — wire keys must map onto model.MigrationInfo tags", label, *got, want)
	}
}
