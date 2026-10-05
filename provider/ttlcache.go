package provider

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

// ttlCache is the generic TTL+LRU cache behind Manager: TTL expiry, bounded
// capacity with least-recently-used eviction, and atomic hit/miss/eviction
// counters. It is internal (lowercase) — the Manager surface stays the only
// public contract — and exists so the cache mechanics are one testable
// component instead of five concerns folded into the Manager type.
//
// Values mirror no shared state: build (the factory) runs OUTSIDE the lock,
// and getOrBuild double-checks under the write lock so concurrent builders
// converge on one winner.
type ttlCache[V any] struct {
	mu      sync.RWMutex
	entries map[string]ttlEntry[V]
	ttl     time.Duration // 0 = never expires
	maxSize int           // 0 = unlimited

	// order tracks access recency for capacity eviction and is non-nil only
	// when maxSize > 0. Values mirror entries' key set; the LRU list is the
	// single source of "least recently used" so eviction targets real usage
	// (hits refresh recency), not creation time.
	order *lru.Cache[string, struct{}]

	// stats counters (atomic)
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

type ttlEntry[V any] struct {
	value     V
	createdAt time.Time
}

// newTTLCache builds a cache with the given expiry and capacity
// (ttl <= 0 means never expire; maxSize <= 0 means unlimited). Panics only
// on an LRU construction failure, which cannot happen for maxSize > 0.
func newTTLCache[V any](ttl time.Duration, maxSize int) *ttlCache[V] {
	c := &ttlCache[V]{entries: map[string]ttlEntry[V]{}, ttl: ttl, maxSize: maxSize}
	if maxSize > 0 {
		order, err := lru.New[string, struct{}](maxSize)
		if err != nil {
			panic(fmt.Sprintf("provider: newTTLCache: %v", err))
		}
		c.order = order
	}
	return c
}

// fresh reports whether the entry is still within its TTL.
func (c *ttlCache[V]) fresh(e ttlEntry[V], now time.Time) bool {
	return c.ttl == 0 || now.Sub(e.createdAt) < c.ttl
}

// get returns the cached value when present and fresh, refreshing access
// recency. The second return reports a hit.
func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	fresh := ok && c.fresh(e, time.Now())
	c.mu.RUnlock()
	if !fresh {
		var zero V
		return zero, false
	}
	// Refresh access recency on its own internal lock; lru.Cache is
	// thread-safe, so hits stay independent of the entries map lock.
	if c.order != nil {
		c.order.Get(key)
	}
	c.hits.Add(1)
	return e.value, true
}

// getOrBuild returns the cached value, or builds it via build (outside the
// lock) and stores it. A concurrent builder that raced ahead wins the
// double-check and its value is returned to both callers.
func (c *ttlCache[V]) getOrBuild(key string, build func() (V, error)) (V, error) {
	if v, ok := c.get(key); ok {
		return v, nil
	}
	c.misses.Add(1)

	v, err := build()
	if err != nil {
		var zero V
		return zero, err
	}

	now := time.Now()
	c.mu.Lock()
	// Check again under write lock in case another goroutine raced ahead.
	if e, ok := c.entries[key]; ok && c.fresh(e, now) {
		c.mu.Unlock()
		return e.value, nil
	}
	// Enforce max size by evicting the least recently used tracked entry.
	if c.order != nil {
		for c.order.Len() >= c.maxSize {
			evKey, _, _ := c.order.RemoveOldest()
			if _, stillCached := c.entries[evKey]; stillCached {
				delete(c.entries, evKey)
				c.evictions.Add(1)
			}
			if evKey == key {
				break // re-inserting an existing key; nothing else to evict
			}
		}
		c.order.Add(key, struct{}{})
	}
	c.entries[key] = ttlEntry[V]{value: v, createdAt: now}
	c.mu.Unlock()

	return v, nil
}

// remove drops one entry.
func (c *ttlCache[V]) remove(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	if c.order != nil {
		c.order.Remove(key)
	}
	c.mu.Unlock()
}

// purge drops every entry.
func (c *ttlCache[V]) purge() {
	c.mu.Lock()
	c.entries = map[string]ttlEntry[V]{}
	if c.order != nil {
		c.order.Purge()
	}
	c.mu.Unlock()
}

// len reports the number of cached entries (including expired-but-not-yet-
// collected ones).
func (c *ttlCache[V]) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// cleanup removes expired entries.
func (c *ttlCache[V]) cleanup() {
	if c.ttl == 0 {
		return
	}
	c.mu.Lock()
	cutoff := time.Now().Add(-c.ttl)
	for k, e := range c.entries {
		if e.createdAt.Before(cutoff) {
			delete(c.entries, k)
			if c.order != nil {
				c.order.Remove(k)
			}
			c.evictions.Add(1)
		}
	}
	c.mu.Unlock()
}
