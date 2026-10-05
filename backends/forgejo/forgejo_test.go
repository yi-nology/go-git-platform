package forgejo_test

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

	forgejosdk "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/backends/forgejo"
	"github.com/yi-nology/go-git-platform/provider"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newTestProvider(t *testing.T, srv *httptest.Server) *forgejo.Provider {
	t.Helper()
	// The Forgejo SDK calls GET /api/v1/version on NewClient; serve it.
	versionSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			writeJSON(w, map[string]string{"version": "8.0.0"})
			return
		}
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	pp, err := forgejo.New(provider.Config{
		Platform: provider.PlatformForgejo,
		BaseURL:  versionSrv.URL,
		Token:    "test-token",
	})
	if err != nil {
		t.Fatalf("forgejo.New: %v", err)
	}
	gp, ok := pp.(*forgejo.Provider)
	if !ok {
		t.Fatalf("expected *forgejo.Provider, got %T", pp)
	}
	return gp
}

func TestListRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []*forgejosdk.Repository{
			{ID: 1, FullName: "owner/r1", Name: "r1", Owner: &forgejosdk.User{UserName: "owner"}, DefaultBranch: "main"},
			{ID: 2, FullName: "owner/r2", Name: "r2", Owner: &forgejosdk.User{UserName: "owner"}, DefaultBranch: "main", Private: true},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2, got %d", len(repos))
	}
	if repos[0].Platform != provider.PlatformForgejo {
		t.Errorf("expected Gitea, got %s", repos[0].Platform)
	}
}

func TestGetRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, forgejosdk.Repository{
			ID: 42, FullName: "owner/repo", Name: "repo",
			Owner: &forgejosdk.User{UserName: "owner"}, DefaultBranch: "main",
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
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
		writeJSON(w, forgejosdk.PullRequest{
			ID: 7, Index: 7, Title: "test", State: forgejosdk.StateOpen,
			Head:   &forgejosdk.PRBranchInfo{Ref: "feature", Sha: "abc"},
			Base:   &forgejosdk.PRBranchInfo{Ref: "main"},
			Poster: &forgejosdk.User{ID: 1, UserName: "dev"},
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

func TestListCRs_MergedDetected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []*forgejosdk.PullRequest{
			{ID: 1, Index: 1, Title: "a", State: forgejosdk.StateOpen,
				Head: &forgejosdk.PRBranchInfo{Ref: "a"}, Base: &forgejosdk.PRBranchInfo{Ref: "main"}},
			{ID: 2, Index: 2, Title: "b", State: forgejosdk.StateClosed, HasMerged: true,
				Head: &forgejosdk.PRBranchInfo{Ref: "b"}, Base: &forgejosdk.PRBranchInfo{Ref: "main"}},
		})
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	crs, _, err := p.ListCRs(context.Background(), provider.ListCROptions{Owner: "owner", Repo: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if crs[1].State != provider.CRStateMerged {
		t.Errorf("expected merged, got %s", crs[1].State)
	}
}

func TestListBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Page-aware: ListBranches exhausts pagination, so page 2 must be
		// the empty terminal page.
		if page := r.URL.Query().Get("page"); page != "" && page != "1" {
			writeJSON(w, []*forgejosdk.Branch{})
			return
		}
		writeJSON(w, []*forgejosdk.Branch{{Name: "main"}, {Name: "dev"}})
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

func TestParseWebhookEvent_PullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"action":"opened","number":1,"sender":{"id":1,"login":"dev"},"repository":{"full_name":"owner/repo"},"pull_request":{"number":1,"title":"t","state":"open","head":{"ref":"f","sha":"abc"},"base":{"ref":"main"},"html_url":"https://codeberg.org/owner/repo/pulls/1","user":{"id":1,"login":"dev"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-Forgejo-Event", "pull_request")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "cr.opened" {
		t.Errorf("expected cr.opened, got %s", ne.Type)
	}
	if ne.CR == nil || ne.CR.Number != "1" {
		t.Errorf("expected CR with number 1, got %+v", ne.CR)
	}
}

func TestParseWebhookEvent_Merged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"action":"closed","number":1,"sender":{"id":1,"login":"dev"},"repository":{"full_name":"owner/repo"},"pull_request":{"number":1,"title":"t","state":"closed","merged":true,"head":{"ref":"f","sha":"abc"},"base":{"ref":"main"},"html_url":"https://codeberg.org/owner/repo/pulls/1","user":{"id":1,"login":"dev"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-Forgejo-Event", "pull_request")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "cr.merged" {
		t.Errorf("expected cr.merged, got %s", ne.Type)
	}
	if ne.CR == nil || ne.CR.Number != "1" {
		t.Errorf("expected CR with number 1, got %+v", ne.CR)
	}
}

func TestParseWebhookEvent_PRClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"action":"closed","number":2,"sender":{"id":1,"login":"dev"},"repository":{"full_name":"owner/repo"},"pull_request":{"number":2,"title":"t","state":"closed","merged":false,"head":{"ref":"f","sha":"abc"},"base":{"ref":"main"},"html_url":"https://codeberg.org/owner/repo/pulls/2","user":{"id":1,"login":"dev"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-Forgejo-Event", "pull_request")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "cr.closed" {
		t.Errorf("expected cr.closed, got %s", ne.Type)
	}
}

func TestParseWebhookEvent_Push(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"ref":"refs/heads/main","after":"abc123","sender":{"id":1,"login":"dev"},"repository":{"full_name":"owner/repo"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-Forgejo-Event", "push")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "push" {
		t.Errorf("expected push, got %s", ne.Type)
	}
	if ne.Branch != "main" {
		t.Errorf("expected main, got %s", ne.Branch)
	}
	if ne.CommitSHA != "abc123" {
		t.Errorf("expected abc123, got %s", ne.CommitSHA)
	}
	if ne.Repo == nil || ne.Repo.FullName != "owner/repo" {
		t.Errorf("expected repo owner/repo, got %+v", ne.Repo)
	}
}

func TestParseWebhookEvent_BranchCreated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"ref":"feature","sender":{"id":1,"login":"dev"},"repository":{"full_name":"owner/repo"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-Forgejo-Event", "create")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "branch.created" {
		t.Errorf("expected branch.created, got %s", ne.Type)
	}
	if ne.Branch != "feature" {
		t.Errorf("expected feature, got %s", ne.Branch)
	}
}

func TestParseWebhookEvent_BranchDeleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	body := `{"ref":"feature","sender":{"id":1,"login":"dev"},"repository":{"full_name":"owner/repo"}}`
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set("X-Forgejo-Event", "delete")
	r.Header.Set("Content-Type", "application/json")
	ne := contracttest.ParseSigned(t, p, r, "corpus-secret")
	if ne.Type != "branch.deleted" {
		t.Errorf("expected branch.deleted, got %s", ne.Type)
	}
	if ne.Branch != "feature" {
		t.Errorf("expected feature, got %s", ne.Branch)
	}
}

func TestProvider_ImplementsProvider(t *testing.T) {
	var _ provider.Provider = (*forgejo.Provider)(nil)
}

func TestRetry_TriggersOn5xx(t *testing.T) {
	var calls int32
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, []*forgejosdk.Repository{})
	}))
	defer inner.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			writeJSON(w, map[string]string{"version": "8.0.0"})
			return
		}
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	p, err := forgejo.New(provider.Config{
		Platform:    provider.PlatformForgejo,
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
