package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// issue #77：CompareCommits 的 commits 必须走分页拉全——GitHub compare 端点
// 单次至多 250 条，旧实现传 nil ListOptions 只取一页 ⇒ >250 提交静默截断。
// files 平台上限 300（仅首页返回、无分页线），len==300 时必须置
// FilesTruncated，截断可见。
//
// 反向验证：退回单页调用，本用例变红（commits=250 < total=300，且
// FilesTruncated 恒 false）。
func TestGitHub_CompareCommits_PaginatesCommitsAndFlagsFileTruncation(t *testing.T) {
	const (
		totalCommits = 300 // 300 = 250 + 50：恰好两页（短页终止）
	)
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/compare/") {
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		if perPage != 250 {
			t.Errorf("compare 分页 per_page 应为 250（平台单次上限）, got %d", perPage)
		}
		pages = append(pages, fmt.Sprintf("page=%d", page))
		start := (page-1)*perPage + 1
		end := start + perPage - 1
		if end > totalCommits {
			end = totalCommits
		}
		commits := make([]map[string]any, 0, end-start+1)
		for i := start; i <= end; i++ {
			commits = append(commits, map[string]any{
				"sha":     fmt.Sprintf("c%04d", i),
				"message": fmt.Sprintf("commit %d", i),
				"commit":  map[string]any{"message": fmt.Sprintf("commit %d", i)},
			})
		}
		files := make([]map[string]any, 0, 300)
		for i := 0; i < 300; i++ {
			files = append(files, map[string]any{
				"filename":          fmt.Sprintf("f%03d.go", i),
				"status":            "modified",
				"additions":         1,
				"deletions":         0,
				"previous_filename": fmt.Sprintf("old%03d.go", i),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_commits": totalCommits,
			"ahead_by":      totalCommits,
			"behind_by":     0,
			"commits":       commits,
			"files":         files,
		})
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitHub, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	cm := p.(provider.CommitManager)

	result, err := cm.CompareCommits(context.Background(), "owner", "repo", "main", "feature")
	if err != nil {
		t.Fatalf("CompareCommits: %v", err)
	}
	if len(pages) != 2 {
		t.Errorf("expected 2 compare GETs (page=1, page=2), got %v", pages)
	}
	if result.TotalCommits != totalCommits {
		t.Errorf("TotalCommits = %d, want %d", result.TotalCommits, totalCommits)
	}
	if len(result.Commits) != totalCommits {
		t.Errorf("commits = %d, want %d — 单页静默截断回归（issue #77）", len(result.Commits), totalCommits)
	}
	if !result.FilesTruncated {
		t.Error("len(files)==300（平台上限）必须置 FilesTruncated，截断必须可见（issue #77）")
	}
	if len(result.Files) != 300 {
		t.Errorf("files = %d, want 300", len(result.Files))
	}
}

// 对照：远小于平台上限的 compare——FilesTruncated 必须保持 false，且单页
// 短页即终止（不多打一页空请求）。
func TestGitHub_CompareCommits_SmallCompareStaysExact(t *testing.T) {
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pages = append(pages, fmt.Sprintf("page=%d", page))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_commits": 2,
			"ahead_by":      2,
			"behind_by":     0,
			"commits": []map[string]any{
				{"sha": "aaa", "message": "one"},
				{"sha": "bbb", "message": "two"},
			},
			"files": []map[string]any{
				{"filename": "a.go", "status": "modified", "additions": 1, "deletions": 0},
			},
		})
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitHub, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	cm := p.(provider.CommitManager)

	result, err := cm.CompareCommits(context.Background(), "owner", "repo", "main", "feature")
	if err != nil {
		t.Fatalf("CompareCommits: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("small compare must terminate on the short page (1 GET), got %v", pages)
	}
	if len(result.Commits) != 2 || len(result.Files) != 1 {
		t.Fatalf("commits=%d files=%d, want 2/1", len(result.Commits), len(result.Files))
	}
	if result.FilesTruncated {
		t.Error("2 条改动不构成截断，FilesTruncated 必须为 false")
	}
}
