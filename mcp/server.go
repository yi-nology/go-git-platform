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
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oekit/pkg/container"

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
	toolset  string // 正在注册的 toolset 名（add() 记账用）
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
			return nil, fmt.Errorf("unknown toolset %q (known: core, crs, issues, status, search, releases, protection, collaborators, webhooks, tencentcode, ci, repo-extras)", t)
		}
		selected[t] = true
	}
	want := func(set string) bool { return len(selected) == 0 || selected[set] }

	for _, ts := range toolsets {
		if !want(ts.name) || !ts.enabled(p) {
			continue
		}
		st.toolset = ts.name
		ts.registered(s, st)
	}
	return s, nil
}

// toolRegistry 是 add() 挂账的工具面台账（key = toolset + "\x00" + 工具名）
// —— README 工具表对账机检的注册面真源（issue #122）。oekit container.
// SafeMap 收编（本仓第 6 处注册表迁移，前五处见 AGENTS.md 规则一）：
// 并发构造 NewServer 的写入由容器层互斥承接，Phase 1 的手写
// toolRegistryMu 随之删除。
var toolRegistry = container.NewSafeMap[string, bool]()

const toolsetSep = "\x00"

func toolsetKey(toolset, name string) string { return toolset + toolsetSep + name }

func recordTool(toolset, name string) {
	if toolset == "" {
		return
	}
	toolRegistry.Set(toolsetKey(toolset, name), true)
}

// recordedToolsOf 返回某 toolset 已挂账的工具名（README 对账读取面）。
// Range 走容器快照、顺序不定 ⇒ 输出排序保证确定性。
func recordedToolsOf(toolset string) []string {
	prefix := toolset + toolsetSep
	var out []string
	toolRegistry.Range(func(k string, _ bool) bool {
		if after, ok := strings.CutPrefix(k, prefix); ok {
			out = append(out, after)
		}
		return true
	})
	slices.Sort(out)
	return out
}

// recordedToolsets 返回全部已挂账的 toolset 名（排序）。
func recordedToolsets() []string {
	var out []string
	toolRegistry.Range(func(k string, _ bool) bool {
		if i := strings.IndexByte(k, toolsetSep[0]); i > 0 && !slices.Contains(out, k[:i]) {
			out = append(out, k[:i])
		}
		return true
	})
	slices.Sort(out)
	return out
}
