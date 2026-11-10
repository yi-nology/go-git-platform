package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// NotificationsHarnessConfig carries the fixtures a backend's main Harness
// needs to auto-mount the notification suite via Harness.Notifications.
type NotificationsHarnessConfig struct {
	// ListResponse is the JSON array the mock returns for notification list
	// requests. It must contain at least one notification with unread=true.
	ListResponse string
	// GlobalUnsupported declares that the platform serves only the
	// repo-scoped half of NotificationManager: the global methods
	// (ListNotifications / MarkNotificationRead / MarkNotificationsRead)
	// must then fail with ErrNotImplemented — the interface couples the
	// global and repo halves, and some platforms (GitCode, live-verified
	// 2026-11-03) have no global inbox. When set, the suite also asserts
	// the repo-scoped methods stay functional.
	GlobalUnsupported bool
}

// testNotificationsSuite auto-mounts RunNotificationsSuite from a main
// Harness, enforcing bidirectional drift checks.
func testNotificationsSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Notifications
	switch {
	case h.Notifications == nil && !declared:
		t.Skipf("%s declares no Notifications capability", h.Name)
	case h.Notifications == nil:
		t.Errorf("%s declares Capabilities().Notifications but its Harness provides no Notifications config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides a Notifications config but the platform does not declare Capabilities().Notifications", h.Name)
	default:
		RunNotificationsSuite(t, NotificationsHarness{
			Name:              h.Name,
			Platform:          h.Platform,
			NewProvider:       h.NewProvider,
			ListResponse:      h.Notifications.ListResponse,
			GlobalUnsupported: h.Notifications.GlobalUnsupported,
		})
	}
}

// NotificationsHarness bundles the inputs needed to run the notification
// contract suite against a backend that implements provider.NotificationManager.
type NotificationsHarness struct {
	Name              string
	Platform          provider.Platform
	NewProvider       func(t *testing.T, cfg provider.Config) provider.Provider
	ListResponse      string
	GlobalUnsupported bool
}

// RunNotificationsSuite executes the notification contract suite.
func RunNotificationsSuite(t *testing.T, h NotificationsHarness) {
	newNM := func(t *testing.T) provider.NotificationManager {
		srv := notificationStubServer(h.ListResponse)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		nm, ok := p.(provider.NotificationManager)
		if !ok {
			t.Fatalf("%s does not implement provider.NotificationManager", h.Name)
		}
		return nm
	}

	if h.GlobalUnsupported {
		runGlobalUnsupportedSuite(t, newNM)
		return
	}

	// Page==0 dual-mode contract (issue #60): notifications are an
	// exhaustive-family list — zero Page walks every page, Page>0 serves
	// exactly that page. The terminal empty page mirrors the fixture's
	// top-level shape (array vs wrapped object).
	runPageZeroDualModeSuite(t, h.NewProvider, PageZeroFixture{
		Platform:  h.Platform,
		ListBody:  h.ListResponse,
		EmptyBody: emptyNotificationPage(h.ListResponse),
		ListZero: func(p provider.Provider) error {
			_, err := p.(provider.NotificationManager).ListNotifications(context.Background(), provider.ListNotificationsOptions{})
			return err
		},
		ListPageOne: func(p provider.Provider) error {
			_, err := p.(provider.NotificationManager).ListNotifications(context.Background(), provider.ListNotificationsOptions{Page: 1, PerPage: 10})
			return err
		},
	})

	t.Run("ListNotifications_MalformedSince_Rejected", func(t *testing.T) {
		// issue #52: the SAME malformed timestamp must produce the same
		// outcome on every backend — an ErrInvalidInput rejection, never a
		// silently unfiltered (full stream) result with err == nil.
		nm := newNM(t)
		_, err := nm.ListNotifications(context.Background(), provider.ListNotificationsOptions{Since: "2026-01-01"})
		if err == nil {
			t.Fatal("malformed Since must error, got nil — the filter was silently dropped")
		}
		if !provider.IsInvalidInput(err) {
			t.Fatalf("malformed Since = %v, want an error wrapping ErrInvalidInput", err)
		}
	})

	t.Run("ListNotifications_ReturnsResults", func(t *testing.T) {
		nm := newNM(t)
		notifications, err := nm.ListNotifications(context.Background(), provider.ListNotificationsOptions{})
		if err != nil {
			t.Fatalf("ListNotifications: %v", err)
		}
		if len(notifications) == 0 {
			t.Fatal("expected at least one notification")
		}
		if notifications[0].ID == "" {
			t.Error("expected notification ID to be non-empty")
		}
		if notifications[0].Subject.Title == "" {
			t.Error("expected notification subject title to be non-empty")
		}
	})

	t.Run("MarkNotificationRead_Succeeds", func(t *testing.T) {
		nm := newNM(t)
		if err := nm.MarkNotificationRead(context.Background(), "1"); err != nil {
			t.Fatalf("MarkNotificationRead: %v", err)
		}
	})

	t.Run("MarkNotificationsRead_Succeeds", func(t *testing.T) {
		nm := newNM(t)
		if err := nm.MarkNotificationsRead(context.Background(), provider.MarkNotificationsOptions{}); err != nil {
			t.Fatalf("MarkNotificationsRead: %v", err)
		}
	})
}

