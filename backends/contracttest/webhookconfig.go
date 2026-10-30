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

// WebhookConfigHarnessConfig carries fixtures for the webhook-config suite
// (single-webhook get/update on top of the core WebhookManager).
type WebhookConfigHarnessConfig struct {
	// GetResponse is one webhook in the platform's native wire shape. It is
	// served for both the GET (get + family read-back) and the update call,
	// so converters must be able to decode it — e.g. GitHub/GitCode
	// `{"id":42,"url":"…","events":["push"]}`, GitLab/Gitee/TencentCode
	// boolean-flag bodies, Gitea-family `{"id":42,"config":{"url":"…"}, …}`.
	GetResponse string
}

func testWebhookConfigSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().WebhookConfig
	switch {
	case h.WebhookConfig == nil && !declared:
		t.Skipf("%s declares no WebhookConfig capability", h.Name)
	case h.WebhookConfig == nil:
		t.Errorf("%s declares Capabilities().WebhookConfig but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides WebhookConfig config but platform does not declare capability", h.Name)
	default:
		RunWebhookConfigSuite(t, WebhookConfigHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			GetResponse: h.WebhookConfig.GetResponse,
		})
	}
}

// WebhookConfigHarness bundles inputs for the webhook-config suite.
type WebhookConfigHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	GetResponse string
}

// isAddressedHook reports whether the request path addresses a single
// webhook resource: the final path segment is the hook ID.
func isAddressedHook(path string, hookID int64) bool {
	seg := path[strings.LastIndex(path, "/")+1:]
	id, err := strconv.ParseInt(seg, 10, 64)
	return err == nil && id == hookID
}

// newWebhookConfigManager mounts a provider over a mock that serves the
// fixture for every request addressing the single webhook resource (GET +
// the PATCH/PUT update verbs of the seven backends) and 404s everything
// else. Routing on the addressed resource means platform verb/path drift
// cannot mask a wrong verb or a wrong hook ID.
func newWebhookConfigManager(t *testing.T, h WebhookConfigHarness) provider.WebhookConfigManager {
	const hookID = int64(42)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if (r.Method == http.MethodGet || r.Method == http.MethodPatch || r.Method == http.MethodPut) &&
			isAddressedHook(r.URL.Path, hookID) {
			_, _ = w.Write([]byte(h.GetResponse))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
	cm, ok := p.(provider.WebhookConfigManager)
	if !ok {
		t.Fatalf("%s does not implement provider.WebhookConfigManager", h.Name)
	}
	return cm
}

// RunWebhookConfigSuite tests single-webhook get/update.
func RunWebhookConfigSuite(t *testing.T, h WebhookConfigHarness) {
	const hookID = int64(42)

	t.Run("GetWebhook", func(t *testing.T) {
		cm := newWebhookConfigManager(t, h)
		wh, err := cm.GetWebhook(context.Background(), "owner", "repo", hookID)
		if err != nil {
			t.Fatalf("GetWebhook: %v", err)
		}
		if wh == nil || wh.ID != hookID || wh.URL == "" || len(wh.Events) == 0 {
			t.Fatalf("GetWebhook mapped incompletely: %+v", wh)
		}
	})

	t.Run("UpdateWebhook", func(t *testing.T) {
		cm := newWebhookConfigManager(t, h)
		url := "https://hooks.example/ci"
		wh, err := cm.UpdateWebhook(context.Background(), "owner", "repo", hookID,
			provider.UpdateWebhookOptions{URL: &url, Events: []string{"push", "pull_request"}})
		if err != nil {
			t.Fatalf("UpdateWebhook: %v", err)
		}
		if wh == nil || wh.ID != hookID {
			t.Fatalf("UpdateWebhook returned wrong hook: %+v", wh)
		}
	})

	t.Run("GetWebhook_NotFound", func(t *testing.T) {
		cm := newWebhookConfigManager(t, h)
		_, err := cm.GetWebhook(context.Background(), "owner", "repo", hookID+1)
		if err == nil {
			t.Fatal("expected an error for an unaddressed hook ID")
		}
		if !provider.IsNotFound(err) {
			t.Fatalf("expected ErrNotFound classification, got: %v", err)
		}
	})
}
