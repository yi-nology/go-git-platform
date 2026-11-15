package contracttest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// assetSignedPath is the same-host second path the redirect subtest serves
// the asset bytes on — the stand-in for GitHub's signed object-store URL.
const assetSignedPath = "/signed/asset.bin"

// ReleaseAssetsHarnessConfig carries fixtures for the release-asset download
// suite.
type ReleaseAssetsHarnessConfig struct {
	// AssetBytes is the exact byte string the mock serves for the asset
	// endpoint — directly (200 form) and on the redirect target (302 form).
	// The suite asserts DownloadReleaseAsset writes it to w verbatim; keep a
	// value with multi-byte UTF-8 so a byte-vs-rune slicing regression shows
	// up as a mismatch.
	AssetBytes string
}

func testReleaseAssetsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().ReleaseAssets
	switch {
	case h.ReleaseAssets == nil && !declared:
		t.Skipf("%s declares no ReleaseAssets capability", h.Name)
	case h.ReleaseAssets == nil:
		t.Errorf("%s declares Capabilities().ReleaseAssets but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides ReleaseAssets config but platform does not declare capability", h.Name)
	default:
		RunReleaseAssetsSuite(t, ReleaseAssetsHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			AssetBytes: h.ReleaseAssets.AssetBytes,
		})
	}
}

// ReleaseAssetsHarness bundles inputs for the release-asset download suite.
type ReleaseAssetsHarness struct {
	Name        string
	Platform    model.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	AssetBytes  string
}

// assetHop records one HTTP hop the download subtests assert on: the request
// line, the Accept header, and whether API credentials were attached.
type assetHop struct {
	method string
	path   string
	accept string
	auth   string
}

// releaseAssetsWire records every hop a download issues — the API first hop
// and, on the 302 form, the followClient second hop.
type releaseAssetsWire struct {
	mu   sync.Mutex
	hops []assetHop
}

func (w *releaseAssetsWire) record(r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hops = append(w.hops, assetHop{
		method: r.Method,
		path:   r.URL.Path,
		accept: r.Header.Get("Accept"),
		auth:   r.Header.Get("Authorization"),
	})
}

func (w *releaseAssetsWire) snapshot() []assetHop {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.hops)
}

// RunReleaseAssetsSuite tests the release-asset download contract: bytes
// reach w verbatim on both wire forms (direct 200 and 302 → token-less
// followClient hop), a zero-length body writes nothing, and a 404 lands in
// the provider NotFound chain with no bytes written.
func RunReleaseAssetsSuite(t *testing.T, h ReleaseAssetsHarness) {
	t.Run("DownloadReleaseAsset_DirectBytes", func(t *testing.T) { testAssetDirect(t, h) })
	t.Run("DownloadReleaseAsset_RedirectFollowed", func(t *testing.T) { testAssetRedirect(t, h) })
	t.Run("DownloadReleaseAsset_EmptyBody_WritesNothing", func(t *testing.T) { testAssetEmptyBody(t, h) })
	t.Run("DownloadReleaseAsset_404NotFound", func(t *testing.T) { testAsset404(t, h) })
}

// newAssetManager builds a provider against the given mock handler, then
// type-asserts the release-asset interface.
func newAssetManager(t *testing.T, h ReleaseAssetsHarness, handler http.HandlerFunc) provider.ReleaseAssetManager {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	rm, ok := p.(provider.ReleaseAssetManager)
	if !ok {
		t.Fatalf("%s does not implement provider.ReleaseAssetManager", h.Name)
	}
	return rm
}

// assetDirectHandler serves the asset bytes straight from the API endpoint —
// the no-redirect form. Site-verified (2C T2): a plain httptest 200-with-body
// satisfies DownloadReleaseAsset without any redirect, so the direct form is
// pinnable on the single stub.
func assetDirectHandler(rec *releaseAssetsWire, assetBytes string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		_, _ = w.Write([]byte(assetBytes))
	}
}

// assetRedirectHandler serves the 302 form real GitHub uses: the API asset
// endpoint answers a redirect, and the bytes live on assetSignedPath (same
// host — site-verified 2C T2: the redirect branch IS expressible on a single
// stub, so it is pinned rather than ruled out of scope).
func assetRedirectHandler(rec *releaseAssetsWire, assetBytes string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		if r.URL.Path == assetSignedPath {
			_, _ = w.Write([]byte(assetBytes))
			return
		}
		http.Redirect(w, r, assetSignedPath, http.StatusFound)
	}
}

// assetEmptyHandler answers 200 with a zero-length body.
func assetEmptyHandler(rec *releaseAssetsWire) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.WriteHeader(http.StatusOK)
	}
}

// asset404Handler answers the GitHub not-found shape.
func asset404Handler(rec *releaseAssetsWire) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}
}

