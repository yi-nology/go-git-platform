package provider

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestProviderError_Error(t *testing.T) {
	tests := []struct {
		name string
		e    *ProviderError
		want string
	}{
		{
			"minimal",
			&ProviderError{Op: "ListRepos"},
			"ListRepos",
		},
		{
			"with platform",
			&ProviderError{Platform: PlatformGitHub, Op: "ListRepos"},
			"github ListRepos",
		},
		{
			"with resource",
			&ProviderError{Platform: PlatformGitHub, Op: "GetRepo", Resource: "owner/repo"},
			"github GetRepo owner/repo",
		},
		{
			"with status",
			&ProviderError{Platform: PlatformGitHub, Op: "GetRepo", Resource: "owner/repo", StatusCode: 404, Cause: ErrNotFound},
			"github GetRepo owner/repo: HTTP 404: resource not found",
		},
		{
			"with cause no status",
			&ProviderError{Platform: PlatformGitHub, Op: "TestConnection", Cause: errors.New("dial tcp: timeout")},
			"github TestConnection: dial tcp: timeout",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.e.Error()
			if got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProviderError_Is(t *testing.T) {
	pe := &ProviderError{Platform: PlatformGitHub, Op: "GetRepo", Cause: ErrNotFound}
	if !errors.Is(pe, ErrNotFound) {
		t.Error("expected errors.Is to match wrapped sentinel")
	}
	if errors.Is(pe, ErrAuthentication) {
		t.Error("expected not to match different sentinel")
	}
}

func TestProviderError_IsStatus(t *testing.T) {
	pe := &ProviderError{StatusCode: 404}
	if !pe.IsStatus(404) {
		t.Error("expected IsStatus(404) true")
	}
	if pe.IsStatus(500) {
		t.Error("expected IsStatus(500) false")
	}
}

func TestProviderError_ClassBoundaries(t *testing.T) {
	for _, c := range []struct {
		code   int
		client bool
		server bool
	}{
		{399, false, false},
		{400, true, false},
		{499, true, false},
		{500, false, true},
		{599, false, true},
		{600, false, false},
	} {
		pe := &ProviderError{StatusCode: c.code}
		if got := pe.IsClientError(); got != c.client {
			t.Errorf("status %d IsClientError=%v, want %v", c.code, got, c.client)
		}
		if got := pe.IsServerError(); got != c.server {
			t.Errorf("status %d IsServerError=%v, want %v", c.code, got, c.server)
		}
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrNotFound},
		{http.StatusUnauthorized, ErrAuthentication},
		{http.StatusForbidden, ErrForbidden},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusConflict, ErrConflict},
		{http.StatusInternalServerError, nil},
		{http.StatusBadGateway, nil},
		{418, nil},
	}
	for _, tc := range tests {
		err := New(PlatformGitHub, "Test", tc.status, "")
		if tc.want != nil && !errors.Is(err, tc.want) {
			t.Errorf("status %d: expected %v, got %v", tc.status, tc.want, err)
		}
	}
}

func TestNew_WithBody(t *testing.T) {
	err := New(PlatformGitHub, "Test", 404, "not found")
	if !errors.Is(err, ErrNotFound) {
		t.Error("expected ErrNotFound")
	}
	if got := err.Error(); !containsSubstr(got, "not found") {
		t.Errorf("expected body in error, got %q", got)
	}
}

func TestWrap(t *testing.T) {
	// Wrap preserves the cause chain
	cause := errors.New("boom")
	err := Wrap(PlatformGitHub, "GetRepo", cause)
	if !errors.Is(err, cause) {
		t.Error("expected cause to be preserved")
	}
	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Error("expected *ProviderError")
	}
	if pe.Op != "GetRepo" {
		t.Errorf("expected op GetRepo, got %q", pe.Op)
	}
}

func TestWrap_WithStatusCoder(t *testing.T) {
	statusErr := &fakeStatusError{code: 404}
	err := Wrap(PlatformGitHub, "GetRepo", statusErr)
	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Fatal("expected *ProviderError")
	}
	if pe.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", pe.StatusCode)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Error("expected ErrNotFound after classification")
	}
}

