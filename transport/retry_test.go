package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetry_ShouldRetry(t *testing.T) {
	rc := DefaultRetryConfig()
	cases := []struct {
		status int
		want   bool
	}{
		{200, false},
		{429, true},
		{500, true},
		{502, true},
		{599, true},
		{400, false},
		{404, false},
		{418, false},
	}
	for _, c := range cases {
		if got := rc.ShouldRetry(c.status); got != c.want {
			t.Errorf("status %d: got %v, want %v", c.status, got, c.want)
		}
	}
}

func TestRetry_CustomStatus(t *testing.T) {
	rc := RetryConfig{MaxAttempts: 1, Statuses: []int{418}}
	if !rc.ShouldRetry(418) {
		t.Error("expected custom 418 to be retried")
	}
	// 5xx is always retried by default
	if !rc.ShouldRetry(500) {
		t.Error("500 should still be retried (default 5xx behavior)")
	}
	if rc.ShouldRetry(404) {
		t.Error("404 should not be retried")
	}
}

func TestRetry_SucceedsOnFirstTry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{}, WithRetry(&RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}))
	resp, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("expected ok, got %q", resp.Body)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected 1 call, got %d", got)
	}
}

func TestRetry_RetriesOn500(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{}, WithRetry(&RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}))
	resp, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retries, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("expected ok, got %q", resp.Body)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 calls, got %d", got)
	}
}

// TestRetry_ReplaysBody guards body replay across attempts. It uses PUT (an
// idempotent method) so the test keeps verifying the replay mechanism only:
// since the method-aware retry fix, a POST would no longer be retried on a
// retryable status (see TestRetry_PostNotRetriedOnRetryableStatus).
func TestRetry_ReplaysBody(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{}, WithRetry(&RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}))
	_, _ = c.Do(context.Background(), &Request{Method: http.MethodPut, Path: "/x", Body: []byte(`{"k":"v"}`)})
	if len(bodies) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(bodies))
	}
	for i, b := range bodies {
		if b != `{"k":"v"}` {
			t.Errorf("attempt %d: expected body to be replayed, got %q", i, b)
		}
	}
}

// TestRetry_HonorsRetryAfterCappedByMaxDelay verifies the Retry-After cap: a
// server-specified delay takes precedence over the backoff calculation, but is
// clamped to MaxDelay so e.g. "Retry-After: 86400" cannot suspend the caller
// for a day. (Before the fix the parsed value was returned unbounded.)
func TestRetry_HonorsRetryAfterCappedByMaxDelay(t *testing.T) {
	rc := RetryConfig{MaxAttempts: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 10 * time.Millisecond}
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "1")
	delay := rc.Backoff(1, resp)
	if delay != 10*time.Millisecond {
		t.Errorf("expected Retry-After capped to MaxDelay (10ms), got %v", delay)
	}
}

func TestRetry_ContextCancelAbortsBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{}, WithRetry(&RetryConfig{MaxAttempts: 5, BaseDelay: 200 * time.Millisecond, MaxDelay: time.Second}))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Do(ctx, &Request{Method: http.MethodGet, Path: "/x"}); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRetry_ExhaustsAttempts(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{}, WithRetry(&RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}))
	_, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"})
	if err == nil {
		t.Fatal("expected the final 502 as transport error")
	}
	var terr *Error
	if !errors.As(err, &terr) || terr.StatusCode() != http.StatusBadGateway {
		t.Fatalf("expected transport error with status 502, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 calls, got %d", got)
	}
}

// --- Fix: Retry-After must be capped and must support HTTP-date form ---

func TestRetry_RetryAfterCappedAtMaxDelay(t *testing.T) {
	rc := DefaultRetryConfig() // MaxDelay defaults to 30s
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "86400") // one full day
	delay := rc.Backoff(1, resp)
	if delay > 30*time.Second {
		t.Errorf("expected Retry-After capped to 30s, got %v", delay)
	}
	if delay != 30*time.Second {
		t.Errorf("expected exactly the 30s cap, got %v", delay)
	}
}

