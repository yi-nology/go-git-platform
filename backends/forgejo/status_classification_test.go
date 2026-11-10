package forgejo_test

// issue #55 回归(forgejo 侧):SDK 错误是纯文本、不带状态码,适配层此前把
// 携带状态的 *Response 整体丢弃 ⇒ 哨兵分类恒 false。适配层现在统一以
// withStatus 把状态串进错误链,本套件钉死 forgejo 的分类矩阵。

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

// TestErrorClassification_Matrix 状态码 → 哨兵矩阵(forgejo)。
func TestErrorClassification_Matrix(t *testing.T) {
	tests := []struct {
		status int
		isFn   func(err error) bool
		name   string
	}{
		{status: http.StatusNotFound, isFn: provider.IsNotFound, name: "ErrNotFound"},
		{status: http.StatusForbidden, isFn: provider.IsForbidden, name: "ErrForbidden"},
		{status: http.StatusTooManyRequests, isFn: provider.IsRateLimited, name: "ErrRateLimited"},
		{status: http.StatusConflict, isFn: provider.IsConflict, name: "ErrConflict"},
		{status: http.StatusUnauthorized, isFn: provider.IsAuthentication, name: "ErrAuthentication"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("HTTP_%d_%s", tt.status, tt.name), func(t *testing.T) {
			srv := newStatusServer(t, tt.status)
			defer srv.Close()
			p := newTestProvider(t, srv)

			_, err := p.GetIssue(context.Background(), "owner", "repo", "7")
			require.Error(t, err)
			assert.True(t, tt.isFn(err), "HTTP %d 必须可判 %s(实际=%v)", tt.status, tt.name, err)
			var pe *provider.ProviderError
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, tt.status, pe.StatusCode)
		})
	}
}

// TestErrorClassification_AcrossMethods 404 在多个此前裸 Wrap 的方法上可判。
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
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			err := c.call()
			require.Error(t, err, "%s 在 404 下必须报错", c.name)
			assert.True(t, provider.IsNotFound(err), "%s 的 404 必须归类 ErrNotFound(实际=%v)", c.name, err)
		})
	}
}
