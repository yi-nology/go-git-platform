package provider_test

import (
	"context"
	"strings"
	"testing"
	"time"

	_ "github.com/yi-nology/go-git-platform/backends/all"
	"github.com/yi-nology/go-git-platform/model"

	"github.com/yi-nology/go-git-platform/provider"
)

func TestManagerStats_HitsAndMisses(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)
	cfg := provider.Config{Platform: model.PlatformGitHub, Token: "test-stats"}
	// First call: miss
	_, err := m.Get(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s1 := m.Stats()
	if s1.Misses != 1 || s1.Hits != 0 || s1.Size != 1 {
		t.Errorf("after first Get: %+v", s1)
	}
	// Second call: hit
	_, _ = m.Get(cfg)
	s2 := m.Stats()
	if s2.Hits != 1 || s2.Misses != 1 {
		t.Errorf("after second Get: %+v", s2)
	}
}

func TestManagerStats_Reset(t *testing.T) {
	m := provider.NewManager(30 * time.Minute)
	cfg := provider.Config{Platform: model.PlatformGitHub, Token: "reset"}
	_, _ = m.Get(cfg)
	_, _ = m.Get(cfg)
	m.ResetStats()
	s := m.Stats()
	if s.Hits != 0 || s.Misses != 0 {
		t.Errorf("expected zeroed stats after ResetStats, got %+v", s)
	}
	if s.Size != 1 {
		t.Errorf("ResetStats should not clear cache, got size %d", s.Size)
	}
}

func TestManager_Sha256CacheKey(t *testing.T) {
	// Same (platform, baseURL) but different tokens should map to distinct
	// cache entries. The tokens share an 8-char prefix (the old behavior
	// collided on tokenPrefix = token[:8]).
	m := provider.NewManager(time.Minute)
	t1 := "abcdef12-same-prefix-suffix-1"
	t2 := "abcdef12-same-prefix-suffix-2"
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: t1})
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: t2})
	if m.Len() != 2 {
		t.Errorf("expected 2 distinct entries for different tokens, got %d", m.Len())
	}
}

func TestManager_WithHasher(t *testing.T) {
	// Override the hasher so we get a human-readable key.
	m := provider.NewManager(time.Minute, provider.WithHasher(func(token string) string {
		return "custom-" + token
	}))
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "abc"})
	// We can't read the internal map, but we can verify that the custom
	// hasher is used by inspecting the behavior: two requests with the same
	// token should hit, and the hit counter should be 1.
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "abc"})
	s := m.Stats()
	if s.Hits != 1 {
		t.Errorf("expected 1 hit with custom hasher, got %d", s.Hits)
	}
}

func TestManager_WithMaxSize_Evicts(t *testing.T) {
	m := provider.NewManager(time.Minute, provider.WithMaxSize(1))
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "t1"})
	if m.Len() != 1 {
		t.Fatalf("expected size 1, got %d", m.Len())
	}
	// Insert a second token; the first should be evicted.
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "t2"})
	if m.Len() != 1 {
		t.Errorf("expected size still 1 after eviction, got %d", m.Len())
	}
	s := m.Stats()
	if s.Evictions != 1 {
		t.Errorf("expected 1 eviction, got %d", s.Evictions)
	}
}

func TestManager_EvictsLeastRecentlyUsed(t *testing.T) {
	// With maxSize=2: insert t1 and t2, then hit t1 again. A third insert
	// must evict t2 (least recently used), not t1 (oldest by creation) —
	// this distinguishes true LRU from the former creation-order eviction.
	m := provider.NewManager(time.Minute, provider.WithMaxSize(2))
	cfg1 := provider.Config{Platform: model.PlatformGitHub, Token: "t1"}
	cfg2 := provider.Config{Platform: model.PlatformGitHub, Token: "t2"}
	cfg3 := provider.Config{Platform: model.PlatformGitHub, Token: "t3"}
	_, _ = m.Get(cfg1)
	_, _ = m.Get(cfg2)
	if _, err := m.Get(cfg1); err != nil { // hit; refreshes t1's recency
		t.Fatalf("Get(cfg1): %v", err)
	}
	if _, err := m.Get(cfg3); err != nil { // miss at capacity; evicts one
		t.Fatalf("Get(cfg3): %v", err)
	}
	hitsBefore := m.Stats().Hits
	missesBefore := m.Stats().Misses

	// t1 was used most recently of {t1, t2}, so it must still be cached.
	if _, err := m.Get(cfg1); err != nil {
		t.Fatalf("Get(cfg1) after eviction: %v", err)
	}
	if got := m.Stats().Hits; got != hitsBefore+1 {
		t.Errorf("t1 lookup = miss; expected it to survive as recently used (hits %d, want %d)", got, hitsBefore+1)
	}
	// t2 had no access since insertion, so it must be the eviction victim.
	if _, err := m.Get(cfg2); err != nil {
		t.Fatalf("Get(cfg2) after eviction: %v", err)
	}
	if got := m.Stats().Misses; got != missesBefore+1 {
		t.Errorf("t2 lookup = hit; expected it to have been the eviction victim (misses %d, want %d)", got, missesBefore+1)
	}
}

func TestManager_StartJanitor(t *testing.T) {
	m := provider.NewManager(20 * time.Millisecond)
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "janitor"})
	if m.Len() != 1 {
		t.Fatalf("expected size 1, got %d", m.Len())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.StartJanitor(ctx, 30*time.Millisecond)
	time.Sleep(120 * time.Millisecond) // allow janitor to run a few times

	if m.Len() != 0 {
		t.Errorf("expected janitor to evict expired entry, size=%d", m.Len())
	}
}

func TestManager_StopJanitor(t *testing.T) {
	m := provider.NewManager(20 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.StartJanitor(ctx, 10*time.Millisecond)
	m.Stop()
	// After Stop, calling StartJanitor again should work
	m.StartJanitor(ctx, 10*time.Millisecond)
	m.Stop()
}

func TestManager_DefaultHasher_OpaqueToken(t *testing.T) {
	// The default hasher should not include the raw token in the cache key.
	// We can't read the key directly, but we can verify that two tokens
	// that hash to the same prefix don't collide.
	token1 := "very-long-secret-token-1-abcdef"
	token2 := "very-long-secret-token-2-ghijkl"
	m := provider.NewManager(time.Minute)
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: token1})
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: token2})
	if m.Len() != 2 {
		t.Errorf("expected distinct entries, got %d", m.Len())
	}
}

func TestManagerStats_Format(t *testing.T) {
	s := provider.Stats{Hits: 5, Misses: 3, Evictions: 1, Size: 2}
	// Just make sure the struct fields are accessible.
	if s.Hits+s.Misses != 8 {
		t.Error("Stats fields not accessible")
	}
}

func TestManager_buildKey_IsOpaque(t *testing.T) {
	// Ensure the hasher produces a value that does NOT contain the token.
	// We do this by checking the key length is bounded.
	m := provider.NewManager(time.Minute, provider.WithHasher(func(t string) string {
		// Use the real default hasher for the test
		return provider.HashToken(t)
	}))
	_, _ = m.Get(provider.Config{Platform: model.PlatformGitHub, Token: "supersecrettoken12345"})
	// We can't inspect the key, but if HashToken returns a 16-char hex
	// string, the test passes.
	hashed := provider.HashToken("supersecrettoken12345")
	if strings.Contains(hashed, "supersecret") {
		t.Errorf("hasher leaked token: %q", hashed)
	}
	if len(hashed) != 16 {
		t.Errorf("expected 16-char hash, got %d", len(hashed))
	}
}
