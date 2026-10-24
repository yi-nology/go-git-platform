package provider

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTTLCache_SingleFlightOnConcurrentMiss 锁定「同一 key 的并发未命中只构造
// 一个实例，所有调用方共享它」——这是收编到上游原语后必须保住的历史语义
// （原实现用双检收敛到同一个赢家；构造 Provider 会新建 http.Client 与连接池，
// N 份并发持有是真实资源浪费）。
func TestTTLCache_SingleFlightOnConcurrentMiss(t *testing.T) {
	c := newTTLCache[string](time.Minute, 0)

	var builds atomic.Int64
	build := func() (string, error) {
		builds.Add(1)
		// 拉长构造窗口，让并发调用确实落在同一个 flight 上。
		time.Sleep(20 * time.Millisecond)
		return "value", nil
	}

	const callers = 32
	var wg sync.WaitGroup
	results := make([]string, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = c.getOrBuild("k", build)
		}(i)
	}
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: unexpected error: %v", i, errs[i])
		}
		if results[i] != "value" {
			t.Fatalf("caller %d: got %q, want %q", i, results[i], "value")
		}
	}
	if got := builds.Load(); got != 1 {
		t.Errorf("expected exactly 1 build for %d concurrent misses, got %d", callers, got)
	}
}

// TestTTLCache_MissCountPerLookup 锁定对外 Stats() 的计数口径：每次查找各计
// 一次（未命中计 Misses，命中计 Hits），互斥。
//
// 这里刻意不用上游 cache.SingleFlightCache 装饰器：它的前置 Peek 会先计一次
// Misses、内层 GetOrLoad 再计一次，一次未命中被计成两次，会破坏该契约。
func TestTTLCache_MissCountPerLookup(t *testing.T) {
	c := newTTLCache[string](time.Minute, 0)
	build := func() (string, error) { return "value", nil }

	if _, err := c.getOrBuild("k", build); err != nil {
		t.Fatalf("first getOrBuild: %v", err)
	}
	s := c.stats()
	if s.Misses != 1 || s.Hits != 0 {
		t.Errorf("after 1 miss: got hits=%d misses=%d, want hits=0 misses=1", s.Hits, s.Misses)
	}

	if _, err := c.getOrBuild("k", build); err != nil {
		t.Fatalf("second getOrBuild: %v", err)
	}
	s = c.stats()
	if s.Hits != 1 || s.Misses != 1 {
		t.Errorf("after 1 hit: got hits=%d misses=%d, want hits=1 misses=1", s.Hits, s.Misses)
	}
}

// TestTTLCache_ConcurrentMissCountsEachLookup 锁定并发未命中时每次查找各计一次
// Misses（与收编前的历史实现一致）。
func TestTTLCache_ConcurrentMissCountsEachLookup(t *testing.T) {
	c := newTTLCache[string](time.Minute, 0)
	build := func() (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "value", nil
	}

	const callers = 8
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.getOrBuild("k", build)
		}()
	}
	wg.Wait()

	s := c.stats()
	if s.Misses != callers {
		t.Errorf("expected %d misses (one per lookup), got %d", callers, s.Misses)
	}
	if s.Hits != 0 {
		t.Errorf("expected 0 hits, got %d", s.Hits)
	}
}

// TestTTLCache_ExpiredEntryCountsAsEviction 锁定 Evictions 的历史合并口径：
// 容量淘汰与 TTL 过期回收都计入 Evictions（上游分列为 Evictions/Expirations）。
func TestTTLCache_ExpiredEntryCountsAsEviction(t *testing.T) {
	c := newTTLCache[string](20*time.Millisecond, 0)
	build := func() (string, error) { return "value", nil }

	if _, err := c.getOrBuild("k", build); err != nil {
		t.Fatalf("getOrBuild: %v", err)
	}
	if s := c.stats(); s.Evictions != 0 {
		t.Errorf("expected 0 evictions before expiry, got %d", s.Evictions)
	}

	time.Sleep(40 * time.Millisecond)
	c.cleanup()

	// 上游把容量淘汰与 TTL 过期回收分列为 Evictions / Expirations；
	// 合并成历史口径（两者都算淘汰）发生在 Manager.Stats()。
	s := c.stats()
	if got := s.Evictions + s.Expirations; got != 1 {
		t.Errorf("expected 1 eviction (merged口径) after TTL sweep, got %d (evictions=%d expirations=%d)",
			got, s.Evictions, s.Expirations)
	}
}

// TestTTLCache_ResetStatsZeroesCounters 锁定 ResetStats 的归零语义（上游计数
// 单调不减，归零以「记基线」实现，对外按差值暴露）。
func TestTTLCache_ResetStatsZeroesCounters(t *testing.T) {
	c := newTTLCache[string](time.Minute, 0)
	build := func() (string, error) { return "value", nil }

	if _, err := c.getOrBuild("k", build); err != nil {
		t.Fatalf("getOrBuild: %v", err)
	}
	c.resetStats()

	s := c.stats()
	if s.Hits != 0 || s.Misses != 0 || s.Evictions != 0 {
		t.Errorf("expected zeroed counters after reset, got %+v", s)
	}
	if s.Size != 1 {
		t.Errorf("reset must not clear cache, got size %d", s.Size)
	}

	// 归零后重新累计：从 0 起算而不是从历史总量起算。
	if _, err := c.getOrBuild("k", build); err != nil {
		t.Fatalf("getOrBuild after reset: %v", err)
	}
	if s := c.stats(); s.Hits != 1 || s.Misses != 0 {
		t.Errorf("expected hits=1 misses=0 after reset, got hits=%d misses=%d", s.Hits, s.Misses)
	}
}
