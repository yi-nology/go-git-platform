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

// TestParseNumberHelpers_RejectWithInvalidInput pins issue #165: every
// malformed issue/PR/milestone number rejection must carry
// provider.ErrInvalidInput on the error chain (same classification as the
// 400/422 write-side rejections) while keeping the structured ProviderError
// face (platform, op, status 0). 规则二 forbids message-text matching.
func TestParseNumberHelpers_RejectWithInvalidInput(t *testing.T) {
	const platform = provider.PlatformGitHub
	cases := []struct {
		name string
		op   string
		call func() error
	}{
		{"ParseIssueNumber", "GetIssue", func() error {
			_, err := ParseIssueNumber(platform, "GetIssue", "not-a-number")
			return err
		}},
		{"ParseIssueNumber64", "ListIssues", func() error {
			_, err := ParseIssueNumber64(platform, "ListIssues", "1x")
			return err
		}},
		{"ParsePRNumber", "GetCR", func() error {
			_, err := ParsePRNumber(platform, "GetCR", "")
			return err
		}},
		{"ParsePRNumber64", "MergeCR", func() error {
			_, err := ParsePRNumber64(platform, "MergeCR", "1.5")
			return err
		}},
		{"ParseMilestoneNumber", "GetMilestone", func() error {
			_, err := ParseMilestoneNumber(platform, "GetMilestone", "m1")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("malformed number must be rejected")
			}
			if !errors.Is(err, provider.ErrInvalidInput) {
				t.Errorf("errors.Is(err, ErrInvalidInput) = false for %v, want true", err)
			}
			var pe *provider.ProviderError
			if !errors.As(err, &pe) {
				t.Fatalf("err type %T, want *provider.ProviderError", err)
			}
			if pe.Platform != platform || pe.Op != tc.op {
				t.Errorf("ProviderError = {%s %s}, want {%s %s}", pe.Platform, pe.Op, platform, tc.op)
			}
			if pe.StatusCode != 0 {
				t.Errorf("StatusCode = %d, want 0 (pre-wire rejection)", pe.StatusCode)
			}
		})
	}
}

// TestParseNumberHelpers_AcceptNumeric pins that the classification change
// adds matchability only — numeric inputs still parse on the success path.
func TestParseNumberHelpers_AcceptNumeric(t *testing.T) {
	if n, err := ParseIssueNumber(provider.PlatformGitHub, "GetIssue", "42"); err != nil || n != 42 {
		t.Errorf("ParseIssueNumber(42) = (%d, %v), want (42, nil)", n, err)
	}
	if n, err := ParseIssueNumber64(provider.PlatformGitLab, "GetIssue", "42"); err != nil || n != 42 {
		t.Errorf("ParseIssueNumber64(42) = (%d, %v), want (42, nil)", n, err)
	}
	if n, err := ParsePRNumber(provider.PlatformGitee, "GetCR", "42"); err != nil || n != 42 {
		t.Errorf("ParsePRNumber(42) = (%d, %v), want (42, nil)", n, err)
	}
	if n, err := ParsePRNumber64(provider.PlatformGitLab, "GetCR", "42"); err != nil || n != 42 {
		t.Errorf("ParsePRNumber64(42) = (%d, %v), want (42, nil)", n, err)
	}
	if n, err := ParseMilestoneNumber(provider.PlatformGitHub, "GetMilestone", "42"); err != nil || n != 42 {
		t.Errorf("ParseMilestoneNumber(42) = (%d, %v), want (42, nil)", n, err)
	}
}
