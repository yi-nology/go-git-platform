package provider

import (
	"context"
	"sync"
	"time"

	"github.com/oekit/pkg/cache"
)

// ttlCache 是 Manager 背后的 TTL+LRU 缓存：TTL 过期、有界容量（最久未用者先
// 淘汰）、命中/未命中/淘汰计数。它是内部类型（小写）——Manager 仍是唯一对外
// 契约面——存在的意义是让缓存机制是一个可测组件，而不是把五件事揉进 Manager。
//
// 实现全部委托上游原语，本文件只做两件事：
//   - 把 Manager 需要的操作面（getOrBuild / remove / purge / len / cleanup）
//     映射到原语；
//   - 保住「ResetStats 归零」语义：上游计数单调不减，ResetStats 改为记基线、
//     Stats 读差值——对外可见的口径不变（归零后重新从 0 累计）。
//
// 两个上游原语各司其职：
//   - oekit/pkg/cache.LocalCache —— 存储 + TTL/LRU 淘汰 + 观测计数；
//   - oekit/pkg/cache.SingleFlightCache —— 在 LocalCache 之上做回源去重：同一
//     key 的并发未命中只构造一个实例，所有调用方共享它（构造 Provider 会新建
//     http.Client 与连接池，N 份并发持有是真实的资源浪费）。
//
// 计数口径（对外契约）：读命中计 Hits、读未命中计 Misses，二者互斥——**每次
// 查找各计一次**，与本仓历史实现一致。装饰器的前置 Peek 计一次未命中、回源
// 加载路径不再重复计（上游 pkg#90 修正）⇒ 一次查找恰好计入命中或未命中之一。
//
// 装饰器不提供 Clear/Size/Sweep（这三项只在内层有），因此内层句柄仍留着，
// 由 remove/purge/len/cleanup 走内层。
//
// 回源函数由**调用点注入**（GetOrLoadWithSource）：Manager.Get 的回源闭包
// 捕获的是本次调用的 cfg（Provider 按 cfg 构造），构造期固定的 Source 拿不到它。
type ttlCache[V any] struct {
	inner *cache.LocalCache[V]       // 装饰器未透传 Clear/Size/Sweep，走内层
	sf    *cache.SingleFlightCache[V] // 读路径 + 回源去重 + 计数

	mu       sync.Mutex
	baseline cache.Stats // ResetStats 时刻的计数基线（对外按差值暴露）
}

// newTTLCache 构建缓存（ttl <= 0 表示不过期；maxSize <= 0 表示不限容量）。
//
// NoBackgroundSweep 置 true：清理由 Manager 自己的 janitor 驱动（cleanup →
// Sweep），不在库层另起常驻 goroutine——与本仓原有的 janitor 口径一致。
// EvictPolicy 取 EvictLRU：读命中也提升顺序，容量淘汰针对真实使用而非写入时间。
func newTTLCache[V any](ttl time.Duration, maxSize int) *ttlCache[V] {
	inner := cache.NewLocalCache[V](cache.LocalConfig[V]{
		TTL:               ttl,
		MaxSize:           maxSize,
		EvictPolicy:       cache.EvictLRU,
		NoBackgroundSweep: true,
	})
	return &ttlCache[V]{
		inner: inner,
		sf:    cache.NewSingleFlightCache[V](inner),
	}
}

// getOrBuild 返回缓存值；未命中则用 build 构造并写入。
//
// 未命中时的构造经上游 singleflight 去重：同一 key 的并发调用只真正构造一次，
// 所有调用方拿到同一个实例——这与本仓历史实现的「双检收敛到同一个赢家」
// 语义一致，且顺带省掉了 N-1 次重复构造。
func (c *ttlCache[V]) getOrBuild(key string, build func() (V, error)) (V, error) {
	ctx := context.Background()

	value, err := c.sf.GetOrLoadWithSource(ctx, key, func(context.Context, string) (*V, error) {
		built, err := build()
		if err != nil {
			// 回源失败不写缓存（含 null marker）：失败必须能被下一次查找重试。
			return nil, err
		}
		// 回填 TTL 由内层按构造期 TTL 落（cfg.TTL；ttl <= 0 = 永不过期）。
		return &built, nil
	})
	if err != nil {
		var zero V
		return zero, err
	}
	if value == nil {
		var zero V
		return zero, nil
	}
	return *value, nil
}

// remove 丢弃一个条目。
func (c *ttlCache[V]) remove(key string) {
	_ = c.sf.Del(context.Background(), key)
}

// purge 丢弃全部条目。
func (c *ttlCache[V]) purge() {
	c.inner.Clear()
}

// len 报告缓存条目数（含已过期但尚未回收的）。
func (c *ttlCache[V]) len() int {
	return c.inner.Size()
}

// cleanup 回收过期条目。
func (c *ttlCache[V]) cleanup() {
	if c.inner == nil {
		return
	}
	c.inner.Sweep(context.Background())
}

// stats 返回本轮（自上次 ResetStats 起）的计数快照。
//
// Evictions 口径与本仓历史实现保持一致：**容量淘汰 + TTL 过期回收都计入**
// （上游把两者分列为 Evictions / Expirations，这里按历史口径合并）。
func (c *ttlCache[V]) stats() cache.Stats {
	cur := c.sf.Stats()
	c.mu.Lock()
	defer c.mu.Unlock()
	return cache.Stats{
		Hits:        cur.Hits - c.baseline.Hits,
		Misses:      cur.Misses - c.baseline.Misses,
		Evictions:   cur.Evictions - c.baseline.Evictions,
		Expirations: cur.Expirations - c.baseline.Expirations,
		Size:        cur.Size,
	}
}

// resetStats 把计数归零（上游计数单调不减，归零以「记基线」实现）。
func (c *ttlCache[V]) resetStats() {
	cur := c.sf.Stats()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseline = cur
}
