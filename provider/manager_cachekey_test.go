package provider_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/yi-nology/go-git-platform/backends/all"

	"github.com/yi-nology/go-git-platform/provider"
)

// cacheKeyHarness 起一个空 HTTP 服务器作 BaseURL(构造 provider 不发请求),
// 返回一个全新 Manager 供逐案例隔离计数。
func cacheKeyHarness(t *testing.T) (*httptest.Server, *provider.Manager) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, provider.NewManager(30 * time.Minute)
}

// baseCacheCfg 返回一个最小可用 Config;各案例在它之上只改一个字段。
func baseCacheCfg(baseURL string) provider.Config {
	return provider.Config{Platform: provider.PlatformGitHub, BaseURL: baseURL, Token: "tok"}
}

// TestManager_CacheKey_CoversAllWireAffectingFields 缓存键必须涵盖全部
// 影响 wire 行为的配置字段(issue #66 回归,探针实证的错凭据互撞面):
// 两个仅差一个字段的 Config **不得**命中同一个 provider——否则
// TokenSource 撞键 = 以错误主体的凭据静默读写;TokenStyle/
// ConditionalRequests/RetryConfig 差异 = 认证头样式/ETag 缓存/重试策略
// 静默错配。反向断言:同配置重复 Get 必须命中同一实例(缓存仍有效)。
func TestManager_CacheKey_CoversAllWireAffectingFields(t *testing.T) {
	// 旋转凭据形态的 TokenSource(指针身份参与键)。
	srcA := provider.StaticTokenSource("cred-A")

	type delta struct {
		name string
		mut  func(*provider.Config)
	}
	deltas := []delta{
		{"TokenStyle", func(c *provider.Config) { c.TokenStyle = "private" }},
		{"ConditionalRequests", func(c *provider.Config) { c.ConditionalRequests = true }},
		{"TokenSource", func(c *provider.Config) { c.TokenSource = srcA }},
		{"RetryConfig", func(c *provider.Config) {
			rc := provider.RetryConfig{MaxRetries: 5, BaseDelay: time.Second, RetryOn: []int{409}}
			c.RetryConfig = &rc
		}},
		{"Hooks", func(c *provider.Config) { c.Hooks = &provider.Hooks{} }},
		{"Logger", func(c *provider.Config) { c.Logger = provider.NewNoopLogger() }},
	}

	for _, d := range deltas {
		t.Run(d.name, func(t *testing.T) {
			// 案例内自建服务器与 Manager,各案例隔离计数。
			s2, mgr := cacheKeyHarness(t)
			base := baseCacheCfg(s2.URL)
			p1, err := mgr.Get(base)
			require.NoError(t, err)

			alt := baseCacheCfg(s2.URL)
			d.mut(&alt)
			p2, err := mgr.Get(alt)
			require.NoError(t, err)

			assert.NotSame(t, p1, p2, "仅差字段 %s 的两个 Config 不得共享缓存 provider", d.name)
			assert.EqualValues(t, 2, mgr.Len(), "仅差字段 %s 应产生两个缓存条目", d.name)
		})
	}
}

// TestManager_CacheKey_EqualValuesShareCache 值语义配置的等值共享:
// 相同内容的 StaticTokenSource / RetryConfig 是等值配置,必须命中同一
// provider(键补全不能把缓存打碎)。
func TestManager_CacheKey_EqualValuesShareCache(t *testing.T) {
	srv, m := cacheKeyHarness(t)

	t.Run("equal_static_token_source", func(t *testing.T) {
		a := baseCacheCfg(srv.URL)
		a.TokenSource = provider.StaticTokenSource("same-cred")
		b := baseCacheCfg(srv.URL)
		b.TokenSource = provider.StaticTokenSource("same-cred")
		pa, err := m.Get(a)
		require.NoError(t, err)
		pb, err := m.Get(b)
		require.NoError(t, err)
		assert.Same(t, pa, pb, "等值 TokenSource 是同一配置,应命中缓存")
	})

	t.Run("equal_retry_config", func(t *testing.T) {
		a := baseCacheCfg(srv.URL)
		a.RetryConfig = &provider.RetryConfig{MaxRetries: 3, BaseDelay: 500 * time.Millisecond}
		b := baseCacheCfg(srv.URL)
		b.RetryConfig = &provider.RetryConfig{MaxRetries: 3, BaseDelay: 500 * time.Millisecond}
		pa, err := m.Get(a)
		require.NoError(t, err)
		pb, err := m.Get(b)
		require.NoError(t, err)
		assert.Same(t, pa, pb, "等值 RetryConfig 是同一策略,应命中缓存")
	})
}

// TestManager_CacheKey_TokenSourceBeatsToken TokenSource 优先于 Token:
// 相同 Token 不同 TokenSource(以及反之)都不得互撞。
func TestManager_CacheKey_TokenSourceBeatsToken(t *testing.T) {
	srv, m := cacheKeyHarness(t)

	withSrc := baseCacheCfg(srv.URL)
	withSrc.Token = ""
	withSrc.TokenSource = provider.StaticTokenSource("rotating-cred")
	p1, err := m.Get(withSrc)
	require.NoError(t, err)

	plainToken := baseCacheCfg(srv.URL)
	plainToken.Token = "rotating-cred" // 值相同,但通道不同(静态 vs source)
	p2, err := m.Get(plainToken)
	require.NoError(t, err)
	assert.NotSame(t, p1, p2, "TokenSource 与静态 Token 通道不得共享 provider")
}

// TestManager_CacheKey_SameConfigStillHits 键补全后同配置必须仍命中缓存
// (回归保护:修复不得把正常复用打碎)。
func TestManager_CacheKey_SameConfigStillHits(t *testing.T) {
	srv, m := cacheKeyHarness(t)
	cfg := baseCacheCfg(srv.URL)
	p1, err := m.Get(cfg)
	require.NoError(t, err)
	p2, err := m.Get(cfg)
	require.NoError(t, err)
	assert.Same(t, p1, p2)
	stats := m.Stats()
	assert.EqualValues(t, 1, stats.Hits)
	assert.EqualValues(t, 1, stats.Misses)
}
