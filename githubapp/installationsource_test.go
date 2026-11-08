package githubapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServer serves installation-token responses with a controllable
// expires_at and counts upstream fetches.
type tokenServer struct {
	srv       *httptest.Server
	fetches   atomic.Int64
	expiresIn time.Duration // zero → omit expires_at
	failFirst atomic.Bool
}

func newTokenServer(t *testing.T, expiresIn time.Duration) *tokenServer {
	t.Helper()
	ts := &tokenServer{expiresIn: expiresIn}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := ts.fetches.Add(1)
		if ts.failFirst.Load() && n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if ts.expiresIn == 0 {
			fmt.Fprintf(w, `{"token":"ghs_%d"}`, n)
			return
		}
		fmt.Fprintf(w, `{"token":"ghs_%d","expires_at":"%s"}`, n, time.Now().Add(ts.expiresIn).UTC().Format(time.RFC3339))
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func TestInstallationTokenSource_CachesWithinValidity(t *testing.T) {
	ts := newTokenServer(t, time.Hour)
	src := NewInstallationTokenSource(ts.srv.URL, 42, 7, testKeyPEM(t, false))

	for i := range 3 {
		tok, err := src.Token(t.Context())
		if err != nil {
			t.Fatalf("Token #%d: %v", i+1, err)
		}
		if tok != "ghs_1" {
			t.Fatalf("Token #%d = %q, want the cached ghs_1", i+1, tok)
		}
	}
	if got := ts.fetches.Load(); got != 1 {
		t.Fatalf("upstream fetches = %d, want exactly 1", got)
	}
	if src.ExpiresAt().Before(time.Now()) {
		t.Fatalf("ExpiresAt = %v, must be in the future", src.ExpiresAt())
	}
}

func TestInstallationTokenSource_RefreshesAtExpiry(t *testing.T) {
	// Token already expired at issue time: every call must refetch.
	ts := newTokenServer(t, -time.Minute)
	src := NewInstallationTokenSource(ts.srv.URL, 42, 7, testKeyPEM(t, false))
	src.RefreshMargin = time.Nanosecond

	if _, err := src.Token(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := ts.fetches.Load(); got != 2 {
		t.Fatalf("upstream fetches = %d, want 2 (expired token must not be served)", got)
	}
}

func TestInstallationTokenSource_RefreshMarginKicksInEarly(t *testing.T) {
	// Token valid for another 30s, margin 1 minute: immediate refresh.
	ts := newTokenServer(t, 30*time.Second)
	src := NewInstallationTokenSource(ts.srv.URL, 42, 7, testKeyPEM(t, false))

	if _, err := src.Token(t.Context()); err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "ghs_2" {
		t.Fatalf("token = %q, want refreshed ghs_2 (margin must trigger)", tok)
	}
}

func TestInstallationTokenSource_MissingExpiryFallsBackToTTL(t *testing.T) {
	ts := newTokenServer(t, 0) // no expires_at in response
	src := NewInstallationTokenSource(ts.srv.URL, 42, 7, testKeyPEM(t, false))

	for range 2 {
		if _, err := src.Token(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if got := ts.fetches.Load(); got != 1 {
		t.Fatalf("upstream fetches = %d, want 1 (fallback TTL must cache)", got)
	}
	if got := time.Until(src.ExpiresAt()); got < 55*time.Minute {
		t.Fatalf("fallback expiry in %v, want ~1h", got)
	}
}

func TestInstallationTokenSource_ConcurrentCallsFetchOnce(t *testing.T) {
	ts := newTokenServer(t, time.Hour)
	src := NewInstallationTokenSource(ts.srv.URL, 42, 7, testKeyPEM(t, false))

	var wg sync.WaitGroup
	tokens := make([]string, 16)
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := src.Token(t.Context())
			if err != nil {
				t.Errorf("Token: %v", err)
				return
			}
			tokens[i] = tok
		}()
	}
	wg.Wait()
	if got := ts.fetches.Load(); got != 1 {
		t.Fatalf("upstream fetches = %d, want 1 (stampede must collapse)", got)
	}
	for i, tok := range tokens {
		if tok != tokens[0] {
			t.Fatalf("caller %d saw %q, caller 0 saw %q", i, tok, tokens[0])
		}
	}
}

func TestInstallationTokenSource_FailureKeepsCacheRetriable(t *testing.T) {
	ts := newTokenServer(t, time.Hour)
	ts.failFirst.Store(true)
	src := NewInstallationTokenSource(ts.srv.URL, 42, 7, testKeyPEM(t, false))

	if _, err := src.Token(t.Context()); err == nil {
		t.Fatal("first fetch must fail (503)")
	}
	// The failed fetch must not poison the cache: the next call retries
	// and succeeds.
	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
	if tok != "ghs_2" {
		t.Fatalf("token = %q, want ghs_2 from the retry", tok)
	}
}

// TestInstallationTokenSource_StaleIfError 复刻 issue #46：margin 窗口内
// 刷新失败（502）不得丢弃仍有效的缓存 token——那会让窗口内每次 API 调用
// 都报错，尽管手里的凭据 GitHub 仍会接受。真实过期后无 stale 可用才报错。
func TestInstallationTokenSource_StaleIfError(t *testing.T) {
	ts := newTokenServer(t, 3*time.Second)
	src := NewInstallationTokenSource(ts.srv.URL, 42, 7, testKeyPEM(t, false))
	src.RefreshMargin = time.Minute // 3s 有效期 < 1m margin ⇒ 第二次调用必进刷新分支

	if _, err := src.Token(t.Context()); err != nil {
		t.Fatal(err)
	}

	// 上游此后恒 502（连旧连接一起断掉，确保刷新路径真走到网络）
	handler := ts.srv.Config.Handler
	ts.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = handler
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	ts.srv.CloseClientConnections()

	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("margin 内刷新失败必须回落缓存 token: %v", err)
	}
	if tok == "" {
		t.Fatal("回落 token 为空")
	}

	// 真实过期之后：无 stale 可用，报错
	src.mu.Lock()
	src.expiresAt = time.Now().Add(-time.Second)
	src.mu.Unlock()
	if _, err := src.Token(t.Context()); err == nil {
		t.Fatal("缓存过期后刷新失败必须报错")
	}
}
