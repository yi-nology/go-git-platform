package giteafamily

import (
	"context"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

type fakeRepoPort struct {
	orgOwner, userOwner string
	orgStatus, userSts  int
	orgRepos, userRepos []*Repo
	orgErr, userErr     error
	orgPage, userPage   ListOptions
	searchCalled        int
}

func (f *fakeRepoPort) SearchRepos(context.Context, SearchRepoOptions) ([]*Repo, error) {
	f.searchCalled++
	return nil, nil
}
func (f *fakeRepoPort) ListMyRepos(context.Context, FamilyListReposOptions) ([]*Repo, error) {
	return nil, nil
}
func (f *fakeRepoPort) ListOrgRepos(_ context.Context, org string, opt FamilyListReposOptions) ([]*Repo, int, error) {
	f.orgOwner, f.orgPage = org, opt.ListOptions
	return f.orgRepos, f.orgStatus, f.orgErr
}
func (f *fakeRepoPort) ListUserRepos(_ context.Context, user string, opt FamilyListReposOptions) ([]*Repo, int, error) {
	f.userOwner, f.userPage = user, opt.ListOptions
	return f.userRepos, f.userSts, f.userErr
}
func (f *fakeRepoPort) GetRepo(context.Context, string, string) (*Repo, int, error) {
	return nil, 0, nil
}
func (f *fakeRepoPort) CreateRepo(context.Context, FamilyCreateRepoOption) (*Repo, error) {
	return nil, nil
}
func (f *fakeRepoPort) CreateOrgRepo(context.Context, string, FamilyCreateRepoOption) (*Repo, error) {
	return nil, nil
}
func (f *fakeRepoPort) CreateFork(context.Context, string, string, FamilyForkRepoOption) (*Repo, error) {
	return nil, nil
}
func (f *fakeRepoPort) DeleteRepo(context.Context, string, string) error { return nil }
func (f *fakeRepoPort) EditRepo(context.Context, string, string, FamilyEditRepoOption) (*Repo, error) {
	return nil, nil
}

// TestListRepos_OwnerTargetsNamespaceEndpoints pins the per-owner listing
// fix (issue #92): an Owner drives the SDK's org/user listing endpoints
// (giteafamily has them both) — never the old "empty-keyword global search
// plus client-side owner filter", which silently dropped every owner repo
// that didn't sit on the globally-Nth search page and misnumbered pages.
// The org endpoint 404s for personal logins, so the engine falls back to
// the user endpoint (github's #80 shape).
func TestListRepos_OwnerTargetsNamespaceEndpoints(t *testing.T) {
	fam := Family{Platform: provider.PlatformForgejo}

	t.Run("org hit", func(t *testing.T) {
		port := &fakeRepoPort{
			orgRepos: []*Repo{{ID: 1, Name: "r1"}, {ID: 2, Name: "r2"}},
		}
		repos, err := ListRepos(context.Background(), fam, port, provider.ListRepoOptions{Owner: "some-org"})
		if err != nil {
			t.Fatalf("ListRepos: %v", err)
		}
		if len(repos) != 2 {
			t.Fatalf("owner repos = %d, want 2", len(repos))
		}
		if port.orgOwner != "some-org" || port.userOwner != "" {
			t.Fatalf("org endpoint must serve the owner (user endpoint untouched), org=%q user=%q", port.orgOwner, port.userOwner)
		}
		if port.searchCalled != 0 {
			t.Fatal("global search must not run for an Owner listing")
		}
		if port.orgPage.Page != 1 || port.orgPage.PageSize != 20 {
			t.Errorf("caller-paged contract: NormalizePageOpts default = page 1 / size 20, got %+v", port.orgPage)
		}
	})

	t.Run("404 falls back to user", func(t *testing.T) {
		port := &fakeRepoPort{
			orgStatus: 404,
			orgErr:    errorsNotFound404(),
			userRepos: []*Repo{{ID: 5, Name: "personal"}},
		}
		repos, err := ListRepos(context.Background(), fam, port, provider.ListRepoOptions{Owner: "a-person"})
		if err != nil {
			t.Fatalf("user fallback: %v", err)
		}
		if len(repos) != 1 || repos[0].ID != 5 {
			t.Fatalf("user repos = %+v, want the personal listing", repos)
		}
		if port.userOwner != "a-person" {
			t.Fatalf("fallback must carry the same login, got %q", port.userOwner)
		}
		if port.searchCalled != 0 {
			t.Fatal("global search must not run for an Owner listing")
		}
	})

	t.Run("non-404 org error surfaces", func(t *testing.T) {
		port := &fakeRepoPort{
			orgStatus: 500,
			orgErr:    errorsInternal500(),
		}
		if _, err := ListRepos(context.Background(), fam, port, provider.ListRepoOptions{Owner: "org"}); err == nil {
			t.Fatal("a server error must surface, not fall back")
		}
		if port.userOwner != "" {
			t.Fatal("only 404 may fall back to the user endpoint")
		}
	})
}

// errorsNotFound404/500 build status-coded causes so familyError classifies
// them exactly like the adapters' WrapStatusError path.
func errorsNotFound404() error {
	return provider.WrapStatusError(errText("404 Not Found"), 404)
}

func errorsInternal500() error {
	return provider.WrapStatusError(errText("500 boom"), 500)
}

type errText string

func (e errText) Error() string { return string(e) }
