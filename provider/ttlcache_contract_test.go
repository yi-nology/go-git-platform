package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oekit/pkg/cache"
	"github.com/oekit/pkg/cache/cachetest"
)

// 本文件用上游 cachetest 套件认证 ttlCache 的读路径。ttlCache 是内部类型，
// 方法面（getOrBuild/remove）与 cache.CacheAside 形状不同，故在测试内写一个
// 仅测试可见的适配器：全部方法构建在 ttlCache 自己的 getOrBuild/remove 上
// （与 Manager.Get/Manager.Remove 同一调用形状），生产代码零修改。
//
// 按套件要求逐变体裁定，未挂载变体的一行原因：
//   - Run：场景3 set_with_ttl_expiry 需 per-key SetWithTTL，ttlCache 回填恒用
//     构造期 TTL（getOrBuild → sf.Set），无 per-key 写路径——SetWithTTL 返回
//     errTTLCachePerKeyTTLUnsupported（fail-loud，不静默降级）；
//   - RunReadIsolation：需 CloneOnRead（读命中返回副本）模式，newTTLCache 未
//     暴露该配置，且适配器 Get 返回共享 *T 指针（改写即污染，正是该场景反例）；
//   - RunWithClock / RunHitWithClock / RunSetNXWithClock /
//     RunSourceInjectionWithClock / RunStaleIfErrorWithClock /
//     RunStaleIfErrorRepeatedWithClock / RunStatsWithClock / RunSweepWithClock：
//     8 个 ClockFactory 变体——newTTLCache 无时钟注入参数（LocalConfig.Now 未
//     接线），且 HitGetter/NXSetter/SourceInjector/StatsProvider/Sweeper 能力面
//     未适配。
//
// 已知限制（挂载的两个变体均为顺序调用，不受影响）：只读 Get/Exists 的 miss
// 经 getOrBuild 的哨兵 build 表达，会走 flight 机制——并发下同 key 的
// Get-miss 与 GetOrLoad 可能共享同一 flight（GetOrLoad 会拿到哨兵错误）。
// Run 的 concurrent_mixed_ops 属此形态，连同场景3 一并不挂载。
var (
	// errTTLCacheMiss 是只读 Get/Exists 经 getOrBuild 表达「未命中」的哨兵：
	// ttlCache 的唯一取值路径 getOrBuild 未命中必回源，只读方法让哨兵 build
	// 立即失败（回源失败不写缓存，语义等价 miss）。
	errTTLCacheMiss = errors.New("provider: ttlcache 适配器只读 miss")

	// errTTLCachePerKeyTTLUnsupported：ttlCache 无 per-key TTL 写路径（见文件头裁定）。
	errTTLCachePerKeyTTLUnsupported = errors.New("provider: ttlcache 不支持 per-key TTL")
)

// ttlCacheAside 把 *ttlCache[*T] 适配成 cache.CacheAside[T]（仅测试内）。
// V 取 *T 使 getOrBuild 的 (V, error) 直接对上 CacheAside 的 (*T, error)。
type ttlCacheAside[T any] struct {
	c      *ttlCache[*T]
	source cache.SourceFunc[T]
}

var _ cache.CacheAside[string] = (*ttlCacheAside[string])(nil)

func newTTLCacheAside[T any](ttl time.Duration, maxSize int, source cache.SourceFunc[T]) *ttlCacheAside[T] {
	return &ttlCacheAside[T]{
		c:      newTTLCache[*T](ttl, maxSize),
		source: source,
	}
}

// Get 只读取值：miss 走哨兵 build；命中返回缓存内的 *T（含显式 nil 真值）。
func (a *ttlCacheAside[T]) Get(_ context.Context, key string) (*T, error) {
	v, err := a.c.getOrBuild(key, func() (*T, error) { return nil, errTTLCacheMiss })
	if errors.Is(err, errTTLCacheMiss) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// Set 覆盖写：ttlCache 无直接写入口，先 remove 再经 getOrBuild 回填
// （与 Manager.Remove + 重建同形状）。
func (a *ttlCacheAside[T]) Set(_ context.Context, key string, value *T) error {
	a.c.remove(key)
	_, err := a.c.getOrBuild(key, func() (*T, error) { return value, nil })
	return err
}

// SetWithTTL 不支持 per-key TTL（ttlCache 回填恒用构造期 TTL）——见文件头裁定。
func (a *ttlCacheAside[T]) SetWithTTL(_ context.Context, _ string, _ *T, _ time.Duration) error {
	return errTTLCachePerKeyTTLUnsupported
}

// Del 丢弃一个条目（幂等）。
func (a *ttlCacheAside[T]) Del(_ context.Context, key string) error {
	a.c.remove(key)
	return nil
}

// DelBatch 批量丢弃（同 Del 幂等）。
func (a *ttlCacheAside[T]) DelBatch(_ context.Context, keys ...string) error {
	for _, key := range keys {
		a.c.remove(key)
	}
	return nil
}

// Exists 经哨兵 build 判定存在：命中（含显式 nil 真值）为 true，miss 为 false。
func (a *ttlCacheAside[T]) Exists(_ context.Context, key string) (bool, error) {
	_, err := a.c.getOrBuild(key, func() (*T, error) { return nil, errTTLCacheMiss })
	if errors.Is(err, errTTLCacheMiss) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetOrLoad 未命中回源回填——与 Manager.Get 同一调用形状（回源闭包按调用点注入）。
func (a *ttlCacheAside[T]) GetOrLoad(ctx context.Context, key string) (*T, error) {
	return a.c.getOrBuild(key, func() (*T, error) { return a.source(ctx, key) })
}

// Refresh 删除后重新从 Source 加载（LocalCache.Refresh 同语义）。
func (a *ttlCacheAside[T]) Refresh(ctx context.Context, key string) (*T, error) {
	a.c.remove(key)
	return a.GetOrLoad(ctx, key)
}

// TestTTLCache_Contract_NoBackgroundGoroutine 用上游套件认证 newTTLCache
// 构造不起后台 goroutine（NoBackgroundSweep: true 的真实行为）。
func TestTTLCache_Contract_NoBackgroundGoroutine(t *testing.T) {
	cachetest.RunNoBackgroundGoroutine[string](t, func(t *testing.T, source cache.SourceFunc[string]) cache.CacheAside[string] {
		a := newTTLCacheAside[string](5*time.Minute, 0, source)
		t.Cleanup(func() { _ = a.c.inner.Close(context.Background()) })
		return a
	})
}

// TestTTLCache_Contract_ReadPromotion 用上游套件认证 ttlCache 的 LRU 读提升：
// newTTLCache 固定 EvictLRU + MaxSize，读命中经 getOrBuild → sf.Peek →
// LocalCache.Peek 的 promoteOne 提升顺序，容量写满后最久未读条目先淘汰。
func TestTTLCache_Contract_ReadPromotion(t *testing.T) {
	const capacity = 8
	cachetest.RunReadPromotion[string](t, cachetest.Comparable[string](), capacity, func(t *testing.T, source cache.SourceFunc[string]) cache.CacheAside[string] {
		a := newTTLCacheAside[string](5*time.Minute, capacity, source)
		t.Cleanup(func() { _ = a.c.inner.Close(context.Background()) })
		return a
	})
}
