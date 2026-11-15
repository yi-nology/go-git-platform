package gitlab_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestGetCRDiffWalksAllPages 复刻 issue #44 的 GitLab 形态：diffs 端点是
// offset 分页、服务端 per_page 缺省 20——此前 nil options 只取第一页，
// >20 个变更文件的 MR 静默截断。修复后显式 per_page=100 且全页行走
// （page1 满 100 + page2 短 5 ⇒ 全量 105）。
func TestGetCRDiffWalksAllPages(t *testing.T) {
	const perPage = 100
	var mu sync.Mutex
	var seen []string // "page=X&per_page=Y"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SDK 会把 o%2Fr 解码成 o/r 送达服务端
		if r.Method != http.MethodGet || r.URL.Path != "/api/v4/projects/o/r/merge_requests/7/diffs" {
			t.Errorf("unexpected wire: %s %s", r.Method, r.URL.Path)
		}
		mu.Lock()
		seen = append(seen, "page="+r.URL.Query().Get("page")+"&per_page="+r.URL.Query().Get("per_page"))
		mu.Unlock()
		page := r.URL.Query().Get("page")
		n := 0
		switch page {
		case "1":
			n = perPage
		case "2":
			n = 5
		}
		out := make([]map[string]any, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, map[string]any{
				"old_path": fmt.Sprintf("p%s-%d", page, i),
				"new_path": fmt.Sprintf("p%s-%d", page, i),
				"diff":     "@@ -1 +1 @@\n+a\n",
				"new_file": true,
			})
		}
		w.Header().Set("X-Total", fmt.Sprint(perPage+5))
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	diff, err := p.GetCRDiff(context.Background(), "o", "r", "7")
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Files) != perPage+5 {
		t.Fatalf("files = %d, want %d（两页全量，服务端缺省 20 不得截断）", len(diff.Files), perPage+5)
	}
	mu.Lock()
	defer mu.Unlock()
	// 「空页即尾」收口：短页后多一次空页探测是行走器既定约定
	if len(seen) != 3 {
		t.Fatalf("应发 3 次列表请求（含末尾空页探测）, got %v", seen)
	}
	if seen[0] != "page=1&per_page=100" {
		t.Fatalf("第一页必须显式 per_page=100, got %q", seen[0])
	}
}
