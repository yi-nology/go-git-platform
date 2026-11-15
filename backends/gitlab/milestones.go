package gitlab

import (
	"context"
	"strconv"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"

	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements provider.MilestoneManager over the GitLab SDK's
// project MilestonesService. GitLab addresses milestones by their project
// milestone ID — the same value MilestoneRef.Number and Milestone.Number
// carry on this platform. Two vocabulary mappings apply (registered):
//
//   - state: GitLab's wire state is "active"/"closed" while the SDK's is
//     open/closed — active maps to open inbound, and open maps to the
//     state_event verb "activate" outbound ("close" for closed).
//   - due date: GitLab exchanges a date-only ISO8601 string (the SDK's
//     ISOTime), so DueOn's time-of-day is lost on the wire.

// ListMilestones implements provider.MilestoneManager. State filters by
// GitLab's "active"/"closed" (open maps to active); an empty State sends no
// filter and GitLab lists all — already the unified contract (issue #61).
//
// Dual-mode pagination is handled by backendutil.PageList (issue #60 — the
// exhaustive family): opts.Page == 0 walks every page (budget-capped, fails
// loud); opts.Page > 0 returns exactly that single caller-driven page. The
// pre-fix shape normalized Page==0 to page 1 and silently returned only
// GitLab's default 20-milestone page.
func (p *Provider) ListMilestones(ctx context.Context, owner, repo string, opts provider.ListMilestonesOptions) ([]model.Milestone, error) {
	milestones, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitlab.Milestone, error) {
			listOpts := &gitlab.ListMilestonesOptions{
				ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
			}
			if opts.State != "" {
				s := opts.State
				if s == string(model.MilestoneStateOpen) {
					s = "active" // GitLab milestones vocabulary
				}
				listOpts.State = new(s)
			}
			list, _, err := p.client.Milestones.ListMilestones(pidOf(owner, repo), listOpts, gitlab.WithContext(ctx))
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListMilestones", err)
	}
	result := make([]model.Milestone, 0, len(milestones))
	for _, ms := range milestones {
		result = append(result, convertMilestone(ms))
	}
	return result, nil
}

// GetMilestone implements provider.MilestoneManager.
func (p *Provider) GetMilestone(ctx context.Context, owner, repo, number string) (*model.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(model.PlatformGitLab, "GetMilestone", number)
	if err != nil {
		return nil, err
	}
	ms, _, err := p.client.Milestones.GetMilestone(pidOf(owner, repo), id, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "GetMilestone", err)
	}
	out := convertMilestone(ms)
	return &out, nil
}

// CreateMilestone implements provider.MilestoneManager.
func (p *Provider) CreateMilestone(ctx context.Context, owner, repo string, opts provider.CreateMilestoneOptions) (*model.Milestone, error) {
	createOpts := &gitlab.CreateMilestoneOptions{
		Title:       new(opts.Title),
		Description: new(opts.Description),
	}
	if opts.DueOn != nil {
		createOpts.DueDate = new(gitlab.ISOTime(*opts.DueOn))
	}
	ms, _, err := p.client.Milestones.CreateMilestone(pidOf(owner, repo), createOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CreateMilestone", err)
	}
	out := convertMilestone(ms)
	return &out, nil
}

// UpdateMilestone implements provider.MilestoneManager. Nil fields in opts
// stay absent from the PUT body, leaving the milestone unchanged; state
// changes travel as GitLab's state_event verb.
func (p *Provider) UpdateMilestone(ctx context.Context, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*model.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(model.PlatformGitLab, "UpdateMilestone", number)
	if err != nil {
		return nil, err
	}
	updateOpts := &gitlab.UpdateMilestoneOptions{}
	if opts.Title != nil {
		updateOpts.Title = opts.Title
	}
	if opts.Description != nil {
		updateOpts.Description = opts.Description
	}
	if opts.DueOn != nil {
		updateOpts.DueDate = new(gitlab.ISOTime(*opts.DueOn))
	}
	switch opts.State {
	case model.MilestoneStateOpen:
		updateOpts.StateEvent = new("activate")
	case model.MilestoneStateClosed:
		updateOpts.StateEvent = new("close")
	case "":
	default:
		return nil, provider.Wrapf(model.PlatformGitLab, "UpdateMilestone", "unsupported milestone state %q", opts.State)
	}
	ms, _, err := p.client.Milestones.UpdateMilestone(pidOf(owner, repo), id, updateOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "UpdateMilestone", err)
	}
	out := convertMilestone(ms)
	return &out, nil
}

// DeleteMilestone implements provider.MilestoneManager.
func (p *Provider) DeleteMilestone(ctx context.Context, owner, repo, number string) error {
	id, err := backendutil.ParseMilestoneNumber(model.PlatformGitLab, "DeleteMilestone", number)
	if err != nil {
		return err
	}
	if _, err := p.client.Milestones.DeleteMilestone(pidOf(owner, repo), id, gitlab.WithContext(ctx)); err != nil {
		return provider.Wrap(model.PlatformGitLab, "DeleteMilestone", err)
	}
	return nil
}

// convertMilestone maps a gitlab.Milestone to a model.Milestone. Number
// carries the GitLab milestone ID (the identifier the write endpoints
// take); the wire state "active" maps to the SDK's "open".
func convertMilestone(m *gitlab.Milestone) model.Milestone {
	var ms model.Milestone
	if m == nil {
		return ms
	}
	ms = model.Milestone{
		Number:      strconv.FormatInt(m.ID, 10),
		Title:       m.Title,
		Description: m.Description,
	}
	switch m.State {
	case "active":
		ms.State = model.MilestoneStateOpen
	default:
		ms.State = model.MilestoneState(m.State) // "closed" (and unknowns) pass through
	}
	if m.DueDate != nil {
		due := time.Time(*m.DueDate)
		ms.DueOn = &due
	}
	return ms
}

var _ provider.MilestoneManager = (*Provider)(nil)
