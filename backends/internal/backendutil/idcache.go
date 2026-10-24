package backendutil

import (
	"context"
	"time"

	"github.com/oekit/pkg/cache"
)

// IDCache is a TTL cache from string keys to int64 IDs. Backends use it to
// memoize name→ID resolutions (labels, users) so repeated writes do not
// rescan. The zero value is not usable; construct with NewIDCache.
//
// Storage, TTL and expiry bookkeeping are delegated to oekit/pkg/cache's
// LocalCache (in-process TTL+LRU cache); the backend-facing contract
// (Get/Put/Evict/Len) is unchanged.
type IDCache struct {
	cache *cache.LocalCache[int64]
}

// NewIDCache constructs an IDCache with the given TTL (TTL <= 0 never
// expires entries).
func NewIDCache(ttl time.Duration) *IDCache {
	return &IDCache{
		cache: cache.NewLocalCache[int64](cache.LocalConfig[int64]{
			TTL: ttl,
			// IDCache 原本没有常驻协程，这里同样不引入后台清扫 goroutine：
			// 过期项由读路径惰性删除，Evict 负责显式清扫。
			NoBackgroundSweep: true,
		}),
	}
}

// Get returns the cached ID for key when present and fresh.
func (c *IDCache) Get(key string) (int64, bool) {
	value, err := c.cache.Get(context.Background(), key)
	if err != nil || value == nil {
		return 0, false
	}
	return *value, true
}

// Put stores id under key with a fresh TTL.
func (c *IDCache) Put(key string, id int64) {
	value := id
	_ = c.cache.Set(context.Background(), key, &value)
}

// Evict removes all expired entries from the cache. Call this periodically
// to prevent unbounded memory growth in long-lived processes.
func (c *IDCache) Evict() {
	c.cache.Sweep(context.Background())
}

// Len returns the number of entries in the cache (including stale ones).
func (c *IDCache) Len() int {
	return c.cache.Size()
}