func TestRetry_RetryAfterHTTPDate(t *testing.T) {
	rc := RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Second}
	resp := &http.Response{Header: http.Header{}}
	// Far-future HTTP-date must be parsed and capped.
	resp.Header.Set("Retry-After", time.Now().Add(90*time.Second).UTC().Format(http.TimeFormat))
	if delay := rc.Backoff(1, resp); delay != 2*time.Second {
		t.Errorf("expected HTTP-date Retry-After capped to 2s, got %v", delay)
	}

	// A near-future HTTP-date is honored (not capped): formatted dates have
	// second granularity, so a date built from now+2s truncated to the second
	// is 1s..2s away.
	rc2 := RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 30 * time.Second}
	resp2 := &http.Response{Header: http.Header{}}
	resp2.Header.Set("Retry-After", time.Now().Add(2*time.Second).Truncate(time.Second).UTC().Format(http.TimeFormat))
	delay := rc2.Backoff(1, resp2)
	if delay < 900*time.Millisecond || delay > 2*time.Second {
		t.Errorf("expected ~1-2s delay from HTTP-date, got %v", delay)
	}

	// A past HTTP-date means "no wait".
	resp.Header.Set("Retry-After", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat))
	if delay := rc.Backoff(1, resp); delay != 0 {
		t.Errorf("expected zero delay for past HTTP-date, got %v", delay)
	}

	// An unparseable value falls back to the exponential backoff (bounded).
	resp.Header.Set("Retry-After", "soon")
	if delay := rc.Backoff(1, resp); delay > rc.MaxDelay {
		t.Errorf("expected bounded exponential fallback, got %v", delay)
	}
}

// --- Fix: retries must be method-aware ---

func TestRetry_IdempotentMethodSet(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		if !isIdempotentMethod(m) {
			t.Errorf("expected %s to be idempotent", m)
		}
		if !isIdempotentMethod(strings.ToLower(m)) {
			t.Errorf("expected method match to be case-insensitive for %s", m)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPatch, http.MethodConnect, http.MethodTrace, ""} {
		if isIdempotentMethod(m) {
			t.Errorf("expected %q NOT to be idempotent", m)
		}
	}
}

func TestRetry_RequestNotSentClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"dial error", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"wrapped dial error", fmt.Errorf("Get %q: %w", "http://x", &net.OpError{Op: "dial", Err: errors.New("refused")}), true},
		{"dns error", &net.DNSError{Err: "no such host", Name: "api.example.com"}, true},
		// A read/write error means bytes already went out: ambiguous for writes.
		{"read error after send", &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}, false},
	}
	for _, tc := range cases {
		if got := requestNotSent(tc.err); got != tc.want {
			t.Errorf("%s: requestNotSent = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRetry_PostNotRetriedOnRetryableStatus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{}, WithRetry(&RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}))
	_, err := c.Do(context.Background(), &Request{Method: http.MethodPost, Path: "/items", Body: []byte(`{"k":"v"}`)})
	if err == nil {
		t.Fatal("expected the 502 to surface as a transport error")
	}
	var terr *Error
	if !errors.As(err, &terr) || terr.StatusCode() != http.StatusBadGateway {
		t.Fatalf("expected transport error with status 502, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("POST executed-but-502 must not be replayed: expected 1 attempt, got %d", got)
	}
}

func TestRetry_GetRetriedOnRetryableStatus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{}, WithRetry(&RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}))
	if _, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"}); err == nil {
		t.Fatal("expected the final 502 as transport error")
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("GET is idempotent: expected 3 attempts, got %d", got)
	}
}

func TestRetry_RetryWriteOptsStatusRetriesForPost(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{}, WithRetry(&RetryConfig{
		MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond,
		RetryWrite: true, // explicit opt-in to write retries
	}))
	if _, err := c.Do(context.Background(), &Request{Method: http.MethodPost, Path: "/items", Body: []byte(`{"k":"v"}`)}); err == nil {
		t.Fatal("expected the final 502 as transport error")
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("RetryWrite=true: expected POST to be retried 3 times, got %d attempts", got)
	}
}

