// Package transport provides a unified HTTP transport layer for the
// go-git-platform. It is the single entry point through which all platform
// implementations send requests, so cross-cutting concerns (authentication
// header injection, retry/backoff, request/response hooks, structured logging,
// body capture for retry) are implemented in exactly one place.
//
// The package is designed to work in two ways, and both share one pipeline:
//
//  1. Direct usage via Client. Higher-level callers build a Request and pass it
//     to Client.Do, Client.DoJSON, or Client.DoRaw. Internally these go through
//     the same RoundTripper chain described below; the response body is
//     captured for the caller to consume.
//
//  2. Transparent integration with third-party SDKs via RoundTripper. The
//     RoundTripper returned by Client.NewRetryingRoundTripper is a standard
//     http.RoundTripper that performs auth/retry/hooks on every request made by
//     any client that builds on net/http. This is how go-github, gitlab
//     client-go, gitea-sdk, forgejo-sdk and go-gitcode are wired up so they
//     benefit from the same cross-cutting behavior without code duplication.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout is the per-request HTTP timeout applied when the caller does
// not specify one explicitly.
const DefaultTimeout = 30 * time.Second

// AuthStrategy applies authentication credentials to an outbound request.
// Implementations must be safe to call concurrently from multiple goroutines.
type AuthStrategy interface {
	Apply(req *http.Request)
}

// AuthFunc adapts a plain function into an AuthStrategy.
type AuthFunc func(req *http.Request)

// Apply implements AuthStrategy.
func (f AuthFunc) Apply(req *http.Request) { f(req) }

// None is the no-op auth strategy.
type None struct{}

// Apply implements AuthStrategy.
func (None) Apply(*http.Request) {}

// BearerToken sets the standard "Authorization: Bearer <token>" header.
type BearerToken struct{ Token string }

// Apply implements AuthStrategy.
func (b BearerToken) Apply(req *http.Request) {
	if b.Token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+b.Token)
}

// PrivateToken sets the GitLab-style "PRIVATE-TOKEN" header.
type PrivateToken struct{ Token string }

// Apply implements AuthStrategy.
func (p PrivateToken) Apply(req *http.Request) {
	if p.Token == "" {
		return
	}
	req.Header.Set("PRIVATE-TOKEN", p.Token)
}

// TokenHeader sets the older "Authorization: token <token>" header used by
// some GitHub-compatible APIs.
type TokenHeader struct{ Token string }

// Apply implements AuthStrategy.
func (t TokenHeader) Apply(req *http.Request) {
	if t.Token == "" {
		return
	}
	req.Header.Set("Authorization", "token "+t.Token)
}

// StaticAuth lets callers inject an arbitrary header value.
type StaticAuth struct {
	Header string
	Value  string
}

// Apply implements AuthStrategy.
func (s StaticAuth) Apply(req *http.Request) {
	if s.Header == "" {
		return
	}
	req.Header.Set(s.Header, s.Value)
}

// Request describes a single HTTP call. It is intentionally a plain value type
// so callers can build and pass it around without lock-in to a fluent API.
type Request struct {
	// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE, ...).
	Method string
	// Path is the request path relative to the client's base URL. It may
	// include a query string; in that case Query is ignored.
	Path string
	// Query holds additional query parameters. Entries with an empty value are
	// dropped.
	Query url.Values
	// Headers carries per-request headers. The auth strategy may overwrite
	// any of these.
	Headers http.Header
	// Body is the request payload. Supported types:
	//   - nil:           no body
	//   - []byte / *bytes.Buffer / *bytes.Reader / *strings.Reader: sent verbatim
	//   - io.Reader:     streamed as-is
	//   - any other:     JSON-encoded
	Body any
	// Result is the target for JSON decoding of the response body. When nil
	// (and StatusCode is not 204), the raw body is still available via
	// Response.Body.
	Result any
}

// Response is the result of a single HTTP call. The body has been read in
// full and is available to the caller. It is the caller's responsibility to
// cap the body size upstream (e.g. via a wrapping RoundTripper) when feeding
// untrusted responses.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Client is the unified transport. It is immutable after construction: every
// field is configured through ClientOption values passed to NewClient, so a
// Client is safe for concurrent use as soon as it is handed out.
type Client struct {
	baseURL     string
	auth        AuthStrategy
	retry       *RetryConfig
	hooks       *Hooks
	logger      Logger
	timeout     time.Duration
	transport   http.RoundTripper
	limiter     *RateLimiter
	etag        *ETagCache
	maxBodySize int64

	// pipelineOnce/pipeline lazily build the RoundTripper chain shared by the
	// Do path. SDK consumers build their own chain per call via
	// NewRetryingRoundTripper, so laziness never races configuration.
	pipelineOnce sync.Once
	pipeline     http.RoundTripper
}

