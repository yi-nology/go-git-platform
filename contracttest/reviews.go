package contracttest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ReviewsHarnessConfig carries the fixtures a backend's main Harness needs to
// auto-mount the review-management suite via Harness.Reviews.
type ReviewsHarnessConfig struct {
	// ListResponse is the JSON array for review-list GETs. First item: id 1,
	// user login "dev", state "APPROVED" (the suite asserts normalization to
	// model.ReviewStateApproved).
	ListResponse string
	// GetResponse is the JSON object for single-review GETs (id 1, user
	// "dev", state "APPROVED").
	GetResponse string
	// MutateResponse is the JSON object for POST/PUT/PATCH (same shape as
	// GetResponse).
	MutateResponse string
	// CreateEvent is the wire value the platform's create must carry under
	// the "event" key for the suite's APPROVE-verdict create. The suite
	// calls CreateReview with Event "APPROVE": GitHub and GitCode forward
	// the option verbatim ("APPROVE"), Gitea/Forgejo translate it to their
	// SDK's review state ("APPROVED"). Empty means the platform's create
	// carries no verdict on the wire (GitLab's note-based create) and the
	// event-key assertion is skipped — the body assertion still runs.
	CreateEvent string
	// IgnoresRequestReviewers opts the backend out of the RequestReviewers
	// wire assertion (see ReviewsHarness.IgnoresRequestReviewers).
	IgnoresRequestReviewers bool
	// RequestReviewersByID declares that the backend resolves reviewer
	// usernames to numeric user IDs through a /users lookup before writing
	// them (GitLab resolves usernames to IDs first): the wire subtest then
	// asserts the lookup GET and an ID-carrying update body instead of the
	// username-carrying create (see assertRequestReviewersByIDWire).
	RequestReviewersByID bool
	// IgnoresDismissal opts the backend out of the DismissReview verb
	// assertion (see ReviewsHarness.IgnoresDismissal). It is for platforms
	// whose ONLY dismissal gap is a registered stub: the platform exposes no
	// dismiss surface at all, so DismissReview returns a wrapped
	// provider.ErrNotImplemented and the subtest asserts that registration
	// instead of a state-changing verb.
	IgnoresDismissal bool
	// ListStateIsCommented declares that the platform's review reads cannot
	// carry a verdict state (see ReviewsHarness.ListStateIsCommented).
	ListStateIsCommented bool
}

// ReviewsHarness is the full harness RunReviewsSuite consumes; auto-mounting
// builds it from the enclosing Harness plus ReviewsHarnessConfig.
type ReviewsHarness struct {
	Name        string
	Platform    model.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	ReviewsHarnessConfig
	// IgnoresRequestReviewers declares that the backend's RequestReviewers is
	// a registered ignore: the platform's reviewer API needs inputs the SDK
	// surface cannot supply (GitLab's reviewer_ids want user IDs, but the SDK
	// addresses reviewers by username and exposes no Users API to resolve
	// them), so the method documents the ignore and performs no request. The
	// wire subtest then only asserts a silent, error-free no-op — anything
	// reaching the network would mean the registration drifted.
	IgnoresRequestReviewers bool
	// IgnoresDismissal declares that the backend's DismissReview is a
	// registered stub: the platform's review surface has no dismissal
	// endpoint at all (Tencent 工蜂 review notes expose no dismiss verb), so
	// the method documents the gap and returns a provider error wrapping
	// provider.ErrNotImplemented without touching the wire. The subtest then
	// asserts exactly that registration — a successful call or any recorded
	// request would mean the stub drifted.
	IgnoresDismissal bool
	// ListStateIsCommented declares that the backend's review reads carry no
	// verdict state: the SDK model behind ListReviews/GetReview has no state
	// field (Tencent 工蜂 review notes — the verdict travels only on the
	// create/update writes as reviewer_state and never comes back), so every
	// read review normalizes to model.ReviewStateCommented, a registered
	// limitation. The List subtest then asserts that commented normalization
	// instead of the default approved one.
	ListStateIsCommented bool
}

