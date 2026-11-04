package provider

import (
	"fmt"

	"github.com/oekit/pkg/container"
)

// ProviderConstructor is a function that creates a Provider from a Config.
type ProviderConstructor func(cfg Config) (Provider, error)

// registry is the platform → constructor table, backed by oekit SafeMap
// (RWMutex-protected; the write side is init()-time, the read side is any
// goroutine).
var registry = container.NewSafeMap[Platform, ProviderConstructor]()

// Register registers a provider constructor for a platform.
// This is typically called from init() functions in platform implementation files.
// Panics on duplicate registration (fail-fast convention shared by every
// registry in this package).
func Register(p Platform, ctor ProviderConstructor) {
	if _, loaded := registry.GetOrSet(p, ctor); loaded {
		panic(fmt.Sprintf("provider: platform %q already registered", p))
	}
}

// RegisteredPlatforms returns a list of all registered platforms.
func RegisteredPlatforms() []Platform {
	platforms := make([]Platform, 0, registry.Len())
	registry.Range(func(p Platform, _ ProviderConstructor) bool {
		platforms = append(platforms, p)
		return true
	})
	return platforms
}

// IsRegistered checks if a platform has been registered.
func IsRegistered(p Platform) bool {
	_, ok := registry.Get(p)
	return ok
}
