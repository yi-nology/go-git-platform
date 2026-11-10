package contracttest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// LabelsHarnessConfig carries the fixtures a backend's main Harness needs to
// auto-mount the label-management suite via Harness.Labels. Name, Platform,
// and NewProvider are reused from the enclosing Harness.
type LabelsHarnessConfig struct {
	// ListResponse is the JSON array the mock returns for GET requests. Its
	// first item must have name "bug" and color "#4cc917".
	ListResponse string
	// MutateResponse is the JSON object the mock returns for POST/PATCH/PUT
	// requests. It must have name "bug" and color "#4cc917".
	MutateResponse string
	// IgnoresListPagination opts the backend out of the wire-level
	// pagination assertion (see LabelsHarness.IgnoresListPagination).
	IgnoresListPagination bool
}

// testLabelsSuite auto-mounts RunLabelsSuite from a main Harness, enforcing
// that the declared capability and the wired fixtures agree in both
// directions.
func testLabelsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Labels
	switch {
	case h.Labels == nil && !declared:
		t.Skipf("%s declares no Labels capability", h.Name)
	case h.Labels == nil:
		t.Errorf("%s declares Capabilities().Labels but its Harness provides no Labels config — the labels suite is not wired", h.Name)
	case !declared:
		t.Errorf("%s Harness provides a Labels config but the platform does not declare Capabilities().Labels", h.Name)
	default:
		RunLabelsSuite(t, LabelsHarness{
			Name:                  h.Name,
			Platform:              h.Platform,
			NewProvider:           h.NewProvider,
			ListResponse:          h.Labels.ListResponse,
			MutateResponse:        h.Labels.MutateResponse,
			IgnoresListPagination: h.Labels.IgnoresListPagination,
		})
	}
}

// LabelsHarness bundles the inputs needed to run the label-management
// contract suite against a backend that implements provider.LabelManager.
type LabelsHarness struct {
	// Name is the human-readable platform identifier (e.g. "GitHub").
	Name string
	// Platform is the provider.Platform constant for this backend.
	Platform provider.Platform
	// NewProvider builds a provider.Provider; the harness fills in BaseURL.
	// It must construct the provider the same way the platform's main
	// harness does (including any VersionProxy wrapping for Gitea/Forgejo).
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	// ListResponse is the JSON array the mock returns for GET requests
	// (label listings and name→ID resolution lookups). Its first item must
	// have name "bug" and color "#4cc917" so the suite can assert color
	// normalization.
	ListResponse string
	// MutateResponse is the JSON object the mock returns for POST/PATCH
	// requests. It must have name "bug" and color "#4cc917".
	MutateResponse string
	// IgnoresListPagination declares that the backend's list endpoint does
	// not accept pagination parameters, so the suite skips its wire-level
	// page/per-page assertion. Currently only GitCode sets this: its SDK's
	// ListIssueLabels exposes no page/page-size parameters, so ListLabels
	// silently drops ListLabelsOptions on the wire. That is a known contract
	// gap, not something the other five backends share.
	IgnoresListPagination bool
}

// recordedRequest captures one HTTP request observed by the labels mock:
// the method, the request path including query string, and the decoded body.
type recordedRequest struct {
	// Method is the HTTP verb of the request.
	Method string
	// Path is the request path with query string, e.g.
	// "/repos/owner/repo/labels?page=1&per_page=10".
	Path string
	// Body holds the decoded request body: a JSON object decoded into a
	// map, or a form-encoded body with each key mapped to its first value
	// (GitCode's create endpoint posts form data). It is nil when the
	// request carries no decodable body.
	Body map[string]any
}

// Query returns the recorded request's query parameters.
func (r recordedRequest) Query() url.Values {
	u, err := url.ParseRequestURI(r.Path)
	if err != nil {
		return url.Values{}
	}
	return u.Query()
}

// methods summarizes the recorded methods for failure messages.
func methodsOf(requests []recordedRequest) string {
	ms := make([]string, len(requests))
	for i, r := range requests {
		ms[i] = r.Method
	}
	return strings.Join(ms, ",")
}