// TestWrap_PreservesOriginalCause verifies that classification no longer
// discards the platform's original error: the sentinel (via Is), the
// StatusError wrapper, and the raw SDK error must all remain matchable, and
// the message must retain the original detail.
func TestWrap_PreservesOriginalCause(t *testing.T) {
	orig := fmt.Errorf("GET /api/v4/projects/42: project not found")
	wrapped := WrapStatusError(orig, 404)
	err := Wrap(PlatformGitLab, "GetRepo", wrapped)

	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Fatal("expected *ProviderError")
	}
	if pe.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", pe.StatusCode)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Error("expected classification sentinel ErrNotFound to still match")
	}
	if !errors.Is(err, wrapped) {
		t.Error("expected the StatusError wrapper to remain in the chain")
	}
	if !errors.Is(err, orig) {
		t.Error("expected the original raw error to remain in the chain")
	}
	if !containsSubstr(err.Error(), "GET /api/v4/projects/42: project not found") {
		t.Errorf("expected original detail in message, got %q", err.Error())
	}
	if !containsSubstr(err.Error(), "HTTP 404") {
		t.Errorf("expected status in message, got %q", err.Error())
	}
}

func TestWrap_PreservesOriginalCause_5xx(t *testing.T) {
	// Classification runs through a registered StatusExtractor (the
	// reflection-over-fields fallback was removed in v0.83). A throwaway
	// platform value keeps the registry clean for other tests — the key
	// must be process-unique: the registry panics on duplicate
	// registration and go test -count=2 runs this test twice (issue #117).
	tp := uniqueTestPlatform("test-wrap-extractor")
	RegisterStatusExtractor(tp, func(err error) (int, bool) {
		var se *sdkStatusError
		if errors.As(err, &se) {
			return se.StatusCode, true
		}
		return 0, false
	})
	orig := fmt.Errorf("connection reset by peer mid-response")
	err := Wrap(tp, "ListRepos", &sdkStatusError{StatusCode: 502, msg: "upstream blew up", inner: orig})

	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Fatal("expected *ProviderError")
	}
	if pe.StatusCode != 502 {
		t.Errorf("expected status 502, got %d", pe.StatusCode)
	}
	if !errors.Is(err, orig) {
		t.Error("expected original cause preserved for 5xx classification")
	}
	if !containsSubstr(err.Error(), "connection reset by peer") {
		t.Errorf("expected original detail in message, got %q", err.Error())
	}
}

// sdkStatusError mimics a third-party SDK error: no StatusCode method, only
// an exported StatusCode int field plus a nested cause (same shape as e.g.
// gitlab client-go's ErrorResponse). Wrap classifies it only through a
// registered StatusExtractor — the reflection-over-fields fallback is gone.
type sdkStatusError struct {
	StatusCode int
	msg        string
	inner      error
}

func (e *sdkStatusError) Error() string {
	return fmt.Sprintf("sdk: HTTP %d: %s: %v", e.StatusCode, e.msg, e.inner)
}
func (e *sdkStatusError) Unwrap() error { return e.inner }

// TestWrap_StringMessage_NotMistakenForStatus: "merge with 500 files" used to
// be parsed as HTTP 500 ("with " prefix); it must now stay unclassified.
func TestWrap_StringMessage_NotMistakenForStatus(t *testing.T) {
	msgs := []string{
		"merge with 500 files",
		"merged 404 changes",
		"operation completed with 403 steps",
	}
	for _, msg := range msgs {
		err := Wrap(PlatformGitHub, "Merge", fmt.Errorf("%s", msg))
		var pe *ProviderError
		if !errors.As(err, &pe) {
			t.Fatalf("%q: expected *ProviderError", msg)
		}
		if pe.StatusCode != 0 {
			t.Errorf("%q: StatusCode = %d, want 0 (plain prose is not an HTTP status)", msg, pe.StatusCode)
		}
		if pe.IsServerError() || IsNotFound(pe) || IsForbidden(pe) {
			t.Errorf("%q: must not be classified as an HTTP failure", msg)
		}
		if !containsSubstr(err.Error(), msg) {
			t.Errorf("%q: message not preserved, got %q", msg, err.Error())
		}
	}
}

