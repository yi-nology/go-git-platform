package provider

import (
	"errors"
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
	//nolint:unparam // 闭包签名由 GetOrLoadWithSource 的 func 类型钉死，error 位恒 nil 是本用例的事实
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
// 收编到上游 cache.SingleFlightCache 装饰器后仍成立：装饰器的前置 Peek 计一次
// 未命中、回源加载路径不再重复计（上游 pkg#90 修正）⇒ 一次查找恰好计入其中
// 之一。
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

	// 到齐门闩 + 沉降窗口：回源函数等到全部调用方完成「计数→进入查找」
	// 后才回填。此前的 10ms sleep 是调度竞态——CI macOS runner 高负载时
	// goroutine 启动延迟可超过 10ms，晚到的查找命中已回填的缓存（v0.103.1
	// CI macos 腿实证 7 miss + 1 hit）；纯门闩（只等计数）在 race 检测器
	// 下仍有「计数后、查找前」被抢占的间隙（压测实证 6 miss + 2 hit），
	// 5ms 沉降覆盖该纯用户态间隙，2s 超时兜底防挂死。
	const callers = 8
	var arrived atomic.Int64
	deadline := time.Now().Add(2 * time.Second)
	//nolint:unparam // 闭包签名由 getOrBuild 的 func 类型钉死，error 位恒 nil 是本用例的事实
	build := func() (string, error) {
		for arrived.Load() < callers {
			if time.Now().After(deadline) {
				break // 防御：调度异常时按超时放行，不挂死测试
			}
			time.Sleep(time.Millisecond)
		}
		time.Sleep(5 * time.Millisecond) // 沉降：等全部调用方真正进入查找
		return "value", nil
	}

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			arrived.Add(1)
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

// TestTTLCache_FailedBuildIsNotCached 锁定「回源失败不写缓存」：失败必须能被
// 下一次查找重试，不能被 null marker 之类的空值标记挡住。
func TestTTLCache_FailedBuildIsNotCached(t *testing.T) {
	c := newTTLCache[string](time.Minute, 0)

	var builds atomic.Int64
	build := func() (string, error) {
		if builds.Add(1) == 1 {
			return "", errors.New("boom")
		}
		return "value", nil
	}

	if _, err := c.getOrBuild("k", build); err == nil {
		t.Fatal("expected error from failing build")
	}
	if _, err := c.getOrBuild("k", build); err != nil {
		t.Fatalf("second getOrBuild should retry the build: %v", err)
	}
	if got := builds.Load(); got != 2 {
		t.Errorf("expected 2 builds (failure must not be cached), got %d", got)
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

// TestTTLCache_RemoveDuringFlightDoesNotResurrect pins the issue #135 fix:
// the self-built flight's backfill must re-check the invalidation generation
// before Set — Remove/Purge racing an in-flight build must not resurrect the
// stale value into the cache (the caller still gets the built value, matching
// the upstream LocalCache load-path semantics). The old shape backfilled
// unconditionally, so a Manager.Remove(cfg) during the build window left the
// removed (stale) instance in the cache for the next Get.
func TestTTLCache_RemoveDuringFlightDoesNotResurrect(t *testing.T) {
	c := newTTLCache[string](time.Minute, 0)

	release := make(chan struct{})
	var builds atomic.Int64
	//nolint:unparam // 闭包签名由 getOrBuild 的 build 类型钉死,error 位恒 nil 是本用例的事实
	build := func() (string, error) {
		builds.Add(1)
		<-release // 构造窗口悬停,给 remove 让出竞争窗口
		return "stale-instance", nil
	}

	// 起飞:winner 卡在 build 窗口内。
	done := make(chan struct{})
	var got string
	var gotErr error
	go func() {
		defer close(done)
		got, gotErr = c.getOrBuild("k", build)
	}()

	// 等 winner 真正起飞(进入 build)。
	deadline := time.Now().Add(2 * time.Second)
	for builds.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if builds.Load() == 0 {
		t.Fatal("winner never entered build")
	}

	// 竞争:构造在途时失效该 key。
	c.remove("k")
	close(release)
	<-done

	if gotErr != nil {
		t.Fatalf("getOrBuild: %v", gotErr)
	}
	if got != "stale-instance" {
		t.Errorf("caller must still receive the built value (upstream semantics), got %q", got)
	}

	// 回填必须被放弃:缓存里不得有旧实例复活,下一次查找必须重新构造。
	if n := c.len(); n != 0 {
		t.Errorf("cache must stay empty after remove-during-flight, got %d entries", n)
	}
	var secondBuilds atomic.Int64
	if _, err := c.getOrBuild("k", func() (string, error) {
		secondBuilds.Add(1)
		return "fresh-instance", nil
	}); err != nil {
		t.Fatalf("second getOrBuild: %v", err)
	}
	if secondBuilds.Load() != 1 {
		t.Errorf("post-remove lookup must rebuild (stale value was resurrected), builds=%d", secondBuilds.Load())
	}
}

// TestTTLCache_PurgeDuringFlightDoesNotResurrect — purge 同世代防线
// (issue #135):清空全部条目后,在途构造的回填同样必须放弃。
func TestTTLCache_PurgeDuringFlightDoesNotResurrect(t *testing.T) {
	c := newTTLCache[string](time.Minute, 0)

	release := make(chan struct{})
	//nolint:unparam // 闭包签名由 getOrBuild 的 build 类型钉死,error 位恒 nil 是本用例的事实
	build := func() (string, error) {
		<-release
		return "stale", nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.getOrBuild("k", build)
	}()
	time.Sleep(10 * time.Millisecond) // 让 winner 起飞

	c.purge()
	close(release)
	<-done

	if n := c.len(); n != 0 {
		t.Errorf("cache must stay empty after purge-during-flight, got %d entries", n)
	}
}
