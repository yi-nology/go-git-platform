package gitee_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/backends/contracttest"

	"github.com/yi-nology/go-git-platform/backends/gitee"
	"github.com/yi-nology/go-git-platform/provider"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newTestProvider(t *testing.T, srv *httptest.Server) *gitee.Provider {
	t.Helper()
	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitee,
		BaseURL:  srv.URL,
		Token:    "test-token",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	gp, ok := p.(*gitee.Provider)
	if !ok {
		t.Fatalf("expected *gitee.Provider, got %T", p)
	}
	return gp
}

// TestBasePath_NoDoubledV5Prefix verifies the URL wiring after the go-gitee
// SDK migration: the SDK builds paths as BasePath + "/v5/..." itself, so the
// Provider must point the SDK at ".../api" (not ".../api/v5") while the raw
// client keeps the full "/api/v5" root. Against a bare server URL and against
// a cfg.BaseURL that already carries "/api/v5", every request must carry
// exactly one "/api/v5" prefix.
func TestBasePath_NoDoubledV5Prefix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL func(srvURL string) string
	}{
		{"bare server URL", func(srvURL string) string { return srvURL }},
		{"explicit api v5", func(srvURL string) string { return srvURL + "/api/v5" }},
		{"explicit api v5 trailing slash", func(srvURL string) string { return srvURL + "/api/v5/" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/branches") || strings.HasSuffix(r.URL.Path, "/user/repos") {
					// Pagination-aware: these list endpoints now walk all
					// pages via backendutil.AllPages, so page ≥ 2 must be
					// empty or the mock loops to the 50-page cap.
					if page := r.URL.Query().Get("page"); page != "" && page != "1" {
						_, _ = w.Write([]byte(`[]`))
						return
					}
					_, _ = w.Write([]byte(`[{"name":"main"}]`))
					return
				}
				_, _ = w.Write([]byte(`{"id":1,"name":"repo","full_name":"owner/repo","owner":{"login":"owner"}}`))
			}))
			defer srv.Close()

			p, err := provider.NewProvider(provider.Config{
				Platform: provider.PlatformGitee,
				BaseURL:  tc.baseURL(srv.URL),
				Token:    "test-token",
			})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			ctx := context.Background()
			if _, err := p.ListBranches(ctx, "owner", "repo"); err != nil {
				t.Fatalf("ListBranches (SDK): %v", err)
			}
			if _, err := p.GetRepo(ctx, "owner", "repo"); err != nil {
				t.Fatalf("GetRepo (SDK): %v", err)
			}
			if _, err := p.ListRepos(ctx, provider.ListRepoOptions{}); err != nil {
				t.Fatalf("ListRepos (raw): %v", err)
			}

			mu.Lock()
			defer mu.Unlock()
			// 5 requests: ListBranches and ListRepos (zero-page opts → full
			// fetch) each make a data page plus an empty terminal-page probe
			// via backendutil.AllPages, plus the single GetRepo.
			if len(paths) != 5 {
				t.Fatalf("expected 5 recorded requests, got %d: %v", len(paths), paths)
			}
			for _, path := range paths {
				if strings.Count(path, "/v5") != 1 {
					t.Errorf("path %q must contain exactly one /v5 segment", path)
				}
				if !strings.HasPrefix(path, "/api/v5/") {
					t.Errorf("path %q must start with /api/v5/", path)
				}
			}
		})
	}
}

func TestListRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]any{
			{"id": 1, "name": "r1", "full_name": "owner/r1",
				"owner": map[string]any{"login": "owner"}, "default_branch": "main"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1, got %d", len(repos))
	}
	if repos[0].Platform != provider.PlatformGitee {
		t.Errorf("expected Gitee, got %s", repos[0].Platform)
	}
}

