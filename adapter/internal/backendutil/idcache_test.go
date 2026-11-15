package backendutil

import (
	"testing"
	"time"
)

func TestIDCachePutGet(t *testing.T) {
	c := NewIDCache(time.Minute)
	if _, ok := c.Get("k"); ok {
		t.Fatal("empty cache returned a hit")
	}
	c.Put("k", 42)
	if id, ok := c.Get("k"); !ok || id != 42 {
		t.Fatalf("Get = (%d, %v), want (42, true)", id, ok)
	}
}

func TestIDCacheTTLExpiry(t *testing.T) {
	c := NewIDCache(time.Nanosecond)
	c.Put("k", 42)
	time.Sleep(time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expired entry returned a hit")
	}
}
