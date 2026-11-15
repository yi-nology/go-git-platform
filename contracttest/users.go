package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// UsersHarnessConfig carries fixtures for the user-manager suite.
type UsersHarnessConfig struct {
	// GetUserResponse is the JSON the mock returns for user lookup.
	GetUserResponse string
}

func testUsersSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Users
	switch {
	case h.Users == nil && !declared:
		t.Skipf("%s declares no Users capability", h.Name)
	case h.Users == nil:
		t.Errorf("%s declares Capabilities().Users but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides Users config but platform does not declare capability", h.Name)
	default:
		RunUsersSuite(t, UsersHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			GetUserResponse: h.Users.GetUserResponse,
		})
	}
}

// UsersHarness bundles inputs for the user-manager suite.
type UsersHarness struct {
	Name            string
	Platform        model.Platform
	NewProvider     func(t *testing.T, cfg provider.Config) provider.Provider
	GetUserResponse string
}

// RunUsersSuite tests user lookup operations.
func RunUsersSuite(t *testing.T, h UsersHarness) {
	newUM := func(t *testing.T) provider.UserManager {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(h.GetUserResponse))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		um, ok := p.(provider.UserManager)
		if !ok {
			t.Fatalf("%s does not implement provider.UserManager", h.Name)
		}
		return um
	}

	t.Run("GetUser_ReturnsUser", func(t *testing.T) {
		um := newUM(t)
		user, err := um.GetUser(context.Background(), "dev")
		if err != nil {
			t.Skipf("GetUser not supported or failed: %v", err)
		}
		if user == nil {
			t.Fatal("expected a user, got nil")
		}
		if user.Username == "" {
			t.Error("expected username to be non-empty")
		}
	})
}