func TestWrap_StatusInMessageIsNeverParsed(t *testing.T) {
	// The pre-0.83 error-message parsing ("returned 404" / "HTTP 502" /
	// "status 422") is gone: a bare error whose text merely contains a
	// status number classifies as no-status. Platforms classify their SDK
	// errors through registered StatusExtractors instead.
	for _, msg := range []string{
		"returned 404",
		"HTTP 502 Bad Gateway",
		"status 422 Unprocessable",
		"request failed: returned 503",
	} {
		err := Wrap(PlatformGitHub, "TestOp", errors.New(msg))
		var pe *ProviderError
		if !errors.As(err, &pe) {
			t.Fatalf("expected *ProviderError for %q", msg)
		}
		if pe.StatusCode != 0 {
			t.Errorf("message %q classified as %d; status-in-message parsing must not resurrect", msg, pe.StatusCode)
		}
	}
}

func TestWrapf(t *testing.T) {
	err := Wrapf(PlatformGitHub, "GetRepo", "%s/%s", "owner", "repo")
	if !errors.Is(err, err) {
		t.Error("non-nil error should self-match")
	}
	if !containsSubstr(err.Error(), "owner/repo") {
		t.Errorf("expected formatted message, got %q", err.Error())
	}
}

func TestWrap_AlreadyWrapped(t *testing.T) {
	inner := Wrap(PlatformGitHub, "ListRepos", errors.New("boom"))
	outer := Wrap(PlatformGitHub, "ListRepos", inner)
	if outer != inner {
		t.Error("Wrap should be idempotent on identical (platform, op)")
	}
}

// TestClassifyStatus pins the full status→sentinel mapping (issue #64):
// 400/422 → ErrInvalidInput, 410 → ErrGone, 5xx → ErrServerSide, and the
// pre-existing 401/403/404/409/429 entries. Statuses outside the mapping
// must stay text-only (non-sentinel) so callers classify via IsStatus.
func TestClassifyStatus(t *testing.T) {
	allSentinels := []error{ErrInvalidInput, ErrGone, ErrServerSide, ErrNotFound, ErrAuthentication, ErrForbidden, ErrRateLimited, ErrConflict}
	tests := []struct {
		name   string
		status int
		want   error // sentinel expected via errors.Is; nil = plain text error
	}{
		{"400 bad request", http.StatusBadRequest, ErrInvalidInput},
		{"401 unauthorized", http.StatusUnauthorized, ErrAuthentication},
		{"403 forbidden", http.StatusForbidden, ErrForbidden},
		{"404 not found", http.StatusNotFound, ErrNotFound},
		{"409 conflict", http.StatusConflict, ErrConflict},
		{"410 gone", http.StatusGone, ErrGone},
		{"422 unprocessable", http.StatusUnprocessableEntity, ErrInvalidInput},
		{"429 rate limited", http.StatusTooManyRequests, ErrRateLimited},
		{"500 internal", http.StatusInternalServerError, ErrServerSide},
		{"502 bad gateway", http.StatusBadGateway, ErrServerSide},
		{"503 unavailable", http.StatusServiceUnavailable, ErrServerSide},
		{"599 edge of range", 599, ErrServerSide},
		{"418 teapot unmapped", http.StatusTeapot, nil},
		{"451 unmapped", http.StatusUnavailableForLegalReasons, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyStatus(tc.status)
			if tc.want == nil {
				if got == nil {
					t.Fatalf("status %d: expected a plain text error, got nil", tc.status)
				}
				for _, s := range allSentinels {
					if errors.Is(got, s) {
						t.Fatalf("status %d: expected a plain text error, got sentinel %v", tc.status, got)
					}
				}
				return
			}
			if !errors.Is(got, tc.want) {
				t.Fatalf("status %d: errors.Is(got=%v, want=%v) = false", tc.status, got, tc.want)
			}
		})
	}
}

// TestWrap_SentinelForValidationRejections pins issue #64's end-to-end
// contract: an HTTP 400/422 arriving from the wire (statusCoder cause) is
// classified as ErrInvalidInput by Wrap, and both IsInvalidInput and
// IsStatus report it — so callers can distinguish "fix the request" from
// "retry the server" with errors.Is alone.
func TestWrap_SentinelForValidationRejections(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		err := Wrap(PlatformGitLab, "CreateLabel", &fakeStatusError{code: status})
		var pe *ProviderError
		if !errors.As(err, &pe) {
			t.Fatalf("status %d: expected a *ProviderError, got %T", status, err)
		}
		if !IsInvalidInput(err) {
			t.Errorf("status %d: IsInvalidInput = false, want true", status)
		}
		if !pe.IsStatus(status) {
			t.Errorf("status %d: IsStatus lost the code", status)
		}
	}
	// 410 and 5xx carry their sentinels through Wrap the same way.
	if !IsGone(Wrap(PlatformGitHub, "GetRepo", &fakeStatusError{code: http.StatusGone})) {
		t.Error("410: IsGone = false, want true")
	}
	if !IsServerSide(Wrap(PlatformGitHub, "ListRepos", &fakeStatusError{code: http.StatusBadGateway})) {
		t.Error("502: IsServerSide = false, want true")
	}
}

