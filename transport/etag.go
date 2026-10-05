package transport

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	lru "github.com/hashicorp/golang-lru/v2"
)

// DefaultETagEntries is the default maximum number of responses kept in an
// ETagCache.
const DefaultETagEntries = 128

// DefaultMaxCachedBodySize caps how large a response body may be to enter
// the ETag cache (1 MiB). Larger bodies (archives, tarballs, huge diffs)
// are fetched in full every time; only their pages'-worth of JSON metadata
// benefit from conditional requests.
const DefaultMaxCachedBodySize = 1024 * 1024

// cacheHitHeader marks responses served from the ETag cache so response
// hooks and callers can distinguish a network 200 from a cached one.
const cacheHitHeader = "X-Go-Git-Platform-Cache"

// ETagCache implements HTTP conditional requests (RFC 9110 §13) for the
// transport pipeline. When attached to a Client (Client.ETag), GET requests
// carry If-None-Match with the stored validator; a 304 answer is
// transparently turned back into the cached 200 response, so callers and
// third-party SDK decoders see an ordinary response while the platform
// skips serializing the body. On the platforms that meter conditional
// requests favorably (GitHub's 304s do not count against the rate-limit
// budget) this materially raises sustainable polling rates — e.g.
// wait-for-CI loops re-reading commit statuses every few seconds.
//
// Semantics and deliberate simplifications:
//
//   - Only GET requests are cached.
//   - The cache key includes the URL, the Accept header, and the
//     Authorization header, so token rotation naturally partitions the
//     cache and Accept variants never cross-contaminate.
//   - Responses carrying Cache-Control: no-store are never cached.
//   - Entries are bounded by count (LRU) and by body size.
//   - Vary is not honored beyond Accept; platforms served by this SDK do
//     not vary their API payloads on other request headers.
//
// An ETagCache is safe for concurrent use.
type ETagCache struct {
	entries     *lru.Cache[string, etagEntry]
	maxBodySize int64
}

type etagEntry struct {
	etag   string
	header http.Header
	body   []byte
}

// NewETagCache builds a cache holding at most maxEntries responses with
// bodies up to DefaultMaxCachedBodySize. maxEntries <= 0 uses
// DefaultETagEntries.
func NewETagCache(maxEntries int) *ETagCache {
	if maxEntries <= 0 {
		maxEntries = DefaultETagEntries
	}
	// lru.New returns an error only for non-positive sizes, handled above.
	c, _ := lru.New[string, etagEntry](maxEntries)
	return &ETagCache{entries: c, maxBodySize: DefaultMaxCachedBodySize}
}

// Len reports the number of cached responses (for tests and diagnostics).
func (c *ETagCache) Len() int { return c.entries.Len() }

// etagKey scopes a cache entry to the exact request shape: method+URL plus
// the Accept and Authorization headers. Authorization participates so a
// refreshed token (different access scopes or identity) never serves
// another principal's cached payload.
func etagKey(req *http.Request) string {
	return req.Method + " " + req.URL.String() + "\n" +
		req.Header.Get("Accept") + "\n" +
		req.Header.Get("Authorization") + "\n" +
		req.Header.Get("PRIVATE-TOKEN")
}

// applyConditional adds If-None-Match to req when a cached validator
// exists for it. It must run after authentication (Authorization is part
// of the cache key) and is a no-op when req already carries
// If-None-Match. Safe on a nil cache.
func (c *ETagCache) applyConditional(req *http.Request) {
	if c == nil || req.Method != http.MethodGet || req.Header.Get("If-None-Match") != "" {
		return
	}
	if e, ok := c.entries.Get(etagKey(req)); ok {
		req.Header.Set("If-None-Match", e.etag)
	}
}

