package provider

import (
	"errors"
	"net/http"
	"testing"
	"time"

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

// issue #50 机制面：任何平台的 SDK 错误路——注册了 ThrottleClassifier 的平台，
// Wrap 在状态分类之前采纳其节流判定并复制恢复窗口；未注册的平台维持裸状态
// 分类。这里用测试专用平台与合成错误类型验证机制本身（不 import 任何后端）。
type sdkThrottleError struct{ response *http.Response }

func (e *sdkThrottleError) Error() string { return "secondary limit exceeded" }

type sdkPlainError struct{ response *http.Response }

func (e *sdkPlainError) Error() string { return "forbidden by policy" }

func TestWrapSDKThrottleClassifierMechanism(t *testing.T) {
	const p Platform = "sdk-throttle-test"
	RegisterStatusExtractor(p, func(err error) (int, bool) {
		if e, ok := err.(*sdkThrottleError); ok && e.response != nil {
			return e.response.StatusCode, true
		}
		if e, ok := err.(*sdkPlainError); ok && e.response != nil {
			return e.response.StatusCode, true
		}
		return 0, false
	})
	RegisterThrottleClassifier(p, func(err error) (time.Duration, time.Time, bool) {
		if _, ok := err.(*sdkThrottleError); ok {
			return 30 * time.Second, time.Time{}, true
		}
		return 0, time.Time{}, false
	})

	err := Wrap(p, "ListRepos", &sdkThrottleError{response: &http.Response{StatusCode: http.StatusForbidden}})
	if !IsRateLimited(err) {
		t.Fatalf("分类器判定节流的 SDK 403 必须归类 ErrRateLimited, got %v", err)
	}
	if ra, _, ok := RateLimitRecovery(err); !ok || ra != 30*time.Second {
		t.Fatalf("分类器恢复窗口应复制进 ProviderError, got (%v, ok=%v)", ra, ok)
	}

	err = Wrap(p, "ListRepos", &sdkPlainError{response: &http.Response{StatusCode: http.StatusForbidden}})
	if !errors.Is(err, ErrForbidden) || IsRateLimited(err) {
		t.Fatalf("非节流 SDK 403 保持 ErrForbidden, got %v", err)
	}
}

// 未注册分类器的平台：同样的 SDK 错误维持裸状态分类（403 → ErrForbidden）。
func TestWrapWithoutThrottleClassifierKeepsStatusClassification(t *testing.T) {
	const p Platform = "sdk-no-classifier-test"
	RegisterStatusExtractor(p, func(err error) (int, bool) {
		if e, ok := err.(*sdkThrottleError); ok && e.response != nil {
			return e.response.StatusCode, true
		}
		return 0, false
	})
	err := Wrap(p, "ListRepos", &sdkThrottleError{response: &http.Response{StatusCode: http.StatusForbidden}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("无分类器平台 403 维持 ErrForbidden, got %v", err)
	}
	if IsRateLimited(err) {
		t.Fatal("无分类器平台不得判为限流")
	}
}
