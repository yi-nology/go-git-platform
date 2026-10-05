package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// rotatingSource hands out tokens in sequence.
type rotatingSource struct {
	mu     sync.Mutex
	tokens []string
	calls  int
}

func (s *rotatingSource) Token(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tok := s.tokens[s.calls%len(s.tokens)]
	s.calls++
	return tok, nil
}

func TestTokenSourceAuthRotatesPerRequest(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := &rotatingSource{tokens: []string{"t-one", "t-two"}}
	c := NewClient(srv.URL, TokenSourceAuth{Source: src})
	hc := &http.Client{Transport: c.RoundTripper()}

	for range 2 {
		resp, err := hc.Get(srv.URL + "/x")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if len(seen) != 2 || seen[0] != "Bearer t-one" || seen[1] != "Bearer t-two" {
		t.Fatalf("authorizations seen = %v, want rotation to t-two", seen)
	}
}

func TestTokenSourceAuthStyles(t *testing.T) {
	cases := map[string]struct {
		style string
		want  string
	}{
		"bearer":  {AuthStyleBearer, "Bearer tok"},
		"private": {AuthStylePrivate, ""},
		"token":   {AuthStyleToken, "token tok"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var gotAuth, gotPrivate string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				gotPrivate = r.Header.Get("PRIVATE-TOKEN")
			}))
			defer srv.Close()

			c := NewClient(srv.URL, TokenSourceAuth{Source: StaticTokenSource("tok"), Style: tc.style})
			if _, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/x"}); err != nil {
				t.Fatalf("Do: %v", err)
			}
			switch tc.style {
			case AuthStylePrivate:
				if gotPrivate != "tok" {
					t.Fatalf("PRIVATE-TOKEN = %q, want tok", gotPrivate)
				}
			default:
				if gotAuth != tc.want {
					t.Fatalf("Authorization = %q, want %q", gotAuth, tc.want)
				}
			}
		})
	}
}

func TestTokenSourceAuthErrorAbortsRequest(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
	}))
	defer srv.Close()

	boom := errors.New("token unavailable")
	c := NewClient(srv.URL, TokenSourceAuth{Source: tokenFunc(func(ctx context.Context) (string, error) {
		return "", boom
	})})

	// RoundTripper path: a rejecting context-auth aborts before the network.
	_, err := (&http.Client{Transport: c.RoundTripper()}).Get(srv.URL + "/x")
	if !errors.Is(err, boom) {
		t.Fatalf("RoundTripper err = %v, want boom", err)
	}
	// Do path: same, surfaced as a wrapped auth error.
	_, err = c.Do(t.Context(), &Request{Method: "GET", Path: "/x"})
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Do err = %v, want wrapped boom", err)
	}
	if requests != 0 {
		t.Fatalf("%d requests reached the server, want 0", requests)
	}
}

type tokenFunc func(ctx context.Context) (string, error)

func (f tokenFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

func TestUserAgentInjected(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	if _, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/x"}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !strings.Contains(ua, "go-git-platform/") {
		t.Fatalf("User-Agent = %q, missing SDK token", ua)
	}
}

func TestUserAgentAppendsToSDKAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, None{}, WithTransport(srv.Client().Transport))
	hc := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.Header.Set("User-Agent", "go-github/v92")
		return c.RoundTripper().RoundTrip(r)
	})}
	resp, err := hc.Get(srv.URL + "/x")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if !strings.Contains(ua, "go-github/v92") || !strings.Contains(ua, "go-git-platform/") {
		t.Fatalf("User-Agent = %q, want both product tokens", ua)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