func TestCreateCR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id": 7, "number": 7, "title": "test", "state": "open",
			"source_branch": "feature", "target_branch": "main",
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	cr, err := p.CreateCR(context.Background(), provider.CreateCROptions{
		Owner: "owner", Repo: "repo", Title: "test",
		SourceBranch: "feature", TargetBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cr.Number != "7" {
		t.Errorf("expected 7, got %q", cr.Number)
	}
}

// TestGetCR_Conversion locks the SDK-model conversion for a merged PR: the
// branch names come from head.ref/base.ref (the REST payload has no top-level
// source_branch/target_branch fields), SHAs from head.sha/base.sha, and the
// string "merged_at" timestamp drives the merged state.
func TestGetCR_Conversion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id": 15971649, "number": 7011, "title": "t", "body": "desc",
			"state": "merged", "html_url": "https://gitee.com/o/r/pulls/7011",
			"head":       map[string]any{"ref": "feature", "sha": "headsha"},
			"base":       map[string]any{"ref": "master", "sha": "basesha"},
			"user":       map[string]any{"id": 1, "login": "dev", "name": "Dev"},
			"labels":     []map[string]any{{"name": "bug"}},
			"assignees":  []map[string]any{{"id": 2, "login": "rev"}},
			"mergeable":  true,
			"created_at": "2026-01-02T03:04:05+08:00",
			"updated_at": "2026-01-03T03:04:05+08:00",
			"merged_at":  "2026-01-03T03:04:05+08:00",
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	cr, err := p.GetCR(context.Background(), "owner", "repo", "7011")
	if err != nil {
		t.Fatal(err)
	}
	if cr.Number != "7011" || cr.ID != 15971649 {
		t.Errorf("number/id: got %q/%d", cr.Number, cr.ID)
	}
	if cr.State != provider.CRStateMerged {
		t.Errorf("state: got %s, want merged", cr.State)
	}
	if cr.SourceBranch != "feature" || cr.TargetBranch != "master" {
		t.Errorf("branches: got %s...%s", cr.SourceBranch, cr.TargetBranch)
	}
	if cr.HeadSHA != "headsha" || cr.BaseSHA != "basesha" {
		t.Errorf("shas: got %s/%s", cr.HeadSHA, cr.BaseSHA)
	}
	if cr.Author == nil || cr.Author.Username != "dev" || cr.Author.Name != "Dev" {
		t.Errorf("author: got %+v", cr.Author)
	}
	if len(cr.Reviewers) != 1 || cr.Reviewers[0].Username != "rev" {
		t.Errorf("reviewers: got %+v", cr.Reviewers)
	}
	if len(cr.Labels) != 1 || cr.Labels[0] != "bug" {
		t.Errorf("labels: got %v", cr.Labels)
	}
	if cr.MergeStatus != "mergeable" {
		t.Errorf("merge status: got %s", cr.MergeStatus)
	}
	if cr.CreatedAt.IsZero() || cr.UpdatedAt.IsZero() {
		t.Errorf("timestamps not parsed: %v/%v", cr.CreatedAt, cr.UpdatedAt)
	}
}

