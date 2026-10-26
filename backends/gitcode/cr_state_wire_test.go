package gitcode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitCode_ListCRs_ZeroStateSendsNoFilter 零值 State ⇒ **不发** state 过滤。
//
// 回归背景(P1):GitCode 曾把零值状态映射成 "open" ⇒ 调用方只想"列出 CR"时,
// closed/merged 的 PR **静默丢失**(GitHub / GitLab / 工蜂 都是"空=不过滤")。
//
// 这条用例是**反向验证**:把零值又映射回 "open",本用例会变红。
func TestGitCode_ListCRs_ZeroStateSendsNoFilter(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Query().Get("state"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitCode, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	cm := p.(provider.ChangeRequestManager)

	// 零值 State ⇒ 不加过滤
	if _, _, err := cm.ListCRs(context.Background(), provider.ListCROptions{Owner: "o", Repo: "r"}); err != nil {
		t.Fatalf("ListCRs: %v", err)
	}
	// 显式 opened ⇒ state=open
	if _, _, err := cm.ListCRs(context.Background(), provider.ListCROptions{Owner: "o", Repo: "r", State: provider.CRStateOpened}); err != nil {
		t.Fatalf("ListCRs: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 requests, got %v", got)
	}
	if got[0] != "" {
		t.Errorf("零值 State 必须不过滤(=全部),实际 state=%q", got[0])
	}
	if got[1] != "open" {
		t.Errorf("opened 必须映射成 open,实际 state=%q", got[1])
	}
}

// TestGitCode_GetFileContent_DecodesBase64 文件内容必须按 Encoding 解码。
//
// 回归背景(P1):GitCode 的内容端点返回 base64,后端原样返回 ⇒ 调用方拿到的是
// base64 串(按 GitHub 直觉读文件全错)。
//
// 这条用例是**反向验证**:去掉解码,本用例会变红。
func TestGitCode_GetFileContent_DecodesBase64(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// GitCode 的内容端点返回 JSON,content 是 base64
		_, _ = w.Write([]byte(`{"type":"file","encoding":"base64","content":"aGVsbG8K","name":"a.txt","path":"a.txt"}`))
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitCode, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	fm := p.(provider.FileManager)

	got, err := fm.GetFileContent(context.Background(), "o", "r", "a.txt", "main")
	if err != nil {
		t.Fatalf("GetFileContent: %v", err)
	}
	if got != "hello\n" {
		t.Errorf("base64 内容必须解码,实际=%q", got)
	}
}