// testReviewsSuite auto-mounts RunReviewsSuite from a main Harness with the
// same bidirectional drift checks as the labels and issues suites.
func testReviewsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Reviews
	switch {
	case h.Reviews == nil && !declared:
		t.Skipf("%s declares no Reviews capability", h.Name)
	case h.Reviews == nil:
		t.Errorf("%s declares Capabilities().Reviews but its Harness provides no Reviews config — the reviews suite is not wired", h.Name)
	case !declared:
		t.Errorf("%s Harness provides a Reviews config but the platform does not declare Capabilities().Reviews", h.Name)
	default:
		RunReviewsSuite(t, ReviewsHarness{
			Name:                    h.Name,
			Platform:                h.Platform,
			NewProvider:             h.NewProvider,
			ReviewsHarnessConfig:    *h.Reviews,
			IgnoresRequestReviewers: h.Reviews.IgnoresRequestReviewers,
			IgnoresDismissal:        h.Reviews.IgnoresDismissal,
			ListStateIsCommented:    h.Reviews.ListStateIsCommented,
		})
	}
}

// RunReviewsSuite executes the review-management contract suite. The mock
// routes by method and path shape so platform-specific paths don't matter:
// GET ending /reviews/{digits} or /notes/{digits} → GetResponse; other GET →
// ListResponse; POST → 201 + MutateResponse; PUT/PATCH → MutateResponse
// (GitHub's dismiss is a PUT to a .../dismissals sub-resource); DELETE → 204.
// Requests are recorded for wire assertions.
func RunReviewsSuite(t *testing.T, h ReviewsHarness) {
	newRM := func(t *testing.T) (provider.ReviewManager, *[]recordedRequest) {
		srv, requests := reviewStubServer(h)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		rm, ok := p.(provider.ReviewManager)
		if !ok {
			t.Fatalf("%s does not implement provider.ReviewManager", h.Name)
		}
		return rm, requests
	}

	t.Run("List_ParsesAndNormalizes", func(t *testing.T) {
		rm, _ := newRM(t)
		assertReviewListNormalized(t, rm, h.ListStateIsCommented)
	})
	t.Run("Get_ReturnsReview", func(t *testing.T) {
		rm, _ := newRM(t)
		assertReviewGet(t, rm)
	})
	t.Run("Create_PostsBody", func(t *testing.T) {
		rm, requests := newRM(t)
		assertReviewCreateWire(t, rm, requests, h.CreateEvent)
	})
	t.Run("RequestReviewers_PostsReviewers", func(t *testing.T) {
		rm, requests := newRM(t)
		if h.IgnoresRequestReviewers {
			assertRequestReviewersIgnored(t, rm, requests)
			return
		}
		if h.RequestReviewersByID {
			assertRequestReviewersByIDWire(t, rm, requests)
			return
		}
		assertRequestReviewersWire(t, rm, requests)
	})
	t.Run("Dismiss_NotGet", func(t *testing.T) {
		rm, requests := newRM(t)
		if h.IgnoresDismissal {
			assertReviewDismissStubbed(t, rm, requests)
			return
		}
		assertReviewDismissWire(t, rm, requests)
	})
}

// assertReviewListNormalized checks that ListReviews returns parsed,
// normalized reviews: id 1, user "dev", and the UPPERCASE wire state
// "APPROVED" normalized to model.ReviewStateApproved. Platforms whose
// review reads carry no verdict state (commentedList; a registered
// limitation — see ReviewsHarness.ListStateIsCommented) assert the
// commented normalization instead.
func assertReviewListNormalized(t *testing.T, rm provider.ReviewManager, commentedList bool) {
	t.Helper()
	reviews, err := rm.ListReviews(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(reviews) == 0 {
		t.Fatal("expected at least one review")
	}
	if reviews[0].ID != 1 {
		t.Errorf("expected first review id 1, got %d", reviews[0].ID)
	}
	if reviews[0].User != "dev" {
		t.Errorf("expected review user %q, got %q", "dev", reviews[0].User)
	}
	wantState := model.ReviewStateApproved
	if commentedList {
		wantState = model.ReviewStateCommented
	}
	if reviews[0].State != wantState {
		t.Errorf("expected normalized state %q, got %q", wantState, reviews[0].State)
	}
}

// assertReviewGet checks that GetReview returns the fixture review.
func assertReviewGet(t *testing.T, rm provider.ReviewManager) {
	t.Helper()
	review, err := rm.GetReview(context.Background(), "owner", "repo", "1", 1)
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}
	if review == nil || review.ID != 1 {
		t.Fatalf("expected review id 1, got %+v", review)
	}
}

