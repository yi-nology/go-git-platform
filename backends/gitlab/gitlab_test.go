package gitlab_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/backends/contracttest"

	"github.com/yi-nology/go-git-platform/backends/gitlab"
	"github.com/yi-nology/go-git-platform/provider"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeListPage serves v on page 1 and an empty array on page ≥ 2 — the
// pagination-aware mock shape for list endpoints whose backends walk all
// pages via backendutil.AllPages.
func writeListPage(w http.ResponseWriter, r *http.Request, v any) {
	if page := r.URL.Query().Get("page"); page != "" && page != "1" {
		writeJSON(w, []any{})
		return
	}
	writeJSON(w, v)
}

func newTestProvider(t *testing.T, srv *httptest.Server) *gitlab.Provider {
	t.Helper()
	p, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitLab,
		BaseURL:  srv.URL,
		Token:    "test-token",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	glp, ok := p.(*gitlab.Provider)
	if !ok {
		t.Fatalf("expected *gitlab.Provider, got %T", p)
	}
	return glp
}

func TestNewProvider_Success(t *testing.T) {
	_, err := provider.NewProvider(provider.Config{
		Platform: provider.PlatformGitLab,
		BaseURL:  "https://gitlab.example.com/api/v4",
		Token:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GitLab client auto-prepends /api/v4
		if r.URL.Path != "/api/v4/projects" {
			t.Errorf("expected /api/v4/projects, got %s", r.URL.Path)
		}
		w.Header().Set("X-Total", "1")
		writeJSON(w, []map[string]any{
			{"id": 1, "name": "r1", "path_with_namespace": "owner/r1",
				"http_url_to_repo": "https://gitlab.com/owner/r1.git",
				"ssh_url_to_repo":  "git@gitlab.com:owner/r1.git",
				"default_branch":   "main", "visibility": "public"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Owner != "owner" || repos[0].Name != "r1" {
		t.Errorf("unexpected repo: %+v", repos[0])
	}
	if repos[0].Platform != provider.PlatformGitLab {
		t.Errorf("expected GitLab, got %s", repos[0].Platform)
	}
}

func TestGetRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id": 42, "name": "repo", "path_with_namespace": "owner/repo",
			"http_url_to_repo": "https://gitlab.com/owner/repo.git",
			"default_branch":   "main", "visibility": "private",
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	repo, err := p.GetRepo(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if repo.ID != 42 || !repo.Private {
		t.Errorf("unexpected repo: %+v", repo)
	}
}

func TestCreateCR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"iid": 7, "title": "test", "state": "opened",
			"source_branch": "feature", "target_branch": "main",
			"web_url": "https://gitlab.com/owner/repo/-/merge_requests/7",
			"author":  map[string]any{"id": 1, "username": "dev", "name": "Dev"},
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
	if cr.State != provider.CRStateOpened {
		t.Errorf("expected opened, got %s", cr.State)
	}
}

func TestListCRs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Total", "2")
		writeJSON(w, []map[string]any{
			{"iid": 1, "title": "a", "state": "opened", "source_branch": "a", "target_branch": "main"},
			{"iid": 2, "title": "b", "state": "merged", "source_branch": "b", "target_branch": "main"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
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

func TestListBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeListPage(w, r, []map[string]any{
			{"name": "main"},
			{"name": "develop"},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	branches, err := p.ListBranches(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2, got %d", len(branches))
	}
}

func TestCreateBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"name": "feature"})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	b, err := p.CreateBranch(context.Background(), "owner", "repo", "feature", "main")
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "feature" {
		t.Errorf("expected feature, got %s", b.Name)
	}
}

func TestListTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeListPage(w, r, []map[string]any{
			{"name": "v1.0", "commit": map[string]any{"id": "abc123"}},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	tags, err := p.ListTags(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Commit != "abc123" {
		t.Errorf("unexpected tags: %+v", tags)
	}
}

func TestListReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeListPage(w, r, []map[string]any{
			{
				"tag_name": "v1.0", "name": "Release 1.0",
				"description": "first", "released_at": "2024-01-01T00:00:00Z",
				"created_at": "2024-01-01T00:00:00Z",
				"assets":     map[string]any{"sources": []any{}},
				"links":      map[string]any{"self": "https://gitlab.com/owner/repo/-/releases/v1.0"},
			},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	releases, err := p.ListReleases(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if releases[0].TagName != "v1.0" {
		t.Errorf("expected v1.0, got %s", releases[0].TagName)
	}
}

func TestValidateWebhookSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	r, _ := http.NewRequest(http.MethodPost, "/hook", nil)
	r.Header.Set("X-Gitlab-Token", "the-secret")
	if err := p.ValidateWebhookSignature(r, "the-secret"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	r2, _ := http.NewRequest(http.MethodPost, "/hook", nil)
	r2.Header.Set("X-Gitlab-Token", "wrong")
	if err := p.ValidateWebhookSignature(r2, "the-secret"); err == nil {
		t.Error("expected error for wrong token")
	}
}

func TestParseWebhookEvent_MergeRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"object_kind":"merge_request","user":{"id":1,"username":"dev","name":"Dev"},"project":{"id":99,"path_with_namespace":"owner/repo"},"object_attributes":{"iid":7,"title":"t","description":"d","state":"opened","source_branch":"f","target_branch":"main","action":"open","merge_status":"can_be_merged","url":"https://gitlab.com/owner/repo/-/merge_requests/7","merge_commit_sha":"mcSHA","work_in_progress":true,"last_commit":{"id":"abc"},"diff_refs":{"base_sha":"bSHA","start_sha":"sSHA","head_sha":"hSHA"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "cr.opened" {
		t.Errorf("expected cr.opened, got %s", ne.Type)
	}
	if ne.CR == nil || ne.CR.Number != "7" {
		t.Errorf("expected CR with number 7, got %+v", ne.CR)
	}
	if ne.Repo == nil || ne.Repo.ID != 99 {
		t.Errorf("expected repo ID 99, got %+v", ne.Repo)
	}
	// merge_commit_sha takes priority over diff_refs.base_sha.
	if ne.CR.BaseSHA != "mcSHA" {
		t.Errorf("expected base SHA mcSHA, got %q", ne.CR.BaseSHA)
	}
	// diff_refs.head_sha overrides last_commit.id.
	if ne.CR.HeadSHA != "hSHA" {
		t.Errorf("expected head SHA hSHA, got %q", ne.CR.HeadSHA)
	}
	if ne.CR.StartSHA != "sSHA" {
		t.Errorf("expected start SHA sSHA, got %q", ne.CR.StartSHA)
	}
	if !ne.CR.Draft {
		t.Errorf("expected draft=true (work_in_progress), got %+v", ne.CR)
	}
}

func TestParseWebhookEvent_MergeRequest_NoDiffRefs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	// No diff_refs and no merge_commit_sha: head must fall back to last_commit
	// and base/start must be empty (open MR never merged).
	body := `{"object_kind":"merge_request","user":{"id":1,"username":"dev","name":"Dev"},"project":{"id":99,"path_with_namespace":"owner/repo"},"object_attributes":{"iid":7,"title":"t","state":"opened","source_branch":"f","target_branch":"main","action":"open","merge_status":"can_be_merged","url":"https://gitlab.com/owner/repo/-/merge_requests/7","last_commit":{"id":"abcOnly"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.CR.HeadSHA != "abcOnly" {
		t.Errorf("expected head SHA to fall back to last_commit abcOnly, got %q", ne.CR.HeadSHA)
	}
	if ne.CR.BaseSHA != "" {
		t.Errorf("expected empty base SHA without merge_commit_sha/diff_refs, got %q", ne.CR.BaseSHA)
	}
	if ne.CR.StartSHA != "" {
		t.Errorf("expected empty start SHA without diff_refs, got %q", ne.CR.StartSHA)
	}
}

func TestParseWebhookEvent_Push(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"object_kind":"push","user":{"id":1,"username":"dev","name":"Dev"},"project":{"path_with_namespace":"owner/repo"},"ref":"refs/heads/main","after":"abc123"}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "push" {
		t.Errorf("expected push, got %s", ne.Type)
	}
	if ne.Branch != "main" {
		t.Errorf("expected main, got %s", ne.Branch)
	}
}

func TestProvider_ImplementsProvider(t *testing.T) {
	var _ provider.Provider = (*gitlab.Provider)(nil)
}

func TestRetry_TriggersOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("X-Total", "0")
		writeJSON(w, []map[string]any{})
	}))
	defer srv.Close()
	p, err := provider.NewProvider(provider.Config{
		Platform:    provider.PlatformGitLab,
		BaseURL:     srv.URL,
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