// DefaultMaxBodySize is the default maximum response body size (10 MB).
const DefaultMaxBodySize = 10 * 1024 * 1024

// ClientOption configures a Client at construction time. Options make the
// Client immutable after NewClient returns, replacing the previous
// assign-fields-after-construction pattern that invited data races.
type ClientOption func(*Client)

// WithTimeout sets the per-request timeout applied when ctx has no deadline.
// A non-positive value falls back to DefaultTimeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.timeout = d }
}

// WithTransport sets the underlying http.RoundTripper. nil falls back to
// http.DefaultTransport.
func WithTransport(rt http.RoundTripper) ClientOption {
	return func(c *Client) { c.transport = rt }
}

// WithRetry sets the exponential-backoff retry policy. nil disables retry.
func WithRetry(cfg *RetryConfig) ClientOption {
	return func(c *Client) { c.retry = cfg }
}

// WithHooks registers request/response observability hooks. nil is fine.
func WithHooks(h *Hooks) ClientOption {
	return func(c *Client) { c.hooks = h }
}

// WithLogger sets the structured logger. nil falls back to a noop logger.
func WithLogger(l Logger) ClientOption {
	return func(c *Client) { c.logger = l }
}

// WithLimiter enables proactive rate limiting. nil disables it.
func WithLimiter(rl *RateLimiter) ClientOption {
	return func(c *Client) { c.limiter = rl }
}

// WithETag enables conditional requests (If-None-Match / 304 replay) for GETs
// on both the Do and the RoundTripper paths. nil disables conditional
// requests entirely. See ETagCache.
func WithETag(cache *ETagCache) ClientOption {
	return func(c *Client) { c.etag = cache }
}

// WithMaxBodySize limits the response body size in bytes. 0 means no limit;
// -1 uses the default limit (DefaultMaxBodySize). This prevents OOM from
// malicious or misconfigured servers.
func WithMaxBodySize(n int64) ClientOption {
	return func(c *Client) { c.maxBodySize = n }
}