// cacheable reports whether resp may be stored for this req. Requests that
// carry an explicit Accept-Encoding are excluded: only Go's transparent
// gzip delivers bodies decoded (with the header stripped), so an explicitly
// negotiated encoding would be stored verbatim while the replay strips the
// Content-Encoding header — handing callers encoded bytes labeled identity.
func (c *ETagCache) cacheable(req *http.Request, resp *http.Response) bool {
	return req.Method == http.MethodGet &&
		resp.StatusCode == http.StatusOK &&
		resp.Header.Get("ETag") != "" &&
		req.Header.Get("Accept-Encoding") == "" &&
		!hasNoStoreDirective(resp.Header)
}

// hasNoStoreDirective reports whether any Cache-Control directive list on
// the response contains no-store (case-insensitive, comma-separated lists
// included: "private, no-store" must not be cached either).
func hasNoStoreDirective(h http.Header) bool {
	for _, v := range h.Values("Cache-Control") {
		for _, d := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(d), "no-store") {
				return true
			}
		}
	}
	return false
}

// store caches body for req/resp. Content-Encoding and Content-Length are
// dropped from the stored headers: bodies are stored decoded, and the
// synthesized response recomputes the length.
func (c *ETagCache) store(req *http.Request, resp *http.Response, body []byte) {
	if !c.cacheable(req, resp) || int64(len(body)) > c.maxBodySize {
		return
	}
	h := resp.Header.Clone()
	h.Del("Content-Encoding")
	h.Del("Content-Length")
	c.entries.Add(etagKey(req), etagEntry{
		etag:   resp.Header.Get("ETag"),
		header: h,
		body:   append([]byte(nil), body...),
	})
}

// synthesize rebuilds a 200 response for a 304 using the cached entry. The
// cache-hit marker header lets response hooks observe conditional hits.
func (e etagEntry) synthesize(req *http.Request) *http.Response {
	h := e.header.Clone()
	h.Set(cacheHitHeader, "hit")
	return &http.Response{
		Status:        http.StatusText(http.StatusOK),
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(e.body)),
		ContentLength: int64(len(e.body)),
		Request:       req,
	}
}

// processRT handles a completed exchange whose body is still streaming: 304s
// are replaced by the cached 200 (or fail, see below), and cacheable 200s are
// buffered up to the entry-size cap and re-wrapped for single consumption by
// the caller. Responses that are not cacheable — including any body known or
// discovered to exceed the cap — stream through untouched, preserving the
// memory bounds and streaming contract of the underlying transport. Both the
// Client.Do path and the SDK RoundTripper path run through here, so the
// conditional-request semantics exist in exactly one implementation.
func (c *ETagCache) processRT(req *http.Request, resp *http.Response) (*http.Response, error) {
	if c == nil || resp == nil || resp.Body == nil {
		return resp, nil
	}
	if resp.StatusCode == http.StatusNotModified {
		e, ok := c.entries.Get(etagKey(req))
		if !ok {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("transport: conditional request for %s answered 304 but the cached entry is gone", redactURL(*req.URL))
		}
		_ = resp.Body.Close()
		return e.synthesize(req), nil
	}
	if !c.cacheable(req, resp) {
		return resp, nil
	}
	// Declared length already over the cap: never buffer, just stream.
	if resp.ContentLength > c.maxBodySize {
		return resp, nil
	}
	// Read at most cap+1 bytes: the +1 distinguishes "fits" from "too
	// big" without committing to buffer an unbounded body.
	head, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBodySize+1))
	if err != nil {
		// A mid-body failure must surface as a transport error, never as a
		// silently truncated 200.
		_ = resp.Body.Close()
		return nil, err
	}
	if int64(len(head)) > c.maxBodySize {
		// Too big to cache: replay the prefix concatenated with the unread
		// remainder so the caller still sees every byte, streamed.
		resp.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(head), resp.Body), Closer: resp.Body}
		return resp, nil
	}
	_ = resp.Body.Close()
	c.store(req, resp, head)
	resp.Body = io.NopCloser(bytes.NewReader(head))
	resp.ContentLength = int64(len(head))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(head)))
	return resp, nil
}

// readCloser pairs an io.Reader (e.g. an io.MultiReader over buffered and
// live bytes) with the original response body's closer.
type readCloser struct {
	io.Reader
	io.Closer
}
