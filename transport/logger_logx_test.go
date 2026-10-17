package transport

import (
	"log/slog"
	"os"
	"testing"

	"github.com/oekit/logx"
)

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

func TestDebugEnabled(t *testing.T) {
	// nil never emits.
	if DebugEnabled(nil) {
		t.Fatal("nil logger must not be debug-enabled")
	}
	// Implementations without Enabled conservatively report true: the cost
	// of evaluating kv args is the status quo, no event may be dropped.
	if !DebugEnabled(LoggerFunc{}) {
		t.Fatal("LoggerFunc (no Enabled) must conservatively report true")
	}
	// logx-backed loggers answer via LevelEnabler with lazy resolution.
	// The process default is NOT assumed here: logx's std backend currently
	// starts at DEBUG (upstream default-level issue), so this test installs
	// its own backends explicitly for both directions.
	logx.SetDefault(logx.NewStdLogger("t", logx.LevelInfo))
	if DebugEnabled(NewLogxLogger("t")) {
		t.Fatal("logx INFO default must not be debug-enabled")
	}
	logx.SetDefault(logx.NewStdLogger("t", logx.LevelDebug))
	if !DebugEnabled(NewLogxLogger("t")) {
		t.Fatal("logx DEBUG default must be debug-enabled (lazy resolution)")
	}
	logx.SetDefault(logx.NewSlogLogger("t"))
	// SlogLogger maps onto its handler's level.
	info := SlogLogger{Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))}
	if DebugEnabled(info) {
		t.Fatal("slog INFO handler must not be debug-enabled")
	}
	dbg := SlogLogger{Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	if !DebugEnabled(dbg) {
		t.Fatal("slog DEBUG handler must be debug-enabled")
	}
}
