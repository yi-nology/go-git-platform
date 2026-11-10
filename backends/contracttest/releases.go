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

// ReleasesHarnessConfig carries the fixtures a backend's main Harness needs
// to auto-mount the release-management suite via Harness.Releases.
//
// Unlike Labels/Issues/Reviews, ReleaseManager is a core interface composed
// into provider.Provider and implemented by every backend, so there is no
// capability-declaration drift to enforce: the config is mandatory for all
// seven platforms and testReleaseSuite fails the run when it is missing.
type ReleasesHarnessConfig struct {
	// ByTagResponse is the JSON object for single-release fetches. Required
	// shape: id 1, tag_name "v1.0.0", name "v1.0.0", body "release notes"
	// (the suite asserts the parsed TagName).
	ByTagResponse string
	// UpdateResponse is the JSON object for PATCH/PUT release updates (same
	// shape as ByTagResponse).
	UpdateResponse string
	// UpdateSendsDescriptionOnly declares that the platform's update
	// endpoint cannot carry a release name (TencentCode's update surface
	// only accepts a description): the update wire subtest then asserts the
	// "description" key instead of "name".
	UpdateSendsDescriptionOnly bool
	// TagsListResponse is the JSON array for the tags-list GET
	// (provider.ReleaseManager.ListTags). Empty = the harness has not wired
	// the tags endpoint yet: the ListTags subtest skips, the rest of the
	// suite is unaffected.
	TagsListResponse string
}

// ReleaseHarness is the full harness RunReleaseSuite consumes; auto-mounting
// builds it from the enclosing Harness plus ReleasesHarnessConfig.
type ReleaseHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	ReleasesHarnessConfig
}

// testReleaseSuite auto-mounts RunReleaseSuite from a main Harness. Because
// ReleaseManager is a core interface (every backend implements it), a missing
// Releases config is always a wiring error — there is no "platform opts out"
// direction to honor.
func testReleaseSuite(t *testing.T, h Harness) {
	if h.Releases == nil {
		t.Fatalf("%s provides no Releases config — ReleaseManager is a core interface, every backend must wire the release suite", h.Name)
	}
	RunReleaseSuite(t, ReleaseHarness{
		Name:                  h.Name,
		Platform:              h.Platform,
		NewProvider:           h.NewProvider,
		ReleasesHarnessConfig: *h.Releases,
	})
}

// RunReleaseSuite executes the release-management contract suite. The mock
// routes by method and path shape so platform-specific paths don't matter:
// GET ending at a releases collection (…/releases) returns a one-element
// array wrapping ByTagResponse (list-shaped); every other GET returns the
// ByTagResponse object — this covers both the native by-tag fetches
// (…/releases/tags/{tag} on GitHub/Gitea/Forgejo/Gitee/GitCode,
// …/releases/{tag} on GitLab/TencentCode) and the tag→id resolution fetches
// ID-addressed platforms make before update/delete, which go through the
// same single-release endpoints; PATCH/PUT return UpdateResponse; DELETE
// returns 200 + ByTagResponse — GitLab's delete responds with the deleted
// release object and its SDK decodes it, while the body-agnostic platforms
// ignore it (the shared transport skips decoding for nil results).
// Requests are recorded for wire assertions.
func RunReleaseSuite(t *testing.T, h ReleaseHarness) {
	newRM := func(t *testing.T) (provider.ReleaseManager, *[]recordedRequest) {
		srv, requests := releaseStubServer(h)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		rm, ok := p.(provider.ReleaseManager)
		if !ok {
			t.Fatalf("%s does not implement provider.ReleaseManager", h.Name)
		}
		return rm, requests
	}

	t.Run("GetByTag_Parses", func(t *testing.T) {
		rm, _ := newRM(t)
		assertReleaseGetByTag(t, rm)
	})
	t.Run("List_Parses", func(t *testing.T) {
		rm, _ := newRM(t)
		assertReleaseList(t, rm)
	})
	t.Run("ListTags_Parses", func(t *testing.T) {
		if h.TagsListResponse == "" {
			t.Skipf("%s harness wires no TagsListResponse fixture", h.Name)
		}
		rm, _ := newRM(t)
		assertReleaseListTags(t, rm)
	})
	t.Run("Update_Mutates", func(t *testing.T) {
		rm, requests := newRM(t)
		assertReleaseUpdateWire(t, rm, requests, h.UpdateSendsDescriptionOnly)
	})
	t.Run("Delete_Deletes", func(t *testing.T) {
		rm, requests := newRM(t)
		assertReleaseDeleteWire(t, rm, requests)
	})
}

