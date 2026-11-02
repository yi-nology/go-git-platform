package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newETagServer returns a server answering GET /data with an ETag and
// honoring If-None-Match, plus counters for total and conditional hits.
func newETagServer(body string, extraHeaders map[string]string) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	var total, conditional atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		total.Add(1)
		for k, v := range extraHeaders {
			w.Header().Set(k, v)
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	return srv, &total, &conditional
}

func TestETagDoPathServes304FromCache(t *testing.T) {
	srv, total, conditional := newETagServer(`{"v":1}`, nil)
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	c.etag = NewETagCache(0)

	resp, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/data"})
	if err != nil {
		t.Fatalf("first Do: %v", err)
	}
	if got := string(resp.Body); got != `{"v":1}` {
		t.Fatalf("first body = %q", got)
	}
	if resp.Header.Get(cacheHitHeader) == "hit" {
		t.Fatal("first response must not be a cache hit")
	}

	resp, err = c.Do(t.Context(), &Request{Method: "GET", Path: "/data"})
	if err != nil {
		t.Fatalf("second Do: %v", err)
	}
	if got := string(resp.Body); got != `{"v":1}` {
		t.Fatalf("second body = %q, want cached body", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second status = %d, want 200 (synthesized)", resp.StatusCode)
	}
	if resp.Header.Get(cacheHitHeader) != "hit" {
		t.Fatal("second response missing cache-hit marker")
	}
	if got := total.Load(); got != 2 {
		t.Fatalf("server saw %d requests, want 2", got)
	}
	if got := conditional.Load(); got != 1 {
		t.Fatalf("server saw %d conditional requests, want 1", got)
	}
}

func TestETagRoundTripperPathServes304FromCache(t *testing.T) {
	srv, total, conditional := newETagServer(`{"v":2}`, nil)
	defer srv.Close()

	c := NewClient(srv.URL, None{}, WithTransport(srv.Client().Transport))
	c.etag = NewETagCache(0)
	hc := &http.Client{Transport: c.RoundTripper()}

	get := func() (string, http.Header) {
		resp, err := hc.Get(srv.URL + "/data")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b), resp.Header
	}

	body, h := get()
	if body != `{"v":2}` {
		t.Fatalf("first body = %q", body)
	}
	if h.Get(cacheHitHeader) == "hit" {
		t.Fatal("first response must not be a cache hit")
	}
	body, h = get()
	if body != `{"v":2}` {
		t.Fatalf("second body = %q, want cached body", body)
	}
	if h.Get(cacheHitHeader) != "hit" {
		t.Fatal("second response missing cache-hit marker")
	}
	if got := conditional.Load(); got != 1 {
		t.Fatalf("server saw %d conditional requests, want 1", got)
	}
	if got := total.Load(); got != 2 {
		t.Fatalf("server saw %d requests, want 2", got)
	}
}

func TestETagSkipsNoStore(t *testing.T) {
	srv, _, conditional := newETagServer(`{"v":3}`, map[string]string{"Cache-Control": "no-store"})
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	c.etag = NewETagCache(0)
	for range 3 {
		if _, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/data"}); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := conditional.Load(); got != 0 {
		t.Fatalf("no-store response was cached (%d conditional requests)", got)
	}
	if c.etag.Len() != 0 {
		t.Fatalf("cache holds %d entries, want 0", c.etag.Len())
	}
}

