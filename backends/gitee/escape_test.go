package gitee_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/gitee"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitee_PathEscaping verifies that owner/repo/label-name/file-path
// segments are percent-encoded on the wire and decode back to the original
// values server-side. Characters like '#', '?', '%', spaces, and non-ASCII
// would otherwise corrupt or truncate the URL.
func TestGitee_PathEscaping(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var refs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		refs = append(refs, r.URL.Query().Get("ref"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
		} else {
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	p, err := gitee.New(provider.Config{BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("gitee.New: %v", err)
	}
	ctx := context.Background()
	const (
		owner = "ow#ner"
		repo  = "re?po"
	)
	_, _ = p.ListBranches(ctx, owner, repo)
	_ = p.(provider.LabelManager).DeleteLabel(ctx, owner, repo, "la bel%1中文")
	_, _ = p.GetFileContent(ctx, owner, repo, "dir one/file#2.txt", "bra nch")

	if len(paths) != 3 {
		t.Fatalf("expected 3 recorded requests, got %d: %v", len(paths), paths)
	}
	for _, path := range paths {
		if !strings.Contains(path, owner) {
			t.Errorf("path %q lost owner %q (segment not escaped)", path, owner)
		}
		if !strings.Contains(path, repo) {
			t.Errorf("path %q lost repo %q (segment not escaped)", path, repo)
		}
	}
	if !strings.Contains(paths[1], "la bel%1中文") {
		t.Errorf("path %q lost label name (segment not escaped)", paths[1])
	}
	if !strings.Contains(paths[2], "dir one/file#2.txt") {
		t.Errorf("path %q lost file path (segments not escaped; '/' must survive)", paths[2])
	}
	if refs[2] != "bra nch" {
		t.Errorf("ref query decoded to %q, want %q (ref not query-escaped)", refs[2], "bra nch")
	}
}

// TestGitee_QueryValueEscaping verifies that string query values appended to
// list endpoints (source/target branch via the SDK's head/base params, sha,
// since, until) are query-escaped and decode back to the original values
// server-side.
func TestGitee_QueryValueEscaping(t *testing.T) {
	var mu sync.Mutex
	var queries []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.Query())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p, err := gitee.New(provider.Config{BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("gitee.New: %v", err)
	}
	ctx := context.Background()
	_, _, _ = p.ListCRs(ctx, provider.ListCROptions{
		Owner: "owner", Repo: "repo",
		SourceBranch: "fea ture#1", TargetBranch: "mai?n",
	})
	_, _ = p.ListCommits(ctx, "owner", "repo", provider.ListCommitsOptions{
		Branch: "bra nch", Since: "2026-01-01T00:00:00+08:00", Until: "2026-06-01T00:00:00Z",
	})

	if len(queries) != 2 {
		t.Fatalf("expected 2 recorded requests, got %d", len(queries))
	}
	if got := queries[0].Get("head"); got != "fea ture#1" {
		t.Errorf("head decoded to %q, want %q", got, "fea ture#1")
	}
	if got := queries[0].Get("base"); got != "mai?n" {
		t.Errorf("base decoded to %q, want %q", got, "mai?n")
	}
	if got := queries[1].Get("sha"); got != "bra nch" {
		t.Errorf("sha decoded to %q, want %q", got, "bra nch")
	}
	if got := queries[1].Get("since"); got != "2026-01-01T00:00:00+08:00" {
		t.Errorf("since decoded to %q, want RFC3339 passthrough", got)
	}
	if got := queries[1].Get("until"); got != "2026-06-01T00:00:00Z" {
		t.Errorf("until decoded to %q, want RFC3339 passthrough", got)
	}
}

// TestGitee_ListCRs_StateMapping verifies that the SDK CRState vocabulary is
// mapped to gitee's pull-list vocabulary (open/closed/merged):
// the SDK's "opened" must go out as "open", and an **empty state must go out
// as no filter at all** (契约:空 = 不过滤 = 全部)。
//
// 回归背景(P1):以前空状态被映射成 "open" ⇒ 调用方只想"列出 CR"时,
// closed/merged 的 **静默丢失**(GitHub / GitLab / 工蜂 都是"空=不过滤")。
func TestGitee_ListCRs_StateMapping(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Query().Get("state"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	p, err := gitee.New(provider.Config{BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("gitee.New: %v", err)
	}
	_, _, _ = p.ListCRs(context.Background(), provider.ListCROptions{Owner: "o", Repo: "r", State: provider.CRStateOpened})
	_, _, _ = p.ListCRs(context.Background(), provider.ListCROptions{Owner: "o", Repo: "r"})
	if len(got) != 2 || got[0] != "open" || got[1] != "" {
		t.Fatalf("expected state mapping [open \"\"(=不过滤)], got %v", got)
	}
}