// testAssetDirect pins the direct form: one GET on the asset endpoint with
// the octet-stream Accept header, and the fixture bytes land in w verbatim.
func testAssetDirect(t *testing.T, h ReleaseAssetsHarness) {
	t.Helper()
	rec := &releaseAssetsWire{}
	rm := newAssetManager(t, h, assetDirectHandler(rec, h.AssetBytes))
	var buf bytes.Buffer
	if err := rm.DownloadReleaseAsset(context.Background(), "owner", "repo", 99, &buf); err != nil {
		t.Fatalf("DownloadReleaseAsset: %v", err)
	}
	hops := rec.snapshot()
	if len(hops) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(hops))
	}
	hop := hops[0]
	if hop.method != http.MethodGet {
		t.Errorf("method = %s, want GET", hop.method)
	}
	if !strings.HasSuffix(hop.path, "/repos/owner/repo/releases/assets/99") {
		t.Errorf("path = %s, want .../repos/owner/repo/releases/assets/99", hop.path)
	}
	if hop.accept != "application/octet-stream" {
		t.Errorf("Accept = %q, want application/octet-stream (asset bytes, not JSON metadata)", hop.accept)
	}
	if buf.String() != h.AssetBytes {
		t.Errorf("w received %q (%d bytes), want the fixture bytes verbatim (%d bytes)",
			buf.String(), buf.Len(), len(h.AssetBytes))
	}
}

// testAssetRedirect pins the 302 form: the API hop answers a redirect, the
// p.followClient hop fetches the target WITHOUT API credentials (the whole
// reason the impl uses a separate client — issue #136's contract), and the
// bytes still land verbatim in w.
func testAssetRedirect(t *testing.T, h ReleaseAssetsHarness) {
	t.Helper()
	rec := &releaseAssetsWire{}
	rm := newAssetManager(t, h, assetRedirectHandler(rec, h.AssetBytes))
	var buf bytes.Buffer
	if err := rm.DownloadReleaseAsset(context.Background(), "owner", "repo", 99, &buf); err != nil {
		t.Fatalf("DownloadReleaseAsset (redirect form): %v", err)
	}
	if buf.String() != h.AssetBytes {
		t.Errorf("w received %q (%d bytes), want the fixture bytes verbatim (%d bytes)",
			buf.String(), buf.Len(), len(h.AssetBytes))
	}
	hops := rec.snapshot()
	if len(hops) != 2 {
		t.Fatalf("expected 2 hops (API → followClient), got %d", len(hops))
	}
	api := hops[0]
	if api.method != http.MethodGet || !strings.HasSuffix(api.path, "/repos/owner/repo/releases/assets/99") {
		t.Errorf("hop 1 = %s %s, want GET .../repos/owner/repo/releases/assets/99", api.method, api.path)
	}
	if api.accept != "application/octet-stream" {
		t.Errorf("hop 1 Accept = %q, want application/octet-stream", api.accept)
	}
	if api.auth == "" {
		t.Error("hop 1 must carry the API credentials (SDK channel), otherwise this pins a fake chain")
	}
	follow := hops[1]
	if follow.path != assetSignedPath {
		t.Errorf("hop 2 path = %s, want %s (followClient must fetch the redirect target)", follow.path, assetSignedPath)
	}
	if follow.auth != "" {
		t.Errorf("hop 2 Authorization = %q, want empty — the followClient hop must not carry API credentials", follow.auth)
	}
}

// testAssetEmptyBody pins the zero-length-body face: a 200 with no bytes
// must deliver nothing to w.
//
// Site-verified (2C T2): the error branch the provider documents
// (release_assets.go's rc==nil "empty asset response") is UNREACHABLE under
// go-github v93 — the SDK returns a nil rc only when followRedirectsClient
// is nil, and this provider always passes a non-nil followClient; a
// zero-length 200 therefore hands back a non-nil rc over a zero-length
// reader. Actual wire behavior: nil error, zero bytes written. That
// contradicts the iface_releases.go doc ("非 2xx 响应或无响应体时返回错误");
// the gap is reported for a separate fix ticket (zero production changes in
// this plan) and this subtest pins the CURRENT wire reality — if the fix
// lands, flip this assertion together with the doc.
func testAssetEmptyBody(t *testing.T, h ReleaseAssetsHarness) {
	t.Helper()
	rec := &releaseAssetsWire{}
	rm := newAssetManager(t, h, assetEmptyHandler(rec))
	var buf bytes.Buffer
	err := rm.DownloadReleaseAsset(context.Background(), "owner", "repo", 99, &buf)
	if err != nil {
		t.Errorf("current wire reality: 200-with-empty-body returns nil error (rc==nil guard unreachable under go-github v93), got %v — deliberate contract fix? flip this subtest together with the iface_releases.go doc", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty body must write 0 bytes to w, got %d (%q)", buf.Len(), buf.String())
	}
	if hops := rec.snapshot(); len(hops) != 1 {
		t.Errorf("expected exactly 1 request, got %d", len(hops))
	}
}

// testAsset404 pins the not-found classification: a 404 asset response must
// error into the provider NotFound chain and write no bytes.
func testAsset404(t *testing.T, h ReleaseAssetsHarness) {
	t.Helper()
	rec := &releaseAssetsWire{}
	rm := newAssetManager(t, h, asset404Handler(rec))
	var buf bytes.Buffer
	err := rm.DownloadReleaseAsset(context.Background(), "owner", "repo", 404, &buf)
	if err == nil {
		t.Fatal("expected an error on a 404 asset response")
	}
	if !provider.IsNotFound(err) {
		t.Errorf("404 must classify into the provider NotFound chain, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("no bytes may reach w on error, got %d (%q)", buf.Len(), buf.String())
	}
}
