package gitea_test

// issue #55 回归:gitea/forgejo 的 SDK 错误是纯文本、不带状态码,而适配层
// 此前把携带状态的 *Response 整体丢弃 ⇒ IsNotFound/IsRateLimited/
// IsConflict 等哨兵在这两个平台恒 false(仅 4 处特判方法可判)。适配层
// 现在统一以 withStatus 把状态串进错误链(WrapStatusError → provider.Wrap
// 的 statusCoder 快路径),本套件按「状态码 × 方法」矩阵逐点钉死分类。

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/go-git-platform/provider"
)

// newStatusServer 对任意 API 路径回固定状态码 + gitea 风格错误体
// (SDK 的 statusCodeToErr 解 message 字段进错误文本)。
func newStatusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"message":"user does not exist [id: 7, name: 7]"}`))
	})
	return httptest.NewServer(mux)
}

// TestErrorClassification_Matrix 状态码 → 哨兵矩阵:任意读方法上的
// 404/403/429/409/401/5xx 都必须可判定,不再依赖散落的特判。
func TestErrorClassification_Matrix(t *testing.T) {
	tests := []struct {
		status     int
		wantIs     func(err error) string // 非空串 = 断言失败原因
		wantStatus int
	}{
		{status: http.StatusNotFound, wantIs: sentinelCheck(provider.IsNotFound, "ErrNotFound"), wantStatus: 404},
		{status: http.StatusForbidden, wantIs: sentinelCheck(provider.IsForbidden, "ErrForbidden"), wantStatus: 403},
		{status: http.StatusTooManyRequests, wantIs: sentinelCheck(provider.IsRateLimited, "ErrRateLimited"), wantStatus: 429},
		{status: http.StatusConflict, wantIs: sentinelCheck(provider.IsConflict, "ErrConflict"), wantStatus: 409},
		{status: http.StatusUnauthorized, wantIs: sentinelCheck(provider.IsAuthentication, "ErrAuthentication"), wantStatus: 401},
		{status: http.StatusInternalServerError, wantIs: func(err error) string {
			var pe *provider.ProviderError
			if !assert.ErrorAs(t, err, &pe) || !pe.IsServerError() {
				return "5xx 必须可判 IsServerError"
			}
			return ""
		}, wantStatus: 500},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("HTTP_%d", tt.status), func(t *testing.T) {
			srv := newStatusServer(t, tt.status)
			defer srv.Close()
			p := newTestProvider(t, srv)

			_, err := p.GetIssue(context.Background(), "owner", "repo", "7")
			require.Error(t, err)
			if reason := tt.wantIs(err); reason != "" {
				t.Errorf("HTTP %d 必须命中对应哨兵(实际=%v):%s", tt.status, err, reason)
			}
			var pe *provider.ProviderError
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, tt.wantStatus, pe.StatusCode)
			assert.Contains(t, err.Error(), "user does not exist", "错误信息保留 SDK 原始细节")
		})
	}
}

func sentinelCheck(fn func(err error) bool, name string) func(err error) string {
	return func(err error) string {
		if !fn(err) {
			return "错误链必须可判 " + name
		}
		return ""
	}
}

// TestErrorClassification_AcrossMethods 404 形态在**多个此前裸 Wrap 的方法**
// 上全部可判(契约此前只采样恰好被特判的 GetRepo)。
func TestErrorClassification_AcrossMethods(t *testing.T) {
	srv := newStatusServer(t, http.StatusNotFound)
	defer srv.Close()
	p := newTestProvider(t, srv)
	ctx := context.Background()

	calls := []struct {
		name string
		call func() error
	}{
		{"GetIssue", func() error { _, err := p.GetIssue(ctx, "owner", "repo", "7"); return err }},
		{"GetCR", func() error { _, err := p.GetCR(ctx, "owner", "repo", "7"); return err }},
		{"GetMilestone", func() error { _, err := p.GetMilestone(ctx, "owner", "repo", "3"); return err }},
		{"GetBranchProtection", func() error { _, err := p.GetBranchProtection(ctx, "owner", "repo", "main"); return err }},
		{"GetReleaseByTag", func() error { _, err := p.GetReleaseByTag(ctx, "owner", "repo", "v1"); return err }},
		{"GetCommit", func() error { _, err := p.GetCommit(ctx, "owner", "repo", "abc"); return err }},
		{"GetWebhook", func() error { _, err := p.GetWebhook(ctx, "owner", "repo", 9); return err }},
		{"DeleteLabel", func() error { return p.DeleteLabel(ctx, "owner", "repo", "bug") }},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			err := c.call()
			require.Error(t, err, "%s 在 404 下必须报错", c.name)
			assert.True(t, provider.IsNotFound(err), "%s 的 404 必须归类 ErrNotFound(实际=%v)", c.name, err)
		})
	}
}
