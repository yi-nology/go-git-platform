package gitbackend

// Logger is the minimal logging surface the gitbackend needs, declared
// consumer-side (Go idiom: the consumer owns the interface) so this low-level
// local-git package does not depend on the forge-abstraction provider
// package. provider.Logger satisfies it structurally, so callers can inject
// the same logger implementation for both.
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// NewNoopLogger returns a Logger that discards all output.
func NewNoopLogger() Logger { return noopLogger{} }

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
