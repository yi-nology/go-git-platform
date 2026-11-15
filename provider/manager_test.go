package provider_test

import (
	"testing"
	"time"

	_ "github.com/yi-nology/go-git-platform/backends/all"
	"github.com/yi-nology/go-git-platform/model"

	"github.com/yi-nology/go-git-platform/provider"
)

func TestNewManager(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.Len() != 0 {
		t.Errorf("expected 0 cached providers, got %d", m.Len())
	}
}

func TestManager_Get_CachesProvider(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)
	cfg := provider.Config{
		Platform: model.PlatformGitHub,
		Token:    "test-token",
	}

	p1, err := m.Get(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m.Len() != 1 {
		t.Errorf("expected 1 cached provider, got %d", m.Len())
	}

	p2, err := m.Get(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m.Len() != 1 {
		t.Errorf("expected 1 cached provider after second get, got %d", m.Len())
	}
	if p1 != p2 {
		t.Error("expected same provider instance from cache")
	}
}

func TestManager_Get_DifferentConfigs(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)

	p1, err := m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "token-1"})
	if err != nil {
		t.Fatal(err)
	}

	p2, err := m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "token-2"})
	if err != nil {
		t.Fatal(err)
	}

	if m.Len() != 2 {
		t.Errorf("expected 2 cached providers, got %d", m.Len())
	}
	if p1 == p2 {
		t.Error("expected different provider instances for different tokens")
	}
}

func TestManager_GetByURL(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)

	p, err := m.GetByURL("https://github.com/owner/repo.git", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if p.Platform() != model.PlatformGitHub {
		t.Errorf("expected GitHub, got %s", p.Platform())
	}
	if m.Len() != 1 {
		t.Errorf("expected 1 cached provider, got %d", m.Len())
	}
}

func TestManager_GetByURL_SameURL(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)

	p1, err := m.GetByURL("https://github.com/owner/repo.git", "test-token")
	if err != nil {
		t.Fatal(err)
	}

	p2, err := m.GetByURL("https://github.com/owner/repo.git", "test-token")
	if err != nil {
		t.Fatal(err)
	}

	if p1 != p2 {
		t.Error("expected same provider instance for same URL")
	}
}

func TestManager_GetByURL_InvalidURL(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)

	_, err := m.GetByURL("://invalid", "test-token")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestManager_Remove(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)
	cfg := provider.Config{Platform: model.PlatformGitHub, Token: "test-token"}

	_, err := m.Get(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m.Len() != 1 {
		t.Fatalf("expected 1, got %d", m.Len())
	}

	m.Remove(cfg)
	if m.Len() != 0 {
		t.Errorf("expected 0 after remove, got %d", m.Len())
	}
}

func TestManager_Purge(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)

	m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "token-1"})
	m.Get(provider.Config{Platform: model.PlatformGitLab, Token: "token-2"})
	if m.Len() != 2 {
		t.Fatalf("expected 2, got %d", m.Len())
	}

	m.Purge()
	if m.Len() != 0 {
		t.Errorf("expected 0 after purge, got %d", m.Len())
	}
}

func TestManager_Cleanup(t *testing.T) {
	m := provider.NewManager(50 * time.Millisecond)

	m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "test-token"})
	if m.Len() != 1 {
		t.Fatalf("expected 1, got %d", m.Len())
	}

	time.Sleep(100 * time.Millisecond)
	m.Cleanup()
	if m.Len() != 0 {
		t.Errorf("expected 0 after cleanup, got %d", m.Len())
	}
}

func TestManager_Cleanup_NoExpiry(t *testing.T) {
	m := provider.NewManager(0) // no expiry

	m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "test-token"})
	time.Sleep(10 * time.Millisecond)
	m.Cleanup()
	if m.Len() != 1 {
		t.Errorf("expected 1 (no expiry), got %d", m.Len())
	}
}

func TestManager_Get_UnsupportedPlatform(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)

	_, err := m.Get(provider.Config{Platform: model.Platform("unsupported")})
	if err == nil {
		t.Error("expected error for unsupported platform")
	}
	if m.Len() != 0 {
		t.Errorf("expected 0 cached providers after error, got %d", m.Len())
	}
}
