package backendutil

import (
	"net/http"
	"strings"

	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// NewTransportClient builds the transport.Client every backend shares between
// its own raw HTTP calls and its SDK round tripper, applying the provider
// config concerns exactly once: auth style (static token or refreshable
// TokenSource), logger, ETag conditional-request cache, retry policy,
// TLS-skip transport, and hooks. The extra options are appended last, so a
// backend with platform-specific plumbing (e.g. TencentCODE's pinned TLS
// transport) can override the standard wiring per option.
func NewTransportClient(cfg provider.Config, baseURL, style string, extra ...transport.ClientOption) *transport.Client {
	logger := cfg.Logger
	if logger == nil {
		logger = provider.NewNoopLogger()
	}
	opts := []transport.ClientOption{
		transport.WithLogger(ToTransportLogger(logger)),
		transport.WithETag(ConditionalCache(cfg)),
		transport.WithRetry(MapRetryConfig(cfg.RetryConfig)),
	}
	if cfg.SkipTLS {
		opts = append(opts, transport.WithTransport(HTTPTransport(true)))
	}
	if cfg.Hooks != nil {
		opts = append(opts, transport.WithHooks(ConvertHooks(cfg.Hooks)))
	}
	opts = append(opts, extra...)
	return transport.NewClient(baseURL, Auth(cfg, style), opts...)
}

// SDKHTTPClient builds the http.Client handed to a platform SDK so that
// SDK-issued requests flow through the same auth/retry/hooks pipeline as the
// backend's own calls: the SkipTLS-honouring transport underneath the
// client's retrying round tripper.
func SDKHTTPClient(tc *transport.Client, skipTLS bool) *http.Client {
	return &http.Client{
		Timeout:   transport.DefaultTimeout,
		Transport: ChainTransport(HTTPTransport(skipTLS), tc.NewRetryingRoundTripper()),
		// 跨 host 的重定向**不带凭据**。
		//
		// Go 只在"请求头已存在"时剥 Authorization/Cookie 等;
		// 认证是在 RoundTripper 里每跳重新施加的(transport 侧另有拦截),
		// 而非标准头(PRIVATE-TOKEN 等)Go 本来就不剥 ⇒ 这里统一兜底。
		//
		// query 也要剥:go-gitee 的 tokenTransport 把令牌放在
		// `access_token` query 参数里,且**每一跳都重新加**(它包在
		// http.Client 的 Transport 外层)⇒ 平台 302 到 CDN 时令牌会外泄。
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 与认证目标(链头,即发起请求)比较,不与紧邻上一跳比较(issue #65):
			// 「平台 → 第三方 → 第三方同域再跳」链形的第二跳若只比上一跳,
			// 会因同 host 而跳过剥除 ⇒ 平台令牌随第二跳外泄第三方主机。
			// Go 只剥 Authorization/Cookie 家族且只比链头 host;非标准头
			// (PRIVATE-TOKEN 等)Go 本来就不剥 ⇒ 这里按链头口径统一兜底。
			if len(via) > 0 && via[0].URL != nil && req.URL != nil &&
				!strings.EqualFold(via[0].URL.Host, req.URL.Host) {
				transport.StripSensitiveCredentials(req)
			}
			return nil
		},
	}
}
