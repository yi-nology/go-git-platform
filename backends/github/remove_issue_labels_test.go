package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestRemoveIssueLabels_StopsAtFirstError 钉 #163 批量形的部分失败语义：
// 首个 label 的 DELETE 失败后立即返回错误，后续 label 不得再发请求
// （已移除的不回滚、首错即停——契约见 provider.IfaceIssues 注释）。
func TestRemoveIssueLabels_StopsAtFirstError(t *testing.T) {
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			if deletes.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitHub, BaseURL: srv.URL, Token: "t",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	im, ok := p.(provider.IssueManager)
	if !ok {
		t.Fatal("github provider must implement provider.IssueManager")
	}

	err = im.RemoveIssueLabels(context.Background(), "owner", "repo", "1", []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("want error from first failing delete, got nil")
	}
	if got := deletes.Load(); got != 1 {
		t.Fatalf("want exactly 1 DELETE (first error stops the loop), got %d", got)
	}
}

// TestRemoveIssueLabels_EmptySendsNothing 钉空列表零请求。
func TestRemoveIssueLabels_EmptySendsNothing(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitHub, BaseURL: srv.URL, Token: "t",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	im := p.(provider.IssueManager)
	if err := im.RemoveIssueLabels(context.Background(), "owner", "repo", "1", nil); err != nil {
		t.Fatalf("empty bulk: %v", err)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("empty RemoveIssueLabels must send zero requests, got %d", got)
	}
}
