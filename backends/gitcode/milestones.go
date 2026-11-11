package gitcode

import (
	"context"
	"time"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements provider.MilestoneManager over go-gitcode's
// milestones surface. GitCode addresses milestones by their ID (the wire's
// "id" — the SDK's Get/Update/Delete path parameter, whatever its
// "number" spelling) — the same value MilestoneRef.Number and
// Milestone.Number carry on this platform. Wire states are
// "open"/"closed", already the SDK's vocabulary.
//
// Since go-gitcode v0.8.0 the SDK marshals create/update bodies with
// `due_on` omitempty (date-only string on the wire), so an unset due date
// stays off the wire instead of wiping the stored one — the SDK covers
// create/update directly and the former raw-transport detour is gone.
// Owner/repo pass through raw: the SDK escapes path segments itself since
// v0.16.0 (issue #116, retiring this backend's #84 pre-escaping — wrapping
// here again would double-escape on the wire).

// ListMilestones implements provider.MilestoneManager. State filters by
// "open"/"closed" (GitCode also accepts "all").
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListMilestones(ctx context.Context, owner, repo string, opts provider.ListMilestonesOptions) ([]provider.Milestone, error) {
	// State contract (issue #61): empty State sends the explicit "all"
	// (GitCode also accepts it) instead of relying on the platform default.
	state := opts.State
	if state == "" {
		state = "all"
	}
	buildOpts := func(page, perPage int) gitcode.ListMilestonesOptions {
		return gitcode.ListMilestonesOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
			State:       state,
		}
	}
	var milestones []*gitcode.Milestone
	milestones, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitcode.Milestone, error) {
			return p.client.ListMilestonesWithOptions(ctx, owner, repo, buildOpts(page, perPage))
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListMilestones", err)
	}
	result := make([]provider.Milestone, 0, len(milestones))
	for _, m := range milestones {
		result = append(result, convertMilestone(m))
	}
	return result, nil
}

// GetMilestone implements provider.MilestoneManager.
func (p *Provider) GetMilestone(ctx context.Context, owner, repo, number string) (*provider.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "GetMilestone", number)
	if err != nil {
		return nil, err
	}
	m, err := p.client.GetMilestone(ctx, owner, repo, int(id))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// CreateMilestone implements provider.MilestoneManager. The SDK carries
// title always and description/due_on only when set (omitempty).
func (p *Provider) CreateMilestone(ctx context.Context, owner, repo string, opts provider.CreateMilestoneOptions) (*provider.Milestone, error) {
	sdkOpts := gitcode.CreateMilestoneOptions{
		Title:       opts.Title,
		Description: opts.Description,
	}
	if opts.DueOn != nil {
		sdkOpts.DueOn = opts.DueOn.Format(time.RFC3339)
	}
	m, err := p.client.CreateMilestoneWithOptions(ctx, owner, repo, sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// UpdateMilestone implements provider.MilestoneManager. Unset provider
// fields stay off the wire (SDK omitempty), leaving the milestone
// unchanged; GitCode addresses the milestone by its ID.
func (p *Provider) UpdateMilestone(ctx context.Context, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*provider.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "UpdateMilestone", number)
	if err != nil {
		return nil, err
	}
	sdkOpts := gitcode.UpdateMilestoneOptions{}
	if opts.Title != nil {
		sdkOpts.Title = *opts.Title
	}
	if opts.Description != nil {
		sdkOpts.Description = *opts.Description
	}
	if opts.State != "" {
		sdkOpts.State = string(opts.State)
	}
	if opts.DueOn != nil {
		sdkOpts.DueOn = opts.DueOn.Format(time.RFC3339)
	}
	m, err := p.client.UpdateMilestone(ctx, owner, repo, int(id), sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// DeleteMilestone implements provider.MilestoneManager.
func (p *Provider) DeleteMilestone(ctx context.Context, owner, repo, number string) error {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "DeleteMilestone", number)
	if err != nil {
		return err
	}
	if err := p.client.DeleteMilestone(ctx, owner, repo, int(id)); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteMilestone", err)
	}
	return nil
}

var _ provider.MilestoneManager = (*Provider)(nil)
