package provider

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/oekit/pkg/container"
)

// ErrWebhookFiltered 是 ParseWebhookEvent 的「噪声过滤」哨兵（issue #45）：
// 后端按平台噪声策略丢弃事件（如 GitLab pipeline 的 success/running 态）
// 时返回 (nil, ErrWebhookFiltered)，而不是 (nil, nil)——后者是契约陷阱，
// 只判 err 的调用方会在 ev 上空指针。调用方以 errors.Is(err,
// ErrWebhookFiltered) 识别并显式忽略；哨兵经 provider.Wrap 包装后链路
// 保持可判。
var ErrWebhookFiltered = errors.New("webhook event filtered out (noise policy)")

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
	ErrGone                 = errors.New("resource permanently gone")
	ErrServerSide           = errors.New("server-side failure")
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
	// 遍历走 walkErrorChain（issue #146）：多 %w（Unwrap() []error）分支同样
	// 下潜，与 errors.Is 的判定口径一致——旧实现 errors.Unwrap 只走单分支，
	// 多 %w 链上状态码与限流提示静默丢失（IsRateLimited=true 而
	// StatusCode=0 的矛盾观测面）。
	walkErrorChain(err, func(cur error) bool {
		// Fast path: anything implementing StatusCode() int. Covers
		// *StatusError in this package and transport.Error.
		if sc, ok := cur.(statusCoder); ok {
			pe.StatusCode = sc.StatusCode()
			pe.Cause = fmt.Errorf("%w: %w", classifyProviderStatus(pe.StatusCode, cur), err)
			copyRateLimitHints(pe, cur)
			return true
		}
		// Platform extractor: an explicit, type-safe lookup each backend
		// registers for its SDK's error types (the pre-0.83 reflection over
		// StatusCode/*http.Response fields and error-message parsing are gone
		// — see RegisterStatusExtractor).
		if code, ok := statusExtractorFor(platform, cur); ok {
			// SDK 限流判定（issue #50）：SDK 错误类型（go-github 的
			// *RateLimitError/*AbuseRateLimitError 等）不实现 rateLimitedMarker，
			// 裸状态分类会把恒 403 的主限流信号丢回 ErrForbidden。平台注册的
			// ThrottleClassifier 在状态分类之前给出权威节流判定，恢复窗口
			// （RetryAfter/ResetAt）一并复制——IsRateLimited/RateLimitRecovery
			// 在 SDK 错误路上与 transport 错误路同样可用。
			if ra, at, throttled := throttleClassifierFor(platform, cur); throttled {
				pe.StatusCode = code
				pe.Cause = fmt.Errorf("%w: %w", ErrRateLimited, err)
				pe.RetryAfter, pe.ResetAt = ra, at
				return true
			}
			pe.StatusCode = code
			pe.Cause = fmt.Errorf("%w: %w", classifyProviderStatus(code, cur), err)
			copyRateLimitHints(pe, cur)
			return true
		}
		return false
	})
	return pe
}

// rateLimitedMarker is the interface through which lower-layer errors carry
// an authoritative "this failure is throttling" verdict up to the provider
// layer (issue #42): the transport already classifies 403 + throttle headers
// (X-RateLimit-Remaining: 0 / Retry-After) as rate limiting — GitHub/Gitea
// 把最常见的节流信号发成 403 是平台现实。classifyStatusCode 只看状态码
// （403 → ErrForbidden），Wrap 走到这两个分支时会丢掉下层已经做对的判定；
// 有此标记的 cause 优先归 ErrRateLimited，IsRateLimited 才能接到平台最常见
// 的节流形态。
type rateLimitedMarker interface {
	IsRateLimited() bool
}

