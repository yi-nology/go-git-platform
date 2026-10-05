package tencentcode

import (
	"context"
	"strconv"

	gongfeng "github.com/studyzy/gongfeng-sdk-go"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements provider.MilestoneManager over the gongfeng SDK's
// MilestonesService. Gongfeng milestones are addressed by their project
// milestone ID — the same value MilestoneRef.Number and Milestone.Number
// carry on this platform. Three vocabulary mappings apply (registered):
//
//   - state: gongfeng's wire state is "active"/"closed" while the SDK's is
//     open/closed — "active" maps to open inbound, and open/closed map to
//     the state_event verbs "active"/"close" outbound (工蜂's documented
//     event vocabulary, per the SDK's bundled API docs).
//   - due date: gongfeng exchanges a date-only "2006-01-02" string, so
//     DueOn's time-of-day is lost on the wire.
//   - ListMilestonesOptions.State is not carried: gongfeng's list options
//     expose pagination only, so all states are listed.

// ListMilestones implements provider.MilestoneManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListMilestones(ctx context.Context, owner, repo string, opts provider.ListMilestonesOptions) ([]provider.Milestone, error) {
	buildOpts := func(page, perPage int) *gongfeng.ListMilestonesOptions {
		return &gongfeng.ListMilestonesOptions{
			ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage},
		}
	}
	var milestones []*gongfeng.Milestone
	milestones, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gongfeng.Milestone, error) {
			list, _, err := p.client.Milestones.ListMilestones(ctx, pid(owner, repo), buildOpts(page, perPage))
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListMilestones", err)
	}
	result := make([]provider.Milestone, 0, len(milestones))
	for _, ms := range milestones {
		result = append(result, convertMilestone(ms))
	}
	return result, nil
}

// GetMilestone implements provider.MilestoneManager.
func (p *Provider) GetMilestone(ctx context.Context, owner, repo, number string) (*provider.Milestone, error) {
	id64, err := backendutil.ParseMilestoneNumber(provider.PlatformTencentCode, "GetMilestone", number)
	if err != nil {
		return nil, err
	}
	ms, _, err := p.client.Milestones.GetMilestone(ctx, pid(owner, repo), int(id64))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetMilestone", err)
	}
	out := convertMilestone(ms)
	return &out, nil
}

// CreateMilestone implements provider.MilestoneManager.
func (p *Provider) CreateMilestone(ctx context.Context, owner, repo string, opts provider.CreateMilestoneOptions) (*provider.Milestone, error) {
	createOpts := &gongfeng.CreateMilestoneOptions{
		Title:       gongfeng.Ptr(opts.Title),
		Description: gongfeng.Ptr(opts.Description),
	}
	if opts.DueOn != nil {
		createOpts.DueDate = gongfeng.Ptr(opts.DueOn.Format("2006-01-02"))
	}
	ms, _, err := p.client.Milestones.CreateMilestone(ctx, pid(owner, repo), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateMilestone", err)
	}
	out := convertMilestone(ms)
	return &out, nil
}

// UpdateMilestone implements provider.MilestoneManager. Nil fields in opts
// stay absent from the PUT body, leaving the milestone unchanged; state
// changes travel as gongfeng's state_event verb.
func (p *Provider) UpdateMilestone(ctx context.Context, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*provider.Milestone, error) {
	id64, err := backendutil.ParseMilestoneNumber(provider.PlatformTencentCode, "UpdateMilestone", number)
	if err != nil {
		return nil, err
	}
	editOpts := &gongfeng.EditMilestoneOptions{}
	if opts.Title != nil {
		editOpts.Title = opts.Title
	}
	if opts.Description != nil {
		editOpts.Description = opts.Description
	}
	if opts.DueOn != nil {
		editOpts.DueDate = gongfeng.Ptr(opts.DueOn.Format("2006-01-02"))
	}
	switch opts.State {
	case provider.MilestoneStateOpen:
		editOpts.StateEvent = gongfeng.Ptr("active")
	case provider.MilestoneStateClosed:
		editOpts.StateEvent = gongfeng.Ptr("close")
	case "":
	default:
		return nil, provider.Wrapf(provider.PlatformTencentCode, "UpdateMilestone", "unsupported milestone state %q", opts.State)
	}
	ms, _, err := p.client.Milestones.EditMilestone(ctx, pid(owner, repo), int(id64), editOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "UpdateMilestone", err)
	}
	out := convertMilestone(ms)
	return &out, nil
}

// DeleteMilestone implements provider.MilestoneManager.
func (p *Provider) DeleteMilestone(ctx context.Context, owner, repo, number string) error {
	id64, err := backendutil.ParseMilestoneNumber(provider.PlatformTencentCode, "DeleteMilestone", number)
	if err != nil {
		return err
	}
	if _, err := p.client.Milestones.DeleteMilestone(ctx, pid(owner, repo), int(id64)); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteMilestone", err)
	}
	return nil
}

// convertMilestone maps a gongfeng.Milestone to a provider.Milestone.
// Number carries the gongfeng milestone ID (the identifier the write
// endpoints take); the wire state "active" maps to the SDK's "open".
func convertMilestone(m *gongfeng.Milestone) provider.Milestone {
	var ms provider.Milestone
	if m == nil {
		return ms
	}
	ms = provider.Milestone{
		Number:      strconv.FormatInt(int64(m.ID), 10),
		Title:       m.Title,
		Description: m.Description,
	}
	switch m.State {
	case "active":
		ms.State = provider.MilestoneStateOpen
	default:
		ms.State = provider.MilestoneState(m.State) // "closed" (and unknowns) pass through
	}
	if !m.DueDate.Time.IsZero() {
		due := m.DueDate.Time
		ms.DueOn = &due
	}
	return ms
}

var _ provider.MilestoneManager = (*Provider)(nil)
