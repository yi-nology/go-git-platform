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
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/oekit/concurrency/retry/httpx"
	oekiterrors "github.com/oekit/pkg/errors"
	"github.com/oekit/pkg/stringx"
)

// DefaultTimeout is the per-request HTTP timeout applied when the caller does
// not specify one explicitly.
const DefaultTimeout = 30 * time.Second

// AuthStrategy applies authentication credentials to an outbound request.
// Apply returns an error when the credential cannot be produced (e.g. a
// TokenSource refresh failure) so the request fails loudly instead of going
// out unauthenticated (issue #68). Implementations must be safe to call
// concurrently from multiple goroutines.
type AuthStrategy interface {
	Apply(req *http.Request) error
}

// AuthFunc adapts a plain function into an AuthStrategy.
type AuthFunc func(req *http.Request)

// Apply implements AuthStrategy.
func (f AuthFunc) Apply(req *http.Request) error {
	f(req)
	return nil
}

// None is the no-op auth strategy.
type None struct{}

// Apply implements AuthStrategy.
func (None) Apply(*http.Request) error { return nil }

// BearerToken sets the standard "Authorization: Bearer <token>" header.
type BearerToken struct{ Token string }

// Apply implements AuthStrategy.
func (b BearerToken) Apply(req *http.Request) error {
	if b.Token == "" {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+b.Token)
	return nil
}

// PrivateToken sets the GitLab-style "PRIVATE-TOKEN" header.
type PrivateToken struct{ Token string }

// Apply implements AuthStrategy.
func (p PrivateToken) Apply(req *http.Request) error {
	if p.Token == "" {
		return nil
	}
	req.Header.Set("PRIVATE-TOKEN", p.Token)
	return nil
}

// TokenHeader sets the older "Authorization: token <token>" header used by
// some GitHub-compatible APIs.
type TokenHeader struct{ Token string }

// Apply implements AuthStrategy.
func (t TokenHeader) Apply(req *http.Request) error {
	if t.Token == "" {
		return nil
	}
	req.Header.Set("Authorization", "token "+t.Token)
	return nil
}

// StaticAuth lets callers inject an arbitrary header value.
type StaticAuth struct {
	Header string
	Value  string
}

// Apply implements AuthStrategy.
func (s StaticAuth) Apply(req *http.Request) error {
	if s.Header == "" {
		return nil
	}
	req.Header.Set(s.Header, s.Value)
	return nil
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

// DefaultMaxBodySize is the maximum response body size applied when the
// caller does not configure one (10 MB). The zero-value Client would
// otherwise leave the OOM guard off (issue #70: the pre-0.125 constructor
// defaulted to 0 = unlimited, so the "default 10 MB" the docs claimed did
// not exist unless the caller explicitly passed -1).
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

// WithMaxBodySize overrides the response body size limit in bytes. The
// constructor already applies the DefaultMaxBodySize cap; this option is for
// callers that need a different bound:
//
//	 0 — no limit (explicit opt-out; read with care, it removes the OOM guard)
//	-1 — the default limit (DefaultMaxBodySize)
//	>0 — the explicit limit in bytes
//
// This prevents OOM from malicious or misconfigured servers on the Do path;
// the raw RoundTripper path hands the live response to the caller uncapped.
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
		// OOM guard on by default (issue #70): the zero value would mean
		// unlimited bodies. WithMaxBodySize(0) explicitly opts out, -1 and
		// >0 keep their documented meanings via effectiveMaxBodySize.
		maxBodySize: DefaultMaxBodySize,
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
		return nil, oekiterrors.Errorf("transport: nil request")
	}
	if req.Method == "" {
		return nil, oekiterrors.Errorf("transport: empty method")
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
			"path", redactPath(req.Path),
			"status", resp.StatusCode,
			"duration", duration,
			"err", err,
		)
		return nil, err
	}

	// Debug events are only worth their argument cost when a real logger
	// is attached AND debug passes its level filter: with the default noop
	// these calls would still evaluate their kv pairs — including
	// truncateForLog's body copy — and with a level-filtered logger the
	// evaluation would be discarded downstream anyway.
	c.logRequestDebug(req, resp.StatusCode, duration, body)

	if resp.StatusCode >= 400 {
		// The RoundTripper layer has logged and hooked the error response;
		// here it becomes the structured transport error for the Do caller.
		return nil, NewStatusErrorWithHeaders(req.Method, redactPath(req.Path), resp.StatusCode, body, resp.Header)
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
		return out, oekiterrors.Wrap(err, "transport: decode response")
	}
	return out, nil
}

