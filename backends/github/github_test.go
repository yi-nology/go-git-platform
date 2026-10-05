package github_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdkgithub "github.com/google/go-github/v92/github"

	ghbackend "github.com/yi-nology/go-git-platform/backends/github"
	"github.com/yi-nology/go-git-platform/provider"
)

func newTestProvider(t *testing.T, baseURL string) *ghbackend.Provider {
	t.Helper()
	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitHub,
		BaseURL:  baseURL,
		Token:    "test-token",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	gp, ok := p.(*ghbackend.Provider)
	if !ok {
		t.Fatalf("expected *ghbackend.Provider, got %T", p)
	}
	return gp
}

func TestNewProvider_Success(t *testing.T) {
	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitHub,
		BaseURL:  "http://example.test/api/v3",
		Token:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Platform() != provider.PlatformGitHub {
		t.Errorf("expected GitHub, got %s", p.Platform())
	}
}

func TestListRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]*sdkgithub.Repository{
			{ID: new(int64(1)), FullName: new("owner/r1"), Name: new("r1"),
				Owner: &sdkgithub.User{Login: new("owner")}, DefaultBranch: new("main")},
			{ID: new(int64(2)), FullName: new("owner/r2"), Name: new("r2"),
				Owner: &sdkgithub.User{Login: new("owner")}, DefaultBranch: new("main"), Private: new(true)},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
	repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2, got %d", len(repos))
	}
	if repos[0].Platform != provider.PlatformGitHub {
		t.Errorf("expected GitHub platform, got %s", repos[0].Platform)
	}
	if !repos[1].Private {
		t.Error("expected r2 to be private")
	}
}

func TestGetRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(&sdkgithub.Repository{
			ID: new(int64(42)), FullName: new("owner/repo"), Name: new("repo"),
			Owner: &sdkgithub.User{Login: new("owner")}, DefaultBranch: new("main"),
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
	repo, err := p.GetRepo(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if repo.ID != 42 || repo.Owner != "owner" {
		t.Errorf("unexpected repo: %+v", repo)
	}
}

func TestCreateCR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(&sdkgithub.PullRequest{
			Number: new(7), Title: new("test"), State: new("open"),
			Head:    &sdkgithub.PullRequestBranch{Ref: new("feature"), SHA: new("abc")},
			Base:    &sdkgithub.PullRequestBranch{Ref: new("main")},
			User:    &sdkgithub.User{ID: new(int64(1)), Login: new("dev"), AvatarURL: new("https://a/v")},
			HTMLURL: new("https://github.com/owner/repo/pull/7"),
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
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
	if cr.State != provider.CRStateOpened {
		t.Errorf("expected opened, got %s", cr.State)
	}
}

func TestListCRs_MergedDetected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]*sdkgithub.PullRequest{
			{Number: new(1), Title: new("a"), State: new("open"),
				Head: &sdkgithub.PullRequestBranch{Ref: new("a")}, Base: &sdkgithub.PullRequestBranch{Ref: new("main")},
				User: &sdkgithub.User{ID: new(int64(1)), Login: new("u")}},
			{Number: new(2), Title: new("b"), State: new("closed"), Merged: new(true),
				Head: &sdkgithub.PullRequestBranch{Ref: new("b")}, Base: &sdkgithub.PullRequestBranch{Ref: new("main")},
				User: &sdkgithub.User{ID: new(int64(1)), Login: new("u")}},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
	crs, total, err := p.ListCRs(context.Background(), provider.ListCROptions{Owner: "owner", Repo: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if crs[1].State != provider.CRStateMerged {
		t.Errorf("expected merged, got %s", crs[1].State)
	}
}

func TestGetCRDiff_Pagination(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// First page: return 100 files to force pagination
			files := make([]*sdkgithub.CommitFile, 100)
			for i := range files {
				files[i] = &sdkgithub.CommitFile{Filename: new("f" + string(rune('a'+i%26))), Status: new("modified"), Additions: new(1), Deletions: new(0)}
			}
			_ = json.NewEncoder(w).Encode(files)
			return
		}
		// Second page: 1 file
		_ = json.NewEncoder(w).Encode([]*sdkgithub.CommitFile{
			{Filename: new("last"), Status: new("added"), Additions: new(5)},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
	diff, err := p.GetCRDiff(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 calls (pagination), got %d", got)
	}
	if diff.TotalAdd == 0 {
		t.Error("expected non-zero additions")
	}
}

func TestParseWebhookEvent_PullRequest(t *testing.T) {
	p := newTestProvider(t, "http://example.test/api/v3")
	body := `{"action":"opened","number":1,"pull_request":{"number":1,"state":"open","title":"t","draft":true,"head":{"ref":"f","sha":"headSHA"},"base":{"ref":"main","sha":"baseSHA"}},"repository":{"id":42,"full_name":"owner/repo"},"sender":{"login":"dev"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-GitHub-Event", "pull_request")
	r.Header.Set("Content-Type", "application/json")
	ne, err := p.ParseWebhookEvent(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if ne.Type != "cr.opened" {
		t.Errorf("expected cr.opened, got %s", ne.Type)
	}
	if ne.CR == nil || ne.CR.Number != "1" {
		t.Errorf("expected PR with number 1, got %+v", ne.CR)
	}
	if ne.Repo == nil || ne.Repo.ID != 42 {
		t.Errorf("expected repo ID 42, got %+v", ne.Repo)
	}
	if ne.CR.HeadSHA != "headSHA" {
		t.Errorf("expected head SHA headSHA, got %q", ne.CR.HeadSHA)
	}
	if ne.CR.BaseSHA != "baseSHA" {
		t.Errorf("expected base SHA baseSHA, got %q", ne.CR.BaseSHA)
	}
	// GitHub exposes no distinct merge-base: StartSHA mirrors the base tip.
	if ne.CR.StartSHA != "baseSHA" {
		t.Errorf("expected start SHA baseSHA, got %q", ne.CR.StartSHA)
	}
	if !ne.CR.Draft {
		t.Errorf("expected draft=true, got %+v", ne.CR)
	}
}

func TestParseWebhookEvent_Merged(t *testing.T) {
	p := newTestProvider(t, "http://example.test/api/v3")
	body := `{"action":"closed","number":1,"pull_request":{"number":1,"state":"closed","merged":true,"head":{"ref":"f","sha":"abc"},"base":{"ref":"main"}},"repository":{"full_name":"owner/repo"},"sender":{"login":"dev"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-GitHub-Event", "pull_request")
	r.Header.Set("Content-Type", "application/json")
	ne, err := p.ParseWebhookEvent(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if ne.Type != "cr.merged" {
		t.Errorf("expected cr.merged, got %s", ne.Type)
	}
}

func TestParseWebhookEvent_Push(t *testing.T) {
	p := newTestProvider(t, "http://example.test/api/v3")
	body := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"owner/repo"},"sender":{"login":"dev"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-GitHub-Event", "push")
	r.Header.Set("Content-Type", "application/json")
	ne, err := p.ParseWebhookEvent(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if ne.Type != "push" {
		t.Errorf("expected push, got %s", ne.Type)
	}
	if ne.Branch != "main" {
		t.Errorf("expected main, got %s", ne.Branch)
	}
	if ne.CommitSHA != "abc123" {
		t.Errorf("expected abc123, got %s", ne.CommitSHA)
	}
}

// writeListPage serves v on page 1 and an empty array on page ≥ 2 — the
// pagination-aware mock shape for list endpoints whose backend walks all
// pages via backendutil.AllPages.
func writeListPage(w http.ResponseWriter, r *http.Request, v any) {
	if page := r.URL.Query().Get("page"); page != "" && page != "1" {
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func TestListBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeListPage(w, r, []*sdkgithub.Branch{
			{Name: new("main")},
			{Name: new("dev")},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL+"/api/v3")
	branches, err := p.ListBranches(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2, got %d", len(branches))
	}
}

func TestCreateBranch_WithCommitSHA(t *testing.T) {
	var gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ref":"refs/heads/newbranch","object":{"sha":"abc"}}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL+"/api/v3")
	sha := strings.Repeat("a", 40)
	_, err := p.CreateBranch(context.Background(), "owner", "repo", "newbranch", sha)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotRef, "git/refs") {
		t.Errorf("expected git/refs endpoint, got %s", gotRef)
	}
}

func TestListTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeListPage(w, r, []*sdkgithub.RepositoryTag{
			{Name: new("v1.0"), Commit: &sdkgithub.Commit{SHA: new("abc")}},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL+"/api/v3")
	tags, err := p.ListTags(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Name != "v1.0" {
		t.Errorf("unexpected tags: %+v", tags)
	}
}

func TestCreateCommitStatus(t *testing.T) {
	var gotState string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State string `json:"state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotState = body.State
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL+"/api/v3")
	err := p.CreateCommitStatus(context.Background(), "owner", "repo", "abc", provider.CommitStatusOptions{
		State: "success", Context: "ci", Description: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotState != "success" {
		t.Errorf("expected success, got %q", gotState)
	}
}

func TestProvider_ImplementsProvider(t *testing.T) {
	var _ provider.Provider = (*ghbackend.Provider)(nil)
}

// TestGetFileContent_NotFound verifies the error classification. The
// go-github SDK's DownloadContents makes 2 HTTP calls (metadata + content
// fetch), which is brittle to mock in full here. Instead we test the
// error path that flows through provider.Wrap.
func TestGetFileContent_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL+"/api/v3")
	_, err := p.GetFileContent(context.Background(), "owner", "repo", "missing.md", "main")
	if err == nil {
		t.Fatal("expected error")
	}
	if !provider.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestRetry_TriggersOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode([]*sdkgithub.Repository{})
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{
		Platform:    provider.PlatformGitHub,
		BaseURL:     srv.URL + "/api/v3",
		Token:       "test",
		RetryConfig: &provider.RetryConfig{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.ListRepos(context.Background(), provider.ListRepoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Errorf("expected at least 2 calls (retry), got %d", got)
	}
}

func TestIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL+"/api/v3")
	_, err := p.GetRepo(context.Background(), "missing", "repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if !provider.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestValidateWebhookSignature_NoSecret(t *testing.T) {
	p := newTestProvider(t, "http://example.test/api/v3")
	r, _ := http.NewRequest(http.MethodPost, "/hook", nil)
	// Since v0.77.0 validation delegates to the registry validator, and an
	// empty secret is rejected: an empty HMAC key is trivially forgeable on
	// predictable payloads.
	if err := p.ValidateWebhookSignature(r, ""); err == nil {
		t.Error("expected empty secret to be rejected, got nil error")
	}
}

func TestListMyGists_Pagination(t *testing.T) {
	var gotPath, gotPage, gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotPage = r.URL.Query().Get("page")
		gotPerPage = r.URL.Query().Get("per_page")
		w.Header().Set("Content-Type", "application/json")
		// 原始 JSON 而非 SDK 结构体:同时验证 provider.Gist 的 json tag
		// 与 GitHub 响应字段逐一对齐。
		_, _ = w.Write([]byte(`[{"id":"g1","description":"notes","public":false,` +
			`"html_url":"https://gist.github.com/g1",` +
			`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z",` +
			`"files":{"a.txt":{"filename":"a.txt","language":"Text",` +
			`"raw_url":"https://gist.githubusercontent.com/raw","size":5,"content":"hello"}}}]`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
	gists, err := p.ListMyGists(context.Background(), 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v3/gists" {
		t.Errorf("expected GET /gists (token 用户列表), got path %s", gotPath)
	}
	if gotPage != "2" || gotPerPage != "5" {
		t.Errorf("expected page=2 per_page=5, got page=%s per_page=%s", gotPage, gotPerPage)
	}
	if len(gists) != 1 {
		t.Fatalf("expected 1 gist, got %d", len(gists))
	}
	g := gists[0]
	if g.ID != "g1" || g.Public {
		t.Errorf("unexpected gist identity: %+v", g)
	}
	if g.CreatedAt.Year() != 2026 || g.UpdatedAt.Year() != 2026 {
		t.Errorf("expected 2026 timestamps, got %v / %v", g.CreatedAt, g.UpdatedAt)
	}
	f := g.Files["a.txt"]
	if f.Content != "hello" || f.Size != 5 || f.Filename != "a.txt" || f.RawURL == "" {
		t.Errorf("unexpected gist file: %+v", f)
	}
}

func TestListStarred_ShapeCompat(t *testing.T) {
	const repoJSON = `{"id":1,"full_name":"octo/hello","name":"hello","owner":{"login":"octo"},` +
		`"clone_url":"https://github.com/octo/hello.git","description":"demo","private":false,` +
		`"archived":true,"fork":true,"stargazers_count":42,"language":"Go"}`
	cases := []struct {
		name string
		body string
	}{
		// Accept: star+json 时 GitHub 返回的包裹形状。
		{"wrapped", `[{"starred_at":"2026-01-01T00:00:00Z","repo":` + repoJSON + `}]`},
		// 服务端忽略 Accept 时的裸 repo 列表形状。
		{"bare", `[` + repoJSON + `]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v3/user/starred" {
					t.Errorf("expected /user/starred, got %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			p := newTestProvider(t, srv.URL+"/api/v3")
			repos, err := p.ListStarred(context.Background(), 1, 100)
			if err != nil {
				t.Fatal(err)
			}
			if len(repos) != 1 {
				t.Fatalf("expected 1 starred repo, got %d", len(repos))
			}
			r := repos[0]
			if r.FullName != "octo/hello" || r.Owner != "octo" {
				t.Errorf("unexpected identity: %+v", r)
			}
			if r.Stars != 42 || r.Language != "Go" || !r.Archived || !r.Fork {
				t.Errorf("expected 元数据字段填全(stars/language/archived/fork), got %+v", r)
			}
			if r.CloneURL == "" || r.Description != "demo" {
				t.Errorf("expected clone_url/description, got %+v", r)
			}
		})
	}
}

func TestCreateMigration_OrgPaths(t *testing.T) {
	cases := []struct {
		name     string
		org      string
		wantPath string
	}{
		{"user_level", "", "/api/v3/user/migrations"},
		{"org_level", "acme", "/api/v3/orgs/acme/migrations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath string
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				_ = json.NewDecoder(r.Body).Decode(&body)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":7,"state":"pending","archive_url":""}`))
			}))
			defer srv.Close()

			p := newTestProvider(t, srv.URL+"/api/v3")
			info, err := p.CreateMigration(context.Background(), tc.org, provider.CreateMigrationOptions{
				LockRepositories: true,
				ExcludeMetadata:  false,
			})
			if err != nil {
				t.Fatal(err)
			}
			if gotMethod != http.MethodPost || gotPath != tc.wantPath {
				t.Errorf("expected POST %s, got %s %s", tc.wantPath, gotMethod, gotPath)
			}
			// 与消费方线上请求体保持同一形状:两个 key 都显式上送。
			if body["lock_repositories"] != true {
				t.Errorf("expected lock_repositories=true, got %v", body["lock_repositories"])
			}
			if v, ok := body["exclude_metadata"]; !ok || v != false {
				t.Errorf("expected explicit exclude_metadata=false, got %v (present=%v)", v, ok)
			}
			if info.ID != 7 || info.State != "pending" {
				t.Errorf("unexpected migration info: %+v", info)
			}
		})
	}
}

func TestGetMigration_OrgPaths(t *testing.T) {
	cases := []struct {
		name     string
		org      string
		wantPath string
	}{
		{"user_level", "", "/api/v3/user/migrations/7"},
		{"org_level", "acme", "/api/v3/orgs/acme/migrations/7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":7,"state":"exported",` +
					`"archive_url":"https://api.github.com/user/migrations/7/artifacts"}`))
			}))
			defer srv.Close()

			p := newTestProvider(t, srv.URL+"/api/v3")
			info, err := p.GetMigration(context.Background(), tc.org, 7)
			if err != nil {
				t.Fatal(err)
			}
			if gotPath != tc.wantPath {
				t.Errorf("expected GET %s, got %s", tc.wantPath, gotPath)
			}
			if info.State != "exported" || info.ArchiveURL == "" {
				t.Errorf("unexpected migration info: %+v", info)
			}
		})
	}
}

func TestDownloadReleaseAsset_Streams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/owner/repo/releases/assets/99" {
			t.Errorf("unexpected asset path %s", r.URL.Path)
		}
		// 附件端点必须带 octet-stream Accept,否则 GitHub 回 JSON 元数据。
		if got := r.Header.Get("Accept"); got != "application/octet-stream" {
			t.Errorf("expected Accept application/octet-stream, got %q", got)
		}
		_, _ = w.Write([]byte("asset-bytes"))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
	var buf bytes.Buffer
	if err := p.DownloadReleaseAsset(context.Background(), "owner", "repo", 99, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "asset-bytes" {
		t.Errorf("expected streamed bytes, got %q", buf.String())
	}
}

func TestDownloadReleaseAsset_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
	var buf bytes.Buffer
	err := p.DownloadReleaseAsset(context.Background(), "owner", "repo", 404, &buf)
	if err == nil {
		t.Fatal("expected error on non-2xx asset response")
	}
	if !provider.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no bytes written on error, got %q", buf.String())
	}
}

func TestListReleases_AssetsConverted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeListPage(w, r, []*sdkgithub.RepositoryRelease{
			{ID: 1, TagName: "v1.0.0", Name: new("v1.0.0"),
				Assets: []*sdkgithub.ReleaseAsset{
					{ID: new(int64(99)), Name: new("app.zip"), Size: new(123),
						ContentType:        new("application/zip"),
						BrowserDownloadURL: new("https://github.com/o/r/releases/download/v1.0.0/app.zip"),
						URL:                new("https://api.github.com/repos/o/r/releases/assets/99")},
				}},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL+"/api/v3")
	rels, err := p.ListReleases(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || len(rels[0].Assets) != 1 {
		t.Fatalf("expected 1 release with 1 asset, got %+v", rels)
	}
	a := rels[0].Assets[0]
	if a.ID != 99 || a.Name != "app.zip" || a.Size != 123 {
		t.Errorf("unexpected asset identity: %+v", a)
	}
	if a.ContentType != "application/zip" ||
		a.BrowserDownloadURL != "https://github.com/o/r/releases/download/v1.0.0/app.zip" ||
		a.URL != "https://api.github.com/repos/o/r/releases/assets/99" {
		t.Errorf("unexpected asset urls/type: %+v", a)
	}
}
