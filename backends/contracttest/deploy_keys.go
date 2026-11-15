package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// DeployKeysHarnessConfig carries fixtures for the deploy-key suite.
type DeployKeysHarnessConfig struct {
	ListResponse   string
	CreateResponse string
}

func testDeployKeysSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().DeployKeys
	switch {
	case h.DeployKeys == nil && !declared:
		t.Skipf("%s declares no DeployKeys capability", h.Name)
	case h.DeployKeys == nil:
		t.Errorf("%s declares Capabilities().DeployKeys but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides DeployKeys config but platform does not declare capability", h.Name)
	default:
		RunDeployKeysSuite(t, DeployKeysHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			ListResponse: h.DeployKeys.ListResponse, CreateResponse: h.DeployKeys.CreateResponse,
		})
	}
}

// DeployKeysHarness bundles inputs for the deploy-key suite.
type DeployKeysHarness struct {
	Name           string
	Platform       model.Platform
	NewProvider    func(t *testing.T, cfg provider.Config) provider.Provider
	ListResponse   string
	CreateResponse string
}

// RunDeployKeysSuite tests deploy-key CRUD operations.
func RunDeployKeysSuite(t *testing.T, h DeployKeysHarness) {
	newDKM := func(t *testing.T) provider.DeploymentKeyManager {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				// Page-aware: AllPages-driven backends probe page 2; serve an
				// empty terminal page so the loop stops.
				if beyondFirstPage(r) {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(h.ListResponse))
			case http.MethodPost:
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(h.CreateResponse))
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		dkm, ok := p.(provider.DeploymentKeyManager)
		if !ok {
			t.Fatalf("%s does not implement provider.DeploymentKeyManager", h.Name)
		}
		return dkm
	}

	t.Run("ListDeployKeys", func(t *testing.T) {
		dkm := newDKM(t)
		keys, err := dkm.ListDeployKeys(context.Background(), "owner", "repo")
		if err != nil {
			t.Fatalf("ListDeployKeys: %v", err)
		}
		if len(keys) == 0 {
			t.Fatal("expected at least one deploy key")
		}
	})

	t.Run("AddDeployKey", func(t *testing.T) {
		dkm := newDKM(t)
		key, err := dkm.AddDeployKey(context.Background(), "owner", "repo", provider.AddDeployKeyOptions{
			Title: "CI", Key: "ssh-rsa AAAA...",
		})
		if err != nil {
			t.Fatalf("AddDeployKey: %v", err)
		}
		if key == nil {
			t.Fatal("expected a deploy key, got nil")
		}
	})

	t.Run("DeleteDeployKey", func(t *testing.T) {
		dkm := newDKM(t)
		if err := dkm.DeleteDeployKey(context.Background(), "owner", "repo", 1); err != nil {
			t.Fatalf("DeleteDeployKey: %v", err)
		}
	})
}
