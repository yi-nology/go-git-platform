package transport

import "github.com/oekit/logx"

// logx.Logger satisfies Logger structurally (same four kv-style methods), so
// any oekit/logx logger — including the consumer's SetDefault chain — plugs
// into Config.Logger without an adapter.
var _ Logger = logx.Delegating()

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