func TestETagSkipsNonGET(t *testing.T) {
	srv, total, conditional := newETagServer(`{"v":4}`, nil)
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	c.etag = NewETagCache(0)
	for range 2 {
		if _, err := c.Do(t.Context(), &Request{Method: "POST", Path: "/data", Body: map[string]any{}}); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := conditional.Load(); got != 0 {
		t.Fatalf("POST was conditionally requested (%d times)", got)
	}
	if got := total.Load(); got != 2 {
		t.Fatalf("server saw %d requests, want 2", got)
	}
}

func TestETagSkipsOversizedBodies(t *testing.T) {
	big := `{"pad":"` + strings.Repeat("x", 2*DefaultMaxCachedBodySize) + `"}`
	srv, _, conditional := newETagServer(big, nil)
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	c.etag = NewETagCache(0)
	for range 2 {
		resp, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/data"})
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		if int64(len(resp.Body)) <= DefaultMaxCachedBodySize {
			t.Fatal("test body unexpectedly small")
		}
	}
	if got := conditional.Load(); got != 0 {
		t.Fatalf("oversized body was cached (%d conditional requests)", got)
	}
}

func TestETagTokenRotationPartitionsCache(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = io.WriteString(w, `{"v":1}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	c.etag = NewETagCache(0)
	c.auth = TokenSourceAuth{Source: StaticTokenSource("t1")}

	if _, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/data"}); err != nil {
		t.Fatalf("Do t1: %v", err)
	}
	// Same URL, different token: the cached entry for t1 must not be reused.
	c.auth = TokenSourceAuth{Source: StaticTokenSource("t2")}
	resp, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/data"})
	if err != nil {
		t.Fatalf("Do t2: %v", err)
	}
	if resp.Header.Get(cacheHitHeader) == "hit" {
		t.Fatal("entry cached under t1 was served for t2")
	}
	if len(seen) != 2 || seen[0] != "Bearer t1" || seen[1] != "Bearer t2" {
		t.Fatalf("authorizations seen = %v", seen)
	}
}

func TestETagOversizedKnownLengthStreamsThrough(t *testing.T) {
	big := `{"pad":"` + strings.Repeat("x", 2*DefaultMaxCachedBodySize) + `"}`
	srv, _, conditional := newETagServer(big, nil)
	defer srv.Close()

	c := NewClient(srv.URL, None{}, WithTransport(srv.Client().Transport))
	c.etag = NewETagCache(0)
	hc := &http.Client{Transport: c.RoundTripper()}

	resp, err := hc.Get(srv.URL + "/data")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatalf("stream oversized body: %v", err)
	}
	if n != int64(len(big)) {
		t.Fatalf("streamed %d bytes, want %d (must not truncate or buffer-cap)", n, len(big))
	}
	if c.etag.Len() != 0 {
		t.Fatalf("cache holds %d entries, want 0", c.etag.Len())
	}
	if got := conditional.Load(); got != 0 {
		t.Fatalf("oversized body was cached (%d conditional requests)", got)
	}
}

func TestETagOversizedChunkedBodyStreamsThrough(t *testing.T) {
	// No Content-Length (chunked): the cap+1 probe must detect overflow and
	// replay prefix + remainder so the caller still receives every byte.
	big := `{"pad":"` + strings.Repeat("y", 2*DefaultMaxCachedBodySize) + `"}`
	srv, _, _ := newETagServer(big, nil)
	defer srv.Close()

	c := NewClient(srv.URL, None{}, WithTransport(srv.Client().Transport))
	c.etag = NewETagCache(0)
	hc := &http.Client{Transport: c.RoundTripper()}

	resp, err := hc.Get(srv.URL + "/data")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatalf("stream chunked oversized body: %v", err)
	}
	if n != int64(len(big)) {
		t.Fatalf("streamed %d bytes, want %d", n, len(big))
	}
	if c.etag.Len() != 0 {
		t.Fatalf("cache holds %d entries, want 0", c.etag.Len())
	}
}

func TestETagMidBodyReadErrorSurfaces(t *testing.T) {
	// A body that fails mid-read must surface as a transport error on the
	// RoundTripper path — never as a silently truncated 200.
	failing := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		h := http.Header{}
		h.Set("ETag", `"v1"`)
		h.Set("Content-Type", "application/json")
		body := io.NopCloser(io.MultiReader(
			strings.NewReader(`{"partial":`),
			&errReader{},
		))
		return &http.Response{StatusCode: 200, Header: h, Body: body, ContentLength: -1}, nil
	})
	c := NewClient("https://example.invalid", None{})
	c.transport = failing
	c.etag = NewETagCache(0)
	hc := &http.Client{Transport: c.RoundTripper()}

	resp, err := hc.Get("https://example.invalid/data")
	if err == nil {
		// The error may arrive on first body read instead of RoundTrip;
		// either way the caller must see it.
		_, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr == nil {
			t.Fatal("mid-body failure swallowed: neither RoundTrip nor read reported it")
		}
		return
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want the underlying read error", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errBoom }

var errBoom = errorString("boom")

type errorString string

func (e errorString) Error() string { return string(e) }

func TestETagNoStoreInDirectiveList(t *testing.T) {
	srv, _, conditional := newETagServer(`{"v":1}`, map[string]string{"Cache-Control": "private, no-store, max-age=0"})
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	c.etag = NewETagCache(0)
	for range 2 {
		if _, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/data"}); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := conditional.Load(); got != 0 {
		t.Fatalf("no-store inside a directive list was cached (%d conditional requests)", got)
	}
}

func TestETagExplicitAcceptEncodingNotCached(t *testing.T) {
	srv, _, conditional := newETagServer(`{"v":1}`, nil)
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	c.etag = NewETagCache(0)
	for range 2 {
		if _, err := c.Do(t.Context(), &Request{
			Method: "GET", Path: "/data",
			Headers: http.Header{"Accept-Encoding": []string{"gzip"}},
		}); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := conditional.Load(); got != 0 {
		t.Fatalf("explicitly-encoded response was cached (%d conditional requests)", got)
	}
	if c.etag.Len() != 0 {
		t.Fatalf("cache holds %d entries, want 0", c.etag.Len())
	}
}

func TestETag304WithoutEntryIsAnError(t *testing.T) {
	c := NewETagCache(1)
	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	resp := &http.Response{StatusCode: http.StatusNotModified, Body: http.NoBody}

	_, err := c.processRT(req, resp)
	if err == nil {
		t.Fatal("304 with no cached entry must be an error, not a bodyless 200-shape passthrough")
	}
	if !errors.Is(err, ErrETagEntryGone) {
		t.Fatalf("err = %v, want the ErrETagEntryGone sentinel (stdlib Is chain)", err)
	}
}

// TestETag304MissSelfHealsUnconditionally drives the issue #31 race
// end-to-end: the validator is evicted from the bounded LRU while its 304
// is in flight (the stub transport evicts exactly there, like concurrent
// walks would). RoundTrip must detect ErrETagEntryGone, strip the
// validator, re-send unconditionally once, and hand the caller a clean
// 200 — with the cache repopulated so later conditional requests work.
func TestETag304MissSelfHealsUnconditionally(t *testing.T) {
	var cache *ETagCache
	var sawConditional []bool
	calls := 0
	var raced atomic.Bool
	stub := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		conditional := r.Header.Get("If-None-Match") != ""
		sawConditional = append(sawConditional, conditional)
		if conditional {
			// The race window, once: evict the entry while the 304 is "in
			// transit" — what concurrent walks do to a bounded LRU. Later
			// conditional requests hit a populated cache and 304 cleanly.
			if raced.CompareAndSwap(false, true) {
				_ = cache.entries.Del(context.Background(), etagKey(r))
			}
			return &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{}, Body: http.NoBody}, nil
		}
		h := http.Header{}
		h.Set("ETag", `"v1"`)
		h.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        h,
			Body:          io.NopCloser(strings.NewReader(`{"v":1}`)),
			ContentLength: int64(len(`{"v":1}`)),
		}, nil
	})
	c := NewClient("https://example.invalid", None{}, WithTransport(stub))
	cache = NewETagCache(4)
	c.etag = cache
	hc := &http.Client{Transport: c.RoundTripper()}

	// Prime: unconditional 200 lands in the cache.
	resp, err := hc.Get("https://example.invalid/data")
	if err != nil {
		t.Fatalf("prime GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("prime status = %d", resp.StatusCode)
	}

	// Race round: conditional 304 with the entry evicted underneath — must
	// self-heal, never surface as an error.
	resp, err = hc.Get("https://example.invalid/data")
	if err != nil {
		t.Fatalf("race GET must self-heal, got error: %v", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		t.Fatalf("read healed body: %v", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healed status = %d, want a clean 200", resp.StatusCode)
	}
	if string(body) != `{"v":1}` {
		t.Fatalf("healed body = %q", body)
	}
	if resp.Header.Get(cacheHitHeader) == "hit" {
		t.Fatal("healed response must be the network 200, not a synthesized hit")
	}
	if calls != 3 {
		t.Fatalf("transport saw %d calls, want 3 (prime, evicted 304, unconditional resend)", calls)
	}
	if len(sawConditional) != 3 || sawConditional[0] || !sawConditional[1] || sawConditional[2] {
		t.Fatalf("conditional header per call = %v, want [false true false]", sawConditional)
	}
	if cache.Len() != 1 {
		t.Fatalf("cache holds %d entries after heal, want 1 (repopulated)", cache.Len())
	}

	// Steady state: the repopulated entry serves 304s from cache again.
	resp, err = hc.Get("https://example.invalid/data")
	if err != nil {
		t.Fatalf("steady GET: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.Header.Get(cacheHitHeader) != "hit" || string(body) != `{"v":1}` {
		t.Fatalf("steady state must serve the synthesized 200, got header=%q body=%q",
			resp.Header.Get(cacheHitHeader), body)
	}
	if calls != 4 {
		t.Fatalf("steady-state conditional hit still hit the wire (calls=%d)", calls)
	}
}
