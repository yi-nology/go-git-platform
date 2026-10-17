package transport

import "github.com/oekit/logx"

// logx.Logger satisfies Logger structurally (same four kv-style methods), so
// any oekit/logx logger — including the consumer's SetDefault chain — plugs
// into Config.Logger without an adapter.
var _ Logger = logx.Delegating("")

// NewLogxLogger returns a Logger backed by the oekit/logx facade. The
// returned logger is lazily resolved (logx.Delegating): it follows whatever
// implementation the consumer installs via logx.SetDefault / logx.UseSlog,
// even when that happens after this SDK is initialized. component identifies
// this subsystem in log output (the logx convention: service name belongs to
// the consumer, packages identify themselves via the component attribute).
//
//	client := transport.NewClient(transport.WithLogger(transport.NewLogxLogger("git-platform")))
//
// For the provider-facing entry point, see provider.NewLogxLogger.
func NewLogxLogger(component string) Logger {
	return logx.Delegating(component)
}

// DebugEnabled reports whether l would emit debug-level events, so hot
// paths can skip expensive kv argument evaluation for filtered events.
// Loggers backed by oekit/logx (NewLogxLogger) answer through its
// LevelEnabler; SlogLogger checks its handler's level. Anything else
// conservatively returns true — paying the evaluation cost is the status
// quo ante, and no event is ever dropped by this check. A nil logger
// never emits.
func DebugEnabled(l Logger) bool {
	if l == nil {
		return false
	}
	if le, ok := l.(interface{ Enabled(logx.Level) bool }); ok {
		return le.Enabled(logx.LevelDebug)
	}
	return true
}