// assertReviewCreateWire checks that CreateReview succeeds and posts a
// review-ish payload: the summary body under the "body" key (the key GitHub
// and GitHub-shaped platforms send), and — unless createEvent is empty — an
// "event" key carrying exactly createEvent for the suite's APPROVE-verdict
// create. The value must match exactly (not substring): "APPROVE" and
// "APPROVED" are different platforms' wire vocabularies and must not pass for
// each other. An empty createEvent (GitLab's note-based create has no
// verdict on the wire) skips only the event assertion.
func assertReviewCreateWire(t *testing.T, rm provider.ReviewManager, requests *[]recordedRequest, createEvent string) {
	t.Helper()
	result, err := rm.CreateReview(context.Background(), "owner", "repo", "1", provider.CreateReviewOptions{Body: "looks good", Event: "APPROVE"})
	if err != nil || result == nil {
		t.Fatalf("CreateReview: result=%v err=%v", result, err)
	}
	assertBodyHas(t, requests, http.MethodPost, "body", "looks good")
	if createEvent == "" {
		return
	}
	for i := range *requests {
		r := &(*requests)[i]
		if r.Method != http.MethodPost {
			continue
		}
		if v, ok := r.Body["event"]; ok {
			if s, ok := v.(string); ok && s == createEvent {
				return
			}
		}
	}
	t.Errorf("expected a POST whose body carries event=%q, recorded: %v", createEvent, *requests)
}

// assertRequestReviewersWire checks that RequestReviewers posts the reviewer
// logins under the "reviewers" key. The value is a JSON array, so the string
// assertion in assertBodyHas does not apply.
func assertRequestReviewersWire(t *testing.T, rm provider.ReviewManager, requests *[]recordedRequest) {
	t.Helper()
	if err := rm.RequestReviewers(context.Background(), "owner", "repo", "1", []string{"dev"}); err != nil {
		t.Fatalf("RequestReviewers: %v", err)
	}
	if !sawMethod(*requests, http.MethodPost) {
		t.Fatalf("expected a POST request, recorded %s", methodsOf(*requests))
	}
	for i := range *requests {
		r := &(*requests)[i]
		if r.Method != http.MethodPost {
			continue
		}
		if v, ok := r.Body["reviewers"]; ok {
			if items, ok := v.([]any); ok && len(items) > 0 {
				for _, item := range items {
					if s, ok := item.(string); ok && strings.Contains(s, "dev") {
						return
					}
				}
			}
		}
	}
	t.Errorf("expected a POST whose body carries reviewers=[\"dev\"], recorded: %v", *requests)
}

// assertRequestReviewersIgnored checks a registered-ignore RequestReviewers:
// the call must succeed and stay completely off the wire (GitLab; see
// ReviewsHarness.IgnoresRequestReviewers). Any recorded request means the
// ignore registration has drifted from the implementation.
func assertRequestReviewersIgnored(t *testing.T, rm provider.ReviewManager, requests *[]recordedRequest) {
	t.Helper()
	if err := rm.RequestReviewers(context.Background(), "owner", "repo", "1", []string{"dev"}); err != nil {
		t.Fatalf("RequestReviewers (registered ignore): %v", err)
	}
	if len(*requests) != 0 {
		t.Errorf("expected the registered ignore to make no HTTP requests, recorded %s", methodsOf(*requests))
	}
}