func TestRetry_PostRetriedOnlyWhenRequestNotSent(t *testing.T) {
	dialErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	ambiguousErr := errors.New("read: connection reset by peer")

	run := func(method string, err error) int {
		var calls int32
		inner := roundTripFunc(func(*http.Request) (*http.Response, error) {
			atomic.AddInt32(&calls, 1)
			return nil, err
		})
		rt := &retryingRoundTripper{
			inner:  inner,
			cfg:    &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
			logger: NoopLogger(),
		}
		req := httptest.NewRequest(method, "http://example.com/x", nil)
		_, _ = rt.RoundTrip(req)
		return int(atomic.LoadInt32(&calls))
	}

	if got := run(http.MethodPost, dialErr); got != 3 {
		t.Errorf("POST with provably-unsent error should retry 3x, got %d attempts", got)
	}
	if got := run(http.MethodPost, ambiguousErr); got != 1 {
		t.Errorf("POST with ambiguous error must not retry, got %d attempts", got)
	}
	if got := run(http.MethodGet, ambiguousErr); got != 3 {
		t.Errorf("GET with ambiguous error should retry 3x, got %d attempts", got)
	}
	if got := run(http.MethodDelete, ambiguousErr); got != 3 {
		t.Errorf("DELETE is idempotent: expected 3 attempts, got %d", got)
	}
}

