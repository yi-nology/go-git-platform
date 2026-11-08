package provider

import (
	"errors"
	"net/http"
	"testing"

	"github.com/yi-nology/go-git-platform/transport"
)

// issue #42：GitHub/Gitea 把最常见的节流信号发成 403 + X-RateLimit-Remaining: 0。
// transport 层已把它判成限流（IsRateLimited），Wrap 不得把这一判定丢回
// ErrForbidden——provider.IsRateLimited 必须接到平台最常见形态。
func TestWrap403WithThrottleHeadersClassifiesRateLimited(t *testing.T) {
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "0")
	terr := transport.NewStatusErrorWithHeaders("GET", "/repos", http.StatusForbidden, []byte("rate limited"), h)

	err := Wrap(PlatformGitHub, "ListRepos", terr)
	if !IsRateLimited(err) {
		t.Fatalf("403 + Remaining:0 必须归类 ErrRateLimited, got %v", err)
	}
	if IsForbidden(err) && errors.Is(err, ErrForbidden) {
		t.Fatalf("限流判定不得与 ErrForbidden 并存: %v", err)
	}
}

// 对照：裸 403（无节流头）仍是授权失败——重试永不成功，不放大负载。
func TestWrap403WithoutThrottleHeadersStaysForbidden(t *testing.T) {
	terr := transport.NewStatusError("GET", "/repos", http.StatusForbidden, []byte("forbidden by policy"))
	err := Wrap(PlatformGitHub, "ListRepos", terr)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("裸 403 保持 ErrForbidden, got %v", err)
	}
	if IsRateLimited(err) {
		t.Fatal("裸 403 不得判为限流")
	}
}
