package provider

import oekiterrors "github.com/oekit/pkg/errors"

// Config holds the configuration for creating a Provider.
type Config struct {
	Platform Platform
	BaseURL  string
	Token    string
	SkipTLS  bool

	// TokenSource, when non-nil, supplies the access token on every
	// request through the transport layer and takes precedence over Token.
	// Use it for credentials that rotate or expire (GitHub App installation
	// tokens, OAuth flows); see the TokenSource interface. Static tokens
	// keep using Token.
	TokenSource TokenSource

	// ConditionalRequests enables HTTP conditional requests for GETs: the
	// transport caches responses keyed by their ETag, sends If-None-Match
	// on re-reads, and replays the cached body when the platform answers
	// 304. On platforms that meter 304s favorably (GitHub) this raises
	// sustainable polling rates for wait-for-CI style workloads. Off by
	// default; see transport.ETagCache for the exact semantics.
	ConditionalRequests bool

	// TokenStyle overrides the default authentication header style.
	// Supported values: "private" (PRIVATE-TOKEN, default for GitLab),
	// "bearer" (Authorization: Bearer). Empty string uses platform default.
	TokenStyle string

	// Logger for provider operations. Defaults to a no-op logger.
	Logger Logger
	// RetryConfig for automatic retry on transient failures. nil means no retry.
	RetryConfig *RetryConfig
	// Hooks for request/response lifecycle interception.
	Hooks *Hooks
}

// NewProvider creates a Provider for the given platform using the registry.
// Returns ErrPlatformNotSupported if the platform is not registered.
//
// Platform backends are registered via init() functions. Import
// "github.com/yi-nology/go-git-platform/backends/all" with a blank
// identifier to register every platform shipped with the SDK.
func NewProvider(cfg Config) (Provider, error) {
	registryMu.RLock()
	ctor, ok := registry[cfg.Platform]
	registryMu.RUnlock()

	if !ok {
		return nil, oekiterrors.Wrap(ErrPlatformNotSupported, "platform not registered", "platform", cfg.Platform)
	}
	return ctor(cfg)
}