func TestRetryingRoundTripper_PostNotRetriedOn502(t *testing.T) {
	var calls int32
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader(`{"message":"bad gateway"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	rt := &retryingRoundTripper{
		inner:  inner,
		cfg:    &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		logger: NoopLogger(),
	}
	req := httptest.NewRequest(http.MethodPost, "http://example.com/items", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("a received response is not a transport error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("POST must not be replayed on 502: expected 1 attempt, got %d", got)
	}
	if resp == nil || resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected the 502 response, got %+v", resp)
	}
}

func TestRetryingRoundTripper_RetryWriteRetriesPost(t *testing.T) {
	var calls int32
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	rt := &retryingRoundTripper{
		inner:  inner,
		cfg:    &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, RetryWrite: true},
		logger: NoopLogger(),
	}
	req := httptest.NewRequest(http.MethodPost, "http://example.com/items", nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("RetryWrite=true: expected 3 attempts, got %d", got)
	}
}

func TestRetry_DefaultConfigDisablesWriteRetries(t *testing.T) {
	rc := DefaultRetryConfig()
	if rc.RetryWrite {
		t.Error("RetryWrite must default to false: replaying writes is unsafe by default")
	}
}

// --- GitHub-style rate-limit 403: retried only when the headers prove throttling ---

// TestRetry_RateLimit403RetriesUntilSuccess covers the primary GitHub rate
// limit: 403 + X-RateLimit-Remaining: 0 must be retried like a 429 and
// eventually succeed.
func TestRetry_RateLimit403RetriesUntilSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Limit", "5000")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{})
	c.retry = &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	resp, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retries, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("expected ok, got %q", resp.Body)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 calls (2 throttled 403 + success), got %d", got)
	}
}

// TestRetry_Bare403NotRetried guards the permission-denial case: a 403 with
// neither X-RateLimit-Remaining nor Retry-After is an authorization failure
// and must never be replayed.
func TestRetry_Bare403NotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{})
	c.retry = &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	_, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"})
	if err == nil {
		t.Fatal("expected the 403 as transport error")
	}
	var terr *Error
	if !errors.As(err, &terr) || terr.StatusCode() != http.StatusForbidden {
		t.Fatalf("expected transport error with status 403, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("bare 403 must not be retried: expected 1 attempt, got %d", got)
	}
}

// TestRetry_RateLimit403WithRemainingNonZeroNotRetried: a 403 that carries
// the rate-limit header but with a non-zero remaining count is not the
// primary rate limit, so it stays unretried.
func TestRetry_RateLimit403WithRemainingNonZeroNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{})
	c.retry = &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	if _, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"}); err == nil {
		t.Fatal("expected the 403 as transport error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("403 with non-zero remaining is not throttling: expected 1 attempt, got %d", got)
	}
}

// TestRetry_RateLimit403RetryAfterZeroRetriesImmediately: Retry-After: 0
// takes precedence over the exponential backoff, so the retry fires without
// a measurable wait.
func TestRetry_RateLimit403RetryAfterZeroRetriesImmediately(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{})
	c.retry = &RetryConfig{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}
	start := time.Now()
	resp, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 calls, got %d", got)
	}
	// Retry-After: 0 must win over the 100ms exponential base delay.
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("Retry-After: 0 not honored: elapsed %v", elapsed)
	}
}

// TestRetry_RateLimit403RetryAfterHonored verifies the Retry-After header is
// preferred over the exponential backoff. Retry-After: 1 exceeds MaxDelay, so
// the wait is clamped to the cap — still far longer than the ~1ms the
// exponential path would produce, proving the header was used.
func TestRetry_RateLimit403RetryAfterHonored(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{})
	c.retry = &RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 50 * time.Millisecond}
	start := time.Now()
	resp, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("Retry-After not honored: elapsed %v, want >= ~50ms cap", elapsed)
	}
}

// TestRetry_RateLimit403PostNotRetried enforces the idempotency gate on the
// new 403 path: even with rate-limit headers, a POST that already reached the
// server must not be replayed (same rule as 429/5xx).
func TestRetry_RateLimit403PostNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, None{})
	c.retry = &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	_, err := c.Do(context.Background(), &Request{Method: http.MethodPost, Path: "/items", Body: []byte(`{"k":"v"}`)})
	if err == nil {
		t.Fatal("expected the 403 as transport error")
	}
	var terr *Error
	if !errors.As(err, &terr) || terr.StatusCode() != http.StatusForbidden {
		t.Fatalf("expected transport error with status 403, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("POST rate-limited 403 must not be replayed: expected 1 attempt, got %d", got)
	}
}

// TestRetryingRoundTripper_RateLimit403 covers the second execution path:
// the RoundTripper wrapper used by third-party SDKs must apply the same
// header-aware 403 logic.
func TestRetryingRoundTripper_RateLimit403(t *testing.T) {
	var calls int32
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		h := make(http.Header)
		status := http.StatusForbidden
		if n == 2 {
			status = http.StatusOK
		} else {
			h.Set("X-RateLimit-Remaining", "0")
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader("body")),
			Header:     h,
			Request:    req,
		}, nil
	})
	rt := &retryingRoundTripper{
		inner:  inner,
		cfg:    &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond},
		logger: NoopLogger(),
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 attempts (throttled 403 then success), got %d", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestRetryingRoundTripper_Bare403NotRetried: the RoundTripper path must
// return a plain 403 on the first attempt.
func TestRetryingRoundTripper_Bare403NotRetried(t *testing.T) {
	var calls int32
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("denied")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	rt := &retryingRoundTripper{
		inner:  inner,
		cfg:    &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond},
		logger: NoopLogger(),
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("bare 403 must not be retried: expected 1 attempt, got %d", got)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// TestRetry_IsRateLimited403 unit-cases the header predicate. Headers are
// built via Set so the keys get MIME-canonicalized, matching what net/http
// produces when parsing a real response.
func TestRetry_IsRateLimited403(t *testing.T) {
	hdr := func(kv ...string) http.Header {
		h := http.Header{}
		for i := 0; i+1 < len(kv); i += 2 {
			h.Set(kv[i], kv[i+1])
		}
		return h
	}
	cases := []struct {
		name   string
		status int
		header http.Header
		want   bool
	}{
		{"403 remaining 0", 403, hdr("X-RateLimit-Remaining", "0"), true},
		{"403 remaining 0 padded", 403, hdr("X-RateLimit-Remaining", " 0 "), true},
		{"403 retry-after", 403, hdr("Retry-After", "1"), true},
		{"403 bare", 403, http.Header{}, false},
		{"403 remaining non-zero", 403, hdr("X-RateLimit-Remaining", "7"), false},
		{"429", 429, http.Header{}, true},
		{"404", 404, hdr("X-RateLimit-Remaining", "0"), false},
		{"nil header", 403, nil, false},
	}
	for _, c := range cases {
		if got := isRateLimitedStatus(c.status, c.header); got != c.want {
			t.Errorf("%s: isRateLimitedStatus = %v, want %v", c.name, got, c.want)
		}
	}
}
