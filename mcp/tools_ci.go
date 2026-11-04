package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- ci: failure diagnostics ---

type ciLogsIn struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	SHA   string `json:"sha"`
}

// registerCI mounts the failure-diagnostics toolset when the backend
// implements CILogManager (GitHub / GitLab / Gitea). The trace is the log
// tail (~20KB) sized for LLM attribution — the same payload the SDK returns,
// not a re-fetch of full logs.
func registerCI(s *mcp.Server, st *state) {
	cim, ok := st.p.(provider.CILogManager)
	if !ok {
		return
	}
	add(s, st, "get_failed_ci_logs", "Get failed CI job logs", "Return the failed jobs of the latest CI run for a commit, each with the tail of its execution log for failure attribution. Empty when the commit is green or has no CI run. Supported on GitHub, GitLab, and Gitea.", false, func(ctx context.Context, in ciLogsIn) ([]provider.JobLog, error) {
		return cim.ListFailedJobLogs(ctx, in.Owner, in.Repo, in.SHA)
	})
}
