package transport

import (
	"errors"
	"net/http"
	"testing"
	"unicode/utf8"
)

func TestError_Is(t *testing.T) {
	a := &Error{Method: "GET", Path: "/x", statusCode: 404}
	b := &Error{Method: "GET", Path: "/x", statusCode: 404}
	if !errors.Is(a, b) {
		t.Error("expected identical errors to be Is-equal")
	}
	// Different status codes should NOT be Is-equal even with same method/path
	c := &Error{Method: "GET", Path: "/x", statusCode: 500}
	if errors.Is(a, c) {
		t.Error("expected different status codes to not match")
	}
	// Same status, different path should still match (status-based identity)
	d := &Error{Method: "POST", Path: "/y", statusCode: 404}
	if !errors.Is(a, d) {
		t.Error("expected same status code to match regardless of method/path")
	}
}

func TestError_IsStatusCode(t *testing.T) {
	e := &Error{statusCode: 404}
	if !errors.Is(e, &Error{statusCode: 404, Method: "GET", Path: "/x"}) {
		t.Error("expected same status code to match via Is")
	}
}

func TestError_IsStatus(t *testing.T) {
	e := &Error{statusCode: 502}
	if !e.IsStatus(502) {
		t.Error("expected IsStatus(502) true")
	}
	if e.IsStatus(500) {
		t.Error("expected IsStatus(500) false")
	}
}

func TestError_StatusClasses(t *testing.T) {
	client := &Error{statusCode: 404}
	server := &Error{statusCode: 503}
	if !client.IsClientError() {
		t.Error("expected 404 to be client error")
	}
	if !server.IsServerError() {
		t.Error("expected 503 to be server error")
	}
	if client.IsServerError() || server.IsClientError() {
		t.Error("cross-class detection should be false")
	}
}

func TestError_IsStatusClass(t *testing.T) {
	e := &Error{statusCode: 599}
	if !e.IsStatusClass(http.StatusInternalServerError) {
		t.Error("599 should be 5xx")
	}
	e2 := &Error{statusCode: 200}
	if e2.IsStatusClass(http.StatusInternalServerError) {
		t.Error("200 should not be 5xx")
	}
}

func TestNewStatusError(t *testing.T) {
	e := NewStatusError("POST", "/x", 500, []byte("oops")).(*Error)
	if e.StatusCode() != 500 {
		t.Errorf("expected 500, got %d", e.StatusCode())
	}
	if string(e.Body) != "oops" {
		t.Errorf("expected body 'oops', got %q", e.Body)
	}
	if e.Method != "POST" || e.Path != "/x" {
		t.Errorf("expected method/path preserved, got %s/%s", e.Method, e.Path)
	}
}

func TestError_TruncatesLargeBodies(t *testing.T) {
	big := make([]byte, 1024)
	for i := range big {
		big[i] = 'x'
	}
	e := NewStatusError("GET", "/x", 500, big).(*Error)
	if len(e.Body) != 1024 {
		// Body is the full bytes; truncation only happens in Error()
		t.Errorf("expected body to be stored fully, got %d", len(e.Body))
	}
	if !containsString(e.Error(), "truncated") {
		t.Errorf("expected Error() to mention truncation, got %q", e.Error())
	}
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// statusCoder mirrors the interface provider.Wrap uses to short-circuit status
// extraction; *Error must satisfy it without reflection.
type statusCoder interface{ StatusCode() int }

// Compile-time check: *transport.Error implements statusCoder.
var _ statusCoder = (*Error)(nil)

// TestError_StatusCodeMethod locks the statusCoder fix: *Error exposes its
// status via a StatusCode() method so interface-based consumers (provider.Wrap)
// match it directly instead of falling back to reflection.
func TestError_StatusCodeMethod(t *testing.T) {
	e := NewStatusError("GET", "/x", 429, []byte("slow down")).(*Error)
	if got := e.StatusCode(); got != 429 {
		t.Errorf("expected 429 from StatusCode(), got %d", got)
	}
	var sc statusCoder = e
	if got := sc.StatusCode(); got != 429 {
		t.Errorf("expected 429 via statusCoder interface, got %d", got)
	}
}

// TestTruncate_CJKStaysValidUTF8 pins the pkg#327 adoption (issue #138): the
// byte-budget truncator must never split a multi-byte rune — the old hard
// byte slice turned a CJK response body into invalid UTF-8 (log shippers'
// JSON encoders choke on it, grep loses the line). ASCII truncation must stay
// byte-for-byte identical to the old implementation.
func TestTruncate_CJKStaysValidUTF8(t *testing.T) {
	// "汉"是 3 字节;10 字节预算恰在第 4 个"汉"的第 2 字节处切断。
	body := []byte("汉汉汉汉汉汉") // 18 bytes
	got := truncate(body, 10)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate produced invalid UTF-8: %q", got)
	}
	if want := "汉汉汉...(truncated)"; got != want {
		t.Errorf("truncate CJK = %q, want %q (budget may only shrink on rune boundaries)", got, want)
	}

	// ASCII:逐字节与旧硬切实现一致(含后缀)。
	ascii := []byte("abcdefghijk") // 11 bytes
	if got, want := truncate(ascii, 10), "abcdefghij...(truncated)"; got != want {
		t.Errorf("truncate ascii = %q, want %q", got, want)
	}

	// 预算内原样返回,无后缀。
	if got, want := truncate([]byte("短"), 10), "短"; got != want {
		t.Errorf("truncate within budget = %q, want %q", got, want)
	}
}
