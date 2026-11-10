package github

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// issue #50：go-github 的 *RateLimitError（主配额耗尽，恒 403）与
// *AbuseRateLimitError（次级限流，恒 403）不实现 StatusCode() int 也不携带
// transport 的 rateLimitedMarker——GitHub 后端每个操作真实返回的就是这两类
// SDK 错误。Wrap 的裸状态分类曾把它们丢回 ErrForbidden，IsRateLimited 接不到
// 平台最常见的节流信号，RateLimitRecovery 也拿不到恢复窗口。
func TestWrapSDKRateLimitErrorClassifiesRateLimited(t *testing.T) {
	reset := time.Now().Add(time.Hour).Truncate(time.Second)
	rl := &github.RateLimitError{
		Response: &http.Response{StatusCode: http.StatusForbidden},
		Rate:     github.Rate{Remaining: 0, Reset: github.Timestamp{Time: reset}},
	}
	err := provider.Wrap(provider.PlatformGitHub, "ListRepos", rl)
	if !provider.IsRateLimited(err) {
		t.Fatalf("*github.RateLimitError 必须归类 ErrRateLimited, got %v", err)
	}
	if errors.Is(err, provider.ErrForbidden) {
		t.Fatalf("节流判定不得与 ErrForbidden 并存: %v", err)
	}
	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("Wrap 应返回 *provider.ProviderError, got %T", err)
	}
	if !pe.IsStatus(http.StatusForbidden) {
		t.Errorf("状态码应保留 403 供 IsStatus 判定, got %d", pe.StatusCode)
	}
	if _, gotReset, ok := provider.RateLimitRecovery(err); !ok {
		t.Fatalf("RateLimitRecovery 应从 Rate.Reset 拿到恢复窗口, ok=false")
	} else if !gotReset.Equal(reset) {
		t.Errorf("ResetAt = %v, want %v", gotReset, reset)
	}
}

func TestWrapSDKAbuseRateLimitErrorClassifiesRateLimited(t *testing.T) {
	retryAfter := 30 * time.Second
	ab := &github.AbuseRateLimitError{
		Response:   &http.Response{StatusCode: http.StatusForbidden},
		RetryAfter: &retryAfter,
	}
	err := provider.Wrap(provider.PlatformGitHub, "CreateIssue", ab)
	if !provider.IsRateLimited(err) {
		t.Fatalf("*github.AbuseRateLimitError 必须归类 ErrRateLimited, got %v", err)
	}
	if got, _, ok := provider.RateLimitRecovery(err); !ok || got != retryAfter {
		t.Fatalf("RetryAfter 提示应随判定复制, got (%v, ok=%v)", got, ok)
	}
}

// 对照：非限流的 SDK 403（如权限不足的 ErrorResponse）必须保持 ErrForbidden，
// 分类器的判定面不得外溢。
func TestWrapSDKPlainForbiddenStaysForbidden(t *testing.T) {
	er := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusForbidden},
		Message:  "Resource not accessible by integration",
	}
	err := provider.Wrap(provider.PlatformGitHub, "ListRepos", er)
	if !errors.Is(err, provider.ErrForbidden) {
		t.Fatalf("普通 SDK 403 保持 ErrForbidden, got %v", err)
	}
	if provider.IsRateLimited(err) {
		t.Fatal("普通 SDK 403 不得判为限流")
	}
}
