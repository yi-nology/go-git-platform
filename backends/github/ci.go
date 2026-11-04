package github

import (
	"context"
	"net/http"

	sdkgithub "github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListFailedJobLogs implements provider.CILogManager. GitHub 的 CI 线路：
// runs-by-SHA（ListRepositoryWorkflowRuns 的 head_sha 过滤）→ 每个 run 的
// 失败 job → presigned job log 拉取尾部。一个 SHA 可有多个 run（每
// workflow 文件一条）——按 workflowID 去重只取最新 run（runs 按 created
// 倒序返回，首见即最新；mapping 分歧已登记）。
func (p *Provider) ListFailedJobLogs(ctx context.Context, owner, repo, sha string) ([]provider.JobLog, error) {
	runs, _, err := p.client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &sdkgithub.ListWorkflowRunsOptions{
		HeadSHA: sha,
		ListOptions: sdkgithub.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListFailedJobLogs", err)
	}
	seen := make(map[int64]struct{}, len(runs.WorkflowRuns))
	out := make([]provider.JobLog, 0, 4)
	for _, run := range runs.WorkflowRuns {
		if run == nil {
			continue
		}
		wid := run.GetWorkflowID()
		if _, dup := seen[wid]; dup {
			continue
		}
		seen[wid] = struct{}{}
		jobs, err := p.failedJobsForRun(ctx, owner, repo, run)
		if err != nil {
			return nil, err
		}
		out = append(out, jobs...)
	}
	return out, nil
}

// failedJobsForRun collects the failed jobs of one workflow run, each with
// the tail of its presigned log.
func (p *Provider) failedJobsForRun(ctx context.Context, owner, repo string, run *sdkgithub.WorkflowRun) ([]provider.JobLog, error) {
	jobs, _, err := p.client.Actions.ListWorkflowJobs(ctx, owner, repo, run.GetID(), &sdkgithub.ListWorkflowJobsOptions{
		ListOptions: sdkgithub.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListFailedJobLogs", err)
	}
	out := make([]provider.JobLog, 0, 2)
	for _, job := range jobs.Jobs {
		// canceled/skipped 是连带或未执行，不产生可归因日志；failure/
		// timed_out/startup_failure 是真失败（timed_out 跑挂了、startup_
		// failure 是配置没跑起来，两种都要归因）。
		if !isFailedConclusion(job.GetConclusion()) {
			continue
		}
		tail, truncated := p.jobLogTail(ctx, owner, repo, job.GetID())
		out = append(out, provider.JobLog{
			Name:    job.GetName(),
			Stage:   run.GetName(),
			WebURL:  job.GetHTMLURL(),
			Trace:   tail,
			Truncat: truncated,
		})
	}
	return out, nil
}

// isFailedConclusion reports whether an Actions job conclusion counts as a
// real failure worth attributing.
func isFailedConclusion(c string) bool {
	switch c {
	case "failure", "timed_out", "startup_failure":
		return true
	}
	return false
}

// jobLogTail fetches the job's presigned log URL and keeps the tail. The
// presigned URL needs no auth; a single job log that cannot be fetched does
// not block attribution of the remaining jobs.
func (p *Provider) jobLogTail(ctx context.Context, owner, repo string, jobID int64) (string, bool) {
	u, _, err := p.client.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 3)
	if err != nil || u == nil {
		return "", false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return "", false
	}
	resp, err := p.client.Client().Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close() //nolint:errcheck // 读侧失败已在 tail 里体现
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	tail, truncated, err := backendutil.TailOfReader(resp.Body, backendutil.CILogTailBytes)
	if err != nil {
		return "", false
	}
	return tail, truncated
}
