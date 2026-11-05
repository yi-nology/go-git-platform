package gitee

import (
	"context"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCommitStatus implements provider.CommitStatusManager.
// Gitee uses the Checks API (check runs) for CI status reporting.
func (p *Provider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts provider.CommitStatusOptions) error {
	status := mapCommitStatus(opts.State)
	_, _, err := p.client.Checks.Create(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &gitee.CreateCheckRunOptions{
		Name:       gitee.String(opts.Context),
		HeadSHA:    gitee.String(sha),
		Status:     gitee.String("completed"),
		Conclusion: gitee.String(status),
		Output: &gitee.CheckRunOutput{
			Title:   gitee.String(opts.Context),
			Summary: gitee.String(opts.Description),
		},
	})
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
// conclusion values.
func mapCommitStatus(state string) string {
	switch state {
	case "success":
		return "success"
	case "failure", "error":
		return "failure"
	case "pending":
		return "neutral"
	default:
		return "neutral"
	}
}

var _ provider.CommitStatusManager = (*Provider)(nil)
