package provider

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Sentinel errors classify transport and provider failures so callers can
// branch on category without inspecting status codes or wrapped causes.
var (
	ErrNotFound             = errors.New("resource not found")
	ErrAuthentication       = errors.New("authentication failed")
	ErrRateLimited          = errors.New("rate limited")
	ErrForbidden            = errors.New("forbidden")
	ErrConflict             = errors.New("conflict")
	ErrNotImplemented       = errors.New("not implemented")
	ErrInvalidInput         = errors.New("invalid input")
	ErrWebhookValidation    = errors.New("webhook validation failed")
	ErrConnectionFailed     = errors.New("connection failed")
	ErrPlatformNotSupported = errors.New("platform not supported")
)

// ProviderError is a structured error from a provider operation. It carries
// enough context (platform, op, resource, status code, cause) to support
// logging, retry, and user-facing messages. It implements errors.Is so the
// sentinel errors above are matched by Cause, and errors.As for typed access.
type ProviderError struct {
	Platform   Platform
	Op         string // operation name, e.g., "ListRepos"
	Resource   string // optional resource identifier, e.g., "owner/repo"
	StatusCode int    // 0 when not applicable (e.g. configuration errors)
	// RetryAfter / ResetAt carry the server-advised recovery window when
	// the error is a rate-limit rejection and the platform sent hints
	// (Retry-After header; X-RateLimit-Reset unix seconds). Zero when
	// absent. Copied from the underlying transport error by Wrap; see
	// RateLimitRecovery for the ergonomic accessor.
	RetryAfter time.Duration
	ResetAt    time.Time
	Cause      error // underlying cause; nil when constructed from raw fields
}

// Error implements the error interface.
func (e *ProviderError) Error() string {
	prefix := e.Op
	if e.Resource != "" {
		prefix = e.Op + " " + e.Resource
	}
	if e.Platform != "" {
		prefix = string(e.Platform) + " " + prefix
	}
	if e.Cause != nil {
		if e.StatusCode != 0 {
			return fmt.Sprintf("%s: HTTP %d: %v", prefix, e.StatusCode, e.Cause)
		}
		return fmt.Sprintf("%s: %v", prefix, e.Cause)
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("%s: HTTP %d", prefix, e.StatusCode)
	}
	return prefix
}

// Unwrap implements errors.Unwrap.
func (e *ProviderError) Unwrap() error { return e.Cause }

// Is implements errors.Is. It matches when the wrapped Cause equals the
// target, which lets callers write `errors.Is(err, provider.ErrNotFound)`.
func (e *ProviderError) Is(target error) bool {
	if e.Cause == nil {
		return false
	}
	return errors.Is(e.Cause, target)
}

// IsStatus reports whether the error has the given HTTP status code.
func (e *ProviderError) IsStatus(code int) bool { return e.StatusCode == code }

// IsClientError reports 4xx.
func (e *ProviderError) IsClientError() bool { return e.StatusCode >= 400 && e.StatusCode < 500 }

// IsServerError reports 5xx.
func (e *ProviderError) IsServerError() bool { return e.StatusCode >= 500 && e.StatusCode < 600 }

// Wrap creates a ProviderError from a raw error, classifying it when the
// cause is a transport error with a known status code. The original error
// chain is always preserved: Cause stays rooted at the raw error (with the
// classification sentinel wrapped in front via %w), so errors.Is matches
// both the sentinel (ErrNotFound & co.) and any error from the original
// chain, and the Error() message retains the platform's original detail.
// Use this in platform implementations to convert transport errors into the
// unified shape.
func Wrap(platform Platform, op string, err error) error {
	if err == nil {
		return nil
	}
	if pe, ok := err.(*ProviderError); ok && pe.Platform == platform && pe.Op == op {
		return pe
	}
	pe := &ProviderError{Platform: platform, Op: op, Cause: err}
	// Walk the cause chain looking for an HTTP status code: the statusCoder
	// interface first (*StatusError, transport.Error), then the platform's
	// registered StatusExtractor over its SDK error types. Errors with
	// neither carry no status (classify explicitly at the call site).
	for cur := err; cur != nil; {
		// Fast path: anything implementing StatusCode() int. Covers
		// *StatusError in this package and transport.Error.
		if sc, ok := cur.(statusCoder); ok {
			pe.StatusCode = sc.StatusCode()
			pe.Cause = fmt.Errorf("%w: %w", classifyStatusCode(pe.StatusCode), err)
			copyRateLimitHints(pe, cur)
			break
		}
		// Platform extractor: an explicit, type-safe lookup each backend
		// registers for its SDK's error types (the pre-0.83 reflection over
		// StatusCode/*http.Response fields and error-message parsing are gone
		// — see RegisterStatusExtractor).
		if code, ok := statusExtractorFor(platform, cur); ok {
			pe.StatusCode = code
			pe.Cause = fmt.Errorf("%w: %w", classifyStatusCode(code), err)
			copyRateLimitHints(pe, cur)
			break
		}
		cur = errors.Unwrap(cur)
	}
	return pe
}

