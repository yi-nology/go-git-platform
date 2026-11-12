package gitbackend

import (
	"fmt"

	"github.com/oekit/pkg/container"
)

// GitBackendConstructor creates a GitBackend instance.
type GitBackendConstructor func(opts Options) (GitBackend, error)

// Options holds configuration for creating a GitBackend.
type Options struct {
	Type   string // "native", "gogit", or "" for auto-detect
	Logger Logger
}

// backendRegistry is the backend-name → constructor registry, backed by an
// oekit container.SafeMap (the write side is init()-time, the read side is
// any goroutine). Same registry shape as provider/registry.go.
var backendRegistry = container.NewSafeMap[string, GitBackendConstructor]()

// Register registers a GitBackend constructor.
// Panics on duplicate registration (fail-fast convention shared by every
// registry in this package).
func Register(name string, ctor GitBackendConstructor) {
	if _, loaded := backendRegistry.GetOrSet(name, ctor); loaded {
		panic(fmt.Sprintf("gitbackend: %q already registered", name))
	}
}

// NewGitBackend creates a GitBackend using the registry.
// If opts.Type is empty, it auto-detects (native first, fallback to gogit).
func NewGitBackend(opts Options) (GitBackend, error) {
	if opts.Type != "" {
		ctor, ok := backendRegistry.Get(opts.Type)
		if !ok {
			return nil, fmt.Errorf("gitbackend: unknown type %q", opts.Type)
		}
		return ctor(opts)
	}

	nativeCtor, hasNative := backendRegistry.Get("native")
	gogitCtor, hasGogit := backendRegistry.Get("gogit")
	return newAutoBackend(opts, nativeCtor, hasNative, gogitCtor, hasGogit)
}

// newAutoBackend implements the auto-detect fallback: the native backend when
// its constructor succeeds, the feature-reduced gogit backend otherwise. The
// degradation is logged at Warn level so a missing git binary never silently
// shrinks the backend's capability set.
func newAutoBackend(opts Options, nativeCtor GitBackendConstructor, hasNative bool, gogitCtor GitBackendConstructor, hasGogit bool) (GitBackend, error) {
	if hasNative {
		backend, err := nativeCtor(opts)
		if err == nil {
			return backend, nil
		}
		if opts.Logger != nil {
			opts.Logger.Warn("gitbackend: native backend unavailable, falling back to the feature-reduced gogit backend", "error", err)
		}
	}
	if hasGogit {
		return gogitCtor(opts)
	}
	return nil, fmt.Errorf("gitbackend: no backends available")
}

func init() {
	Register("native", func(opts Options) (GitBackend, error) {
		return NewNativeGitBackend(opts)
	})
	Register("gogit", func(opts Options) (GitBackend, error) {
		return NewGoGitBackend(opts), nil
	})
}
