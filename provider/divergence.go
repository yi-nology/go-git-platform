//go:generate go run ../internal/tools/genledger

package provider

import "github.com/oekit/pkg/slicex"

// DivergenceKind classifies how a backend's behavior departs from the
// unified provider semantics for a given method.
type DivergenceKind string

const (
	// DivergenceStub marks a method the platform cannot serve at all: the
	// call returns an error wrapping ErrNotImplemented and touches no wire.
	DivergenceStub DivergenceKind = "stub"
	// DivergenceIgnore marks a field or parameter that is silently dropped:
	// the call succeeds but the ignored input has no effect.
	DivergenceIgnore DivergenceKind = "ignore"
	// DivergenceMapping marks a semantic mapping: the call succeeds and
	// returns the closest platform equivalent, an approximation of the
	// unified semantics.
	DivergenceMapping DivergenceKind = "mapping"
	// DivergenceDetour marks an implementation detour: the method bypasses
	// the platform's third-party SDK and drives the raw transport client.
	// Behavior is unchanged; the entry exists for maintainers.
	DivergenceDetour DivergenceKind = "detour"
)

// Divergence is one registered entry of a backend's divergence ledger.
// Capability and Method carry the provider interface and method names;
// Field names the affected option/result field for ignore and mapping
// entries (empty when the divergence is method-scoped). Reason is a
// one-sentence explanation surfaced in docs/divergence-ledger.md.
//
// Backends expose their ledger via a package-level Divergences function and
// the Provider.Divergences method; the ledger is the machine-readable
// successor of the former "(spec §4.6)" comment registrations.
type Divergence struct {
	Capability string
	Method     string
	Field      string
	Kind       DivergenceKind
	Reason     string
}

// FindByMethod returns the ledger entries registered for method.
func FindByMethod(divs []Divergence, method string) []Divergence {
	return slicex.Filter(divs, func(d Divergence) bool { return d.Method == method })
}

// Ignores reports whether the ledger registers an ignore of field on method.
// 保留手写循环：slicex 无早退谓词原语（Any/FindFunc），Filter 会为一次布尔判定白分配全量结果。
func Ignores(divs []Divergence, method, field string) bool {
	for _, d := range divs {
		if d.Method == method && d.Kind == DivergenceIgnore && d.Field == field {
			return true
		}
	}
	return false
}

// Stubs reports whether the ledger registers method as a stub.
// 同 Ignores：早退谓词无 slicex 原语对应，保留手写循环。
func Stubs(divs []Divergence, method string) bool {
	for _, d := range divs {
		if d.Method == method && d.Kind == DivergenceStub {
			return true
		}
	}
	return false
}
