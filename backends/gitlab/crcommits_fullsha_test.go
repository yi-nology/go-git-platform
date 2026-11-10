package gitlab_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitLab_ListCRCommits_FullSHA pins issue #82: CRCommit.SHA carries the
// full 40-hex commit SHA (gitlab Commit.ID), matching every other backend
// and this backend's own ListCommits — the mapping used to read ShortID
// (7 hex), breaking joins and dedup keys across backends.
func TestGitLab_ListCRCommits_FullSHA(t *testing.T) {
	const (
		fullSHA  = "5a3c8e0f1b2d4a6c8e0f1b2d4a6c8e0f1b2d4a6c"
		shortSHA = "5a3c8e0"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"` + fullSHA + `","short_id":"` + shortSHA + `","title":"fix","created_at":"2026-11-01T00:00:00Z","author_name":"dev"}]`))
	}))
	defer srv.Close()
	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitLab, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	commits, err := p.ListCRCommits(context.Background(), "owner", "repo", "7")
	if err != nil {
		t.Fatalf("ListCRCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0].SHA != fullSHA {
		t.Errorf("CRCommit.SHA = %q, want full SHA %q (not the %q short form)", commits[0].SHA, fullSHA, shortSHA)
	}
	if ok, _ := regexp.MatchString(`^[0-9a-f]{40}$`, commits[0].SHA); !ok {
		t.Errorf("CRCommit.SHA = %q, want the 40-hex full-SHA shape", commits[0].SHA)
	}
}

// TestGitLab_MarkRepoNotificationsRead_NotImplemented pins issue #83: the
// stub must fail with the ErrNotImplemented sentinel (errors.Is, not a
// message-text match) so IsNotImplemented classifies it — callers can tell
// "platform cannot serve this" from a real failure.
func TestGitLab_MarkRepoNotificationsRead_NotImplemented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("stub must touch no wire, got %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitLab, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	nm := p.(provider.NotificationManager)
	err = nm.MarkRepoNotificationsRead(context.Background(), "owner", "repo", provider.MarkNotificationsOptions{})
	if err == nil {
		t.Fatal("expected ErrNotImplemented, got nil")
	}
	if !errors.Is(err, provider.ErrNotImplemented) {
		t.Fatalf("errors.Is(err, ErrNotImplemented) = false, got %v", err)
	}
	if !provider.IsNotImplemented(err) {
		t.Fatalf("provider.IsNotImplemented(err) = false, got %v", err)
	}
}