// logRequestDebug emits the per-request debug events when a real logger is
// attached and debug passes its level filter (DebugEnabled): with the
// default noop, or with a level-filtered logger, the kv arguments —
// including truncateForLog's body copy — are not evaluated at all.
func (c *Client) logRequestDebug(req *Request, status int, duration time.Duration, body []byte) {
	if c.logger == nil || !DebugEnabled(c.logger) {
		return
	}
	c.logger.Debug("transport request ok",
		"method", req.Method,
		"path", redactPath(req.Path),
		"status", status,
		"duration", duration,
		"body_size", len(body),
	)
	if len(body) > 0 {
		c.logger.Debug("transport response body",
			"method", req.Method,
			"path", redactPath(req.Path),
			"body", truncateForLog(body, 2048),
		)
	}
}

// buildRequest assembles the http.Request for a transport Request: URL join,
// query encoding, default headers, and body encoding. Authentication, the
// User-Agent default and request hooks are applied by the RoundTripper layer
// (clientRoundTripper) so every path applies them identically.
func (c *Client) buildRequest(ctx context.Context, req *Request) (*http.Request, error) {
	bodyReader, contentType, err := encodeBody(req.Body)
	if err != nil {
		return nil, oekiterrors.Wrap(err, "transport: encode body")
	}

	full := c.baseURL + req.Path
	if i := strings.IndexByte(req.Path, '?'); i < 0 && len(req.Query) > 0 {
		full += "?" + req.Query.Encode()
	}
	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(req.Method), full, bodyReader)
	if err != nil {
		return nil, oekiterrors.Wrap(err, "transport: build request")
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
	if err := c.applyAuthFor(req); err != nil {
		return err
	}
	setUserAgentDefault(req)
	return c.Hooks().ExecuteRequest(req.Context(), req)
}

// applyAuthFor 施加认证;**跨 host 的重定向跳一律不带凭据**。
//
// 为什么:认证是在 RoundTripper **内部**施加的(每一跳都重新加一次),
// 而 Go 的 net/http 只对"请求头已存在"的情况做跨 host 剥离
// (Authorization / Cookie / Www-Authenticate / Cookie2)⇒ 平台 302 到
// 第三方主机时,令牌会跟着走 —— 实测 GitLab 的 archive 302 到对象存储
// 就把 PRIVATE-TOKEN 发给了那个主机。
//
// 而 PRIVATE-TOKEN / X-Auth-Token 这类**非标准头 Go 本来就不剥**
// ⇒ 必须在这里自己处理。
func (c *Client) applyAuthFor(req *http.Request) error {
	if CrossHostRedirect(req) {
		StripSensitiveCredentials(req)
		return nil
	}
	return applyAuth(req.Context(), c.auth, req)
}

// CrossHostRedirect 判断该请求是否落在**认证目标之外的主机**上——即该请求
// 已离开发起请求的原始 host。
//
// 为什么不能只比紧邻上一跳(issue #65):认证在 RoundTripper 内部**每一跳
// 重新施加**,而剥凭据的判据若只看 `req.Response.Request`(紧邻上一跳),
// 则「平台 → 第三方 → 第三方同域再跳」链形(http→https 升级、S3 尾斜杠
// 301、签名 URL 重发都是现实形态)的第二跳会因「与上一跳同 host」而把
// 平台令牌**重新施加**到第三方主机上。判据必须对齐**重定向链的起点**:
// req.Response.Request 沿链回溯到第一跳(其 Response 为 nil 的那个请求)。
//
// Go 的 net/http 在跟随重定向时会新建请求,并把上一跳的响应挂在
// req.Response 上(Request.Response 文档:只在客户端重定向时有值)
// ⇒ 用它逐跳回溯。链是线性的(每跳的 Response.Request 指向上一跳),
// 回溯必然终止于发起请求。
//
// 判据用**完整 host(含端口)**:同一台机器上的不同端口也是不同源
// (Go 的规则只比主机名,会把 127.0.0.1:A → 127.0.0.1:B 当成同主机)。
func CrossHostRedirect(req *http.Request) bool {
	origin := OriginRedirectHost(req)
	if origin == "" || req == nil || req.URL == nil {
		return false
	}
	return !strings.EqualFold(origin, req.URL.Host)
}

// OriginRedirectHost 回溯重定向链到发起请求,返回其 host。
// req 不是重定向跳(或链头 URL 异常)时返回空串。
func OriginRedirectHost(req *http.Request) string {
	if req == nil {
		return ""
	}
	cur := req
	for cur.Response != nil && cur.Response.Request != nil {
		cur = cur.Response.Request
	}
	if cur.URL == nil {
		return ""
	}
	return cur.URL.Host
}