// RunLabelsSuite executes the label-management contract suite. The mock
// server routes by HTTP method so platform-specific paths don't matter:
// GET returns ListResponse, POST returns 201 + MutateResponse, PATCH/PUT
// return 200 + MutateResponse, DELETE returns 204. The mock also records
// every request (method, path+query, decoded body) so the subtests can
// assert the wire behavior behind each operation: pagination parameters on
// list, the verb and body on create/update/delete, and that update does not
// clobber fields the caller left nil.
func RunLabelsSuite(t *testing.T, h LabelsHarness) {
	newLM := func(t *testing.T) (provider.LabelManager, *[]recordedRequest) {
		srv, requests := labelStubServer(h)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		lm, ok := p.(provider.LabelManager)
		if !ok {
			t.Fatalf("%s does not implement provider.LabelManager", h.Name)
		}
		return lm, requests
	}

	t.Run("List_NormalizesColor", func(t *testing.T) {
		lm, requests := newLM(t)
		labels, err := lm.ListLabels(context.Background(), "owner", "repo", provider.ListLabelsOptions{Page: 1, PerPage: 10})
		if err != nil {
			t.Fatalf("ListLabels: %v", err)
		}
		if len(labels) == 0 {
			t.Fatal("expected at least one label")
		}
		if labels[0].Name != "bug" {
			t.Errorf("expected first label name %q, got %q", "bug", labels[0].Name)
		}
		if labels[0].Color != "4cc917" {
			t.Errorf("expected normalized color %q, got %q — backends must strip the leading '#'", "4cc917", labels[0].Color)
		}
		assertListPaginationWire(t, h, requests)
	})
	// Page==0 dual-mode contract (issue #60): labels are an exhaustive-family
	// list — zero Page walks every page, Page>0 serves exactly that page.
	runPageZeroDualModeSuite(t, h.NewProvider, PageZeroFixture{
		Platform:  h.Platform,
		ListBody:  h.ListResponse,
		EmptyBody: `[]`,
		ListZero: func(p provider.Provider) error {
			_, err := p.(provider.LabelManager).ListLabels(context.Background(), "owner", "repo", provider.ListLabelsOptions{})
			return err
		},
		ListPageOne: func(p provider.Provider) error {
			_, err := p.(provider.LabelManager).ListLabels(context.Background(), "owner", "repo", provider.ListLabelsOptions{Page: 1, PerPage: 10})
			return err
		},
	})

	t.Run("Create_ReturnsLabel", func(t *testing.T) {
		lm, requests := newLM(t)
		l, err := lm.CreateLabel(context.Background(), "owner", "repo", provider.CreateLabelOptions{
			Name: "bug", Color: "4cc917", Description: "something broke",
		})
		if err != nil {
			t.Fatalf("CreateLabel: %v", err)
		}
		if l == nil || l.Name != "bug" {
			t.Fatalf("expected created label named bug, got %+v", l)
		}
		if l.Color != "4cc917" {
			t.Errorf("expected normalized color 4cc917, got %q", l.Color)
		}
		assertCreateWire(t, requests)
	})

	t.Run("Update_Succeeds", func(t *testing.T) {
		lm, requests := newLM(t)
		newName := "bug-2"
		l, err := lm.UpdateLabel(context.Background(), "owner", "repo", "bug", provider.UpdateLabelOptions{NewName: &newName})
		if err != nil {
			t.Fatalf("UpdateLabel: %v", err)
		}
		if l == nil {
			t.Fatal("expected updated label, got nil")
		}
		assertUpdateWire(t, requests, newName)
	})

	t.Run("Delete_Succeeds", func(t *testing.T) {
		lm, requests := newLM(t)
		if err := lm.DeleteLabel(context.Background(), "owner", "repo", "bug"); err != nil {
			t.Fatalf("DeleteLabel: %v", err)
		}
		assertDeleteWire(t, requests)
	})
}

// assertListPaginationWire asserts that the pagination options the suite
// passed (Page:1, PerPage:10) reached the server on the list request.
// Gitea/Forgejo spell the page-size parameter "limit" instead of
// "per_page", so both names are accepted. Backends whose list endpoint
// offers no pagination (h.IgnoresListPagination, currently GitCode) skip
// the check.
func assertListPaginationWire(t *testing.T, h LabelsHarness, requests *[]recordedRequest) {
	t.Helper()
	if len(*requests) == 0 {
		t.Fatal("expected the list request to reach the mock server")
	}
	if h.IgnoresListPagination {
		return
	}
	q := (*requests)[0].Query()
	if q.Get("page") == "" {
		t.Errorf("expected a non-empty %q query parameter, got request %q", "page", (*requests)[0].Path)
	}
	if q.Get("per_page") == "" && q.Get("limit") == "" {
		t.Errorf("expected a %q or %q query parameter, got request %q", "per_page", "limit", (*requests)[0].Path)
	}
}

