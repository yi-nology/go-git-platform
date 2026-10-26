package gitee_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/gitee"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitee_GetArchive_ReturnsBytes 归档下载必须真的拿到字节。
//
// 回归背景(P0):go-gitee 的 Client.Do 内部 `defer resp.Body.Close()`
// ⇒ SDK 的 DownloadTarball/DownloadZipball 返回时 body 已关闭,
// 原实现随后读它 ⇒ 每次都是 "http: read on closed response body"
// ⇒ 归档下载 100% 失败。
//
// 这条用例是**反向验证**:改回走 SDK 的 DownloadZipball,本用例会变红。
func TestGitee_GetArchive_ReturnsBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/repos/o/r/zipball") {
			t.Errorf("归档路径不对:%s", r.URL.Path)
		}
		if got := r.URL.Query().Get("ref"); got != "v1.0.0" {
			t.Errorf("ref 参数不对:%q", got)
		}
		_, _ = w.Write([]byte("PK\x03\x04fake-zip"))
	}))
	defer srv.Close()

	p, err := gitee.New(provider.Config{BaseURL: srv.URL, Token: "secret-token"})
	if err != nil {
		t.Fatalf("gitee.New: %v", err)
	}

	got, err := p.GetArchive(context.Background(), "o", "r", "v1.0.0", "zip")
	if err != nil {
		t.Fatalf("GetArchive: %v", err)
	}
	if string(got) != "PK\x03\x04fake-zip" {
		t.Fatalf("归档字节不对:%q", got)
	}
}

// TestGitee_GetArchive_TarballPath tar.gz 必须打 tarball 端点。
func TestGitee_GetArchive_TarballPath(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte("tarball-bytes"))
	}))
	defer srv.Close()

	p, err := gitee.New(provider.Config{BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("gitee.New: %v", err)
	}
	got, err := p.GetArchive(context.Background(), "o", "r", "main", "tar.gz")
	if err != nil {
		t.Fatalf("GetArchive: %v", err)
	}
	if !strings.HasSuffix(path, "/tarball") {
		t.Errorf("tar.gz 必须走 tarball 端点,实际=%s", path)
	}
	if string(got) != "tarball-bytes" {
		t.Errorf("归档字节不对:%q", got)
	}
}

// TestGitee_GetArchive_CrossHostRedirect_NoTokenLeak 302 到第三方主机时
// **不得**把平台令牌带走 —— 头和 query 都不行。
//
// 回归背景(P0):go-gitee 的 tokenTransport 把令牌放在 `access_token` query
// 参数里,且**每一跳都重新加**(它包在 http.Client 的 Transport 外层)
// ⇒ 平台 302 到 CDN/对象存储时令牌会跟着 query 发给第三方。
// Go 的自动剥除只覆盖标准头,管不到 query。
func TestGitee_GetArchive_CrossHostRedirect_NoTokenLeak(t *testing.T) {
	var cdnQuery url.Values
	var cdnAuth string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnQuery = r.URL.Query()
		cdnAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("cdn-archive-bytes"))
	}))
	defer cdn.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cdn.URL+"/blob", http.StatusFound)
	}))
	defer api.Close()

	p, err := gitee.New(provider.Config{BaseURL: api.URL, Token: "super-secret"})
	if err != nil {
		t.Fatalf("gitee.New: %v", err)
	}

	got, err := p.GetArchive(context.Background(), "o", "r", "main", "zip")
	if err != nil {
		t.Fatalf("GetArchive: %v", err)
	}
	if string(got) != "cdn-archive-bytes" {
		t.Fatalf("重定向后的归档字节不对:%q", got)
	}
	if cdnQuery.Has("access_token") {
		t.Errorf("access_token 泄漏到第三方主机:query=%v", cdnQuery)
	}
	if cdnAuth != "" {
		t.Errorf("Authorization 泄漏到第三方主机:%q", cdnAuth)
	}
}
