package provider

import "github.com/oekit/logx"

// Logger is a minimal logging interface compatible with most Go logging libraries.
// Implementations include slog, zap, zerolog, logrus, etc.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// noopLogger is a Logger that discards all output.
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// NewNoopLogger returns a Logger that discards all output.
func NewNoopLogger() Logger { return noopLogger{} }

// logx.Logger satisfies Logger structurally (same four kv-style methods).
var _ Logger = logx.Delegating("")

// NewLogxLogger returns a Logger backed by the oekit/logx facade, for
// consumers who run the oekit observability stack. The logger is lazily
// resolved (logx.Delegating): it follows whatever implementation the
// consumer installs via logx.SetDefault / logx.UseSlog, even when that
// happens after NewProvider. component identifies this SDK in log output
// (the logx convention: service name belongs to the consumer, packages
// identify themselves via the component attribute).
//
//	p, err := provider.NewProvider(provider.Config{
//	    Platform: provider.PlatformGitHub,
//	    Token:    token,
//	    Logger:   provider.NewLogxLogger("git-platform"),
//	})
func NewLogxLogger(component string) Logger {
	return logx.Delegating(component)
}
