package provider

import "context"

// JobLog is one failed CI job's identifying metadata plus the tail of its
// execution log, sized for LLM consumption rather than archival.
type JobLog struct {
	Name      string `json:"name"`
	Stage     string `json:"stage"`
	WebURL    string `json:"web_url,omitempty"`
	Trace     string `json:"trace"` // 失败日志尾部（后端截断，量级 ~20KB）
	Truncated bool   `json:"truncated"`
}

// CILogManager fetches failure diagnostics from a platform's CI system.
// It is an optional capability interface: consumers should check
// Capabilities().CILogs (or type-assert) before use. GitLab reads its
// native pipelines; GitHub, Gitea, and GitCode map onto their Actions
// surfaces (runs-by-SHA → failed jobs → job log tails); platforms without
// a job-log wire express absence by not declaring the capability.
type CILogManager interface {
	// ListFailedJobLogs returns the failed jobs of the most recent pipeline
	// run for a commit, each with the tail of its execution log. A commit
	// with no CI run (or a green one) yields an empty slice, not an error.
	// Platforms whose CI fans out a commit into multiple pipelines (one per
	// workflow) report the latest run per pipeline definition — see each
	// backend's divergence ledger for the exact fan-out semantics.
	ListFailedJobLogs(ctx context.Context, owner, repo, sha string) ([]JobLog, error)
}