// walkErrorChain visits every error reachable from err — both the
// `Unwrap() error` chain and every branch of `Unwrap() []error`（多 %w /
// errors.Join 节点）—— in depth-first preorder, the same order stdlib
// errors.Is/As traverse. visit returning true stops the walk; its boolean
// result reports whether that happened.
//
// 本仓 house 先例（issue #132、timeparse.go）：多 %w 走 Unwrap() []error，
// cause 链随哨兵一路保留。stdlib 的 errors.Unwrap 只认单分支——遇多 %w
// 节点返回 nil，手写链遍历到此为止，而 errors.Is/As 却能穿透（issue #146）：
// 同一条链上「是不是限流」判得出、「要等多久」取不到，观测面自相矛盾。
// 链遍历一律走本 helper，与 stdlib 同口径。seen 按错误值身份剪环。
func walkErrorChain(err error, visit func(error) bool) bool {
	if err == nil {
		return false
	}
	seen := make(map[error]bool)
	var walk func(error) bool
	walk = func(cur error) bool {
		if cur == nil || seen[cur] {
			return false
		}
		seen[cur] = true
		if visit(cur) {
			return true
		}
		if br, ok := cur.(interface{ Unwrap() []error }); ok {
			for _, child := range br.Unwrap() {
				if walk(child) {
					return true
				}
			}
			return false
		}
		if u, ok := cur.(interface{ Unwrap() error }); ok {
			return walk(u.Unwrap())
		}
		return false
	}
	return walk(err)
}

// classifyProviderStatus 在裸状态码分类之上叠加下层的限流判定（见
// rateLimitedMarker）。其余状态码与 classifyStatusCode 完全同型。
func classifyProviderStatus(status int, cause error) error {
	if status == http.StatusForbidden {
		if m, ok := cause.(rateLimitedMarker); ok && m.IsRateLimited() {
			return ErrRateLimited
		}
	}
	return classifyStatusCode(status)
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
	// oekiterrors.Errorf (issue #68): same shape as fmt.Errorf but keeps the
	// cause chain for %w verbs and satisfies the "no bare fmt.Errorf" rule —
	// this spot is outside the package's double-chain exemption quota.
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
		// oekiterrors.Errorf（issue #132）：单 %w + 动态 body 的格式化包装
		// 不在「纯 %w 双链」永久豁免面内——oekiterrors.Errorf 与 fmt.Errorf
		// 同型且保留 cause 链,规则一面归零。
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

// statusExtractors maps each platform to its SDK error extractor, backed
// by oekit container.SafeMap. Written from backend init() (single-goroutine
// startup), read from Wrap on any goroutine.
var statusExtractors = container.NewSafeMap[Platform, StatusExtractor]()

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
	if _, loaded := statusExtractors.GetOrSet(p, fn); loaded {
		panic("provider: duplicate status extractor for " + string(p))
	}
}

// statusExtractorFor returns the platform's registered extractor's verdict
// for err.
func statusExtractorFor(p Platform, err error) (int, bool) {
	fn, _ := statusExtractors.Get(p)
	if fn == nil {
		return 0, false
	}
	return fn(err)
}

// ThrottleClassifier reports whether one of the platform's SDK error values
// is an authoritative throttle verdict (issue #50), optionally carrying the
// server-advised recovery window. It is the SDK-error-path counterpart of
// rateLimitedMarker: SDK error types (go-github's *RateLimitError — always
// HTTP 403 with the primary quota exhausted — and *AbuseRateLimitError for
// secondary limits) implement neither StatusCode() int nor IsRateLimited(),
// so Wrap's status classification alone would drop them into ErrForbidden.
// retryAfter/resetAt mirror ProviderError's recovery fields (zero values
// when the platform sent no hint).
type ThrottleClassifier func(err error) (retryAfter time.Duration, resetAt time.Time, throttled bool)

// throttleClassifiers maps each platform to its SDK throttle classifier,
// backed by oekit container.SafeMap (same shape as statusExtractors).
var throttleClassifiers = container.NewSafeMap[Platform, ThrottleClassifier]()

// RegisterThrottleClassifier teaches Wrap which of the named platform's SDK
// error values are throttle verdicts. Consulted per link of the chain Wrap
// walks, before the bare-status classification, wherever a status extractor
// produced a code. Backends register at init next to RegisterStatusExtractor;
// a platform without a classifier keeps the status-only behavior. Panics on
// duplicate registration (same convention as RegisterStatusExtractor).
func RegisterThrottleClassifier(p Platform, fn ThrottleClassifier) {
	if fn == nil {
		panic("provider: RegisterThrottleClassifier with nil classifier")
	}
	if _, loaded := throttleClassifiers.GetOrSet(p, fn); loaded {
		panic("provider: duplicate throttle classifier for " + string(p))
	}
}