// NewClient builds a Client with the given base URL (trimmed of trailing
// slashes) and auth strategy, then applies the options in order.
func NewClient(baseURL string, auth AuthStrategy, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		auth:    auth,
		timeout: DefaultTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Do executes req and returns the captured response. It does NOT decode JSON
// into req.Result; use DoJSON for that. Use Do to inspect the raw body or
// status code manually.
func (c *Client) Do(ctx context.Context, req *Request) (*Response, error) {
	return c.do(ctx, req, false)
}

// DoJSON executes req and, when req.Result is non-nil and the response has a
// body, decodes the body as JSON into req.Result. It returns ErrEmptyResponse
// when the response has no body and req.Result is non-nil.
func (c *Client) DoJSON(ctx context.Context, req *Request) (*Response, error) {
	return c.do(ctx, req, true)
}

// DoRaw executes req and returns the response body as raw bytes. It is a
// convenience over Do for endpoints that return non-JSON payloads (archives,
// tarballs, plain text, ...).
func (c *Client) DoRaw(ctx context.Context, req *Request) ([]byte, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// do runs the request through the same RoundTripper pipeline that
// NewRetryingRoundTripper exposes to SDKs — limiter, auth, hooks, ETag and
// retry exist in exactly one orchestration. On top of the shared pipeline it
// adds the Do-path specifics: the per-request timeout as a context deadline,
// response-body capture with the configured size cap, transport.Error
// construction for 4xx/5xx, and JSON decoding.
func (c *Client) do(ctx context.Context, req *Request, decode bool) (*Response, error) {
	if req == nil {
		return nil, fmt.Errorf("transport: nil request")
	}
	if req.Method == "" {
		return nil, fmt.Errorf("transport: empty method")
	}

	// Overall per-request bound as a context deadline, applied to the request
	// itself (buildRequest binds it): unlike the old per-call
	// http.Client.Timeout this also covers reading the response body, and it
	// composes with any caller deadline (the earlier of the two wins). The
	// stalled-header bound stays in clientRoundTripper.
	timeout := c.timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	resp, err := c.sharedPipeline().RoundTrip(httpReq)
	duration := time.Since(start)
	if err != nil {
		// The RoundTripper layer has already logged the failure (with a
		// redacted URL) and run the response hooks.
		return nil, err
	}
	body, err := readAndClose(resp, c.effectiveMaxBodySize())
	if err != nil {
		c.log().Error("transport request failed",
			"method", req.Method,
			"path", req.Path,
			"status", resp.StatusCode,
			"duration", duration,
			"err", err,
		)
		return nil, err
	}

	c.log().Debug("transport request ok",
		"method", req.Method,
		"path", req.Path,
		"status", resp.StatusCode,
		"duration", duration,
		"body_size", len(body),
	)
	if len(body) > 0 {
		c.log().Debug("transport response body",
			"method", req.Method,
			"path", req.Path,
			"body", truncateForLog(body, 2048),
		)
	}

	if resp.StatusCode >= 400 {
		// The RoundTripper layer has logged and hooked the error response;
		// here it becomes the structured transport error for the Do caller.
		return nil, NewStatusErrorWithHeaders(req.Method, req.Path, resp.StatusCode, body, resp.Header)
	}

	out := &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
	}
	if !decode || req.Result == nil {
		return out, nil
	}
	if len(body) == 0 || resp.StatusCode == http.StatusNoContent {
		return out, ErrEmptyResponse
	}
	if err := json.Unmarshal(body, req.Result); err != nil {
		return out, fmt.Errorf("transport: decode response: %w", err)
	}
	return out, nil
}

// buildRequest assembles the http.Request for a transport Request: URL join,
// query encoding, default headers, and body encoding. Authentication, the
// User-Agent default and request hooks are applied by the RoundTripper layer
// (clientRoundTripper) so every path applies them identically.
func (c *Client) buildRequest(ctx context.Context, req *Request) (*http.Request, error) {
	bodyReader, contentType, err := encodeBody(req.Body)
	if err != nil {
		return nil, fmt.Errorf("transport: encode body: %w", err)
	}

	full := c.baseURL + req.Path
	if i := strings.IndexByte(req.Path, '?'); i < 0 && len(req.Query) > 0 {
		full += "?" + req.Query.Encode()
	}
	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(req.Method), full, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("transport: build request: %w", err)
	}

	for k, vs := range req.Headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	if contentType != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	if httpReq.Header.Get("Accept") == "" {
		httpReq.Header.Set("Accept", "application/json")
	}
	return httpReq, nil
}

// roundTripRequest applies auth/hooks on a request that was not built by
// buildRequest. It is used by the round-tripper path so that third-party SDK
// requests still receive auth/hooks without going through buildRequest.
// A rejecting request hook aborts the request: the error is returned to the
// caller (matching the Client.do path) rather than silently discarded.
func (c *Client) roundTripRequest(req *http.Request) error {
	if err := applyAuth(req.Context(), c.auth, req); err != nil {
		return err
	}
	setUserAgentDefault(req)
	return c.Hooks().ExecuteRequest(req.Context(), req)
}

// encodeBody returns the io.Reader for the request body, the content-type
// header to set, and any encoding error. A nil body yields a nil reader and
// empty content-type.
func encodeBody(body any) (io.Reader, string, error) {
	switch b := body.(type) {
	case nil:
		return nil, "", nil
	case []byte:
		return bytes.NewReader(b), "application/octet-stream", nil
	case *bytes.Buffer:
		return b, "application/octet-stream", nil
	case *bytes.Reader:
		return b, "application/octet-stream", nil
	case *strings.Reader:
		return b, "application/octet-stream", nil
	case string:
		return strings.NewReader(b), "text/plain; charset=utf-8", nil
	case io.Reader:
		return b, "", nil
	default:
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		return bytes.NewReader(raw), "application/json", nil
	}
}

// Logger returns the configured logger or a noop logger when none was set.
func (c *Client) log() Logger {
	if c.logger != nil {
		return c.logger
	}
	return NoopLogger()
}

// Hooks returns the configured hooks; nil-safe for the zero value.
func (c *Client) Hooks() *Hooks { return c.hooks }