// rateLimitInfo is the interface shape through which transport errors
// expose server-advised recovery windows without the provider package
// importing transport.
type rateLimitInfo interface {
	RateLimitInfo() (retryAfter time.Duration, resetAt time.Time)
}

func copyRateLimitHints(pe *ProviderError, cur error) {
	if ri, ok := cur.(rateLimitInfo); ok {
		pe.RetryAfter, pe.ResetAt = ri.RateLimitInfo()
	}
}

// Wrapf is a convenience for creating a ProviderError with a formatted
// message. The format is intentionally simple ("%s/%s") so platform code can
// embed the resource identifier without re-implementing the prefix logic.
func Wrapf(platform Platform, op, format string, args ...any) error {
	cause := fmt.Errorf(format, args...)
	return &ProviderError{Platform: platform, Op: op, Cause: cause}
}

// New builds a ProviderError directly from the given status code.
func New(platform Platform, op string, status int, body string) error {
	pe := &ProviderError{
		Platform:   platform,
		Op:         op,
		StatusCode: status,
		Cause:      classifyStatusCode(status),
	}
	if body != "" && pe.Cause != nil {
		pe.Cause = fmt.Errorf("%w: %s", pe.Cause, body)
	}
	return pe
}

// StatusError wraps an error with an explicit HTTP status code. Use this in
// platform backends when you need to attach a status code to an error from a
// third-party SDK that doesn't implement the statusCoder interface. This avoids
// the need for the reflection-based fallback in Wrap.
//
// Example:
//
//	err := someSDK.DoSomething()
//	if err != nil {
//	    return provider.WrapStatusError(err, 404)
//	}
type StatusError struct {
	Status int
	Cause  error
}

func (e *StatusError) Error() string   { return e.Cause.Error() }
func (e *StatusError) Unwrap() error   { return e.Cause }
func (e *StatusError) StatusCode() int { return e.Status }

// WrapStatusError wraps an error with an explicit HTTP status code.
// This is the preferred way to attach status codes to third-party SDK errors
// instead of relying on reflection-based detection.
func WrapStatusError(err error, statusCode int) error {
	if err == nil {
		return nil
	}
	return &StatusError{Status: statusCode, Cause: err}
}

// statusCoder is the interface implemented by *StatusError in this package
// and by transport.Error. We avoid importing transport here to keep the
// provider package dependency-free.
type statusCoder interface {
	StatusCode() int
}

// StatusExtractor reports the HTTP status carried by one of a platform's
// SDK error types. It is called per link of the chain Wrap walks, so the
// extractor only needs to handle its own error values (the chain walk
// handles wrapping).
type StatusExtractor func(err error) (int, bool)

// statusExtractors maps each platform to its SDK error extractor. Written
// from backend init() (single-goroutine startup), read from Wrap on any
// goroutine; the RWMutex mirrors the provider registry's discipline.
var (
	statusMu         sync.RWMutex
	statusExtractors = map[Platform]StatusExtractor{}
)

// RegisterStatusExtractor teaches Wrap how to read the HTTP status out of the
// named platform's SDK error values — the explicit, type-safe replacement for
// the old reflection + error-message-parsing fallback. Backends register one
// extractor at init covering their SDK's error types via errors.As; a nil
// extractor (or no registration) means the platform's errors carry no status
// on the error value itself (use WrapStatusError / New at the call site, or
// thread the *Response status through the port as the family backends do).
// Panics on duplicate registration (same convention as Register).
func RegisterStatusExtractor(p Platform, fn StatusExtractor) {
	if fn == nil {
		panic("provider: RegisterStatusExtractor with nil extractor")
	}
	statusMu.Lock()
	defer statusMu.Unlock()
	if _, dup := statusExtractors[p]; dup {
		panic("provider: duplicate status extractor for " + string(p))
	}
	statusExtractors[p] = fn
}

