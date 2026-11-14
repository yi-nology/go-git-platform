package contracttest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// ErrorClassSuite 钉「状态码 → 哨兵」分类经各后端真实错误链成立
// （2B spec §3.2）。分类单源在 provider.Wrap/classifyStatusCode——
// 本套件证明每个后端的 SDK/transport 错误都流经该单源。
// 自带 httptest（stubHandler 不能改状态码——既有模式：testIsNotFound/testRetry）。
//
// 重试前置（Step 1）：baseCfg 不设 RetryConfig ⇒ MapRetryConfig(nil) ⇒
// transport.WithRetry(nil) ⇒ retryingRoundTripper 单次尝试——429/5xx 不被
// 重试，套件无需额外关重试。
func testErrorClassSuite(t *testing.T, h Harness) {
	t.Helper()
	cases := []struct {
		status int
		want   error
		name   string
	}{
		{http.StatusUnauthorized, provider.ErrAuthentication, "401→ErrAuthentication"},
		{http.StatusForbidden, provider.ErrForbidden, "403→ErrForbidden"},
		{http.StatusConflict, provider.ErrConflict, "409→ErrConflict"},
		{http.StatusTooManyRequests, provider.ErrRateLimited, "429→ErrRateLimited"},
		{http.StatusInternalServerError, provider.ErrServerSide, "500→ErrServerSide"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
			}))
			defer srv.Close()
			p := h.NewProvider(t, baseCfg(h, srv.URL))
			im, ok := p.(provider.IssueManager)
			if !ok {
				t.Skipf("%s lacks IssueManager — no wire method to drive", h.Name)
			}
			_, _, err := im.ListIssues(context.Background(), provider.ListIssuesOptions{Owner: "o", Repo: "r"})
			if err == nil {
				t.Fatalf("status %d: want error, got nil", c.status)
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("status %d: want %v, got %v", c.status, c.want, err)
			}
		})
	}
}
