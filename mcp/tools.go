package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// toolsetNames are the selectable toolset identifiers (Options.Toolsets).
const (
	toolsetCore     = "core"     // repos, branches, files, CR reads
	toolsetCRs      = "crs"      // CR lifecycle writes
	toolsetIssues   = "issues"   // issue reads/writes
	toolsetStatus   = "status"   // commit status read/write/wait
	toolsetSearch   = "search"   // repo/issue/user search
	toolsetReleases = "releases" // tags and release reads/writes
)

// defaultPerPage is the page size used when a tool caller leaves per_page
// unset. It stays below the SDK's MaxPerPage (100) ceiling on purpose:
// list payloads are the main context-cost driver for agents.
const defaultPerPage = 30

// Toolset mounting has a single capability gate: the runtime type assertion
// against the provider manager interface the toolset needs. Mounting and the
// register functions consult the same mechanism, so declaration and behavior
// cannot drift (the Capabilities() flags remain a parallel public contract
// for non-MCP consumers, enforced bidirectionally by contracttest).
var toolsets = []toolset{
	{name: toolsetCore, enabled: func(provider.Provider) bool { return true }, registered: registerCore},
	{name: toolsetCRs, enabled: func(provider.Provider) bool { return true }, registered: registerCRs},
	{name: toolsetIssues, enabled: func(p provider.Provider) bool { _, ok := p.(provider.IssueManager); return ok }, registered: registerIssues},
	{name: toolsetStatus, enabled: func(p provider.Provider) bool { _, ok := p.(provider.CommitStatusManager); return ok }, registered: registerStatus},
	{name: toolsetSearch, enabled: func(p provider.Provider) bool { _, ok := p.(provider.SearchManager); return ok }, registered: registerSearch},
	// ReleaseManager is a core Provider interface (every backend implements
	// it), so the toolset mounts unconditionally.
	{name: toolsetReleases, enabled: func(provider.Provider) bool { return true }, registered: registerReleases},
}

var knownToolsets = map[string]bool{
	toolsetCore: true, toolsetCRs: true, toolsetIssues: true,
	toolsetStatus: true, toolsetSearch: true, toolsetReleases: true,
}

// perPage resolves the per_page tool parameter with the default fallback.
func perPage(n int) int {
	if n <= 0 {
		return defaultPerPage
	}
	if n > provider.MaxPerPage {
		return provider.MaxPerPage
	}
	return n
}

// --- registration helpers ---

// add mounts one tool. write tools (write=true) are dropped entirely
// under ReadOnly: the model should not see (and cannot call) mutations
// it is not allowed to make — the strongest form of read-only mode.
func add[In, Out any](s *mcp.Server, st *state, name, title, desc string, write bool, h func(context.Context, In) (Out, error)) {
	if write && st.readonly {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: desc,
		Annotations: &mcp.ToolAnnotations{Title: title},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		out, err := h(ctx, in)
		if err != nil {
			// Provider failures are tool outcomes, not protocol errors:
			// surface them as IsError text the model can read and react
			// to (retry, adjust, report). The structured payload is
			// zeroed so clients never see a success-shaped body paired
			// with an error.
			var zero Out
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, zero, nil
		}
		return nil, out, nil
	})
}