// assertReleaseGetByTag checks that GetReleaseByTag returns the fixture
// release with the requested tag.
func assertReleaseGetByTag(t *testing.T, rm provider.ReleaseManager) {
	t.Helper()
	rel, err := rm.GetReleaseByTag(context.Background(), "owner", "repo", "v1.0.0")
	if err != nil {
		t.Fatalf("GetReleaseByTag: %v", err)
	}
	if rel == nil {
		t.Fatal("expected a release, got nil")
	}
	if rel.TagName != "v1.0.0" {
		t.Errorf("expected tag %q, got %q", "v1.0.0", rel.TagName)
	}
}

// assertReleaseUpdateWire checks that UpdateRelease succeeds and reaches the
// server with a state-changing verb (PATCH or PUT — GitLab and TencentCode
// PUT) whose body carries the new release name. UpdateSendsDescriptionOnly
// platforms assert the body/description key instead, since their update
// surface cannot carry a name (registered limitation).
func assertReleaseUpdateWire(t *testing.T, rm provider.ReleaseManager, requests *[]recordedRequest, descriptionOnly bool) {
	t.Helper()
	newName, newBody := "v1.0.0-renamed", "updated notes"
	rel, err := rm.UpdateRelease(context.Background(), "owner", "repo", "v1.0.0", provider.UpdateReleaseOptions{
		Name: &newName,
		Body: &newBody,
	})
	if err != nil || rel == nil {
		t.Fatalf("UpdateRelease: result=%v err=%v", rel, err)
	}
	wantKey, wantVal := "name", newName
	if descriptionOnly {
		wantKey, wantVal = "description", newBody
	}
	for i := range *requests {
		r := &(*requests)[i]
		if r.Method != http.MethodPatch && r.Method != http.MethodPut {
			continue
		}
		if v, ok := r.Body[wantKey]; ok && v == wantVal {
			return
		}
	}
	t.Errorf("expected a PATCH/PUT whose body carries %s=%q, recorded: %v", wantKey, wantVal, *requests)
}

// assertReleaseDeleteWire checks that DeleteRelease succeeds and reached the
// server with a DELETE (every platform's release delete is a DELETE; a
// missing or different verb means the delete never happened).
func assertReleaseDeleteWire(t *testing.T, rm provider.ReleaseManager, requests *[]recordedRequest) {
	t.Helper()
	if err := rm.DeleteRelease(context.Background(), "owner", "repo", "v1.0.0"); err != nil {
		t.Fatalf("DeleteRelease: %v", err)
	}
	for i := range *requests {
		if (*requests)[i].Method == http.MethodDelete {
			return
		}
	}
	t.Errorf("expected a DELETE request, recorded %s", methodsOf(*requests))
}

// releaseStubServer returns the method/shape-routed recording mock; see
// RunReleaseSuite for the routing contract.
func releaseStubServer(h ReleaseHarness) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeRecordedBody(r)
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.RequestURI(), Body: body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(h.ByTagResponse))
		case r.Method == http.MethodPatch || r.Method == http.MethodPut:
			_, _ = w.Write([]byte(h.UpdateResponse))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/tags"):
			_, _ = w.Write([]byte(h.TagsListResponse))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releases"):
			_, _ = w.Write([]byte("[" + h.ByTagResponse + "]"))
		default:
			_, _ = w.Write([]byte(h.ByTagResponse))
		}
	}))
	return srv, &requests
}

// assertReleaseList checks that ListReleases parses the fixture release
// array and surfaces TagName on the provider shape.
func assertReleaseList(t *testing.T, rm provider.ReleaseManager) {
	t.Helper()
	rels, err := rm.ListReleases(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 release, got %d", len(rels))
	}
	if rels[0].TagName != "v1.0.0" {
		t.Errorf("expected TagName %q, got %q", "v1.0.0", rels[0].TagName)
	}
}

// assertReleaseListTags checks that ListTags parses the tags fixture into
// TagInfo{Name, Commit} on the provider shape.
func assertReleaseListTags(t *testing.T, rm provider.ReleaseManager) {
	t.Helper()
	tags, err := rm.ListTags(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Name != "v1" {
		t.Errorf("expected tag name %q, got %q", "v1", tags[0].Name)
	}
	if tags[0].Commit != "abc123def456" {
		t.Errorf("expected commit sha %q, got %q", "abc123def456", tags[0].Commit)
	}
}
