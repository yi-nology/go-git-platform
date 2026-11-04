package gitlab

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListFailedJobLogs implements provider.CILogManager：
// 最近一条 pipeline 的失败 job → trace 尾部。无 pipeline / 全绿 → 空切片。
func (p *Provider) ListFailedJobLogs(ctx context.Context, owner, repo, sha string) ([]provider.JobLog, error) {
	pid := pidOf(owner, repo)
	shaOpt := sha
	pl, _, err := p.client.Pipelines.ListProjectPipelines(pid, &gitlab.ListProjectPipelinesOptions{
		SHA:     &shaOpt,
		OrderBy: new("id"),
		Sort:    new("desc"),
		ListOptions: gitlab.ListOptions{
			PerPage: 1,
		},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListProjectPipelines", err)
	}
	if len(pl) == 0 {
		return nil, nil
	}
	jobs, _, err := p.client.Jobs.ListPipelineJobs(pid, pl[0].ID, &gitlab.ListJobsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListPipelineJobs", err)
	}
	out := make([]provider.JobLog, 0, len(jobs))
	for _, j := range jobs {
		// canceled 的 job 是被连带取消的，不产生可归因日志；created 的没轮到
		// 跑——只收真正跑挂了的。
		if j.Status != "failed" {
			continue
		}
		tail, truncated, terr := p.traceTail(ctx, pid, j.ID)
		if terr != nil {
			// 单 job trace 拉不到不阻塞其余 job 的归因
			tail, truncated = "", false
		}
		out = append(out, provider.JobLog{
			Name: j.Name, Stage: j.Stage, WebURL: j.WebURL,
			Trace: tail, Truncated: truncated,
		})
	}
	return out, nil
}

// traceTail 拉 job trace 并只留尾部（GetTraceFile 返回全文流）。
func (p *Provider) traceTail(ctx context.Context, pid string, jobID int64) (string, bool, error) {
	r, _, err := p.client.Jobs.GetTraceFile(pid, jobID, gitlab.WithContext(ctx))
	if err != nil {
		return "", false, provider.Wrap(provider.PlatformGitLab, "GetTraceFile", err)
	}
	return backendutil.TailOfReader(r, backendutil.CILogTailBytes)
}