// throttleClassifierFor returns the platform's registered classifier's
// verdict for err.
func throttleClassifierFor(p Platform, err error) (time.Duration, time.Time, bool) {
	fn, _ := throttleClassifiers.Get(p)
	if fn == nil {
		return 0, time.Time{}, false
	}
	return fn(err)
}

// ClassifyStatus maps an HTTP status code to a sentinel error. Exposed so
// transports and platform implementations can produce consistent error
// categories without importing each other.
//
// The full mapping (issue #64): 400 and 422 → ErrInvalidInput, 401 →
// ErrAuthentication, 403 → ErrForbidden, 404 → ErrNotFound, 409 →
// ErrConflict, 410 → ErrGone, 429 → ErrRateLimited, 5xx → ErrServerSide;
// every other status yields a plain formatted error (classify by IsStatus).
// Statuses without a dedicated sentinel above stay text-only by design —
// match them with ProviderError.IsStatus, not errors.Is.
func ClassifyStatus(statusCode int) error {
	return classifyStatusCode(statusCode)
}

func classifyStatusCode(statusCode int) error {
	switch {
	case statusCode == http.StatusBadRequest,
		statusCode == http.StatusUnprocessableEntity:
		// Both "the request payload was rejected" shapes (issue #64: 400 and
		// 422 are the two most common validation-rejection statuses, and both
		// previously fell to the text default, leaving IsInvalidInput false
		// for real server rejections).
		return ErrInvalidInput
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
	case statusCode == http.StatusGone:
		// Resource existed but is permanently gone (e.g. an emptied
		// paginated page marker) — distinct from "never existed".
		return ErrGone
	case statusCode >= 500 && statusCode < 600:
		return ErrServerSide
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
// should fall back to their own backoff. The walk covers both `Unwrap()
// error` chains and `Unwrap() []error` branches（多 %w / errors.Join，
// issue #146）——判定口径与 errors.Is 一致，同一份错误上 IsRateLimited 为真
// 时本函数不会再因链形态不同而丢恢复窗口。
//
//	if ra, reset, ok := provider.RateLimitRecovery(err); ok {
//	    wait := ra
//	    if reset.After(time.Now()) && (wait == 0 || time.Until(reset) > wait) {
//	        wait = time.Until(reset)
//	    }
//	    time.Sleep(wait)
//	}
func RateLimitRecovery(err error) (retryAfter time.Duration, resetAt time.Time, ok bool) {
	walkErrorChain(err, func(cur error) bool {
		if pe, isPE := cur.(*ProviderError); isPE {
			if pe.RetryAfter != 0 || !pe.ResetAt.IsZero() {
				retryAfter, resetAt, ok = pe.RetryAfter, pe.ResetAt, true
				return true
			}
			return false
		}
		if ri, isRI := cur.(rateLimitInfo); isRI {
			if ra, at := ri.RateLimitInfo(); ra != 0 || !at.IsZero() {
				retryAfter, resetAt, ok = ra, at, true
				return true
			}
		}
		return false
	})
	return retryAfter, resetAt, ok
}

// IsForbidden reports whether err wraps ErrForbidden (HTTP 403).
func IsForbidden(err error) bool { return errors.Is(err, ErrForbidden) }

// IsConflict reports whether err wraps ErrConflict (HTTP 409).
func IsConflict(err error) bool { return errors.Is(err, ErrConflict) }

// IsGone reports whether err wraps ErrGone (HTTP 410): the resource existed
// and is now permanently gone — distinct from IsNotFound ("never existed").
func IsGone(err error) bool { return errors.Is(err, ErrGone) }

// IsServerSide reports whether err wraps ErrServerSide (any HTTP 5xx). True
// here means "retrying after backoff is meaningful"; a false result with
// IsClientError true means the request itself must change.
func IsServerSide(err error) bool { return errors.Is(err, ErrServerSide) }

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
