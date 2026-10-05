package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Manager caches Provider instances keyed by configuration so repeated
// constructions of the same (platform, baseURL, token) reuse the underlying
// HTTP client pools instead of leaking a fresh one per call.
//
// It automatically detects the platform from clone URLs and reuses existing
// Provider instances within the TTL window. The cache key is derived from
// platform + baseURL + a SHA-256 hash of the token, so different tokens map
// to different entries without leaking the token itself in logs or memory
// dumps.
//
// The type is deliberately thin: the cache mechanics (TTL, LRU capacity
// eviction, counters) live in ttlCache and the background-cleanup lifecycle
// in janitor, leaving Manager exactly key derivation + factory call.
type Manager struct {
	cache  *ttlCache[Provider]
	ttl    time.Duration
	hasher func(token string) string

	jan janitor
}

// Stats reports cache hit/miss counters and the current size. Counters are
// atomically incremented and safe to read concurrently.
type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int
}

// ManagerOption configures a Manager at construction time.
type ManagerOption func(*Manager)

// WithMaxSize caps the cache at n entries. When the cap is reached, the least
// recently used entry is evicted before a new one is inserted.
func WithMaxSize(n int) ManagerOption {
	return func(m *Manager) { m.cache = newTTLCache[Provider](m.ttl, n) }
}

// WithHasher overrides the default SHA-256 token hasher. Useful for tests
// that want deterministic, human-readable keys.
func WithHasher(h func(token string) string) ManagerOption {
	return func(m *Manager) { m.hasher = h }
}

// NewManager creates a new Provider Manager with the given TTL.
// A TTL of 0 means providers never expire (until the process exits).
func NewManager(ttl time.Duration, opts ...ManagerOption) *Manager {
	m := &Manager{
		cache:  newTTLCache[Provider](ttl, 0),
		ttl:    ttl,
		hasher: defaultHasher,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// defaultHasher returns the first 16 hex characters of SHA-256(token). The
// truncation keeps cache keys short while still providing 64 bits of entropy
// (collision probability ~10^-19 for 1000 keys).
func defaultHasher(token string) string {
	return HashToken(token)
}

// HashToken returns the first 16 hex characters of SHA-256(token). It is the
// default token hasher used by Manager and is exported so callers can compute
// cache keys for diagnostics or external caches.
//
// An empty token hashes to an empty string (no anonymous cache entries).
func HashToken(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}

// GetByURL detects the platform from the clone URL and returns a cached or
// newly created Provider.
func (m *Manager) GetByURL(cloneURL, token string) (Provider, error) {
	result, err := DetectPlatform(cloneURL)
	if err != nil {
		return nil, fmt.Errorf("detect platform: %w", err)
	}
	return m.Get(Config{
		Platform: result.Platform,
		BaseURL:  result.BaseURL,
		Token:    token,
	})
}

// Get returns a cached or newly created Provider for the given config. The
// cache key is platform + baseURL + hash(token), so the same (platform,
// baseURL) with a different token gets a distinct entry.
func (m *Manager) Get(cfg Config) (Provider, error) {
	key := m.buildKey(cfg)
	return m.cache.getOrBuild(key, func() (Provider, error) { return NewProvider(cfg) })
}

// Remove removes a cached Provider by config.
func (m *Manager) Remove(cfg Config) {
	m.cache.remove(m.buildKey(cfg))
}

// Purge removes all cached Providers.
func (m *Manager) Purge() {
	m.cache.purge()
}

// Len returns the number of cached Providers.
func (m *Manager) Len() int {
	return m.cache.len()
}

// Stats returns a snapshot of cache counters. The counters continue to
// accumulate across calls; reset them with ResetStats.
func (m *Manager) Stats() Stats {
	return Stats{
		Hits:      m.cache.hits.Load(),
		Misses:    m.cache.misses.Load(),
		Evictions: m.cache.evictions.Load(),
		Size:      m.cache.len(),
	}
}

// ResetStats zeroes the hit/miss/eviction counters. Cache entries are not
// affected.
func (m *Manager) ResetStats() {
	m.cache.hits.Store(0)
	m.cache.misses.Store(0)
	m.cache.evictions.Store(0)
}

// Cleanup removes expired entries from the cache. Safe to call manually; also
// invoked periodically by StartJanitor.
func (m *Manager) Cleanup() {
	m.cache.cleanup()
}

// StartJanitor launches a background goroutine that calls Cleanup every
// interval until ctx is cancelled or Stop is called. Calling StartJanitor
// more than once without an intervening Stop (or without the previous
// goroutine having exited) is a no-op. When the goroutine exits on its own
// — e.g. because ctx was cancelled — it resets the janitor state, so a
// subsequent StartJanitor with a fresh ctx launches a new goroutine instead
// of being mistaken for "already running".
func (m *Manager) StartJanitor(ctx context.Context, interval time.Duration) {
	m.jan.cleanup = m.Cleanup
	m.jan.start(ctx, interval)
}

// Stop halts the background janitor and blocks until it has exited.
func (m *Manager) Stop() {
	m.jan.stopAndWait()
}

// buildKey derives a stable cache key from the config. The token is passed
// through the configured hasher (SHA-256 by default) so the raw token never
// appears in the key. This avoids accidental leakage via logs, error
// messages, or heap dumps.
func (m *Manager) buildKey(cfg Config) string {
	// SkipTLS 参与 key:TLS 策略变化必须产生新 provider,
	// 否则更新平台配置后仍命中旧缓存的严格 TLS client,直到重启才生效。
	return fmt.Sprintf("%s:%s:%s:%t", cfg.Platform, cfg.BaseURL, m.hasher(cfg.Token), cfg.SkipTLS)
}