// sharedPipeline returns the RoundTripper chain used by the Do path:
// retryingRoundTripper{clientRoundTripper{c}}. Built once on first use; all
// configuration was fixed at NewClient time, so laziness cannot race.
func (c *Client) sharedPipeline() http.RoundTripper {
	c.pipelineOnce.Do(func() {
		c.pipeline = &retryingRoundTripper{
			inner:  &clientRoundTripper{client: c},
			cfg:    c.retry,
			logger: c.log(),
		}
	})
	return c.pipeline
}

// RoundTripper exposes the auth/hooks/logging of this Client as a standard
// http.RoundTripper so third-party SDK clients built on net/http benefit from
// the same pipeline. It does NOT apply retries; retries are request-scoped
// and belong to the call site (e.g. Client.Do). Use NewRetryingRoundTripper
// to wrap this with retry, if needed.
func (c *Client) RoundTripper() http.RoundTripper {
	return &clientRoundTripper{client: c}
}

// NewRetryingRoundTripper wraps rt in retry/backoff. The returned RoundTripper
// can be plugged into any http.Client; retries fire on 429, 5xx, GitHub-style
// rate-limit 403 (see RetryConfig.canRetryResponse), and the configured retry
// list, but only for idempotent methods
// {GET, HEAD, PUT, DELETE, OPTIONS} — non-idempotent requests (POST, PATCH,
// ...) are retried exclusively on errors proving the request never reached
// the network, unless RetryConfig.RetryWrite is set.
func (c *Client) NewRetryingRoundTripper() http.RoundTripper {
	return &retryingRoundTripper{inner: c.RoundTripper(), cfg: c.retry, logger: c.log()}
}

// clientRoundTripper adapts a Client into an http.RoundTripper. It must be
// created via Client.RoundTripper.
type clientRoundTripper struct {
	client *Client

	// once guards the one-time derivation of the base transport. The previous
	// implementation rewrote rt.client.Transport from inside RoundTrip to
	// install a ResponseHeaderTimeout, racing with concurrent readers of the
	// shared Client (go test -race caught it under parallel requests).
	once      sync.Once
	transport http.RoundTripper
}

// baseTransport resolves the underlying RoundTripper exactly once. When the
// configured transport is a *http.Transport without a ResponseHeaderTimeout,
// a clone carrying the client timeout is used so that a stalled (but
// established) connection cannot hang the header wait forever —
// http.Client.Timeout does not apply on the raw Transport.RoundTrip path.
// The derived value is stored on the clientRoundTripper itself; the shared
// Client is never mutated, so RoundTrip only reads client state.
func (rt *clientRoundTripper) baseTransport() http.RoundTripper {
	rt.once.Do(func() {
		timeout := rt.client.timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		tr := rt.client.transport
		if tr == nil {
			tr = http.DefaultTransport
		}
		ht, ok := tr.(*http.Transport)
		if !ok || ht.ResponseHeaderTimeout > 0 {
			rt.transport = tr
			return
		}
		ht = ht.Clone()
		ht.ResponseHeaderTimeout = timeout
		rt.transport = ht
	})
	return rt.transport
}

// RoundTrip implements http.RoundTripper.
//
// The request context is used as-is: it is NOT re-wrapped in a client-timeout
// deadline. A previous implementation wrapped the context in
// context.WithTimeout and deferred the cancel, which fired the moment
// RoundTrip returned — while resp.Body was still bound to that context, so
// large or slow response bodies were truncated with "context canceled" once
// the caller read them. The header-wait stall protection now lives in
// baseTransport (ResponseHeaderTimeout); overall request bounds remain the
// caller's context responsibility (Client.do adds its timeout as a deadline).
func (rt *clientRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	tr := rt.baseTransport()
	// Proactive rate limiting.
	if rt.client.limiter != nil {
		if err := rt.client.limiter.WaitContext(ctx); err != nil {
			return nil, err
		}
	}
	if err := rt.client.roundTripRequest(req); err != nil {
		// A rejecting request hook must abort the request instead of being
		// silently swallowed (same semantics as the Client.do path).
		return nil, err
	}
	rt.client.etag.applyConditional(req)
	start := time.Now()
	resp, err := tr.RoundTrip(req)
	duration := time.Since(start)
	// Update rate limiter state from the real response headers — a 304's
	// headers describe the current quota, the replayed 200's do not.
	if resp != nil && rt.client.limiter != nil {
		rt.client.limiter.UpdateFromResponse(resp)
	}
	if err == nil {
		resp, err = rt.client.etag.processRT(req, resp)
	}
	rt.client.Hooks().ExecuteResponse(ctx, req, resp, duration, err)
	if err != nil {
		rt.client.log().Error("transport roundtrip failed",
			"method", req.Method,
			"url", redactURL(*req.URL),
			"duration", duration,
			"err", err,
		)
		return nil, err
	}
	if resp.StatusCode >= 400 {
		rt.client.log().Warn("transport roundtrip error",
			"method", req.Method,
			"url", redactURL(*req.URL),
			"status", resp.StatusCode,
			"duration", duration,
		)
	}
	return resp, nil
}

