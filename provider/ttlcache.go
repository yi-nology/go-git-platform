package provider

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oekit/pkg/cache"
	"github.com/oekit/pkg/container"
	oekiterrors "github.com/oekit/pkg/errors"
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
// 回源函数由**调用点注入**（按 key 收敛见 getOrBuild）：Manager.Get 的回源闭包
// 捕获的是本次调用的 cfg（Provider 按 cfg 构造），构造期固定的 Source 拿不到它。
type ttlCache[V any] struct {
	inner *cache.LocalCache[V]        // 装饰器未透传 Clear/Size/Sweep，走内层
	sf    *cache.SingleFlightCache[V] // 读路径 + 计数 + 回填

	// 按 key 收敛的回源 flight（赢家构造、等待者共享）。容器层互斥
	// 取代手写 flightsMu（oekit 收编；自建 flight 语义不变——#353
	// 存档只禁委托 SingleFlightCache，不禁换容器）。
	flights *container.SafeMap[string, *ttlFlight[V]]
	// 失效世代：remove/purge 推进（issue #135）。原子计数取代与
	// flights 共用的互斥段——捕获/比较语义与锁版逐点等价。
	gen atomic.Uint64

	mu       sync.Mutex
	baseline cache.Stats // ResetStats 时刻的计数基线（对外按差值暴露）
}

// ttlFlight 是一次按 key 收敛的回源：赢家跑 build 并回填缓存，等待者在 done
// 关闭后共享结果（v/err 在 done 关闭后只读——channel close 提供 happens-before）。
type ttlFlight[V any] struct {
	done chan struct{}
	v    V
	err  error
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
		inner:   inner,
		sf:      cache.NewSingleFlightCache[V](inner),
		flights: container.NewSafeMap[string, *ttlFlight[V]](),
	}
}

// getOrBuild 返回缓存值；未命中则用 build 构造并写入。
//
// 未命中时的构造按 key 单飞去重：同一 key 的并发调用只真正构造一次，所有
// 调用方拿到同一个实例——与本仓历史实现的「双检收敛到同一个赢家」语义
// 一致，且省掉 N-1 次重复构造。
//
// 为什么在本地自建 flight 而不用上游装饰器的 GetOrLoadWithSource：回源闭包
// 捕获本次调用的 cfg，构造期固定的 Source 拿不到它；而上游 pkg v0.44.0 起
// （pkg#353）按调用注入的 source **有意不进 singleflight**——source 不可比较，
// 泛化 API 按 key 合并会让等待者共享先到者的回源结果（同 key 不同凭据时
// 串扰），这是上游正确的保守。本缓存的用法下同 key 必然同 cfg（key 由
// buildKey(cfg) 派生）、构造结果可互换，去重安全且必要（构造 Provider 新建
// http.Client 与连接池，N 份并发持有是真实的资源浪费），故按 key 自建。
func (c *ttlCache[V]) getOrBuild(key string, build func() (V, error)) (V, error) {
	ctx := context.Background()

	// 快路径：命中即返（含值为 nil 的真值命中，照上游 #203 口径早退）。
	// 未命中恰好计一次 miss；加载/等待路径不再触碰内层计数——一次查找恰好
	// 计入命中或未命中之一（对外计数契约不变）。
	value, hit, err := c.sf.Peek(ctx, key)
	if err == nil && hit {
		if value == nil {
			var zero V
			return zero, nil
		}
		return *value, nil
	}

	// 按 key 单飞：GetOrSet 原子收敛——不存在则写入新 flight 并当赢家，
	// 已存在则取既有 flight 等 done（与旧「查+存持锁一步」逐点等价）。
	f, loaded := c.flights.GetOrSet(key, &ttlFlight[V]{done: make(chan struct{})})
	if loaded {
		<-f.done
		if f.err != nil {
			var zero V
			return zero, f.err
		}
		return f.v, nil
	}

	// 赢家路径。build panic 时：等待者共享为错误、赢家自身照旧向上抛
	// （recover 后重 panic，panic 值原样传递）。
	defer func() {
		r := recover()
		if r != nil {
			// panic 收编单源（pkg/errors.Recovered，#381 收编）：error 型
			// panic 身份（Unwrap）不丢、非 error 带 %T 类型与 goroutine 栈
			// ——手写双分支删除。
			f.err = oekiterrors.Wrap(oekiterrors.Recovered(r), "provider: 回源构造 panic（同 key 等待者共享同一失败）")
		}
		close(f.done)
		c.flights.Delete(key)
		if r != nil {
			panic(r)
		}
	}()

	// 捕获起飞世代（issue #135）：build 执行期间 remove/purge 会推进世代。
	gen := c.gen.Load()

	f.v, f.err = build()
	if f.err != nil {
		// 回源失败不写缓存（含 null marker）：失败必须能被下一次查找重试。
		var zero V
		return zero, f.err
	}
	// 回填前复查世代（issue #135，对齐上游 LocalCache 加载路径的 #200 防线）：
	// build 执行期间该 key 被 remove/purge（Manager.Remove/Purge 删旧实例）
	// ⇒ 本次回填放弃，旧实例不得复活——「Remove 后必须重建」在并发窗口内
	// 也成立。返回值仍给调用方（与上游口径一致：在途构造的结果不丢）。
	stale := c.gen.Load() != gen
	if !stale {
		// 回填 TTL 由内层按构造期 TTL 落（cfg.TTL；ttl <= 0 = 永不过期）。
		// best-effort 回填：值已构造并返回调用方，写失败只损失下次命中率。
		_ = c.sf.Set(ctx, key, &f.v)
	}
	return f.v, nil
}

// remove 丢弃一个条目。
func (c *ttlCache[V]) remove(key string) {
	c.gen.Add(1)
	// best-effort 删除：remove 无错误通道（Manager.Remove 契约为 void），
	// 失败仅条目残留至 TTL 过期或清扫回收，世代已推进不会被在途回填复活。
	_ = c.sf.Del(context.Background(), key)
}

// purge 丢弃全部条目。
func (c *ttlCache[V]) purge() {
	c.gen.Add(1)
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
