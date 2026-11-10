package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oversizedBody generates a body of n bytes ('a' repeated).
func oversizedBody(n int) string { return strings.Repeat("a", n) }

// TestClient_DefaultMaxBodySizeApplies 缺省构造的 Client 就有 10MB 上限
// (issue #70 回归:修复前缺省 0=无限制,文档宣称的「默认 10MB」实际不存在)。
func TestClient_DefaultMaxBodySizeApplies(t *testing.T) {
	body := oversizedBody(int(DefaultMaxBodySize) + 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, None{})
	_, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/big"})
	require.Error(t, err, "超过缺省上限的响应体必须报错")
	assert.Contains(t, err.Error(), "maximum size", "错误要指明是尺寸上限")
}

// TestClient_MaxBodySizeOptions 表格化钉死 WithMaxBodySize 三种取值语义:
// 0=显式无限制、-1=缺省上限(10MB)、>0=显式上限。响应体统一给
// DefaultMaxBodySize+1 字节:无限制的取值要原样通过,其余三种全部撞限。
func TestClient_MaxBodySizeOptions(t *testing.T) {
	const explicitLimit = 64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(oversizedBody(int(DefaultMaxBodySize) + 1)))
	}))
	defer srv.Close()

	tests := []struct {
		name    string
		option  func(c *Client)
		wantErr bool
	}{
		{
			name:    "zero_means_unlimited",
			option:  func(c *Client) { c.maxBodySize = 0 },
			wantErr: false,
		},
		{
			name:    "negative_means_default_limit",
			option:  func(c *Client) { c.maxBodySize = -1 },
			wantErr: true,
		},
		{
			name:    "explicit_limit",
			option:  func(c *Client) { c.maxBodySize = explicitLimit },
			wantErr: true,
		},
		{
			name:    "constructor_default_applies_limit",
			option:  func(c *Client) {},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(srv.URL, None{})
			tt.option(c)
			_, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/big"})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestClient_MaxBodySizeUnderLimit 限制内的响应体原样通过。
func TestClient_MaxBodySizeUnderLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("exactly-fit"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, None{}, WithMaxBodySize(int64(len("exactly-fit"))))
	resp, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/small"})
	require.NoError(t, err)
	assert.Equal(t, "exactly-fit", string(resp.Body))
}
