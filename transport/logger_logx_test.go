package transport

import "testing"

// The lazy-resolution guarantee (following logx.SetDefault) is logx's own
// tested feature; here we pin the bridge contract: non-nil and all four
// levels callable without any consumer-installed backend.
func TestNewLogxLoggerSmoke(t *testing.T) {
	l := NewLogxLogger("transport-test")
	if l == nil {
		t.Fatal("NewLogxLogger returned nil")
	}
	l.Debug("d", "k", "v")
	l.Info("i", "k", "v")
	l.Warn("w", "k", "v")
	l.Error("e", "k", "v")
}
