package transport

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides proactive rate limiting by tracking X-RateLimit-*
// headers returned by the server. It combines a configurable requests-per-second
// cap (enforced by golang.org/x/time/rate) with adaptive throttling when the
// server reports low remaining quota.
//
// RateLimiter is safe for concurrent use.
type RateLimiter struct {
	mu sync.Mutex

	// Configuration
	rps       float64       // max requests per second (0 = unlimited)
	minDelay  time.Duration // minimum delay between requests
	threshold int           // start throttling when remaining <= threshold

	// State
	limiter   *rate.Limiter // token bucket for the RPS cap; nil when unlimited
	remaining int           // last seen X-RateLimit-Remaining
	resetAt   time.Time     // last seen X-RateLimit-Reset
	lastReq   time.Time     // time of last request
}

// RateLimiterOption configures a RateLimiter.
type RateLimiterOption func(*RateLimiter)

// WithRPS sets the maximum requests per second. A value <= 0 means unlimited.
func WithRPS(rps float64) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.rps = rps
	}
}

// WithMinDelay sets the minimum delay between consecutive requests.
func WithMinDelay(d time.Duration) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.minDelay = d
	}
}

// WithThrottleThreshold sets the X-RateLimit-Remaining threshold below which
// the limiter starts adding delays to avoid hitting the limit. Default is 10.
func WithThrottleThreshold(n int) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.threshold = n
	}
}

// NewRateLimiter creates a RateLimiter with the given options.
func NewRateLimiter(opts ...RateLimiterOption) *RateLimiter {
	rl := &RateLimiter{
		threshold: 10,
	}
	for _, opt := range opts {
		opt(rl)
	}
	if rl.rps > 0 {
		// Burst 1 spaces requests exactly one interval apart, matching the
		// fixed-interval pacing this cap replaces.
		rl.limiter = rate.NewLimiter(rate.Limit(rl.rps), 1)
	}
	return rl
}

// Wait blocks until the next request is allowed. It considers both the
// configured RPS cap and the adaptive throttling based on remaining quota.
// Use WaitContext if you need cancellation support.
func (rl *RateLimiter) Wait() {
	// 后台等待：context.Background() 不可取消，WaitContext 的错误仅在 ctx
	// 取消时产生——此路径不可达，丢弃无损失；需要取消语义的调用方用 WaitContext。
	_ = rl.WaitContext(context.Background())
}

// WaitContext blocks until the next request is allowed or the context is
// cancelled. It returns ctx.Err() if the context is done before the wait
// completes. It considers both the configured RPS cap and the adaptive
// throttling based on remaining quota.
//
// The lock is only held to compute the wait duration and to reserve a local
// quota slot; the actual sleep happens outside the critical section, so a
// long adaptive wait does not block other waiters, UpdateFromResponse calls,
// or state inspection.
func (rl *RateLimiter) WaitContext(ctx context.Context) error {
	// 1. RPS-based pacing via the token bucket. rate.Limiter is
	// concurrency-safe and does its own waiting, so no lock is held here.
	if rl.limiter != nil {
		if err := rl.limiter.Wait(ctx); err != nil {
			return err
		}
	}

	rl.mu.Lock()
	now := time.Now()

	// 2. Minimum delay between consecutive requests.
	var delay time.Duration
	if rl.minDelay > 0 {
		if elapsed := now.Sub(rl.lastReq); elapsed < rl.minDelay {
			if d := rl.minDelay - elapsed; d > delay {
				delay = d
			}
		}
	}

	// 3. Adaptive throttle based on remaining quota. remaining == 0 (quota
	// exhausted) throttles too — it waits for the reset window — which the
	// previous `remaining > 0` guard got backwards.
	// Each waiter consumes a local reservation by decrementing remaining, so
	// concurrent waiters target progressively later slots instead of all
	// sleeping for the same duration and firing at once (thundering herd).
	// The value is re-synced from real response headers by UpdateFromResponse.
	if rem := max(rl.remaining, 0); rem <= rl.threshold && !rl.resetAt.IsZero() {
		timeUntilReset := time.Until(rl.resetAt)
		if timeUntilReset > 0 {
			if d := timeUntilReset / time.Duration(rem+1); d > time.Second {
				if d > delay {
					delay = d
				}
				rl.remaining--
			}
		}
	}

	// Reserve the slot: the request will effectively fire at now+delay, so
	// lastReq is advanced by the same amount to keep spacing correct for the
	// next waiter. The reservation is kept even if the sleep below is
	// cancelled — a cancelled caller should not hand its slot to a burst.
	rl.lastReq = now.Add(delay)
	rl.mu.Unlock()

	if delay > 0 {
		// Sleep outside the critical section: adaptive delays can reach the
		// whole quota-reset window (tens of minutes), and holding the lock
		// for them would stall every other request and state update.
		if err := rl.sleep(ctx, delay); err != nil {
			return err
		}
	}
	return nil
}

// sleep waits for the given duration or until the context is cancelled.
func (rl *RateLimiter) sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UpdateFromResponse reads X-RateLimit-* headers from the response to update
// the internal state. Call this after every HTTP response.
func (rl *RateLimiter) UpdateFromResponse(resp *http.Response) {
	if resp == nil {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if v := resp.Header.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.remaining = n
		}
	}

	if v := resp.Header.Get("X-RateLimit-Reset"); v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			rl.resetAt = time.Unix(ts, 0)
		}
	}
}

// Remaining returns the last seen X-RateLimit-Remaining value.
func (rl *RateLimiter) Remaining() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.remaining
}

// ResetAt returns the last seen X-RateLimit-Reset time.
func (rl *RateLimiter) ResetAt() time.Time {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.resetAt
}
