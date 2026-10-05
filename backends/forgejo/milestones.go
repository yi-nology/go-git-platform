package forgejo

import (
	"context"
	"strconv"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements provider.MilestoneManager over the forgejo SDK's
// milestone surface. Forgejo addresses milestones by their ID — the same
// value MilestoneRef.Number and Milestone.Number carry on this platform.
// Wire states are forgejo's StateType ("open"/"closed"), already the SDK's
// vocabulary.

// ListMilestones implements provider.MilestoneManager. State filters by
// "open"/"closed" (forgejo also accepts "all"); forgejo defaults to open.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListMilestones(ctx context.Context, owner, repo string, opts provider.ListMilestonesOptions) ([]provider.Milestone, error) {
	listOpts := forgejo.ListMilestoneOption{}
	if opts.State != "" {
		listOpts.State = forgejo.StateType(opts.State)
	}
	var milestones []*forgejo.Milestone
	milestones, err := backendutil.PageList(opts.Page, opts.PerPage, listPageSize,
		func(page, perPage int) ([]*forgejo.Milestone, error) {
			listOpts.ListOptions = forgejo.ListOptions{Page: page, PageSize: perPage}
			list, _, err := p.client.ListRepoMilestones(owner, repo, listOpts)
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "ListMilestones", err)
	}
	result := make([]provider.Milestone, 0, len(milestones))
	for _, m := range milestones {
		result = append(result, convertMilestone(m))
	}
	return result, nil
}

// GetMilestone implements provider.MilestoneManager.
func (p *Provider) GetMilestone(ctx context.Context, owner, repo, number string) (*provider.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformForgejo, "GetMilestone", number)
	if err != nil {
		return nil, err
	}
	m, _, err := p.client.GetMilestone(owner, repo, id)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "GetMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// CreateMilestone implements provider.MilestoneManager.
func (p *Provider) CreateMilestone(ctx context.Context, owner, repo string, opts provider.CreateMilestoneOptions) (*provider.Milestone, error) {
	createOpts := forgejo.CreateMilestoneOption{
		Title:       opts.Title,
		Description: opts.Description,
		Deadline:    opts.DueOn,
	}
	m, _, err := p.client.CreateMilestone(owner, repo, createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "CreateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// UpdateMilestone implements provider.MilestoneManager. Nil fields in opts
// stay unset on the edit option: Description/State/Deadline marshal as JSON
// null and the server leaves them unchanged. Title is the SDK's only
// non-pointer edit field, so an update that does not rename sends an empty
// title — Forgejo's API keeps the existing title for blank values (a title
// is required to be non-empty, so blank cannot be a legitimate rename).
func (p *Provider) UpdateMilestone(ctx context.Context, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*provider.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformForgejo, "UpdateMilestone", number)
	if err != nil {
		return nil, err
	}
	editOpts := forgejo.EditMilestoneOption{}
	if opts.Title != nil {
		editOpts.Title = *opts.Title
	}
	if opts.Description != nil {
		editOpts.Description = opts.Description
	}
	if opts.State != "" {
		state := forgejo.StateType(opts.State)
		editOpts.State = &state
	}
	if opts.DueOn != nil {
		editOpts.Deadline = opts.DueOn
	}
	m, _, err := p.client.EditMilestone(owner, repo, id, editOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "UpdateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// DeleteMilestone implements provider.MilestoneManager.
func (p *Provider) DeleteMilestone(ctx context.Context, owner, repo, number string) error {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformForgejo, "DeleteMilestone", number)
	if err != nil {
		return err
	}
	if _, err := p.client.DeleteMilestone(owner, repo, id); err != nil {
		return provider.Wrap(provider.PlatformForgejo, "DeleteMilestone", err)
	}
	return nil
}

// convertMilestone maps a forgejo.Milestone to a provider.Milestone. Number
// carries the forgejo milestone ID (the identifier the write endpoints
// take); the SDK model keys the due date as Deadline on the wire's due_on.
func convertMilestone(m *forgejo.Milestone) provider.Milestone {
	var ms provider.Milestone
	if m == nil {
		return ms
	}
	ms = provider.Milestone{
		Number:      strconv.FormatInt(m.ID, 10),
		Title:       m.Title,
		Description: m.Description,
		State:       provider.MilestoneState(m.State),
		DueOn:       m.Deadline,
	}
	return ms
}

var _ provider.MilestoneManager = (*Provider)(nil)
