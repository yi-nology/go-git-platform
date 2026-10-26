package gitee_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/gitee"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitee_GetFileContent_DecodesBase64 文件内容必须按 Encoding 解码。
//
// 回归背景(P1):Gitee 的内容端点返回 base64,后端原样返回 ⇒ 调用方拿到的是
// base64 串(按 GitHub 直觉读文件全错:解析 YAML/JSON 直接失败)。
//
// 这条用例是**反向验证**:去掉解码,本用例会变红。
func TestGitee_GetFileContent_DecodesBase64(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Gitee 的内容端点返回 JSON 数组,content 是 base64
		_, _ = w.Write([]byte(`[{"type":"file","encoding":"base64","content":"aGVsbG8K","name":"a.txt","path":"a.txt"}]`))
	}))
	defer srv.Close()

	p, err := gitee.New(provider.Config{BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("gitee.New: %v", err)
	}

	got, err := p.GetFileContent(context.Background(), "o", "r", "a.txt", "main")
	if err != nil {
		t.Fatalf("GetFileContent: %v", err)
	}
	if got != "hello\n" {
		t.Errorf("base64 内容必须解码,实际=%q", got)
	}
}
