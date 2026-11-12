package provider

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/transport"
)

// 多 %w（Unwrap() []error）链上的判定一致性回归（issue #146）。
//
// house 先例（issue #132、timeparse.go）：多 %w 走 Unwrap() []error，cause
// 链随哨兵一路保留。stdlib errors.Unwrap 只认单分支——旧实现的 Wrap 与
// RateLimitRecovery 手写链遍历走到多 %w 节点即停，而同一条链上 errors.Is
// （stdlib，会遍历多分支）判得出「是限流」：观测面自相矛盾——「是限流」
// 为真、「要等多久」/状态码却取不到（RetryAfter=0、StatusCode=0）。
// 触发面不小：ParseTimestamp 等多 %w 产出点 + 后端 Wrap 的组合使链天然
// 呈多分支形态。

func TestRateLimitRecovery_MultiBranchChain(t *testing.T) {
	pe := &ProviderError{Platform: PlatformGitHub, Op: "ListRepos", StatusCode: 429, RetryAfter: 30 * time.Second}
	// 多分支节点：哨兵 + 带 hint 的 ProviderError 并列（errors.Join 形态）
	multi := stderrors.Join(fmt.Errorf("%w: context lost", ErrRateLimited), pe)
	outer := fmt.Errorf("op: %w", multi)

	if !stderrors.Is(outer, ErrRateLimited) {
		t.Fatal("precondition: stdlib errors.Is must traverse the multi-branch chain")
	}
	ra, reset, ok := RateLimitRecovery(outer)
	if !ok {
		t.Fatal("RateLimitRecovery must traverse multi-branch chains like errors.Is — hints silently lost before the fix")
	}
	if ra != 30*time.Second {
		t.Fatalf("retryAfter = %v, want 30s", ra)
	}
	if !reset.IsZero() {
		t.Fatalf("resetAt = %v, want zero", reset)
	}

	// 分支序（depth-first preorder）：首个带 hint 的节点胜出
	first := &ProviderError{Platform: PlatformGitHub, Op: "A", RetryAfter: 5 * time.Second}
	second := &ProviderError{Platform: PlatformGitHub, Op: "B", RetryAfter: 9 * time.Second}
	joined := stderrors.Join(first, second)
	if ra, _, ok := RateLimitRecovery(joined); !ok || ra != 5*time.Second {
		t.Fatalf("first matching branch must win: ra=%v ok=%v", ra, ok)
	}
}

func TestRateLimitRecovery_SingleBranchUnchanged(t *testing.T) {
	pe := &ProviderError{Platform: PlatformGitHub, Op: "ListRepos", StatusCode: 429, RetryAfter: 30 * time.Second}
	ra, _, ok := RateLimitRecovery(fmt.Errorf("op: %w", pe))
	if !ok || ra != 30*time.Second {
		t.Fatalf("single-branch control: ra=%v ok=%v, want 30s true", ra, ok)
	}
	// ProviderError 无 hint 时继续下潜（旧行为保留）
	peNoHint := &ProviderError{Platform: PlatformGitHub, Op: "A"}
	inner := &ProviderError{Platform: PlatformGitHub, Op: "B", ResetAt: time.Unix(1, 0)}
	nested := fmt.Errorf("%w: %w", peNoHint, inner)
	if _, at, ok := RateLimitRecovery(nested); !ok || at.Unix() != 1 {
		t.Fatalf("walk must descend past hint-less ProviderError: at=%v ok=%v", at, ok)
	}
}

func TestWrap_MultiBranchChainKeepsStatusAndHints(t *testing.T) {
	reset := time.Now().Add(time.Minute).Unix()
	h := http.Header{}
	h.Set("Retry-After", "5")
	h.Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
	statusErr := transport.NewStatusErrorWithHeaders("GET", "/repos", http.StatusTooManyRequests, []byte(`{}`), h)

	// 真实形态：transport 429 与上下文错误被 errors.Join / 多 %w 包装后
	// 再进 Wrap（ParseTimestamp 类多 %w 产出点在后端被 Wrap 的同构面）
	multi := stderrors.Join(fmt.Errorf("fetch page: %w", stderrors.New("boom")),
		fmt.Errorf("server said: %w", statusErr))
	outer := fmt.Errorf("ListRepos: %w", multi)

	pe := Wrap(PlatformGitHub, "ListRepos", outer).(*ProviderError)
	if pe.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d, want 429（多 %%w 链上状态码不得静默丢失）", pe.StatusCode)
	}
	if !pe.IsStatus(http.StatusTooManyRequests) || !pe.IsClientError() {
		t.Fatalf("IsStatus/IsClientError must see the 429 through a multi-branch chain")
	}
	if !stderrors.Is(pe, ErrRateLimited) {
		t.Fatalf("errors.Is(pe, ErrRateLimited) = false, want true (429 classifies as ErrRateLimited)")
	}
	ra, _, ok := RateLimitRecovery(pe)
	if !ok || ra != 5*time.Second {
		t.Fatalf("RateLimitRecovery after Wrap: ra=%v ok=%v, want 5s true", ra, ok)
	}
}

// TestWalkErrorChain_CycleGuard：多分支与环（自引用 Join）不得挂死遍历。
func TestWalkErrorChain_CycleGuard(t *testing.T) {
	var cyclic error
	{
		a := fmt.Errorf("a: %w", stderrors.New("leaf"))
		b := stderrors.Join(a, stderrors.New("b"))
		// 构造环：b 的分支里再包一层含 b 的错误（凭 identity 剪环）
		cyclic = stderrors.Join(b, fmt.Errorf("outer: %w", b))
	}
	visited := 0
	walkErrorChain(cyclic, func(error) bool {
		visited++
		return false
	})
	if visited == 0 {
		t.Fatal("walk must visit nodes")
	}
}
