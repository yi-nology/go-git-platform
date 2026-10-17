package transport

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/oekit/logx"
)

// Logger is a minimal logging interface compatible with log/slog, zap,
// zerolog, logrus, and most other Go logging libraries.
//
// Implementations should be safe to call concurrently. The transport layer
// emits a small set of fixed events: "request ok", "request error", "request
// failed", "retry: network error", "retry: retryable status", and
// "roundtrip ok/failed".
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// noopLogger is the default logger. It discards every event.
type noopLogger struct{}

// Debug implements Logger.
func (noopLogger) Debug(string, ...any) {}

// Info implements Logger.
func (noopLogger) Info(string, ...any) {}

// Warn implements Logger.
func (noopLogger) Warn(string, ...any) {}

// Error implements Logger.
func (noopLogger) Error(string, ...any) {}

// NoopLogger returns a logger that discards every event.
func NoopLogger() Logger { return noopLogger{} }

// SlogLogger adapts a *slog.Logger to the Logger interface.
type SlogLogger struct{ Logger *slog.Logger }

// Enabled reports whether the handler would emit the given level, so hot
// paths can skip expensive kv argument evaluation for filtered events
// (logx.LevelEnabler shape). Map onto slog's level scale; debug events
// bound for NewDebugLogger therefore honor GO_GIT_PLATFORM_DEBUG.
func (l SlogLogger) Enabled(level logx.Level) bool {
	if l.Logger == nil {
		return false
	}
	sl := slog.LevelInfo
	switch level {
	case logx.LevelDebug:
		sl = slog.LevelDebug
	case logx.LevelWarn:
		sl = slog.LevelWarn
	case logx.LevelError:
		sl = slog.LevelError
	}
	return l.Logger.Enabled(context.Background(), sl)
}

// Debug implements Logger.
func (l SlogLogger) Debug(msg string, kv ...any) {
	if l.Logger == nil {
		return
	}
	l.Logger.Debug(msg, kv...)
}

// Info implements Logger.
func (l SlogLogger) Info(msg string, kv ...any) {
	if l.Logger == nil {
		return
	}
	l.Logger.Info(msg, kv...)
}

// Warn implements Logger.
func (l SlogLogger) Warn(msg string, kv ...any) {
	if l.Logger == nil {
		return
	}
	l.Logger.Warn(msg, kv...)
}

// Error implements Logger.
func (l SlogLogger) Error(msg string, kv ...any) {
	if l.Logger == nil {
		return
	}
	l.Logger.Error(msg, kv...)
}

// LoggerFunc adapts a plain function into a Logger. Each level can be nil
// to disable that level.
type LoggerFunc struct {
	DebugFunc func(msg string, kv ...any)
	InfoFunc  func(msg string, kv ...any)
	WarnFunc  func(msg string, kv ...any)
	ErrorFunc func(msg string, kv ...any)
}

// Debug implements Logger.
func (l LoggerFunc) Debug(msg string, kv ...any) {
	if l.DebugFunc != nil {
		l.DebugFunc(msg, kv...)
	}
}

// Info implements Logger.
func (l LoggerFunc) Info(msg string, kv ...any) {
	if l.InfoFunc != nil {
		l.InfoFunc(msg, kv...)
	}
}

// Warn implements Logger.
func (l LoggerFunc) Warn(msg string, kv ...any) {
	if l.WarnFunc != nil {
		l.WarnFunc(msg, kv...)
	}
}

// Error implements Logger.
func (l LoggerFunc) Error(msg string, kv ...any) {
	if l.ErrorFunc != nil {
		l.ErrorFunc(msg, kv...)
	}
}

// NewDebugLogger returns a Logger that writes to stderr at debug level.
// It checks the GO_GIT_PLATFORM_DEBUG environment variable: if set to
// "1", "true", or "yes", debug-level messages are emitted. Otherwise
// only info and above are shown.
//
// This is a convenience for troubleshooting; production code should
// inject a structured logger (SlogLogger or a custom implementation).
func NewDebugLogger() Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("GO_GIT_PLATFORM_DEBUG")) {
	case "1", "true", "yes":
		level = slog.LevelDebug
	}
	return SlogLogger{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})),
	}
}