// assertRequestReviewersByIDWire checks a reviewer request that resolves
// usernames through a /users lookup and writes numeric reviewer IDs (GitLab).
func assertRequestReviewersByIDWire(t *testing.T, rm provider.ReviewManager, requests *[]recordedRequest) {
	t.Helper()
	if err := rm.RequestReviewers(context.Background(), "owner", "repo", "1", []string{"dev"}); err != nil {
		t.Fatalf("RequestReviewers: %v", err)
	}
	var putBody string
	sawUsersLookup := false
	for _, req := range *requests {
		// The recorded path carries the query string (RequestURI), so the
		// lookup arrives as "/api/v4/users?username=dev"; strip the query
		// before the suffix check.
		path, _, _ := strings.Cut(req.Path, "?")
		if strings.HasSuffix(path, "/users") {
			sawUsersLookup = true
			continue
		}
		if req.Method == "PUT" || req.Method == "PATCH" {
			b, _ := json.Marshal(req.Body)
			putBody = string(b)
		}
	}
	if !sawUsersLookup {
		t.Error("expected a /users username lookup before writing reviewers")
	}
	if !strings.Contains(putBody, "reviewer_ids") || !strings.Contains(putBody, "101") {
		t.Errorf("merge-request update body = %q, want reviewer_ids containing resolved id 101", putBody)
	}
}

// assertReviewDismissStubbed checks a registered-stub DismissReview: the
// call must fail with an error wrapping provider.ErrNotImplemented (the
// platform exposes no dismissal surface; see
// ReviewsHarness.IgnoresDismissal) and must stay completely off the wire.
// A nil error or any recorded request means the stub registration has
// drifted from the implementation.
func assertReviewDismissStubbed(t *testing.T, rm provider.ReviewManager, requests *[]recordedRequest) {
	t.Helper()
	err := rm.DismissReview(context.Background(), "owner", "repo", "1", 1, "stale review")
	if err == nil {
		t.Fatal("expected the registered stub to return an error, got nil")
	}
	if !provider.IsNotImplemented(err) {
		t.Fatalf("expected the registered stub to wrap provider.ErrNotImplemented, got %v", err)
	}
	if len(*requests) != 0 {
		t.Errorf("expected the registered stub to make no HTTP requests, recorded %s", methodsOf(*requests))
	}
}

// assertReviewDismissWire checks that DismissReview reached the server with a
// state-changing verb. Platforms differ: GitHub PUTs a dismissal sub-resource,
// others DELETE the review. A GET-only recording means the dismissal never
// mutated anything.
func assertReviewDismissWire(t *testing.T, rm provider.ReviewManager, requests *[]recordedRequest) {
	t.Helper()
	if err := rm.DismissReview(context.Background(), "owner", "repo", "1", 1, "stale review"); err != nil {
		t.Fatalf("DismissReview: %v", err)
	}
	if len(*requests) == 0 {
		t.Fatal("expected the dismissal to reach the mock server")
	}
	for i := range *requests {
		if (*requests)[i].Method != http.MethodGet {
			return
		}
	}
	t.Errorf("expected a non-GET dismissal request, recorded %s", methodsOf(*requests))
}

// reviewPathID matches a path that addresses a single review or a single
// review-bearing note, e.g. "/repos/owner/repo/pulls/1/reviews/42" (GitHub,
// Gitea) or "/api/v3/projects/1/merge_requests/1/notes/42" (Tencent 工蜂's
// note-based reviews). No platform's review-LIST GET ends in either shape,
// so both route to the single-review fixture.
var reviewPathID = regexp.MustCompile(`/(reviews|notes)/\d+$`)

// reviewStubServer returns the method/shape-routed recording mock.
func reviewStubServer(h ReviewsHarness) (*httptest.Server, *[]recordedRequest) {
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
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users"):
			// Username→ID resolution lookup (GitLab's ListUsers with the
			// exact-match username filter): echo the query username back
			// under a fixed numeric ID.
			resp, _ := json.Marshal([]map[string]any{{"id": 101, "username": r.URL.Query().Get("username")}})
			_, _ = w.Write(resp)
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(h.MutateResponse))
		case r.Method == http.MethodPatch || r.Method == http.MethodPut:
			_, _ = w.Write([]byte(h.MutateResponse))
		case r.Method == http.MethodGet && reviewPathID.MatchString(r.URL.Path):
			_, _ = w.Write([]byte(h.GetResponse))
		default:
			// Page-aware list: AllPages-driven backends probe page 2; serve
			// an empty terminal page so the loop stops.
			if r.Method == http.MethodGet && beyondFirstPage(r) {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(h.ListResponse))
		}
	}))
	return srv, &requests
}
