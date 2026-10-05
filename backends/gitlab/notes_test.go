package gitlab_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gitlab "github.com/yi-nology/go-git-platform/backends/gitlab"
	"github.com/yi-nology/go-git-platform/provider"
)

// 编译期锚：gitlab Provider 实现 NoteManager（MR Notes API 语义）。
var _ provider.NoteManager = (*gitlab.Provider)(nil)

// v0.67.2: NoteManager targets the MergeRequest Notes API — IssueManager's
// comment methods go through the Issues Notes API and 404 for merge requests
// (MRs and issues live in separate iid namespaces). These tests pin both the
// URL shape and the paginated listing.

func TestGitLab_UpdateNote_HitsMRNotesEndpoint(t *testing.T) {
	var gotPath, gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":77,"body":"updated body"}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	c, err := p.UpdateNote(context.Background(), "grp", "proj", "234", "77", "updated body")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/merge_requests/234/notes/77") {
		t.Fatalf("应打 MR notes 端点，got %s", gotPath)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("应为 PUT，got %s", gotMethod)
	}
	if !strings.Contains(gotBody, "updated body") {
		t.Fatalf("请求体应携带新正文: %s", gotBody)
	}
	if c.ID != 77 || c.Body != "updated body" {
		t.Fatalf("返回评论不符: %+v", c)
	}
}

func TestGitLab_ListNotes_PaginatesMRNotes(t *testing.T) {
	mu := 0
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu++
		paths = append(paths, r.URL.Path+"?page="+r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		switch mu {
		case 1: // 第一页非满
			_, _ = w.Write([]byte(`[{"id":1,"body":"a"},{"id":2,"body":"b"}]`))
		case 2: // 第二页非空
			_, _ = w.Write([]byte(`[{"id":3,"body":"c"}]`))
		default: // 空页终止（AllPages 语义）
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	cs, err := p.ListNotes(context.Background(), "grp", "proj", "234")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 || cs[0].ID != 1 || cs[2].ID != 3 {
		t.Fatalf("分页聚合不符: got %d 条", len(cs))
	}
	if len(paths) < 3 {
		t.Fatalf("应发生翻页+空页终止: %v", paths)
	}
	for _, pa := range paths {
		if !strings.Contains(pa, "/merge_requests/234/notes") {
			t.Fatalf("应打 MR notes 端点: %s", pa)
		}
	}
}
