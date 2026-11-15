package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// rotatingTokenSource hands out tokens in order, one per call, and then
// keeps returning the last one: a rotation, not a cycle, so ordering
// assertions stay monotonic.
type rotatingTokenSource struct {
	mu     sync.Mutex
	tokens []string
	calls  int
}

func (s *rotatingTokenSource) Token(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.calls
	if i >= len(s.tokens) {
		i = len(s.tokens) - 1
	}
	s.calls++
	return s.tokens[i], nil
}

// authRecorder records the auth-bearing header values of every request so
// subtests can assert which credential (and only which credential) reached
// the server, regardless of the platform's header scheme.
type authRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (a *authRecorder) record(r *http.Request) {
	var v string
	switch {
	case r.Header.Get("Authorization") != "":
		v = r.Header.Get("Authorization")
	case r.Header.Get("PRIVATE-TOKEN") != "":
		v = "PRIVATE-TOKEN " + r.Header.Get("PRIVATE-TOKEN")
	}
	if v == "" {
		return
	}
	a.mu.Lock()
	a.seen = append(a.seen, v)
	a.mu.Unlock()
}

func (a *authRecorder) values() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.seen...)
}

// testTokenSourceRotation verifies that a Config.TokenSource is consulted
// per request: a rotated credential is used by the very next call without
// rebuilding the provider. This is the contract that makes expiring
// credentials (GitHub App installation tokens, OAuth flows) usable.
func testTokenSourceRotation(t *testing.T, h Harness) {
	rec := &authRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.EmptyListResponse))
	}))
	defer srv.Close()

	src := &rotatingTokenSource{tokens: []string{"rot-one", "rot-two"}}
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, TokenSource: src})

	if _, err := p.ListRepos(t.Context(), provider.ListRepoOptions{Page: 1, PerPage: 10}); err != nil {
		t.Fatalf("ListRepos #1: %v", err)
	}
	if _, err := p.ListRepos(t.Context(), provider.ListRepoOptions{Page: 1, PerPage: 10}); err != nil {
		t.Fatalf("ListRepos #2: %v", err)
	}

	seen := rec.values()
	if len(seen) == 0 {
		t.Fatal("no authenticated request reached the server")
	}
	// Some SDKs issue a construction-time probe (e.g. a version check)
	// against an endpoint the recorder does not cover, consuming the first
	// token off-record. The contract is ordering, not bookkeeping: tokens
	// may only appear in rot-one-before-rot-two order, and the most recent
	// request must carry the freshest token.
	var lastOne, firstTwo = -1, -1
	for i, v := range seen {
		switch {
		case strings.Contains(v, "rot-one"):
			lastOne = i
		case strings.Contains(v, "rot-two"):
			if firstTwo == -1 {
				firstTwo = i
			}
		default:
			t.Fatalf("unexpected credential %q reached the server", v)
		}
	}
	if lastOne != -1 && firstTwo != -1 && lastOne > firstTwo {
		t.Fatalf("credentials regressed: rot-one at %d after rot-two at %d (%v)", lastOne, firstTwo, seen)
	}
	if last := seen[len(seen)-1]; !strings.Contains(last, "rot-two") {
		t.Fatalf("last auth = %q, want rot-two (rotation must apply per request)", last)
	}
}

// testConditionalRequests verifies Config.ConditionalRequests end to end
// through a real backend: the second identical GET carries If-None-Match,
// the 304 is replayed as the cached 200, and the caller observes the
// original (here: empty) list without error.
func testConditionalRequests(t *testing.T, h Harness) {
	var conditional atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"contract"`)
		if r.Header.Get("If-None-Match") == `"contract"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(h.EmptyListResponse))
	}))
	defer srv.Close()

	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test", ConditionalRequests: true})

	for i := range 2 {
		repos, err := p.ListRepos(t.Context(), provider.ListRepoOptions{Page: 1, PerPage: 10})
		if err != nil {
			t.Fatalf("ListRepos #%d: %v", i+1, err)
		}
		if len(repos) != 0 {
			t.Fatalf("ListRepos #%d: got %d repos, want cached empty list", i+1, len(repos))
		}
	}
	if got := conditional.Load(); got != 1 {
		t.Fatalf("server saw %d conditional requests, want exactly 1 (second call revalidates)", got)
	}
}
