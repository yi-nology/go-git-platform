package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_Wait_RPS(t *testing.T) {
	rl := NewRateLimiter(WithRPS(100)) // 100 rps = 10ms interval

	start := time.Now()
	rl.Wait()
	rl.Wait()
	elapsed := time.Since(start)

	// Second wait should have been delayed by ~10ms
	if elapsed < 5*time.Millisecond {
		t.Errorf("expected at least 5ms delay, got %v", elapsed)
	}
}

func TestRateLimiter_Wait_MinDelay(t *testing.T) {
	rl := NewRateLimiter(WithMinDelay(20 * time.Millisecond))

	start := time.Now()
	rl.Wait()
	rl.Wait()
	elapsed := time.Since(start)

	if elapsed < 15*time.Millisecond {
		t.Errorf("expected at least 15ms delay, got %v", elapsed)
	}
}

func TestRateLimiter_UpdateFromResponse(t *testing.T) {
	rl := NewRateLimiter()

	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "42")
	h.Set("X-RateLimit-Reset", "1700000000")
	resp := &http.Response{Header: h}
	rl.UpdateFromResponse(resp)

	if rl.Remaining() != 42 {
		t.Errorf("expected remaining 42, got %d", rl.Remaining())
	}

	expectedReset := time.Unix(1700000000, 0)
	if !rl.ResetAt().Equal(expectedReset) {
		t.Errorf("expected reset at %v, got %v", expectedReset, rl.ResetAt())
	}
}

func TestRateLimiter_UpdateFromResponse_Nil(t *testing.T) {
	rl := NewRateLimiter()
	rl.UpdateFromResponse(nil) // should not panic
}

func TestRateLimiter_UpdateFromResponse_MissingHeaders(t *testing.T) {
	rl := NewRateLimiter()

	resp := &http.Response{Header: http.Header{}}
	rl.UpdateFromResponse(resp)

	if rl.Remaining() != 0 {
		t.Errorf("expected remaining 0, got %d", rl.Remaining())
	}
}

func TestRateLimiter_UpdateFromResponse_InvalidValues(t *testing.T) {
	rl := NewRateLimiter()

	resp := &http.Response{
		Header: http.Header{
			"X-RateLimit-Remaining": []string{"not-a-number"},
			"X-RateLimit-Reset":     []string{"not-a-number"},
		},
	}
	rl.UpdateFromResponse(resp) // should not panic

	if rl.Remaining() != 0 {
		t.Errorf("expected remaining 0, got %d", rl.Remaining())
	}
}

func TestRateLimiter_AdaptiveThrottle(t *testing.T) {
	rl := NewRateLimiter(WithThrottleThreshold(5))

	// Simulate low remaining quota with reset far in the future
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "3")
	h.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(10*time.Second).Unix(), 10))
	resp := &http.Response{Header: h}
	rl.UpdateFromResponse(resp)

	start := time.Now()
	rl.Wait()
	elapsed := time.Since(start)

	// Should have waited some time to spread requests
	if elapsed < 500*time.Millisecond {
		t.Errorf("expected adaptive throttle delay, got %v", elapsed)
	}
}

func TestRateLimiter_NoLimit(t *testing.T) {
	rl := NewRateLimiter() // no RPS, no min delay

	start := time.Now()
	rl.Wait()
	rl.Wait()
	elapsed := time.Since(start)

	// Should return immediately. Upper bound is generous (100ms): the bug class
	// here is "Wait() wrongly sleeps through a ticker/timer" — second-scale and
	// caught by any bound ≥ the enforced delay. 5ms was flaky under parallel
	// suite load (scheduler jitter pushed it past 6ms in make check, 3期 T5).
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected near-instant return, got %v", elapsed)
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter(WithRPS(1000))

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			rl.Wait()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// Should not panic or deadlock
}

// setQuota is a helper that feeds X-RateLimit-* state into the limiter. The
// reset timestamp is rounded UP to the next whole second: the header carries
// unix seconds, and a naive truncation could otherwise shorten the effective
// window by up to a second, making delay assertions flaky.
func setQuota(t *testing.T, rl *RateLimiter, remaining int, resetIn time.Duration) {
	t.Helper()
	ts := time.Now().Add(resetIn)
	unix := ts.Unix()
	if ts.Nanosecond() > 0 {
		unix++
	}
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(unix, 10))
	rl.UpdateFromResponse(&http.Response{Header: h})
}

