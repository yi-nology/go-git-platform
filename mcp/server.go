// Package mcpserver exposes go-git-platform as a Model Context Protocol
// (MCP) server so AI agents can operate GitHub, GitLab, Gitea, Forgejo,
// Gitee, GitCode, and Tencent Code through one tool surface.
//
// Design conventions mirror the platform MCP servers agents already know:
//
//   - Toolsets group tools by domain (core, crs, issues, status, search,
//     releases) and can be selected at construction time.
//   - Read/write separation: every mutating tool carries a
//     "mutating: ..." annotation; Options.ReadOnly drops them all.
//   - Capability gating: a tool is only registered when the connected
//     provider's Capabilities() declares the underlying capability, so
//     the model never sees tools that would fail with
//     ErrNotImplemented.
package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// Version is the MCP server's version. It follows the go-git-platform
// module version (see provider.Version) so a server binary always reports
// the SDK feature level it was built against; "dev" for local builds.
func Version() string { return provider.Version() }

// Options configures NewServer. A zero Options is valid: all toolsets
// are enabled, read/write mode is on.
type Options struct {
	// Toolsets selects which toolsets to mount. Empty or nil mounts all.
	// Unknown names make NewServer return an error.
	Toolsets []string
	// ReadOnly drops every mutating tool at registration time: the model
	// never sees (and cannot call) the mutations.
	ReadOnly bool
	// Name/Version reported in the MCP initialize handshake; defaults
	// to "go-git-platform" / the module version.
	Name string
}

// toolset is a named group of tools sharing one capability gate.
type toolset struct {
	name       string
	enabled    func(p provider.Provider) bool
	registered func(s *mcp.Server, st *state)
}

// state carries everything the tool handlers need.
type state struct {
	p        provider.Provider
	readonly bool
}

// NewServer builds an MCP server over the given provider. The provider
// must already be constructed and authenticated (see the cmd package for
// the flag/environment wiring). Unknown names in opts.Toolsets are an
// error: a misspelled toolset would otherwise silently shrink the tool
// surface the model sees.
func NewServer(p provider.Provider, opts Options) (*mcp.Server, error) {
	name := opts.Name
	if name == "" {
		name = "go-git-platform"
	}
	s := mcp.NewServer(&mcp.Implementation{Name: name, Version: Version()}, nil)

	st := &state{p: p, readonly: opts.ReadOnly}
	selected := map[string]bool{}
	for _, t := range opts.Toolsets {
		if !knownToolsets[t] {
			return nil, fmt.Errorf("unknown toolset %q (known: core, crs, issues, status, search, releases)", t)
		}
		selected[t] = true
	}
	want := func(set string) bool { return len(selected) == 0 || selected[set] }

	for _, ts := range toolsets {
		if !want(ts.name) || !ts.enabled(p) {
			continue
		}
		ts.registered(s, st)
	}
	return s, nil
}
