package provider

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
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
	// platform value keeps the registry clean for other tests.
	const tp Platform = "test-wrap-extractor"
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

func TestClassifyStatus(t *testing.T) {
	if !errors.Is(ClassifyStatus(404), ErrNotFound) {
		t.Error("expected 404 to be ErrNotFound")
	}
	// 5xx returns a non-sentinel error; just verify no panic.
	_ = ClassifyStatus(502)
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
