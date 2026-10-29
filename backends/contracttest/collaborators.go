package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// CollaboratorsHarnessConfig carries fixtures for the collaborator suite.
type CollaboratorsHarnessConfig struct {
	ListResponse string
	// UserLookupResponse, when set, is served for the username→ID lookup
	// GET (…/users/{username}) that platforms with ID-addressed member
	// APIs (GitLab, TencentCode) perform before add/remove mutations.
	// Empty means the platform addresses collaborators by username
	// directly and makes no lookup.
	UserLookupResponse string
}

func testCollaboratorsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Collaborators
	switch {
	case h.Collaborators == nil && !declared:
		t.Skipf("%s declares no Collaborators capability", h.Name)
	case h.Collaborators == nil:
		t.Errorf("%s declares Capabilities().Collaborators but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides Collaborators config but platform does not declare capability", h.Name)
	default:
		RunCollaboratorsSuite(t, CollaboratorsHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			ListResponse:       h.Collaborators.ListResponse,
			UserLookupResponse: h.Collaborators.UserLookupResponse,
		})
	}
}

// CollaboratorsHarness bundles inputs for the collaborator suite.
type CollaboratorsHarness struct {
	Name               string
	Platform           provider.Platform
	NewProvider        func(t *testing.T, cfg provider.Config) provider.Provider
	ListResponse       string
	UserLookupResponse string
}

// isUsernameLookup reports whether the path targets …/users/{username} with
// a non-numeric final segment — the username→ID resolution shape.
func isUsernameLookup(path string) bool {
	const usersSegment = "/users/"
	idx := strings.LastIndex(path, usersSegment)
	if idx < 0 {
		return false
	}
	name := path[idx+len(usersSegment):]
	if name == "" {
		return false
	}
	_, err := strconv.Atoi(name)
	return err != nil
}

// RunCollaboratorsSuite tests collaborator list operations.
func RunCollaboratorsSuite(t *testing.T, h CollaboratorsHarness) {
	newCM := func(t *testing.T) provider.CollaboratorManager {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				// Username→ID lookup for ID-addressed member APIs: a
				// non-numeric final segment under /users/ is the lookup
				// (GET …/users/{username}), distinct from user-by-ID reads.
				if h.UserLookupResponse != "" && isUsernameLookup(r.URL.Path) {
					_, _ = w.Write([]byte(h.UserLookupResponse))
					return
				}
				// Page-aware: AllPages-driven backends probe page 2; serve an
				// empty terminal page so the loop stops.
				if beyondFirstPage(r) {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(h.ListResponse))
			// The PUT/DELETE pair covers the permission-push family
			// (GitHub/Gitea/Forgejo/Gitee/GitCode); POST covers the
			// member-create family (GitLab/TencentCode add members via a
			// POST to the members collection, not a PUT per user).
			case http.MethodPut:
				w.WriteHeader(http.StatusNoContent)
			case http.MethodPost:
				// 201 + an object body: member-creating SDKs decode the
				// created member, so an empty body would EOF them.
				_, _ = w.Write([]byte(`{}`))
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		cm, ok := p.(provider.CollaboratorManager)
		if !ok {
			t.Fatalf("%s does not implement provider.CollaboratorManager", h.Name)
		}
		return cm
	}

	t.Run("ListCollaborators", func(t *testing.T) {
		cm := newCM(t)
		collaborators, err := cm.ListCollaborators(context.Background(), "owner", "repo")
		if err != nil {
			t.Fatalf("ListCollaborators: %v", err)
		}
		if len(collaborators) == 0 {
			t.Fatal("expected at least one collaborator")
		}
	})

	t.Run("AddCollaborator", func(t *testing.T) {
		cm := newCM(t)
		if err := cm.AddCollaborator(context.Background(), "owner", "repo", "dev", provider.AddCollaboratorOptions{Permission: "write"}); err != nil {
			t.Fatalf("AddCollaborator: %v", err)
		}
	})

	t.Run("RemoveCollaborator", func(t *testing.T) {
		cm := newCM(t)
		if err := cm.RemoveCollaborator(context.Background(), "owner", "repo", "dev"); err != nil {
			t.Fatalf("RemoveCollaborator: %v", err)
		}
	})
}
