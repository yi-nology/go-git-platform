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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 && via[len(via)-1].URL != nil && req.URL != nil &&
				!strings.EqualFold(via[len(via)-1].URL.Host, req.URL.Host) {
				transport.StripSensitiveHeaders(req)
			}
			return nil
		},
	}
}