// statusExtractorFor returns the platform's registered extractor's verdict
// for err.
func statusExtractorFor(p Platform, err error) (int, bool) {
	statusMu.RLock()
	fn := statusExtractors[p]
	statusMu.RUnlock()
	if fn == nil {
		return 0, false
	}
	return fn(err)
}

// ClassifyStatus maps an HTTP status code to a sentinel error. Exposed so
// transports and platform implementations can produce consistent error
// categories without importing each other.
func ClassifyStatus(statusCode int) error {
	return classifyStatusCode(statusCode)
}

func classifyStatusCode(statusCode int) error {
	switch {
	case statusCode == http.StatusNotFound:
		return ErrNotFound
	case statusCode == http.StatusUnauthorized:
		return ErrAuthentication
	case statusCode == http.StatusForbidden:
		return ErrForbidden
	case statusCode == http.StatusTooManyRequests:
		return ErrRateLimited
	case statusCode == http.StatusConflict:
		return ErrConflict
	case statusCode >= 500 && statusCode < 600:
		return fmt.Errorf("server error (status %d)", statusCode)
	default:
		return fmt.Errorf("HTTP %d", statusCode)
	}
}

// IsNotFound reports whether err wraps ErrNotFound (HTTP 404).
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsAuthentication reports whether err wraps ErrAuthentication (HTTP 401).
func IsAuthentication(err error) bool { return errors.Is(err, ErrAuthentication) }

// IsRateLimited reports whether err wraps ErrRateLimited (HTTP 429).
func IsRateLimited(err error) bool { return errors.Is(err, ErrRateLimited) }

// RateLimitRecovery extracts the server-advised recovery window from a
// rate-limited error, walking the wrapped chain for the transport hints
// (Retry-After / X-RateLimit-Reset). ok is true only when at least one of
// retryAfter or resetAt is known — callers rate-limited without hints
// should fall back to their own backoff.
//
//	if ra, reset, ok := provider.RateLimitRecovery(err); ok {
//	    wait := ra
//	    if reset.After(time.Now()) && (wait == 0 || time.Until(reset) > wait) {
//	        wait = time.Until(reset)
//	    }
//	    time.Sleep(wait)
//	}
func RateLimitRecovery(err error) (retryAfter time.Duration, resetAt time.Time, ok bool) {
	for cur := err; cur != nil; cur = errors.Unwrap(cur) {
		if pe, isPE := cur.(*ProviderError); isPE {
			if pe.RetryAfter != 0 || !pe.ResetAt.IsZero() {
				return pe.RetryAfter, pe.ResetAt, true
			}
			continue
		}
		if ri, ok := cur.(rateLimitInfo); ok {
			if ra, at := ri.RateLimitInfo(); ra != 0 || !at.IsZero() {
				return ra, at, true
			}
		}
	}
	return 0, time.Time{}, false
}

// IsForbidden reports whether err wraps ErrForbidden (HTTP 403).
func IsForbidden(err error) bool { return errors.Is(err, ErrForbidden) }

// IsConflict reports whether err wraps ErrConflict (HTTP 409).
func IsConflict(err error) bool { return errors.Is(err, ErrConflict) }

// IsNotImplemented reports whether err wraps ErrNotImplemented.
func IsNotImplemented(err error) bool { return errors.Is(err, ErrNotImplemented) }

// IsInvalidInput reports whether err wraps ErrInvalidInput.
func IsInvalidInput(err error) bool { return errors.Is(err, ErrInvalidInput) }

// IsWebhookValidation reports whether err wraps ErrWebhookValidation.
func IsWebhookValidation(err error) bool {
	return errors.Is(err, ErrWebhookValidation)
}

// IsPlatformNotSupported reports whether err wraps ErrPlatformNotSupported.
func IsPlatformNotSupported(err error) bool {
	return errors.Is(err, ErrPlatformNotSupported)
}
