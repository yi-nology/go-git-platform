package gitee

import (
	"context"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListMilestones implements provider.MilestoneManager via the SDK.
//
// Dual pagination mode: opts.Page > 0 means the caller manages paging
// explicitly and receives exactly that one page (opts.Page/opts.PerPage
// honored via NormalizePageOpts); opts.Page == 0 (the default) exhausts
// pagination via backendutil.AllPages — fetching 100 per page until an
// empty page — so every milestone is returned.
func (p *Provider) ListMilestones(ctx context.Context, owner, repo string, opts provider.ListMilestonesOptions) ([]provider.Milestone, error) {
	// State contract (issue #61): empty State sends the explicit "all" —
	// relying on the platform default silently dropped closed milestones.
	state := opts.State
	if state == "" {
		state = "all"
	}
	baseOpts := gitee.MilestoneListOptions{State: gitee.String(state)}
	var milestones []*gitee.Milestone
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		baseOpts.Page = gitee.Int(page)
		baseOpts.PerPage = gitee.Int(perPage)
		list, _, err := p.client.Milestones.List(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &baseOpts)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitee, "ListMilestones", err)
		}
		milestones = list
	} else {
		var err error
		milestones, err = backendutil.AllPages(func(page int) ([]*gitee.Milestone, error) {
			o := baseOpts
			o.Page = gitee.Int(page)
			o.PerPage = gitee.Int(100)
			list, _, err := p.client.Milestones.List(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &o)
			return list, err
		})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitee, "ListMilestones", err)
		}
	}
	result := make([]provider.Milestone, 0, len(milestones))
	for _, m := range milestones {
		result = append(result, convertMilestone(m))
	}
	return result, nil
}

// GetMilestone implements provider.MilestoneManager via the SDK.
func (p *Provider) GetMilestone(ctx context.Context, owner, repo, number string) (*provider.Milestone, error) {
	n64, err := backendutil.ParseMilestoneNumber(provider.PlatformGitee, "GetMilestone", number)
	if err != nil {
		return nil, err
	}
	m, _, err := p.client.Milestones.Get(ctx, backendutil.Esc(owner), backendutil.Esc(repo), int(n64))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// CreateMilestone implements provider.MilestoneManager.
func (p *Provider) CreateMilestone(ctx context.Context, owner, repo string, opts provider.CreateMilestoneOptions) (*provider.Milestone, error) {
	createOpts := &gitee.CreateMilestoneOptions{
		Title: gitee.String(opts.Title),
	}
	if opts.Description != "" {
		createOpts.Description = gitee.String(opts.Description)
	}
	if opts.DueOn != nil {
		createOpts.DueOn = gitee.String(formatGiteeDueOn(*opts.DueOn))
	}
	m, _, err := p.client.Milestones.Create(ctx, backendutil.Esc(owner), backendutil.Esc(repo), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// UpdateMilestone implements provider.MilestoneManager.
func (p *Provider) UpdateMilestone(ctx context.Context, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*provider.Milestone, error) {
	n64, err := backendutil.ParseMilestoneNumber(provider.PlatformGitee, "UpdateMilestone", number)
	if err != nil {
		return nil, err
	}
	updateOpts := &gitee.UpdateMilestoneOptions{}
	if opts.Title != nil {
		updateOpts.Title = gitee.String(*opts.Title)
	}
	if opts.Description != nil {
		updateOpts.Description = gitee.String(*opts.Description)
	}
	if opts.State != "" {
		updateOpts.State = gitee.String(string(opts.State))
	}
	if opts.DueOn != nil {
		updateOpts.DueOn = gitee.String(formatGiteeDueOn(*opts.DueOn))
	}
	m, _, err := p.client.Milestones.Edit(ctx, backendutil.Esc(owner), backendutil.Esc(repo), int(n64), updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "UpdateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// DeleteMilestone implements provider.MilestoneManager.
func (p *Provider) DeleteMilestone(ctx context.Context, owner, repo, number string) error {
	n64, err := backendutil.ParseMilestoneNumber(provider.PlatformGitee, "DeleteMilestone", number)
	if err != nil {
		return err
	}
	_, err = p.client.Milestones.Delete(ctx, backendutil.Esc(owner), backendutil.Esc(repo), int(n64))
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "DeleteMilestone", err)
	}
	return nil
}

var _ provider.MilestoneManager = (*Provider)(nil)
