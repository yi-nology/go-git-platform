package backendutil

import (
	"errors"
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

// TestParseNumberRejectionsCarryInvalidInput_Issue165 pins the sentinel
// contract of the shared parse-rejection family: every rejection is
// matchable via errors.Is(err, provider.ErrInvalidInput) at the unified
// ProviderError shape (pre-wire: StatusCode 0), and valid numbers are
// unaffected. Before #165 the rejections were bare ProviderErrors with no
// sentinel in the cause chain, so CR/PR negative cases could only pin the
// structural face.
func TestParseNumberRejectionsCarryInvalidInput_Issue165(t *testing.T) {
	const (
		platform = provider.PlatformGitHub
		op       = "GetThing"
		bad      = "not-a-number"
	)
	if _, err := ParseIssueNumber(platform, op, bad); !errors.Is(err, provider.ErrInvalidInput) {
		t.Errorf("ParseIssueNumber: errors.Is(err, ErrInvalidInput) = false, got %v", err)
	}
	if _, err := ParseIssueNumber64(platform, op, bad); !errors.Is(err, provider.ErrInvalidInput) {
		t.Errorf("ParseIssueNumber64: errors.Is(err, ErrInvalidInput) = false, got %v", err)
	}
	if _, err := ParsePRNumber(platform, op, bad); !errors.Is(err, provider.ErrInvalidInput) {
		t.Errorf("ParsePRNumber: errors.Is(err, ErrInvalidInput) = false, got %v", err)
	}
	if _, err := ParsePRNumber64(platform, op, bad); !errors.Is(err, provider.ErrInvalidInput) {
		t.Errorf("ParsePRNumber64: errors.Is(err, ErrInvalidInput) = false, got %v", err)
	}
	if _, err := ParseMilestoneNumber(platform, op, bad); !errors.Is(err, provider.ErrInvalidInput) {
		t.Errorf("ParseMilestoneNumber: errors.Is(err, ErrInvalidInput) = false, got %v", err)
	}

	// Unified shape: pre-wire ProviderError keeps StatusCode 0.
	_, err := ParsePRNumber(platform, "RequestTeamReviewers", bad)
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("err type %T, want *provider.ProviderError", err)
	}
	if pe.StatusCode != 0 {
		t.Errorf("ProviderError.StatusCode = %d, want 0 (pre-wire, never HTTP-classified)", pe.StatusCode)
	}
	if pe.Op != "RequestTeamReviewers" {
		t.Errorf("ProviderError.Op = %q, want RequestTeamReviewers", pe.Op)
	}

	// Valid numbers are unaffected (success path unchanged).
	if n, err := ParsePRNumber(platform, op, "42"); err != nil || n != 42 {
		t.Errorf("ParsePRNumber(42) = %d, %v; want 42, nil", n, err)
	}
	if n, err := ParsePRNumber64(platform, op, "42"); err != nil || n != 42 {
		t.Errorf("ParsePRNumber64(42) = %d, %v; want 42, nil", n, err)
	}
}