// runGlobalUnsupportedSuite pins the partial-capability contract for
// platforms whose inbox is repo-scoped only (the provider interface
// couples the global and repo halves): the global methods must fail with
// ErrNotImplemented, the repo-scoped methods must stay functional.
func runGlobalUnsupportedSuite(t *testing.T, newNM func(*testing.T) provider.NotificationManager) {
	t.Run("GlobalMethods_NotImplemented", func(t *testing.T) {
		nm := newNM(t)
		_, listErr := nm.ListNotifications(context.Background(), provider.ListNotificationsOptions{})
		if !provider.IsNotImplemented(listErr) {
			t.Errorf("ListNotifications err = %v, want ErrNotImplemented", listErr)
		}
		if err := nm.MarkNotificationRead(context.Background(), "1"); !provider.IsNotImplemented(err) {
			t.Errorf("MarkNotificationRead err = %v, want ErrNotImplemented", err)
		}
		if err := nm.MarkNotificationsRead(context.Background(), provider.MarkNotificationsOptions{}); !provider.IsNotImplemented(err) {
			t.Errorf("MarkNotificationsRead err = %v, want ErrNotImplemented", err)
		}
	})
	t.Run("RepoScoped_Succeeds", func(t *testing.T) {
		nm := newNM(t)
		notifications, err := nm.ListRepoNotifications(context.Background(), "owner", "repo", provider.ListNotificationsOptions{})
		if err != nil {
			t.Fatalf("ListRepoNotifications: %v", err)
		}
		if len(notifications) == 0 || notifications[0].ID == "" {
			t.Fatalf("repo-scoped list incomplete: %+v", notifications)
		}
		if err := nm.MarkRepoNotificationsRead(context.Background(), "owner", "repo", provider.MarkNotificationsOptions{}); err != nil {
			t.Errorf("MarkRepoNotificationsRead: %v", err)
		}
	})
	t.Run("RepoScoped_MalformedSince_Rejected", func(t *testing.T) {
		// issue #52: the same malformed timestamp on the repo-scoped half —
		// ErrInvalidInput, never a silently unfiltered stream.
		nm := newNM(t)
		_, err := nm.ListRepoNotifications(context.Background(), "owner", "repo", provider.ListNotificationsOptions{Since: "2026-01-01"})
		if err == nil || !provider.IsInvalidInput(err) {
			t.Fatalf("malformed Since = %v, want an error wrapping ErrInvalidInput", err)
		}
	})
}

// emptyNotificationPage renders the terminal empty page in the same
// top-level JSON shape as listResponse: an object fixture yields an empty
// wrapped object (total_count 0, empty list), an array fixture yields [].
func emptyNotificationPage(listResponse string) string {
	if len(listResponse) > 0 && listResponse[0] == '{' {
		return `{"total_count":0,"list":[]}`
	}
	return `[]`
}

// notificationStubServer returns a mock server for the notification suite.
func notificationStubServer(listResponse string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			// Page-aware: AllPages-driven backends probe page 2; serve an
			// empty terminal page so the loop stops. The empty page keeps
			// the first page's top-level shape (array vs wrapped object) so
			// shape-sensitive raw-HTTP backends (gitee) parse it too.
			if beyondFirstPage(r) {
				_, _ = w.Write([]byte(emptyNotificationPage(listResponse)))
				return
			}
			_, _ = w.Write([]byte(listResponse))
		case http.MethodPost, http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}
