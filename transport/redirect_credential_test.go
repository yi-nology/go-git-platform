package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// redirectProbe 记录重定向目标收到的请求头。
type redirectProbe struct {
	mu      sync.Mutex
	auth    string
	private string
	calls   int
}

func (p *redirectProbe) record(r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.auth = r.Header.Get("Authorization")
	p.private = r.Header.Get("PRIVATE-TOKEN")
}

func (p *redirectProbe) snapshot() (string, string, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.auth, p.private, p.calls
}

// TestClient_CrossHostRedirect_DoesNotLeakCredentials 跨 host 重定向**不得**带走凭据。
//
// 回归背景(P0,凭据外泄到平台侧可控的第三方主机):
// 认证是在 RoundTripper **内部**施加的(每一跳都重新加一次),
// 而 Go 的 net/http 只对"请求头已存在"的情况做跨 host 剥离
// (Authorization / Cookie / Www-Authenticate / Cookie2)
// ⇒ 平台 302 到第三方主机时,令牌会跟着走。
// 而 PRIVATE-TOKEN 这类**非标准头 Go 本来就不剥**。
//
// 实测形态:GitLab 的 archive 端点 302 到对象存储 ⇒ 目标主机收到 PRIVATE-TOKEN。
//
// 这条用例是**反向验证**:把 applyAuthFor 里的跨 host 拦截去掉,本用例变红。
func TestClient_CrossHostRedirect_DoesNotLeakCredentials(t *testing.T) {
	probe := &redirectProbe{}

	// 目标主机(与源主机不同端口 ⇒ 不同 host)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probe.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	// 源主机:302 → 目标主机
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/cdn/blob", http.StatusFound)
	}))
	defer source.Close()

	c := NewClient(source.URL, BearerToken{Token: "SUPER-SECRET-TOKEN"})
	// 真实链路:http.Client + 本 transport 的 RoundTripper(与 SDKHTTPClient 交给
	// 平台 SDK 的形状一致)—— transport.Client.Do 自己不跟随重定向,走那条路测不到。
	httpCli := &http.Client{Transport: c.NewRetryingRoundTripper()}

	req, err := http.NewRequest(http.MethodGet, source.URL+"/api/resource", nil)
	require.NoError(t, err)
	resp, err := httpCli.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	auth, private, calls := probe.snapshot()
	require.Equal(t, 1, calls, "重定向应被跟随一次")
	assert.Empty(t, auth, "跨 host 重定向不得带走 Authorization(令牌会落到平台侧可控的主机)")
	assert.Empty(t, private, "跨 host 重定向不得带走 PRIVATE-TOKEN")
}

// TestClient_CrossHostRedirect_PrivateTokenNotLeaked PRIVATE-TOKEN 同样不能外泄。
func TestClient_CrossHostRedirect_PrivateTokenNotLeaked(t *testing.T) {
	probe := &redirectProbe{}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probe.record(r)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/x", http.StatusFound)
	}))
	defer source.Close()

	c := NewClient(source.URL, PrivateToken{Token: "GLPAT-SUPER-SECRET"})
	httpCli := &http.Client{Transport: c.NewRetryingRoundTripper()}

	req, err := http.NewRequest(http.MethodGet, source.URL+"/projects/o/r/repository/archive.tar.gz", nil)
	require.NoError(t, err)
	resp, err := httpCli.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	auth, private, _ := probe.snapshot()
	assert.Empty(t, private, "PRIVATE-TOKEN 不得随跨 host 重定向外泄")
	assert.Empty(t, auth)
}

// TestClient_SameHostRedirect_KeepsCredentials 同 host 重定向**要**继续带凭据。
//
// 否则会把正常 API 的 302(如 http→https、路径改写)也打断。
func TestClient_SameHostRedirect_KeepsCredentials(t *testing.T) {
	probe := &redirectProbe{}

	var mux http.ServeMux
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		probe.record(r)
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	c := NewClient(srv.URL, BearerToken{Token: "TOKEN-OK"})
	httpCli := &http.Client{Transport: c.NewRetryingRoundTripper()}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	require.NoError(t, err)
	resp, err := httpCli.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	auth, _, calls := probe.snapshot()
	require.Equal(t, 1, calls)
	assert.True(t, strings.Contains(auth, "TOKEN-OK"),
		"同 host 重定向必须继续带凭据(实际=%q)", auth)
}

// TestStripSensitiveHeaders 敏感头删除要覆盖非标准头。
func TestStripSensitiveHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com/x", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("PRIVATE-TOKEN", "y")
	req.Header.Set("X-Auth-Token", "z")
	req.Header.Set("X-Api-Key", "k")
	req.Header.Set("X-Request-Id", "keep-me")

	StripSensitiveHeaders(req)

	assert.Empty(t, req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("PRIVATE-TOKEN"))
	assert.Empty(t, req.Header.Get("X-Auth-Token"))
	assert.Empty(t, req.Header.Get("X-Api-Key"))
	assert.Equal(t, "keep-me", req.Header.Get("X-Request-Id"), "非敏感头必须保留")
}

// TestRedactURL_MasksUserInfo URL 里的 userinfo 密码不能进日志。
//
// 回归背景(P2,当前不可达但形状危险):`redactURL` 只处理 query ⇒
// `https://user:pass@host/` 的密码会原样进日志。仓内构造带 userinfo 的 URL 的
// 只有 `pkg/credential.BuildAuthURL`(暂无调用点)⇒ 这是"改一行就会泄漏"的形状。
func TestRedactURL_MasksUserInfo(t *testing.T) {
	u, err := http.NewRequest(http.MethodGet, "https://user:SECRET-PASS@host/path", nil)
	require.NoError(t, err)
	got := redactURL(*u.URL)
	assert.NotContains(t, got, "SECRET-PASS", "userinfo 里的密码必须被脱敏")
	assert.Contains(t, got, "host", "host 要保留")
}

// TestRedactPath_MasksSecretQuery 请求路径里的 query 秘密不能进日志。
//
// 回归背景(P2):`Request.Path` 可以自带 query 串 ⇒ 日志/错误信息里原样打出来。
func TestRedactPath_MasksSecretQuery(t *testing.T) {
	assert.Equal(t, "/api/v1/x", redactPath("/api/v1/x"))
	assert.Equal(t, "/api/v1/x?page=2", redactPath("/api/v1/x?page=2"))
	got := redactPath("/api/v1/x?access_token=SECRET&page=2")
	assert.NotContains(t, got, "SECRET", "query 里的 token 必须被脱敏")
	assert.Contains(t, got, "page=2", "非秘密参数要保留")
}