// sensitiveHeaders 可能随重定向外泄的凭据头。
//
// Go 只剥 Authorization / Cookie / Www-Authenticate / Cookie2;
// 其余的(PRIVATE-TO-TOKEN、X-Auth-Token、X-Api-Key …)Go 不管 ⇒ 统一处理。
var sensitiveHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Private-Token",
	"X-Auth-Token",
	"X-Access-Token",
	"X-Api-Key",
	"X-Gitlab-Token",
	"Cookie",
	"Www-Authenticate",
}

// StripSensitiveHeaders 删掉可能随跨 host 重定向外泄的凭据头。
//
// 宿主判据用**重定向链头**（OriginRedirectHost），与决定层
// CrossHostRedirect 完全同判据（issue #127）：判定与执行必须同口径——
// 旧实现这里用「紧邻上一跳」，而 Go 的重定向实现把**初始请求头快照**逐跳
// 复制到新请求（六名标准敏感头之外的头每跳无条件复制），于是
// 「平台 → 第三方 → 第三方同域再跳」链的 hop2 上，SDK 注入初始请求头的
// Private-Token 等凭据被 Go 复活，而本函数因「上一跳同 host」no-op 不剥
// ——判定层（链头）说要剥、执行层（上一跳）没剥。判据统一到链头后，
// 任何落到非认证 host 的跳（无论第几跳、凭据来自 RT 施加还是初始头）
// 都不带凭据。
func StripSensitiveHeaders(req *http.Request) {
	if req == nil {
		return
	}
	// oekit httpx.StripSensitiveHeadersWith（concurrency#200 落地）：宿主
	// 判定一并委托（链头 vs current 宿主，与 CrossHostRedirect 同判据）；
	// 词表经 With 变体传入——本仓词表多 X-Access-Token/X-Gitlab-Token 两
	// 个平台形态（保留自旧实现，非 oekit 缺省词表成员）。
	var to string
	from := OriginRedirectHost(req)
	if req.URL != nil {
		to = req.URL.Host
	}
	httpx.StripSensitiveHeadersWith(req.Header, from, to, sensitiveHeaders)
}

// sensitiveQueryParams 可能随跨 host 重定向外泄的**凭据 query 参数**。
//
// 为什么单列一组:有一类平台 SDK 把令牌放在 URL query 而不是请求头 ——
// go-gitee 的 tokenTransport 每跳都 `q.Set("access_token", token)`,
// 它包在 http.Client 的 Transport 外层 ⇒ 平台 302 到 CDN/对象存储时,
// **令牌会跟着 query 一起发给第三方主机**。
//
// 头部的剥除帮不上忙(令牌根本不在头部),而 Go 也不会剥 query ⇒ 只能自己来。
var sensitiveQueryParams = []string{
	"access_token",
	"private_token",
	"token",
	"api_key",
	"apikey",
	"auth",
	"authorization",
	"jwt",
	"session",
	"signature",
	"sig",
}

// StripSensitiveQueryParams 删掉请求 URL 里可能随跨 host 重定向外泄的凭据参数。
//
// 只在**跨 host** 的重定向跳上调用:同 host 的正常重定向(改路径、http→https)
// 必须继续带凭据,否则打断正常 API。
//
// 剥离机制委托 concurrency#208 落地的 httpx.StripSensitiveQueryParamsWith
// (raw-query 外科手术:不动无关参数的字节,免 re-encode 翻转转义形态);
// 词表保留本仓 11 项(上游缺省 3 项,本仓是超集——api_key/jwt/session 等
// 平台特化项)。
func StripSensitiveQueryParams(req *http.Request) {
	if req == nil || req.URL == nil {
		return
	}
	httpx.StripSensitiveQueryParamsWith(req.URL, sensitiveQueryParams)
}

