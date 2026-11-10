package gitee

import (
	"context"
	"strings"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCommitStatus implements provider.CommitStatusManager.
// Gitee uses the Checks API (check runs) for CI status reporting.
//
// in-flight 态不得写成 completed+neutral(issue #58):读侧
// convertCommitStatuses 对 completed 的 check run 取 conclusion 归一,
// 而 "neutral" 归一成 Success——「写 pending 读回 success」的静默反转
// 会把合并门禁提前放行。统一词表的终态动词(Terminal)才落
// completed+conclusion;pending/running/未知动词一律写 in_progress
// 形态(不带 conclusion),读侧走 Status 分支归一回 Pending/Running,
// round-trip 语义闭环。
func (p *Provider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts provider.CommitStatusOptions) error {
	check := &gitee.CreateCheckRunOptions{
		Name:    gitee.String(opts.Context),
		HeadSHA: gitee.String(sha),
		Output: &gitee.CheckRunOutput{
			Title:   gitee.String(opts.Context),
			Summary: gitee.String(opts.Description),
		},
	}
	if provider.CommitStatusState(strings.ToLower(strings.TrimSpace(opts.State))).Terminal() {
		check.Status = gitee.String("completed")
		check.Conclusion = gitee.String(mapCommitStatus(opts.State))
	} else {
		check.Status = gitee.String("in_progress")
	}
	_, _, err := p.client.Checks.Create(ctx, backendutil.Esc(owner), backendutil.Esc(repo), check)
	return provider.Wrap(provider.PlatformGitee, "CreateCommitStatus", err)
}

// ListCommitStatuses implements provider.CommitStatusManager.
// Gitee's public REST API has no commit-status endpoint; statuses read back
// through the Checks API as the check runs attached to the head SHA.
func (p *Provider) ListCommitStatuses(ctx context.Context, owner, repo, sha string) ([]provider.CommitStatus, error) {
	runs, err := backendutil.AllPages(func(page int) ([]*gitee.CheckRun, error) {
		pageNo, perPage := page, 100
		list, _, err := p.client.Checks.List(ctx, backendutil.Esc(owner), backendutil.Esc(repo), sha,
			&gitee.CheckRunListOptions{Page: &pageNo, PerPage: &perPage})
		if err != nil {
			return nil, err
		}
		if list == nil || list.CheckRuns == nil {
			return nil, nil
		}
		return *list.CheckRuns, nil
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListCommitStatuses", err)
	}
	return convertCommitStatuses(runs), nil
}

// convertCommitStatuses folds check runs into unified statuses. A check
// run's context is its Name and its page the details_url; the state comes
// from Conclusion once the run is completed and from the in-flight Status
// verb otherwise, both normalized via the shared vocabulary. Description is
// dropped: Output is a free-form interface{} with no stable wire shape.
func convertCommitStatuses(runs []*gitee.CheckRun) []provider.CommitStatus {
	result := make([]provider.CommitStatus, 0, len(runs))
	for _, run := range runs {
		if run == nil {
			continue
		}
		var state, name, targetURL string
		if run.Status != nil && *run.Status != "completed" {
			state = *run.Status
		} else if run.Conclusion != nil {
			state = *run.Conclusion
		}
		if run.Name != nil {
			name = *run.Name
		}
		if run.DetailsURL != nil {
			targetURL = *run.DetailsURL
		}
		result = append(result, provider.CommitStatus{
			State:     provider.NormalizeCommitStatusState(state),
			Context:   name,
			TargetURL: targetURL,
		})
	}
	return result
}

// mapCommitStatus maps the SDK's normalized state strings to Gitee check-run
// conclusion values. 只接受终态动词——in-flight 态在 CreateCommitStatus
// 里已经分流成 in_progress 形态,不落 conclusion(issue #58;pending→neutral
// 的旧映射已删:它是「写 pending 读回 success」反转的写侧根源)。
func mapCommitStatus(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "success":
		return "success"
	case "failure", "error":
		return "failure"
	default:
		return "neutral"
	}
}

var _ provider.CommitStatusManager = (*Provider)(nil)
