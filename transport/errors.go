package transport

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/oekit/concurrency/retry/httpx"
)

// NewStatusError builds an error for an HTTP response with status >= 400. The
// returned error is an *Error with the request method, path, status code and
// response body captured for diagnostics.
func NewStatusError(method, path string, status int, body []byte) error {
	return &Error{
		Method:     method,
		Path:       path,
		statusCode: status,
		Body:       body,
	}
}

// NewStatusErrorWithHeaders is NewStatusError plus the response headers,
// from which rate-limit recovery hints are extracted onto the error:
// Retry-After (RFC 9110 §10.2.3) becomes Error.RetryAfter, and
// X-RateLimit-Reset (unix seconds, the GitHub/GitLab/Gitea convention)
// becomes Error.ResetAt.
func NewStatusErrorWithHeaders(method, path string, status int, body []byte, header http.Header) error {
	e := &Error{
		Method:     method,
		Path:       path,
		statusCode: status,
		Body:       body,
	}
	if header == nil {
		return e
	}
	if ra := header.Get("Retry-After"); ra != "" {
		if d, ok := parseRetryAfter(ra, time.Now()); ok {
			e.RetryAfter = d
		}
	}
	if v := header.Get("X-RateLimit-Reset"); v != "" {
		if sec, err := strconv.ParseInt(v, 10, 64); err == nil && sec > 0 {
			e.ResetAt = time.Unix(sec, 0)
		}
	}
	if status == http.StatusForbidden && isRateLimitedStatus(status, header) {
		e.rateLimited403 = true
	}
	return e
}

// isRateLimitedStatus is the single rate-limit predicate for the transport
// package: it classifies the HTTP exchange (status + response headers) so
// error construction (NewStatusErrorWithHeaders) and retry gating
// (RetryConfig.canRetryResponse) cannot drift apart. It reports:
//
//   - 429, always;
//   - 403 carrying GitHub-style throttle headers: X-RateLimit-Remaining: 0
//     (primary limit) or a Retry-After header (secondary limits / abuse
//     detection).
//
// A bare 403 is an authorization failure, not throttling — retrying it can
// never succeed and would only amplify load.
//
// Delegated to httpx.RetryableWithHeaders (oekit/concurrency#22): 429/5xx
// retryable, 403 retryable only with Retry-After or X-RateLimit-Remaining
// exactly "0". Header values arrive textproto-parsed (OWS already trimmed
// per RFC 7230), matching httpx's exact comparison; the transport kept the
// wrapper so the single-predicate contract above stays in one place.
func isRateLimitedStatus(status int, h http.Header) bool {
	return httpx.RetryableWithHeaders(status, h)
}

// Error is the structured error returned by Client when a request completes
// with a non-2xx status code, or when the underlying transport fails. It
// implements errors.Is / errors.As so callers can branch on either the status
// class (via IsStatusClass) or the wrapped cause.
//
// The status code is exposed through the StatusCode() method rather than an
// exported field so that *Error satisfies the statusCoder interface
// (StatusCode() int) consumed by provider.Wrap — that interface check is what
// removes the reflection-based fallback there. A struct cannot have a field
// and a method with the same name, hence the unexported storage field.
type Error struct {
	Method string
	Path   string
	// statusCode is the HTTP status code. Read it via StatusCode().
	statusCode int
	Body       []byte
	Cause      error
	// RetryAfter, when non-zero, is the server-advised wait before the
	// request may be retried (Retry-After header). Populated by
	// NewStatusErrorWithHeaders.
	RetryAfter time.Duration
	// ResetAt, when non-zero, is when the rate-limit window reopens
	// (X-RateLimit-Reset, unix seconds). Populated by
	// NewStatusErrorWithHeaders.
	ResetAt time.Time
	// rateLimited403 records a 403 classified as throttling by
	// isRateLimitedStatus — how GitHub and Gitea signal an exhausted or
	// abuse-detected quota without using 429.
	rateLimited403 bool
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("transport: %s %s: %d: %v", e.Method, e.Path, e.statusCode, e.Cause)
	}
	return fmt.Sprintf("transport: %s %s: %d: %s", e.Method, e.Path, e.statusCode, truncate(e.Body, 200))
}

// Unwrap implements errors.Unwrap so callers can recover the underlying cause.
func (e *Error) Unwrap() error { return e.Cause }

// Is implements errors.Is. Two *Error values match when they share the same
// status code, which is the most useful predicate for callers that branch on
// error class. Identity comparison still works via errors.Is's default path
// (same pointer).
func (e *Error) Is(target error) bool {
	if t, ok := target.(*Error); ok {
		return e.statusCode == t.statusCode
	}
	return false
}

// StatusCode returns the HTTP status code of the failed response. It makes
// *Error satisfy the statusCoder-style interface (StatusCode() int) used by
// provider.Wrap, so the fast interface path matches *transport.Error directly
// and no reflection fallback is needed.
func (e *Error) StatusCode() int { return e.statusCode }

// IsStatus reports whether the error has the given status code.
func (e *Error) IsStatus(code int) bool { return e.statusCode == code }

// IsStatusClass reports whether the error is in the given status class. For
// example IsStatusClass(http.StatusInternalServerError) reports 5xx.
func (e *Error) IsStatusClass(lo int) bool {
	return e.statusCode >= lo && e.statusCode < lo+100
}

// IsClientError reports 4xx.
func (e *Error) IsClientError() bool { return e.IsStatusClass(http.StatusBadRequest) }

// IsServerError reports 5xx.
func (e *Error) IsServerError() bool { return e.IsStatusClass(http.StatusInternalServerError) }

// IsRateLimited reports whether the error is a rate-limit rejection: HTTP
// 429, or a 403 carrying throttle headers (X-RateLimit-Remaining: 0, or a
// Retry-After alone — how GitHub and Gitea signal exhausted quotas and
// secondary limits without using 429). RetryAfter/ResetAt, when set, say
// when the window reopens.
func (e *Error) IsRateLimited() bool {
	return e.statusCode == http.StatusTooManyRequests || e.rateLimited403
}

// RateLimitInfo exposes the server-advised recovery window for
// rate-limited errors. Both values are zero when the server sent no
// hints (or the error is not rate-limit shaped). The provider package
// consumes this through the interface shape; it is also handy directly.
func (e *Error) RateLimitInfo() (retryAfter time.Duration, resetAt time.Time) {
	return e.RetryAfter, e.ResetAt
}

// ErrEmptyResponse is returned by DoJSON when the server returned a 2xx with
// no body and the caller asked for a non-nil result.
var ErrEmptyResponse = fmt.Errorf("transport: empty response body")

// truncate returns s shortened to at most n bytes. It is used to keep error
// messages bounded when the response body is large.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}
