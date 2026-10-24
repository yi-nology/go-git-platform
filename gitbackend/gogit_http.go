package gitbackend

import (
	"net"
	"net/http"
	"sync"
	"time"

	xclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	xhttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Git 传输的默认超时。可按需调整（进程级）。
var (
	// GitHTTPDialTimeout 是 TCP 连接超时。
	//
	// 没有它时,上游"地址不可达"会一直卡到操作系统的 TCP 重试超时(常见 ~2 分钟),
	// 才以 "dial tcp ... i/o timeout" 结束 —— 一次失败拖两分钟。
	GitHTTPDialTimeout = 30 * time.Second

	// GitHTTPResponseHeaderTimeout 是"已连上、等响应头"的超时。
	//
	// 覆盖另一半挂死形态:连接建立成功但服务端不回(过载/限流/中间设备吞包),
	// 此时连接一直 ESTABLISHED,既不报错也不结束。
	GitHTTPResponseHeaderTimeout = 60 * time.Second

	// GitHTTPTLSHandshakeTimeout 是 TLS 握手超时。
	GitHTTPTLSHandshakeTimeout = 15 * time.Second
)

var installGitHTTPTimeoutsOnce sync.Once

// newGitHTTPTransport 构造带超时的 HTTP 传输。
//
// 只加**拨号超时**与**响应头超时**,**不设 http.Client.Timeout(总超时)**:
// 大仓克隆的响应体可以流很久,设总超时会把正常的大克隆打断。
//
// 实现注意:go-git 的 newSession 要求装好的 client 的 Transport 必须是
// `*http.Transport`(它要 Clone 后再叠加 endpoint 级设置,比如 InsecureSkipTLS)
// ⇒ 这里的 Transport 必须是具体类型,不能是包装器。
// 叠加逻辑在 go-git 侧(transport.Clone + configureTransport),因此
// **skip-TLS 等 endpoint 级能力不受影响**,超时也会被 Clone 保留。
func newGitHTTPTransport(dial, header, handshake time.Duration) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dial,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   handshake,
		ResponseHeaderTimeout: header,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}
}

// installGitHTTPTimeouts 给 go-git 的 http/https 传输装上超时。
//
// 背景:go-git 的默认 client 用 `http.DefaultTransport` —— **没有拨号超时、
// 没有响应头超时**。于是"上游连不上/连上不回"时,git 操作会一直挂着,
// 调用方只能等自己 context 的超时兜底。实测:GitHub 间歇性不可达时,
// 一次克隆卡到操作系统默认的 TCP 重试超时(约 2 分钟)才失败,
// 期间连接一直是 ESTABLISHED。
//
// go-git 的协议注册是**进程级**的,所以用 sync.Once 只装一次。
func installGitHTTPTimeouts() {
	installGitHTTPTimeoutsOnce.Do(func() {
		tr := newGitHTTPTransport(GitHTTPDialTimeout, GitHTTPResponseHeaderTimeout, GitHTTPTLSHandshakeTimeout)
		c := &http.Client{Transport: tr}
		xclient.InstallProtocol("http", xhttp.NewClient(c))
		xclient.InstallProtocol("https", xhttp.NewClient(c))
	})
}
