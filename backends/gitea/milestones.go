package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListMilestones implements provider.MilestoneManager.
func (p *Provider) ListMilestones(ctx context.Context, owner, repo string, opts provider.ListMilestonesOptions) ([]model.Milestone, error) {
	return giteafamily.ListMilestones(ctx, p.family, p.ports, owner, repo, opts)
}

// GetMilestone implements provider.MilestoneManager.
func (p *Provider) GetMilestone(ctx context.Context, owner, repo, number string) (*model.Milestone, error) {
	return giteafamily.GetMilestone(ctx, p.family, p.ports, owner, repo, number)
}

// CreateMilestone implements provider.MilestoneManager.
func (p *Provider) CreateMilestone(ctx context.Context, owner, repo string, opts provider.CreateMilestoneOptions) (*model.Milestone, error) {
	return giteafamily.CreateMilestone(ctx, p.family, p.ports, owner, repo, opts)
}

// UpdateMilestone implements provider.MilestoneManager.
func (p *Provider) UpdateMilestone(ctx context.Context, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*model.Milestone, error) {
	return giteafamily.UpdateMilestone(ctx, p.family, p.ports, owner, repo, number, opts)
}

// DeleteMilestone implements provider.MilestoneManager.
func (p *Provider) DeleteMilestone(ctx context.Context, owner, repo, number string) error {
	return giteafamily.DeleteMilestone(ctx, p.family, p.ports, owner, repo, number)
}

var _ provider.MilestoneManager = (*Provider)(nil)
