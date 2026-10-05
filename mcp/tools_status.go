package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- status ---

// validCommitStatusStates is the write-side vocabulary for
// set_commit_status. Read-side states like "running"/"canceled" describe
// pipeline states on some platforms and have no portable write mapping,
// so they are rejected at the tool boundary instead of failing deep
// inside a platform SDK with an obscure 4xx.
var validCommitStatusStates = map[string]bool{
	"pending": true, "success": true, "failure": true, "error": true,
}

type setStatusIn struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	SHA         string `json:"sha"`
	State       string `json:"state" jsonschema:"pending|success|failure|error"`
	Context     string `json:"context" jsonschema:"status context, e.g. ci/lint"`
	Description string `json:"description,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
}

type waitStatusIn struct {
	Owner          string   `json:"owner"`
	Repo           string   `json:"repo"`
	SHA            string   `json:"sha"`
	Contexts       []string `json:"contexts,omitempty" jsonschema:"wait only for these status contexts; empty = all"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty" jsonschema:"overall wait bound in seconds; default 600, negative = wait unboundedly"`
}

type waitStatusOut struct {
	State provider.CommitStatusState `json:"state"`
}

func registerStatus(s *mcp.Server, st *state) {
	csm, ok := st.p.(provider.CommitStatusManager)
	if !ok {
		return
	}

	add(s, st, "get_commit_statuses", "Get commit statuses", "List the CI statuses reported on a commit (full history, newest first; use the first entry per context).", false, func(ctx context.Context, in getCommitIn) ([]*provider.CommitStatus, error) {
		list, err := csm.ListCommitStatuses(ctx, in.Owner, in.Repo, in.SHA)
		if err != nil {
			return nil, err
		}
		out := make([]*provider.CommitStatus, len(list))
		for i := range list {
			out[i] = &list[i]
		}
		return out, nil
	})

	add(s, st, "set_commit_status", "Set commit status", "Report a CI status on a commit (e.g. after running checks).", true, func(ctx context.Context, in setStatusIn) (map[string]any, error) {
		if !validCommitStatusStates[in.State] {
			return nil, fmt.Errorf("invalid state %q (pending|success|failure|error)", in.State)
		}
		err := csm.CreateCommitStatus(ctx, in.Owner, in.Repo, in.SHA, provider.CommitStatusOptions{
			State: in.State, Context: in.Context,
			Description: in.Description, TargetURL: in.TargetURL,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	add(s, st, "wait_for_status", "Wait for commit status", "Poll until a commit's combined CI state is terminal and return it. States: pending/running until done, then success/failure/error/canceled. CI re-runs are folded: only the newest report per context counts.", false, func(ctx context.Context, in waitStatusIn) (waitStatusOut, error) {
		opts := provider.WaitOptions{Interval: 5 * time.Second}
		if in.TimeoutSeconds != 0 {
			opts.Timeout = time.Duration(in.TimeoutSeconds) * time.Second
		}
		opts.Contexts = in.Contexts
		state, err := provider.WaitForCommitStatus(ctx, st.p, in.Owner, in.Repo, in.SHA, opts)
		if err != nil {
			return waitStatusOut{}, err
		}
		return waitStatusOut{State: state}, nil
	})
}