// assertCreateWire asserts that the create reached the server as a POST
// carrying the label name in its body (key "name" on all six platforms;
// GitCode posts form-encoded, the rest JSON — decodeRecordedBody normalizes
// both).
func assertCreateWire(t *testing.T, requests *[]recordedRequest) {
	t.Helper()
	if len(*requests) == 0 {
		t.Fatal("expected the create request to reach the mock server")
	}
	req := (*requests)[len(*requests)-1]
	if req.Method != http.MethodPost {
		t.Errorf("expected create to use POST, got %s", req.Method)
	}
	if name, _ := req.Body["name"].(string); !strings.Contains(name, "bug") {
		t.Errorf("expected create body to carry name %q, got body %v", "bug", req.Body)
	}
}

// assertUpdateWire asserts that the update mutated via PATCH or PUT (GitLab
// uses PUT; the other five use PATCH — a POST would mean update was routed
// to the create endpoint), that the new name traveled in the body ("name" on
// five platforms, "new_name" on GitLab), and that fields the caller left
// nil were not clobbered: the body may carry JSON null color/description
// (the gitea/forgejo SDK's EditLabelOption marshals nil pointers without
// omitempty) but never a non-null value.
func assertUpdateWire(t *testing.T, requests *[]recordedRequest, wantName string) {
	t.Helper()
	var update *recordedRequest
	for i := range *requests {
		if r := &(*requests)[i]; r.Method == http.MethodPatch || r.Method == http.MethodPut {
			update = r
		}
	}
	if update == nil {
		t.Fatalf("expected a PATCH/PUT update request, recorded methods: %s", methodsOf(*requests))
	}
	var gotName string
	for _, key := range []string{"name", "new_name"} {
		if s, ok := update.Body[key].(string); ok {
			gotName = s
		}
	}
	if !strings.Contains(gotName, wantName) {
		t.Errorf("expected update body to carry new name %q under %q or %q, got body %v", wantName, "name", "new_name", update.Body)
	}
	for _, key := range []string{"color", "description"} {
		if v, ok := update.Body[key]; ok && v != nil {
			t.Errorf("update body carries non-nil %q (%v) although only the name was updated", key, v)
		}
	}
}

// assertDeleteWire asserts that the delete reached the server as a DELETE
// round trip (possibly after a name→ID resolution GET).
func assertDeleteWire(t *testing.T, requests *[]recordedRequest) {
	t.Helper()
	for _, r := range *requests {
		if r.Method == http.MethodDelete {
			return
		}
	}
	t.Errorf("expected a DELETE request, recorded methods: %s", methodsOf(*requests))
}

// labelStubServer returns a method-routed recording mock for the labels
// suite, along with the slice of requests it has recorded. Handler goroutines
// append under a mutex; the suite's calls are sequential, so subtests read
// the slice only after the operation under test has returned.
func labelStubServer(h LabelsHarness) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeRecordedBody(r)
		mu.Lock()
		requests = append(requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.RequestURI(),
			Body:   body,
		})
		mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			// The label listing now uses AllPages pagination; return
			// the fixture only on the first page so the loop
			// terminates cleanly on the second.
			if page, _ := strconv.Atoi(r.URL.Query().Get("page")); page > 1 {
				_, _ = w.Write([]byte(`[]`))
			} else {
				_, _ = w.Write([]byte(h.ListResponse))
			}
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(h.MutateResponse))
		case http.MethodPatch, http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(h.MutateResponse))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	return srv, &requests
}

// decodeRecordedBody decodes a request body into a generic map so
// assertions stay platform-agnostic: JSON object bodies decode directly,
// form-encoded bodies (GitCode's create endpoint) map each key to its first
// value. It returns nil for empty or undecodable bodies.
func decodeRecordedBody(r *http.Request) map[string]any {
	raw, err := io.ReadAll(r.Body)
	if err != nil || len(raw) == 0 {
		return nil
	}
	ct := r.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "application/json"):
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			return m
		}
	case strings.Contains(ct, "application/x-www-form-urlencoded"):
		if vals, err := url.ParseQuery(string(raw)); err == nil {
			m := make(map[string]any, len(vals))
			for k, v := range vals {
				if len(v) > 0 {
					m[k] = v[0]
				}
			}
			return m
		}
	}
	return nil
}
