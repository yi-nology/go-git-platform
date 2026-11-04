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

type ciLogOut struct {
	Name    string `json:"name"`
	Stage   string `json:"stage,omitempty"`
	WebURL  string `json:"web_url,omitempty"`
	Trace   string `json:"trace"`
	Truncat bool   `json:"truncated"`
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
	add(s, st, "get_failed_ci_logs", "Get failed CI job logs", "Return the failed jobs of the latest CI run for a commit, each with the tail of its execution log for failure attribution. Empty when the commit is green or has no CI run. Supported on GitHub, GitLab, and Gitea.", false, func(ctx context.Context, in ciLogsIn) ([]ciLogOut, error) {
		logs, err := cim.ListFailedJobLogs(ctx, in.Owner, in.Repo, in.SHA)
		if err != nil {
			return nil, err
		}
		out := make([]ciLogOut, 0, len(logs))
		for _, l := range logs {
			out = append(out, ciLogOut{
				Name:    l.Name,
				Stage:   l.Stage,
				WebURL:  l.WebURL,
				Trace:   l.Trace,
				Truncat: l.Truncat,
			})
		}
		return out, nil
	})
}
