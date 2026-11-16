package transport

// 3期 T1：零覆盖清单补 transport/retry.go 的 RetryConfig.Do（缓冲体适配器
// over retryLoop）。钉 nil/零配置直通、重试至成功、5xx 耗尽后响应体可重放、
// ensureReplayable 的 GetBody 安装与读体失败上抛。

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// doErrBody 是读取即失败的请求体（etag_test 的 errReader 无 Close，
// 不能直接当 req.Body 用）。
type doErrBody struct{}

func (doErrBody) Read([]byte) (int, error) { return 0, errBoom }
func (doErrBody) Close() error             { return nil }

func TestRetryConfigDo(t *testing.T) {
	ctx := context.Background()

	t.Run("nil config: single direct attempt buffers body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("pong"))
		}))
		defer srv.Close()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		var rc *RetryConfig
		resp, body, err := rc.Do(ctx, srv.Client(), req, NoopLogger(), 1<<20)
		if err != nil {
			t.Fatalf("Do() err = %v, want nil", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
		}
		if string(body) != "pong" {
			t.Errorf("buffered body = %q, want %q", body, "pong")
		}
	})

	t.Run("nil config: transport error propagates", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close() // closed server: dial fails
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		var rc *RetryConfig
		if _, _, err := rc.Do(ctx, http.DefaultClient, req, NoopLogger(), 1<<20); err == nil {
			t.Fatal("Do() err = nil, want dial error")
		}
	})

	t.Run("zero MaxAttempts disables retry", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		}))
		defer srv.Close()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		rc := &RetryConfig{} // MaxAttempts: 0 -> direct attempt
		resp, body, err := rc.Do(ctx, srv.Client(), req, NoopLogger(), 1<<20)
		if err != nil {
			t.Fatalf("Do() err = %v, want nil", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("StatusCode = %d, want 500 (no retry)", resp.StatusCode)
		}
		if string(body) != "boom" {
			t.Errorf("buffered body = %q, want %q", body, "boom")
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("attempts = %d, want 1 (retry disabled)", got)
		}
	})

	t.Run("idempotent GET retried to success", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&calls, 1) < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("flaky"))
				return
			}
			_, _ = w.Write([]byte("recovered"))
		}))
		defer srv.Close()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		rc := &RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
		resp, body, err := rc.Do(ctx, srv.Client(), req, NoopLogger(), 1<<20)
		if err != nil {
			t.Fatalf("Do() err = %v, want nil", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
		}
		if string(body) != "recovered" {
			t.Errorf("buffered body = %q, want %q", body, "recovered")
		}
		if got := atomic.LoadInt32(&calls); got != 3 {
			t.Errorf("attempts = %d, want 3", got)
		}
	})

	t.Run("5xx exhaustion returns final response with replayable body", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server exploded"))
		}))
		defer srv.Close()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		rc := &RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
		resp, body, err := rc.Do(ctx, srv.Client(), req, NoopLogger(), 1<<20)
		if err != nil {
			t.Fatalf("Do() err = %v, want nil (response, not transport error)", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("StatusCode = %d, want 500 (retries exhausted)", resp.StatusCode)
		}
		if string(body) != "server exploded" {
			t.Errorf("buffered body = %q, want %q", body, "server exploded")
		}
		// 契约：返回的 Response 带全新 body reader，可再读一次。
		fresh, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading fresh resp.Body: %v", err)
		}
		if string(fresh) != "server exploded" {
			t.Errorf("fresh resp.Body = %q, want %q", fresh, "server exploded")
		}
		if got := atomic.LoadInt32(&calls); got != 2 {
			t.Errorf("attempts = %d, want 2", got)
		}
	})

	t.Run("POST body flows through on success", func(t *testing.T) {
		const payload = `{"k":"v"}`
		var gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("server reading body: %v", err)
				return
			}
			gotBody = string(raw)
			_, _ = w.Write([]byte("created"))
		}))
		defer srv.Close()
		req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		rc := &RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
		resp, body, err := rc.Do(ctx, srv.Client(), req, NoopLogger(), 1<<20)
		if err != nil {
			t.Fatalf("Do() err = %v, want nil", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
		}
		if gotBody != payload {
			t.Errorf("server saw body = %q, want %q", gotBody, payload)
		}
		if string(body) != "created" {
			t.Errorf("buffered body = %q, want %q", body, "created")
		}
	})

	t.Run("ensureReplayable read failure surfaces error", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, "http://example.invalid/hook", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Body = doErrBody{}
		req.GetBody = nil // force the buffering path in ensureReplayable
		rc := &RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
		if _, _, err := rc.Do(ctx, http.DefaultClient, req, NoopLogger(), 1<<20); !errors.Is(err, errBoom) {
			t.Errorf("Do() err = %v, want errors.Is(errBoom)", err)
		}
	})

	t.Run("body exceeding maxBodySize is rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("too long for budget"))
		}))
		defer srv.Close()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		var rc *RetryConfig
		resp, body, err := rc.Do(ctx, srv.Client(), req, NoopLogger(), 4)
		if err == nil {
			t.Fatal("Do() err = nil, want body-too-large error")
		}
		if body != nil {
			t.Errorf("body = %q, want nil on oversize", body)
		}
		if resp == nil {
			t.Error("resp = nil, want the received response alongside the error")
		}
	})
}
