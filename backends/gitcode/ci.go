package gitcode

import (
	"bytes"
	"context"
	"errors"
	"strings"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListFailedJobLogs implements provider.CILogManager. GitCode 的 CI 线路：
// FindActionRunByHeadSHA（wire 无 head_sha 过滤，SDK 按最新序客户端扫描）→
// run 的失败 job → download-log 拉全量后本地截尾。语义分歧（mapping 已登
// 记）：SDK 只返回单条最新命中 run——同 SHA 其他 workflow 的 run 不归因
// （GitHub/Gitea 是每 workflow 取最新）；job status 词表为大写（夹具实证
// COMPLETED/RUNNING），失败判定大小写不敏感、live 词表待实证。
func (p *Provider) ListFailedJobLogs(ctx context.Context, owner, repo, sha string) ([]provider.JobLog, error) {
	run, err := p.client.FindActionRunByHeadSHA(ctx, owner, repo, sha, gitcode.ListWorkflowRunsOptions{})
	if errors.Is(err, gitcode.ErrActionRunNotFound) {
		// 无 run（绿提交或无 CI）→ 空切片而非错误（接口契约）。
		return nil, nil
	}
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListFailedJobLogs", err)
	}
	jobs, err := p.client.ListActionRunJobs(ctx, owner, repo, run.WorkflowRunID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListFailedJobLogs", err)
	}
	out := make([]provider.JobLog, 0, 4)
	for _, job := range jobs.Jobs {
		if !isFailedGitCodeStatus(job.Status) {
			continue
		}
		tail, truncated := p.jobLogTail(ctx, owner, repo, run.WorkflowRunID, job.ID)
		out = append(out, provider.JobLog{
			Name:      job.Name,
			Stage:     run.WorkflowName,
			WebURL:    "", // wire 无 job 级 web URL 字段
			Trace:     tail,
			Truncated: truncated,
		})
	}
	return out, nil
}

// isFailedGitCodeStatus reports whether a job status counts as a real
// failure worth attributing. GitCode v8 的状态词表是大写（夹具实证
// COMPLETED/RUNNING），失败词按大写惯例推断为 FAILED——判定大小写不
// 敏感并放宽到 failure/error/timeout 家族：词表未知时宁可多带一条日
// 志（多归因好过漏归因），live 实证后再收紧。
func isFailedGitCodeStatus(status string) bool {
	switch strings.ToLower(status) {
	case "failed", "failure", "error", "timed_out", "timeout", "startup_failure":
		return true
	}
	return false
}

// jobLogTail downloads the job's full log and keeps the tail; a single job
// log that cannot be fetched does not block attribution of the others.
func (p *Provider) jobLogTail(ctx context.Context, owner, repo, runID, jobID string) (string, bool) {
	data, err := p.client.DownloadActionJobLogs(ctx, owner, repo, runID, jobID)
	if err != nil || len(data) == 0 {
		return "", false
	}
	tail, truncated, err := backendutil.TailOfReader(bytes.NewReader(data), backendutil.CILogTailBytes)
	if err != nil {
		return "", false
	}
	return tail, truncated
}
