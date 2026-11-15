package backendutil

import (
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestMapRetryConfig_PlusOneConversion guards the off-by-one mapping between
// provider.RetryConfig (counts retries) and transport.RetryConfig (counts
// total attempts): MaxRetries=2 must become MaxAttempts=3.
func TestMapRetryConfig_PlusOneConversion(t *testing.T) {
	rc := &provider.RetryConfig{MaxRetries: 2, BaseDelay: time.Second, MaxDelay: 10 * time.Second, RetryOn: []int{409}}
	got := MapRetryConfig(rc)
	if got == nil {
		t.Fatal("expected non-nil transport retry config")
	}
	if got.MaxAttempts != 3 {
		t.Errorf("MaxAttempts: expected 3 (MaxRetries+1), got %d", got.MaxAttempts)
	}
	if got.BaseDelay != time.Second || got.MaxDelay != 10*time.Second {
		t.Errorf("delays not carried over: %+v", got)
	}
	if len(got.Statuses) != 1 || got.Statuses[0] != 409 {
		t.Errorf("statuses not carried over: %+v", got.Statuses)
	}
}

func TestMapRetryConfig_Nil(t *testing.T) {
	if got := MapRetryConfig(nil); got != nil {
		t.Errorf("expected nil for nil input, got %+v", got)
	}
}

func TestDefaultBaseURL(t *testing.T) {
	if got := DefaultBaseURL("", "https://def.example.com"); got != "https://def.example.com" {
		t.Errorf("expected default, got %q", got)
	}
	if got := DefaultBaseURL("https://custom.example.com", "https://def.example.com"); got != "https://custom.example.com" {
		t.Errorf("expected custom to win, got %q", got)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://gitea.example.com/":        "https://gitea.example.com",
		"https://gitea.example.com/api/v1":  "https://gitea.example.com",
		"https://gitea.example.com/api/v1/": "https://gitea.example.com",
		"https://gitea.example.com":         "https://gitea.example.com",
	}
	for in, want := range cases {
		if got := NormalizeBaseURL(in); got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConvertHooks_Nil(t *testing.T) {
	if got := ConvertHooks(nil); got != nil {
		t.Errorf("expected nil for nil hooks, got %+v", got)
	}
}

func TestHTTPTransport_SecureDefault(t *testing.T) {
	// When SkipTLS is false we must get the default transport (verification on).
	if HTTPTransport(false) == nil {
		t.Fatal("expected non-nil transport")
	}
}