// retryingRoundTripper is the RoundTripper-shaped adapter over the single
// retry engine (RetryConfig.retryLoop): each attempt replays auth/hooks/ETag
// through the inner chain, responses classified as retryable have their body
// buffered for replay, and the final upstream response — even a 5xx — is
// returned with a nil error per the http.RoundTripper contract.
type retryingRoundTripper struct {
	inner  http.RoundTripper
	cfg    *RetryConfig
	logger Logger
}

// RoundTrip implements http.RoundTripper and re-issues the request on
// transient failures. The request body is buffered so it can be replayed.
func (rt *retryingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.cfg == nil || rt.cfg.MaxAttempts <= 0 {
		return rt.inner.RoundTrip(req)
	}
	if err := ensureReplayable(req); err != nil {
		return nil, err
	}
	resp, _, err := rt.cfg.retryLoop(req.Context(), req, rt.logger,
		func() (*http.Response, error) { return rt.inner.RoundTrip(req) },
		func(resp *http.Response) (*http.Response, []byte, bool, error) {
			// canRetryResponse adds the header-aware GitHub-style rate-limit
			// 403 on top of the status-only canRetryStatus gate.
			if !rt.cfg.canRetryResponse(req, resp) {
				// Final response: leave the body live for the caller.
				return resp, nil, false, nil
			}
			// Buffer the body before closing so the final response returned to
			// the caller still carries the upstream error payload. The previous
			// implementation closed the body without buffering, so callers (and
			// the SDK response decoders sitting on top of this RoundTripper)
			// received a closed, empty body and could not see the real 5xx
			// payload.
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				return nil, nil, false, err
			}
			return resp, body, true, nil
		})
	return resp, err
}

// effectiveMaxBodySize returns the resolved max body size.
// -1 means DefaultMaxBodySize, 0 means no limit, >0 is the explicit limit.
func (c *Client) effectiveMaxBodySize() int64 {
	switch {
	case c.maxBodySize < 0:
		return DefaultMaxBodySize
	case c.maxBodySize == 0:
		return 0
	default:
		return c.maxBodySize
	}
}

// readAndClose reads the full response body and closes it. The body is
// returned as a byte slice so it can be replayed by the caller.
// If maxBodySize > 0, the read is limited to that many bytes.
func readAndClose(resp *http.Response, maxBodySize int64) ([]byte, error) {
	if resp == nil {
		return nil, nil
	}
	defer func() { _ = resp.Body.Close() }()
	if maxBodySize > 0 {
		limited := io.LimitReader(resp.Body, maxBodySize+1)
		body, err := io.ReadAll(limited)
		if err != nil {
			return nil, err
		}
		if int64(len(body)) > maxBodySize {
			return nil, fmt.Errorf("response body exceeds maximum size of %d bytes", maxBodySize)
		}
		return body, nil
	}
	return io.ReadAll(resp.Body)
}

// redactURL masks credential-bearing query parameters in logged URLs. Some
// platforms authenticate via query string (e.g. Gitee's access_token, GitLab's
// private_token), and those tokens must never reach log output. The receiver
// URL is a value copy, so the outgoing request keeps its real credentials;
// only the logged form is masked.
func redactURL(u url.URL) string {
	q := u.Query()
	changed := false
	for _, k := range []string{"access_token", "token", "private_token"} {
		if q.Get(k) != "" {
			q.Set(k, "***")
			changed = true
		}
	}
	if !changed {
		return u.String()
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// truncateForLog returns a string representation of body, capped at maxLen
// bytes. Bodies exceeding the limit are truncated with "...".
func truncateForLog(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "... (truncated)"
}