// StripSensitiveCredentials 剥掉可能随跨 host 重定向外泄的凭据:
// 请求头(Authorization / PRIVATE-TOKEN / Cookie …)+ URL query(access_token …)。
//
// 头与 query 是两条独立的泄漏面:SDK 可能把令牌放在任一处,
// 而 Go 的自动剥除**只覆盖标准头**。
func StripSensitiveCredentials(req *http.Request) {
	StripSensitiveHeaders(req)
	StripSensitiveQueryParams(req)
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

// healETag304Miss is the in-process self-heal for a 304 whose cached entry
// was evicted underneath it (issue #31): strip If-None-Match and re-send
// unconditionally once. applyConditional skips requests that already carry
// If-None-Match and none is present now, so the resend cannot re-enter the
// conditional path; a protocol-violating 304 without a validator would
// surface unchanged (no loop). The 200 repopulates the cache, so later
// conditional requests work again. Returns the healed exchange, the extra
// wall time spent, and the exchange error.
func (rt *clientRoundTripper) healETag304Miss(ctx context.Context, req *http.Request, tr http.RoundTripper) (*http.Response, time.Duration, error) {
	rt.client.log().Warn("etag entry evicted under an in-flight 304; resending unconditionally",
		"url", redactURL(*req.URL))
	req.Header.Del("If-None-Match")
	if rt.client.limiter != nil {
		if err := rt.client.limiter.WaitContext(ctx); err != nil {
			return nil, 0, err
		}
	}
	start := time.Now()
	resp, err := tr.RoundTrip(req)
	duration := time.Since(start)
	if resp != nil && rt.client.limiter != nil {
		rt.client.limiter.UpdateFromResponse(resp)
	}
	if err == nil {
		resp, err = rt.client.etag.processRT(req, resp)
	}
	return resp, duration, err
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
		if oekiterrors.Is(err, ErrETagEntryGone) {
			// The bounded LRU can evict the validator between
			// applyConditional and a 304's arrival (concurrent walks over
			// DefaultETagEntries evict fast) — a pure local bookkeeping
			// race that must not surface as an API error (issue #31).
			// One unconditional resend repopulates the cache and hands the
			// caller a clean 200; see healETag304Miss.
			var healAdded time.Duration
			resp, healAdded, err = rt.healETag304Miss(ctx, req, tr)
			duration += healAdded
		}
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
		// Level by who owns the semantics: 5xx/429 are server-side failure
		// signals (the retry engine classifies exactly these as retryable),
		// so they stay Warn. Other 4xx are contract-level "response with a
		// status" — RoundTrip hands the caller the full response and the
		// caller decides (existence probes treat 404 as the expected
		// default); the transport cannot tell a probe miss from a wrong
		// path, so it stays observable at Debug only.
		lg := rt.client.log()
		event := []any{
			"method", req.Method,
			"url", redactURL(*req.URL),
			"status", resp.StatusCode,
			"duration", duration,
		}
		switch {
		case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
			lg.Warn("transport roundtrip error", event...)
		default:
			lg.Debug("transport roundtrip error", event...)
		}
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
			return nil, oekiterrors.Errorf("response body exceeds maximum size of %d bytes", maxBodySize)
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
	for _, k := range []string{
		"access_token", "token", "private_token", "password", "passwd",
		"secret", "api_key", "apikey", "key",
	} {
		if q.Get(k) != "" {
			q.Set(k, "***")
			changed = true
		}
	}
	// **userinfo 也要脱敏**:https://user:pass@host/ 的密码部分同样进日志。
	// (BuildAuthURL 这类构造带 userinfo 的 URL 目前没有调用点,但这是"改一行
	// 就会泄漏"的形状 ⇒ 顺手堵上。)
	if u.User != nil {
		if name := u.User.Username(); name != "" {
			u.User = url.UserPassword(name, "***")
		} else {
			u.User = url.User("***")
		}
		changed = true
	}
	if !changed {
		return u.String()
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// redactPath 脱敏请求路径里的秘密。
//
// Request.Path **可能自带 query 串**(见 Request 注释)⇒ 日志里不能原样打:
// 调用方把 token 放进 query 就会随日志与错误信息(→ 响应体)一起外泄。
func redactPath(p string) string {
	i := strings.IndexByte(p, '?')
	if i < 0 {
		return p
	}
	base, rawQuery := p[:i], p[i+1:]
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return base + "?***"
	}
	changed := false
	for _, k := range []string{
		"access_token", "token", "private_token", "password", "passwd",
		"secret", "api_key", "apikey", "key",
	} {
		if q.Get(k) != "" {
			q.Set(k, "***")
			changed = true
		}
	}
	if !changed {
		return p
	}
	return base + "?" + q.Encode()
}

// truncateForLog returns a string representation of body, capped at maxLen
// bytes. Bodies exceeding the limit are truncated with "...".
//
// 委托 stringx.TruncBytes(pkg#327 收编)：字节预算语义不变（at most maxLen
// bytes），RuneStart 回退顺带修复硬字节切对 CJK 响应体产出非法 UTF-8 的
// 缺陷（日志采集端 JSON 编码报错/grep 失效）；ASCII 输入逐字节等价。
func truncateForLog(body []byte, maxLen int) string {
	out, truncated := stringx.TruncBytes(string(body), maxLen)
	if !truncated {
		return out
	}
	return out + "... (truncated)"
}