// TestRateLimiter_ZeroRemainingThrottles locks the condition fix: with
// remaining == 0 the quota is exhausted, which must throttle (wait towards the
// reset), not skip throttling entirely as the previous `remaining > 0` guard
// did.
func TestRateLimiter_ZeroRemainingThrottles(t *testing.T) {
	rl := NewRateLimiter(WithThrottleThreshold(5))
	setQuota(t, rl, 0, 2*time.Second)

	start := time.Now()
	rl.Wait()
	elapsed := time.Since(start)

	if elapsed < 1500*time.Millisecond {
		t.Errorf("expected exhausted quota (remaining=0) to throttle ~2s until reset, got %v", elapsed)
	}
}

// TestRateLimiter_AdaptiveWaitDoesNotHoldLock locks the lock-scope fix: the
// adaptive wait (which can span the whole quota-reset window) must sleep
// OUTSIDE the mutex, so state updates and other waiters proceed meanwhile.
// With the previous lock-held sleep both operations below blocked for the
// full adaptive delay.
func TestRateLimiter_AdaptiveWaitDoesNotHoldLock(t *testing.T) {
	rl := NewRateLimiter()
	setQuota(t, rl, 0, 2*time.Second) // adaptive delay ~= full reset window

	waiterDone := make(chan struct{})
	go func() {
		_ = rl.WaitContext(context.Background())
		close(waiterDone)
	}()
	time.Sleep(100 * time.Millisecond) // let the waiter enter its sleep

	start := time.Now()
	rl.UpdateFromResponse(&http.Response{Header: http.Header{}})
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("UpdateFromResponse blocked behind a sleeping waiter for %v", elapsed)
	}

	// A second waiter must compute its own (long) delay and sleep outside the
	// lock, so its cancellation is honored promptly.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start = time.Now()
	if err := rl.WaitContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("second waiter queued behind the first sleeper for %v", elapsed)
	}

	select {
	case <-waiterDone:
	case <-time.After(4 * time.Second):
		t.Error("first adaptive waiter never finished")
	}
}

// TestRateLimiter_AdaptiveReservationStaggersWaiters locks the thundering-herd
// fix: each waiter consumes a local reservation (decrementing remaining) so
// concurrent waiters target progressively later slots instead of all sleeping
// for the same duration and firing simultaneously.
func TestRateLimiter_AdaptiveReservationStaggersWaiters(t *testing.T) {
	rl := NewRateLimiter(WithThrottleThreshold(5))
	setQuota(t, rl, 3, 40*time.Second)

	// The wait would take 40s/4 = 10s; cancel early, but the slot must already
	// have been reserved under the lock.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_ = rl.WaitContext(ctx)

	if got := rl.Remaining(); got != 2 {
		t.Errorf("expected the local reservation to decrement remaining 3 -> 2, got %d", got)
	}

	// A second waiter now sees remaining=2 and computes a strictly longer
	// delay (40s/3) than the first (40s/4): delays are staggered, not equal.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	_ = rl.WaitContext(ctx2)
	if got := rl.Remaining(); got != 1 {
		t.Errorf("expected the second reservation to decrement remaining 2 -> 1, got %d", got)
	}
}

// TestRateLimiter_UpdateFromResponseRestoresReservation verifies that real
// response headers overwrite the local reservations, keeping the adaptive
// state honest across request/response cycles.
func TestRateLimiter_UpdateFromResponseRestoresReservation(t *testing.T) {
	rl := NewRateLimiter()
	setQuota(t, rl, 3, 40*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_ = rl.WaitContext(ctx)
	if got := rl.Remaining(); got != 2 {
		t.Fatalf("expected reservation to decrement 3 -> 2, got %d", got)
	}

	setQuota(t, rl, 42, time.Minute)
	if got := rl.Remaining(); got != 42 {
		t.Errorf("expected headers to restore remaining=42, got %d", got)
	}
}

// TestRateLimiter_ConcurrentAdaptive stresses concurrent waiters (adaptive
// state present) against UpdateFromResponse; primarily a -race detector seed.
func TestRateLimiter_ConcurrentAdaptive(t *testing.T) {
	rl := NewRateLimiter()
	setQuota(t, rl, 2, 60*time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_ = rl.WaitContext(ctx)
			rl.UpdateFromResponse(&http.Response{Header: http.Header{}})
		}()
	}
	wg.Wait()
}
