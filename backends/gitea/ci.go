package gitea

import (
	"context"

	gitea "gitea.dev/sdk"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListFailedJobLogs implements provider.CILogManager. Gitea 的 CI 线路：
// runs-by-SHA（ListRepoRuns 的 head_sha 过滤）→ 每个 run 的失败 job →
// job log reader 拉尾部。一个 SHA 可有多个 run（每 workflow 一条）——
// 按 workflowID 去重只取最新 run（首见即最新；mapping 分歧已登记）。
func (p *Provider) ListFailedJobLogs(ctx context.Context, owner, repo, sha string) ([]provider.JobLog, error) {
	runs, _, err := p.client.Actions.ListRepoRuns(ctx, owner, repo, gitea.ListRepoActionsRunsOptions{
		HeadSHA: sha,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitea, "ListFailedJobLogs", err)
	}
	seen := make(map[string]struct{}, len(runs.WorkflowRuns))
	out := make([]provider.JobLog, 0, 4)
	for _, run := range runs.WorkflowRuns {
		if run == nil {
			continue
		}
		wid := run.Path
		if _, dup := seen[wid]; dup {
			continue
		}
		seen[wid] = struct{}{}
		jobs, _, err := p.client.Actions.ListRepoJobsByRun(ctx, owner, repo, run.ID, gitea.ListRepoActionsJobsOptions{})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitea, "ListFailedJobLogs", err)
		}
		for _, job := range jobs.Jobs {
			// canceled/skipped 是连带或未执行，不产生可归因日志；failure/
			// timed_out/startup_failure 是真失败（与 GitHub 后端口径一致）。
			if !isFailedCIConclusion(job.Conclusion) {
				continue
			}
			tail, truncated := p.jobLogTail(ctx, owner, repo, job.ID)
			out = append(out, provider.JobLog{
				Name:    job.Name,
				Stage:   run.Path,
				WebURL:  job.HTMLURL,
				Trace:   tail,
				Truncat: truncated,
			})
		}
	}
	return out, nil
}

// jobLogTail fetches the job's log stream and keeps the tail; a single job
// log that cannot be fetched does not block attribution of the others.
func (p *Provider) jobLogTail(ctx context.Context, owner, repo string, jobID int64) (string, bool) {
	r, _, err := p.client.Actions.GetRepoRunJobLogsReader(ctx, owner, repo, jobID)
	if err != nil || r == nil {
		return "", false
	}
	defer r.Close() //nolint:errcheck // 读侧失败已在 tail 里体现
	tail, truncated, err := backendutil.TailOfReader(r, backendutil.CILogTailBytes)
	if err != nil {
		return "", false
	}
	return tail, truncated
}

// isFailedCIConclusion reports whether an Actions job conclusion counts as a
// real failure worth attributing (same vocabulary as the GitHub backend).
func isFailedCIConclusion(c string) bool {
	switch c {
	case "failure", "timed_out", "startup_failure":
		return true
	}
	return false
}
