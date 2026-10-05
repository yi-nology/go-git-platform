package backendutil

import (
	"net/http"

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
	}
}
