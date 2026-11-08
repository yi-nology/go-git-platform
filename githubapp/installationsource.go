package githubapp

import (
	"context"
	"sync"
	"time"

	"github.com/yi-nology/go-git-platform/transport"
)

// DefaultTokenTTL is the assumed validity of an installation token when the
// response omits expires_at. GitHub issues one-hour tokens; assuming the
// documented default is strictly safer than caching forever.
const DefaultTokenTTL = time.Hour

// DefaultRefreshMargin is how long before ExpiresAt a replacement token is
// fetched. GitHub rejects tokens the instant they expire, so a token used
// at 59:59.9 fails; refreshing a minute early keeps in-flight requests that
// obtained the token moments ago inside its validity window.
const DefaultRefreshMargin = time.Minute

// InstallationTokenSource is a caching TokenSource over
// FetchInstallationToken: it fetches an installation access token once,
// serves it until shortly before its expires_at, and only then mints a
// fresh JWT and fetches a replacement. Concurrent callers collapse onto a
// single fetch (the first one wins, the rest reuse its result), so wiring
// this straight into provider.Config.TokenSource costs one JWT signature
// plus one POST per hour — not per request.
//
//	src := githubapp.NewInstallationTokenSource("", appID, installationID, pem)
//	p, err := provider.NewProvider(provider.Config{
//	    Platform:    provider.PlatformGitHub,
//	    TokenSource: src, // satisfies provider.TokenSource structurally
//	})
//
// Refresh failures return the error and leave the cache untouched, so the
// next call retries; a token already cached is never served past its
// expiry.
type InstallationTokenSource struct {
	APIBase        string
	AppID          int64
	InstallationID int64
	PrivateKeyPEM  string
	// RefreshMargin, when positive, overrides DefaultRefreshMargin. Tests
	// use small values to exercise the expiry paths without sleeping.
	RefreshMargin time.Duration
	// FallbackTTL, when positive, overrides DefaultTokenTTL for responses
	// without expires_at.
	FallbackTTL time.Duration

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// compile-time guarantee: usable wherever a transport (or the structurally
// identical provider) TokenSource is accepted.
var _ transport.TokenSource = (*InstallationTokenSource)(nil)

// NewInstallationTokenSource builds a source for one app installation.
// apiBase follows FetchInstallationToken's rules (empty = GitHub public
// API; GHES passes its own /api/v3 root).
func NewInstallationTokenSource(apiBase string, appID, installationID int64, privateKeyPEM string) *InstallationTokenSource {
	return &InstallationTokenSource{
		APIBase:        apiBase,
		AppID:          appID,
		InstallationID: installationID,
		PrivateKeyPEM:  privateKeyPEM,
	}
}

// Token returns a currently-valid installation access token, fetching or
// refreshing as needed. It is safe for concurrent use.
func (s *InstallationTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Add(s.margin()).Before(s.expiresAt) {
		return s.token, nil
	}
	token, expiresAt, err := fetchInstallationToken(ctx, s.APIBase, s.AppID, s.InstallationID, s.PrivateKeyPEM)
	if err != nil {
		// stale-if-error（issue #46）：margin 窗口内的刷新失败（5xx/超时/
		// DNS 抖动）不得丢弃仍然有效的缓存 token——那会让 margin 内的每次
		// API 调用都报错，尽管手里握着 GitHub 仍会接受的凭据。只要缓存
		// 未过真实过期时间就继续用（并留告警），没有可用凭据时才报错。
		if s.token != "" && time.Now().Before(s.expiresAt) {
			return s.token, nil
		}
		// 缓存保持原样：本次失败不清旧值，下一次调用照常重试。
		return "", err
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(s.fallbackTTL())
	}
	s.token, s.expiresAt = token, expiresAt
	return token, nil
}

// ExpiresAt reports when the cached token expires (zero value when nothing
// has been fetched yet) — for dashboards and tests.
func (s *InstallationTokenSource) ExpiresAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expiresAt
}

func (s *InstallationTokenSource) margin() time.Duration {
	if s.RefreshMargin > 0 {
		return s.RefreshMargin
	}
	return DefaultRefreshMargin
}

func (s *InstallationTokenSource) fallbackTTL() time.Duration {
	if s.FallbackTTL > 0 {
		return s.FallbackTTL
	}
	return DefaultTokenTTL
}
