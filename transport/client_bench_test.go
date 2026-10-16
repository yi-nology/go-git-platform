package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// benchResponse is a realistic API-list payload (a repo list page is
// tens of KB; file contents can be larger).
var benchPayload = []byte(`{"items":[` + strings.Repeat(`{"name":"repo-some-name","stars":42,"archived":false},`, 400) + `{"name":"repo-last","stars":1,"archived":true}]}`)

func newBenchClient(b *testing.B, srv *httptest.Server) *Client {
	b.Helper()
	return NewClient(srv.URL, None{})
}

func BenchmarkDoNoopLogger(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(benchPayload)
	}))
	defer srv.Close()
	c := newBenchClient(b, srv)
	req := &Request{Method: "GET", Path: "/repos"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Do(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDoJSONNoopLogger(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(benchPayload)
	}))
	defer srv.Close()
	c := newBenchClient(b, srv)
	type result struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &Request{Method: "GET", Path: "/repos", Result: &result{}}
		if _, err := c.DoJSON(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}