// TestNew_ValidationShapes pins the locally-constructed "unified 422 shape"
// path (provider.New with a validation status): the sentinel must surface
// through errors.Is there too — gitea repo_template, github deployments and
// repo_extras all reject invalid local input via provider.New(..., 422/400).
func TestNew_ValidationShapes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"422 with body", http.StatusUnprocessableEntity, "name is invalid", ErrInvalidInput},
		{"422 no body", http.StatusUnprocessableEntity, "", ErrInvalidInput},
		{"400 with body", http.StatusBadRequest, `{"message":"bad"}`, ErrInvalidInput},
		{"410 no body", http.StatusGone, "", ErrGone},
		{"500 no body", http.StatusInternalServerError, "", ErrServerSide},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := New(PlatformGitHub, "CreateRepo", tc.status, tc.body)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("errors.Is(err, %v) = false, err=%v", tc.wantErr, err)
			}
			if tc.body != "" && !strings.Contains(err.Error(), tc.body) {
				t.Errorf("Error() must retain the platform body, got %q", err.Error())
			}
		})
	}
}

// fakeStatusError is a test double for the statusCoder interface.
type fakeStatusError struct {
	code int
}

func (e *fakeStatusError) Error() string   { return "fake" }
func (e *fakeStatusError) StatusCode() int { return e.code }
func (e *fakeStatusError) Unwrap() error   { return nil }

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestWrapStatusError(t *testing.T) {
	inner := fmt.Errorf("not found")
	err := WrapStatusError(inner, 404)
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	se, ok := err.(*StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if se.StatusCode() != 404 {
		t.Errorf("expected status 404, got %d", se.StatusCode())
	}
	if se.Unwrap() != inner {
		t.Error("expected Unwrap to return inner error")
	}
}

func TestWrapStatusError_Nil(t *testing.T) {
	err := WrapStatusError(nil, 404)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestWrapStatusError_InProviderError(t *testing.T) {
	inner := WrapStatusError(fmt.Errorf("gone"), 410)
	pe := Wrap(PlatformGitHub, "DeleteRepo", inner)

	var perr *ProviderError
	if !errors.As(pe, &perr) {
		t.Fatal("expected ProviderError")
	}
	if perr.StatusCode != 410 {
		t.Errorf("expected status 410, got %d", perr.StatusCode)
	}
	// 410 Gone is not a standard sentinel — verify it doesn't match NotFound.
	if IsNotFound(pe) {
		t.Error("410 Gone should not be classified as NotFound")
	}
}

func TestStatusError_ImplementsStatusCoder(t *testing.T) {
	se := &StatusError{Status: 429, Cause: fmt.Errorf("rate limited")}
	var sc statusCoder
	if !reflect.TypeOf(se).Implements(reflect.TypeOf(&sc).Elem()) {
		t.Error("StatusError should implement statusCoder interface")
	}
	if se.StatusCode() != 429 {
		t.Errorf("expected 429, got %d", se.StatusCode())
	}
}

func TestWrap_NonStructError_DoesNotPanic(t *testing.T) {
	// url.EscapeError is a string-kind error value; this is a regression
	// guard for the pre-0.83 reflection fallback, which panicked on
	// non-struct kinds (the reflection path no longer exists).
	err := url.EscapeError("invalid%escape")

	got := Wrap(PlatformGitHub, "TestOp", err)
	var pe *ProviderError
	if !errors.As(got, &pe) {
		t.Fatalf("expected *ProviderError, got %T", got)
	}
	if pe.StatusCode != 0 {
		t.Errorf("expected StatusCode 0 for a non-HTTP error, got %d", pe.StatusCode)
	}

	// Pointer form exercises the pointer-deref path into the same guard.
	got2 := Wrap(PlatformGitHub, "TestOp", &err)
	if !errors.As(got2, &pe) {
		t.Fatalf("expected *ProviderError for pointer form, got %T", got2)
	}
}
