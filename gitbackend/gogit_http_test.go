package gitbackend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	xclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	xhttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/require"
)

// TestGitHTTPTransport_BoundsStalledResponse 钉住"上游连上但不回"能被有界地打断。
//
// 回归背景:go-git 默认传输没有响应头超时 ⇒ 连接一直 ESTABLISHED,
// git 操作无限期挂着(实测:GitHub 间歇性不可达时一次克隆卡两分钟)。
// 装了带超时的传输后,必须在超时窗口内明确失败。
func TestGitHTTPTransport_BoundsStalledResponse(t *testing.T) {
	// 一个"接受连接但永不回响应"的服务
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	// 用 1 秒的响应头超时(生产默认 60s,测试里等太久)
	tr := newGitHTTPTransport(2*time.Second, 1*time.Second, 2*time.Second)
	xclient.InstallProtocol("http", xhttp.NewClient(&http.Client{Transport: tr}))

	cl, err := xclient.NewClient(&transport.Endpoint{
		Protocol: "http",
		Host:     srv.Listener.Addr().String(),
		Path:     "/",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	start := time.Now()
	sess, err := cl.NewUploadPackSession(&transport.Endpoint{
		Protocol: "http",
		Host:     srv.Listener.Addr().String(),
		Path:     "/",
	}, nil)
	if err == nil {
		_, err = sess.AdvertisedReferencesContext(ctx)
		_ = sess.Close()
	}
	elapsed := time.Since(start)

	require.Error(t, err, "上游不回响应头时必须报错,而不是一直挂着")
	require.Less(t, elapsed, 10*time.Second,
		"必须在超时窗口内失败(实际耗时 %s)", elapsed)
}

// TestGitHTTPTransport_DefaultsAreBounded 默认超时必须是**有界**的:
// 无界的超时等于没有超时(挂死只能等调用方的 context 兜底)。
func TestGitHTTPTransport_DefaultsAreBounded(t *testing.T) {
	require.Positive(t, GitHTTPDialTimeout, "拨号超时要大于 0")
	require.Positive(t, GitHTTPResponseHeaderTimeout, "响应头超时要大于 0")
	require.Positive(t, GitHTTPTLSHandshakeTimeout, "TLS 握手超时要大于 0")
	require.Less(t, GitHTTPDialTimeout, 2*time.Minute, "拨号超时不该长到跟操作系统默认一个量级")
	require.Less(t, GitHTTPResponseHeaderTimeout, 5*time.Minute, "响应头超时要能兜住上游卡死")
}

// TestGitHTTPTransport_KeepsInsecureSkipTLS 装了自定义传输**不能**破坏
// endpoint 级的 InsecureSkipTLS(自建实例常见自签/过期证书,靠它才能连)。
func TestGitHTTPTransport_KeepsInsecureSkipTLS(t *testing.T) {
	tr := newGitHTTPTransport(time.Second, time.Second, time.Second)
	// go-git 会在 Clone 出来的 transport 上叠加 endpoint 级设置;
	// 这里断言我们给的 Transport 是可 Clone 的具体类型(否则 go-git 直接报错)。
	cloned := tr.Clone()
	require.Equal(t, tr.ResponseHeaderTimeout, cloned.ResponseHeaderTimeout,
		"Clone 必须保留超时设置")
	require.Equal(t, tr.DialContext != nil, cloned.DialContext != nil, "Clone 必须保留拨号器")
}