// TestListCRs_SDKQueryAndTotal verifies the SDK list call sends the mapped
// state and head/base branch filters plus normalized pagination, and that the
// X-Total-Count response header is surfaced as the total.
func TestListCRs_SDKQueryAndTotal(t *testing.T) {
	type captured struct {
		method string
		path   string
		query  url.Values
	}
	var mu sync.Mutex
	var got []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, captured{r.Method, r.URL.Path, r.URL.Query()})
		mu.Unlock()
		w.Header().Set("X-Total-Count", "42")
		writeJSON(w, []map[string]any{})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	crs, total, err := p.ListCRs(context.Background(), provider.ListCROptions{
		Owner: "owner", Repo: "repo", State: provider.CRStateMerged,
		SourceBranch: "feat", TargetBranch: "main", Page: 2, PerPage: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(crs) != 0 || total != 42 {
		t.Errorf("expected 0 CRs / total 42, got %d/%d", len(crs), total)
	}
	if len(got) != 1 || got[0].path != "/api/v5/repos/owner/repo/pulls" {
		t.Fatalf("unexpected request: %+v", got)
	}
	q := got[0].query
	for key, want := range map[string]string{"state": "merged", "head": "feat", "base": "main", "page": "2", "per_page": "30"} {
		if got := q.Get(key); got != want {
			t.Errorf("query %s: got %q, want %q", key, got, want)
		}
	}
}

// TestCreateCR_BodyKeys verifies the create call forwards the SDK body-param
// vocabulary (head/base, comma-joined labels) rather than the legacy
// source_branch/target_branch keys.
func TestCreateCR_BodyKeys(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writeJSON(w, map[string]any{"id": 7, "number": 7, "title": "test", "state": "open"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	if _, err := p.CreateCR(context.Background(), provider.CreateCROptions{
		Owner: "owner", Repo: "repo", Title: "test",
		SourceBranch: "feature", TargetBranch: "main",
		Description: "d", Labels: []string{"a", "b"}, RemoveSourceBranch: true,
	}); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"title": "test", "head": "feature", "base": "main", "body": "d", "labels": "a,b", "prune_source_branch": true} {
		if got := body[key]; got != want {
			t.Errorf("body %s: got %v, want %v", key, got, want)
		}
	}
}

// TestMergeCR_PutThenRefetch verifies MergeCR issues the SDK PUT merge and
// then re-fetches the PR (the SDK merge method returns no decoded body).
func TestMergeCR_PutThenRefetch(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	var paths []string
	var mergeBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/merge") {
			_ = json.NewDecoder(r.Body).Decode(&mergeBody)
		}
		writeJSON(w, map[string]any{"id": 7, "number": 7, "title": "t", "state": "merged", "merged_at": "2026-01-03T03:04:05+08:00"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	cr, err := p.MergeCR(context.Background(), "owner", "repo", "7", provider.MergeCROptions{MergeCommitMessage: "msg", Squash: true})
	if err != nil {
		t.Fatal(err)
	}
	if cr.State != provider.CRStateMerged {
		t.Errorf("state: got %s", cr.State)
	}
	if len(methods) != 2 || methods[0] != "PUT" || methods[1] != "GET" {
		t.Fatalf("expected PUT then GET, got %v", methods)
	}
	if paths[0] != "/api/v5/repos/owner/repo/pulls/7/merge" || paths[1] != "/api/v5/repos/owner/repo/pulls/7" {
		t.Fatalf("unexpected paths: %v", paths)
	}
	if mergeBody["merge_method"] != "squash" || mergeBody["description"] != "msg" {
		t.Errorf("merge body: got %v", mergeBody)
	}
}

// TestUpdateCRLabels_Endpoint verifies label replacement hits the dedicated
// PUT /pulls/{number}/labels endpoint with a JSON array body (the SDK
// PullRequestLabelPostParam), not the generic PR-update endpoint.
func TestUpdateCRLabels_Endpoint(t *testing.T) {
	var method, path string
	var body []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writeJSON(w, []map[string]any{})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	if err := p.UpdateCRLabels(context.Background(), "owner", "repo", "3", []string{"x", "y"}); err != nil {
		t.Fatal(err)
	}
	if method != "PUT" || path != "/api/v5/repos/owner/repo/pulls/3/labels" {
		t.Errorf("request: %s %s", method, path)
	}
	if len(body) != 2 || body[0] != "x" || body[1] != "y" {
		t.Errorf("labels body: got %v", body)
	}
}

// TestLabels_SDKCreateDelete locks the SDK label wiring: create posts a JSON
// body via PostV5ReposOwnerRepoLabels (access_token, name, color — and no
// description key, matching the live API, which has no label description),
// delete hits the name-addressed DELETE endpoint with the per-call
// access_token query param, and the returned color is normalized (leading
// '#' stripped) on both paths.
func TestLabels_SDKCreateDelete(t *testing.T) {
	var mu sync.Mutex
	var got []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		mu.Lock()
		got = append(got, capturedRequest{r.Method, r.URL.Path, r.URL.Query(), body})
		mu.Unlock()
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, map[string]any{"id": 1, "name": "bug", "color": "#4cc917"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	l, err := p.CreateLabel(context.Background(), "owner", "repo", provider.CreateLabelOptions{
		Name: "bug", Color: "4cc917", Description: "dropped on gitee",
	})
	if err != nil {
		t.Fatal(err)
	}
	if l.Color != "4cc917" {
		t.Errorf("color normalization lost: got %q", l.Color)
	}
	if err := p.DeleteLabel(context.Background(), "owner", "repo", "la bel"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 recorded requests, got %d: %+v", len(got), got)
	}
	create, del := got[0], got[1]
	if create.method != "POST" || create.path != "/api/v5/repos/owner/repo/labels" {
		t.Errorf("create request: %s %s", create.method, create.path)
	}
	if create.body["name"] != "bug" || create.body["color"] != "4cc917" {
		t.Errorf("create body keys: got %v", create.body)
	}
	if _, ok := create.body["description"]; ok {
		t.Errorf("create body carries description %v — the live Gitee label API has no description", create.body["description"])
	}
	if del.method != "DELETE" || del.path != "/api/v5/repos/owner/repo/labels/la bel" {
		t.Errorf("delete request: %s %s", del.method, del.path)
	}
	if del.query.Get("access_token") != "test-token" {
		t.Errorf("delete access_token: got %q", del.query.Get("access_token"))
	}
}

// capturedRequest records one observed HTTP request for the SDK-wiring tests.
type capturedRequest struct {
	method string
	path   string
	query  url.Values
	body   map[string]any
}

// TestNotes_CreateDelete exercises the PR-comment CRUD path: POST returns the
// comment id as a string note ID, DELETE addresses it numerically.
func TestNotes_CreateDelete(t *testing.T) {
	var mu sync.Mutex
	var ops []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ops = append(ops, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Method == "POST" {
			writeJSON(w, map[string]any{"id": 48745338, "body": "note", "user": map[string]any{"id": 1, "login": "dev", "name": "Dev"}, "created_at": "2026-01-02T03:04:05+08:00", "updated_at": "2026-01-02T03:04:05+08:00"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	id, err := p.CreateNote(context.Background(), "owner", "repo", "5", "note")
	if err != nil {
		t.Fatal(err)
	}
	if id != "48745338" {
		t.Errorf("note id: got %q", id)
	}
	if err := p.DeleteNote(context.Background(), "owner", "repo", "5", id); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[1] != "DELETE /api/v5/repos/owner/repo/pulls/comments/48745338" {
		t.Errorf("operations: %v", ops)
	}
}

// TestGetCRDiff_SDKFiles locks the PR-files conversion against the live wire
// shape: string-typed additions/deletions and the nested patch object
// (diff/old_path/new_path/new_file flags).
func TestGetCRDiff_SDKFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]any{{
			"sha": "s", "filename": "a.txt", "status": "changed",
			"additions": "3", "deletions": "2",
			"patch": map[string]any{
				"diff":     "@@ -1 +1 @@\n-x\n+y\n",
				"old_path": "a.txt", "new_path": "a.txt",
				"new_file": false, "renamed_file": false, "deleted_file": false,
			},
		}})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	diff, err := p.GetCRDiff(context.Background(), "owner", "repo", "9")
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(diff.Files))
	}
	f := diff.Files[0]
	if f.OldPath != "a.txt" || f.NewPath != "a.txt" {
		t.Errorf("paths: got %s/%s", f.OldPath, f.NewPath)
	}
	if f.Additions != 3 || f.Deletions != 2 {
		t.Errorf("stats: got %d/%d", f.Additions, f.Deletions)
	}
	if f.IsNew || f.IsDeleted || f.IsRenamed {
		t.Errorf("flags: new=%v deleted=%v renamed=%v", f.IsNew, f.IsDeleted, f.IsRenamed)
	}
	if diff.TotalAdd != 3 || diff.TotalDel != 2 {
		t.Errorf("totals: got %d/%d", diff.TotalAdd, diff.TotalDel)
	}
}

// TestCompareCommits_WireShape locks the raw compare fallback: numeric
// stats, string patch diffs, status vocabulary, and commit conversion.
func TestCompareCommits_WireShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/compare/base1...head1") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{
			"commits": []map[string]any{{
				"sha": "c1",
				"commit": map[string]any{
					"message": "m",
					"author":  map[string]any{"name": "n", "email": "e", "date": "2026-01-02T03:04:05+08:00"},
				},
				"author": map[string]any{"id": 1, "login": "dev"},
			}},
			"files": []map[string]any{{
				"filename": "new.txt", "status": "added", "additions": 4, "deletions": 0,
				"patch": "@@ -0,0 +1 @@\n+hello\n",
			}},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	res, err := p.CompareCommits(context.Background(), "owner", "repo", "base1", "head1")
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalCommits != 1 || len(res.Commits) != 1 || len(res.Files) != 1 {
		t.Fatalf("counts: %d commits, %d files", len(res.Commits), len(res.Files))
	}
	if res.Commits[0].SHA != "c1" || res.Commits[0].Message != "m" || res.Commits[0].Author == nil || res.Commits[0].Author.Username != "dev" {
		t.Errorf("commit: %+v", res.Commits[0])
	}
	f := res.Files[0]
	if f.NewPath != "new.txt" || !f.IsNew || f.IsDeleted {
		t.Errorf("file: %+v", f)
	}
	if f.Additions != 4 || f.Deletions != 0 {
		t.Errorf("file stats: %d/%d", f.Additions, f.Deletions)
	}
}

func TestParseWebhookEvent_PullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"action":"open","number":1,"title":"t","state":"open","source_branch":"f","target_branch":"main","html_url":"https://gitee.com/owner/repo/pulls/1","user":{"id":1,"login":"dev","name":"Dev"},"repository":{"full_name":"owner/repo"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-Gitee-Event", "pull_request")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "cr.opened" {
		t.Errorf("expected cr.opened, got %s", ne.Type)
	}
}

func TestParseWebhookEvent_Push(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"ref":"refs/heads/main","after":"abc123","user":{"id":1,"login":"dev","name":"Dev"},"repository":{"full_name":"owner/repo"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-Gitee-Event", "push")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "push" {
		t.Errorf("expected push, got %s", ne.Type)
	}
	if ne.Branch != "main" {
		t.Errorf("expected main, got %s", ne.Branch)
	}
}

// TestIssues_SDKWiring locks the issue wiring after the SDK migration: the
// alphanumeric string issue number travels in the URL unconverted, the
// update PATCHes Gitee's owner-scoped issues endpoint with the repo path in
// the JSON body, and the raw create posts to the documented owner-scoped
// endpoint (repo in body, comma-joined assignees, no assignees key when
// none are set).
func TestIssues_SDKWiring(t *testing.T) {
	var mu sync.Mutex
	var got []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		mu.Lock()
		got = append(got, capturedRequest{r.Method, r.URL.Path, r.URL.Query(), body})
		mu.Unlock()
		writeJSON(w, map[string]any{
			"id": 1, "number": "IAINVA", "title": "t", "state": "open",
			"user":      map[string]any{"id": 1, "login": "dev", "name": "Dev"},
			"milestone": map[string]any{"id": 7, "number": 3, "title": "v1"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	if _, err := p.GetIssue(context.Background(), "owner", "repo", "IAINVA"); err != nil {
		t.Fatal(err)
	}
	issue, err := p.CreateIssue(context.Background(), provider.CreateIssueOptions{
		Owner: "owner", Repo: "repo", Title: "t", Assignees: []string{"a", "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Number != "IAINVA" || issue.Milestone == nil || issue.Milestone.Number != "3" || issue.Milestone.Title != "v1" {
		t.Errorf("issue conversion: number=%q milestone=%+v", issue.Number, issue.Milestone)
	}
	if _, err := p.UpdateIssue(context.Background(), "owner", "repo", "IAINVA", provider.UpdateIssueOptions{Title: "t2"}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("expected 3 recorded requests, got %d: %+v", len(got), got)
	}
	get, create, update := got[0], got[1], got[2]
	if get.method != "GET" || get.path != "/api/v5/repos/owner/repo/issues/IAINVA" {
		t.Errorf("get request: %s %s", get.method, get.path)
	}
	if create.method != "POST" || create.path != "/api/v5/repos/owner/issues" {
		t.Errorf("create request: %s %s (Gitee's create endpoint is owner-scoped)", create.method, create.path)
	}
	if create.body["repo"] != "repo" || create.body["title"] != "t" || create.body["assignee"] != "a,b" {
		t.Errorf("create body keys: got %v", create.body)
	}
	if _, ok := create.body["assignees"]; ok {
		t.Errorf("create body carries undocumented assignees key: %v", create.body["assignees"])
	}
	if update.method != "PATCH" || update.path != "/api/v5/repos/owner/issues/IAINVA" {
		t.Errorf("update request: %s %s", update.method, update.path)
	}
	if update.body["repo"] != "repo" || update.body["title"] != "t2" {
		t.Errorf("update body keys: got %v", update.body)
	}
}

// TestWebhooks_CreateBodyAndDeleteWire locks the webhook wiring: create
// stays on the raw client with Gitee's documented vocabulary (password as
// the signing key, *_events booleans derived from the event names, no
// undocumented "events"/"secret" keys), while delete rides the SDK with the
// per-call access_token query param.
func TestWebhooks_CreateBodyAndDeleteWire(t *testing.T) {
	var mu sync.Mutex
	var got []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		mu.Lock()
		got = append(got, capturedRequest{r.Method, r.URL.Path, r.URL.Query(), body})
		mu.Unlock()
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, map[string]any{"id": 9, "url": "https://example.com/hook", "push_events": true})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)

	hook, err := p.CreateWebhook(context.Background(), provider.CreateWebhookOptions{
		Owner: "owner", Repo: "repo", URL: "https://example.com/hook",
		Secret: "s3cret", Events: []string{"push", "issues", "member"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hook.ID != 9 || len(hook.Events) != 1 || hook.Events[0] != "push" {
		t.Errorf("hook conversion: %+v", hook)
	}
	if err := p.DeleteWebhook(context.Background(), "owner", "repo", 9); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 recorded requests, got %d: %+v", len(got), got)
	}
	create, del := got[0], got[1]
	if create.method != "POST" || create.path != "/api/v5/repos/owner/repo/hooks" {
		t.Errorf("create request: %s %s", create.method, create.path)
	}
	if create.body["password"] != "s3cret" || create.body["push_events"] != true || create.body["issues_events"] != true {
		t.Errorf("create body keys: got %v", create.body)
	}
	if v, ok := create.body["events"]; ok {
		t.Errorf("create body carries undocumented events key: %v", v)
	}
	if v, ok := create.body["secret"]; ok {
		t.Errorf("create body carries non-Gitee secret key: %v", v)
	}
	if v, ok := create.body["member"]; ok {
		t.Errorf("create body carries non-Gitee event key member: %v", v)
	}
	if del.method != "DELETE" || del.path != "/api/v5/repos/owner/repo/hooks/9" {
		t.Errorf("delete request: %s %s", del.method, del.path)
	}
	if del.query.Get("access_token") != "test-token" {
		t.Errorf("delete access_token: got %q", del.query.Get("access_token"))
	}
}

func TestProvider_ImplementsProvider(t *testing.T) {
	var _ provider.Provider = (*gitee.Provider)(nil)
}

func TestRetry_TriggersOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, []map[string]any{})
	}))
	defer srv.Close()
	p, err := gitee.New(provider.Config{
		Platform:    provider.PlatformGitee,
		BaseURL:     srv.URL,
		Token:       "test",
		RetryConfig: &provider.RetryConfig{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Errorf("expected at least 2 calls, got %d", got)
	}
}

func TestIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]string{"message": "Not Found"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, err := p.GetRepo(context.Background(), "missing", "repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if !provider.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}
