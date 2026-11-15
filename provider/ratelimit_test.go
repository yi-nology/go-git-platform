package provider

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/transport"
)

func TestWrapCopiesRateLimitHints(t *testing.T) {
	reset := time.Now().Add(time.Minute).Unix()
	h := http.Header{}
	h.Set("Retry-After", "5")
	h.Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
	src := transport.NewStatusErrorWithHeaders("GET", "/repos", 429, []byte(`{}`), h)

	pe := Wrap(model.PlatformGitHub, "ListRepos", src).(*ProviderError)
	if !IsRateLimited(pe) {
		t.Fatal("wrapped error should classify as rate-limited")
	}
	ra, at, ok := RateLimitRecovery(pe)
	if !ok {
		t.Fatal("RateLimitRecovery found no hints")
	}
	if ra != 5*time.Second {
		t.Fatalf("retryAfter = %v, want 5s", ra)
	}
	if at.Unix() != reset {
		t.Fatalf("resetAt unix = %d, want %d", at.Unix(), reset)
	}
}

func TestRateLimitRecoveryWithoutHints(t *testing.T) {
	if _, _, ok := RateLimitRecovery(Wrap(model.PlatformGitHub, "GetRepo", transport.NewStatusError("GET", "/x", 404, nil))); ok {
		t.Fatal("404 must not carry rate-limit hints")
	}
	if _, _, ok := RateLimitRecovery(nil); ok {
		t.Fatal("nil must not carry hints")
	}
}
