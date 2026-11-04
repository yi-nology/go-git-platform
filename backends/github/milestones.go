package github

import (
	"context"
	"strconv"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements provider.MilestoneManager over go-github's
// IssuesService milestone surface. GitHub addresses milestones by their
// per-repo number — the same value MilestoneRef.Number and Milestone.Number
// carry.

// ListMilestones implements provider.MilestoneManager. State filters by
// "open"/"closed" (GitHub also accepts "all"); GitHub defaults to open.
//
// Dual pagination mode: opts.Page > 0 means the caller manages paging
// explicitly and receives exactly that one page (opts.Page/opts.PerPage
// honored via NormalizePageOpts); opts.Page == 0 (the default) exhausts
// pagination via backendutil.AllPages — fetching 100 per page until an
// empty page — so every matching milestone is returned.
func (p *Provider) ListMilestones(ctx context.Context, owner, repo string, opts provider.ListMilestonesOptions) ([]provider.Milestone, error) {
	baseOpts := github.MilestoneListOptions{State: opts.State}
	var milestones []*github.Milestone
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		baseOpts.ListOptions = github.ListOptions{Page: page, PerPage: perPage}
		list, _, err := p.client.Issues.ListMilestones(ctx, owner, repo, &baseOpts)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "ListMilestones", err)
		}
		milestones = list
	} else {
		var err error
		milestones, err = backendutil.AllPages(func(page int) ([]*github.Milestone, error) {
			o := baseOpts
			o.ListOptions = github.ListOptions{Page: page, PerPage: 100}
			list, _, err := p.client.Issues.ListMilestones(ctx, owner, repo, &o)
			return list, err
		})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "ListMilestones", err)
		}
	}
	result := make([]provider.Milestone, 0, len(milestones))
	for _, m := range milestones {
		result = append(result, convertMilestone(m))
	}
	return result, nil
}

// GetMilestone implements provider.MilestoneManager.
func (p *Provider) GetMilestone(ctx context.Context, owner, repo, number string) (*provider.Milestone, error) {
	n, err := backendutil.ParseMilestoneNumber(provider.PlatformGitHub, "GetMilestone", number)
	if err != nil {
		return nil, err
	}
	m, _, err := p.client.Issues.GetMilestone(ctx, owner, repo, int(n))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// CreateMilestone implements provider.MilestoneManager.
func (p *Provider) CreateMilestone(ctx context.Context, owner, repo string, opts provider.CreateMilestoneOptions) (*provider.Milestone, error) {
	createOpts := &github.CreateMilestoneRequest{
		Title:       opts.Title,
		Description: new(opts.Description),
	}
	if opts.DueOn != nil {
		createOpts.DueOn = &github.Timestamp{Time: *opts.DueOn}
	}
	m, _, err := p.client.Issues.CreateMilestone(ctx, owner, repo, *createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// UpdateMilestone implements provider.MilestoneManager. Nil fields in opts
// stay absent from the PATCH body, leaving the milestone unchanged.
func (p *Provider) UpdateMilestone(ctx context.Context, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*provider.Milestone, error) {
	n, err := backendutil.ParseMilestoneNumber(provider.PlatformGitHub, "UpdateMilestone", number)
	if err != nil {
		return nil, err
	}
	editOpts := &github.UpdateMilestoneRequest{}
	if opts.Title != nil {
		editOpts.Title = opts.Title
	}
	if opts.Description != nil {
		editOpts.Description = opts.Description
	}
	if opts.State != "" {
		editOpts.State = new(string(opts.State))
	}
	if opts.DueOn != nil {
		editOpts.DueOn = &github.Timestamp{Time: *opts.DueOn}
	}
	m, _, err := p.client.Issues.UpdateMilestone(ctx, owner, repo, int(n), *editOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// DeleteMilestone implements provider.MilestoneManager.
func (p *Provider) DeleteMilestone(ctx context.Context, owner, repo, number string) error {
	n, err := backendutil.ParseMilestoneNumber(provider.PlatformGitHub, "DeleteMilestone", number)
	if err != nil {
		return err
	}
	if _, err := p.client.Issues.DeleteMilestone(ctx, owner, repo, int(n)); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DeleteMilestone", err)
	}
	return nil
}

// convertMilestone maps a github.Milestone to a provider.Milestone.
// GitHub's wire states ("open"/"closed") are already the SDK's vocabulary;
// anything else passes through unchanged.
func convertMilestone(m *github.Milestone) provider.Milestone {
	var ms provider.Milestone
	if m == nil {
		return ms
	}
	ms = provider.Milestone{
		Number:      strconv.Itoa(m.GetNumber()),
		Title:       m.GetTitle(),
		Description: m.GetDescription(),
		State:       provider.MilestoneState(m.GetState()),
	}
	if m.DueOn != nil {
		due := m.DueOn.Time
		ms.DueOn = &due
	}
	return ms
}

var _ provider.MilestoneManager = (*Provider)(nil)
