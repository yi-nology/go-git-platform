package gitea

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListReviews implements provider.ReviewManager.
func (p *Provider) ListReviews(ctx context.Context, owner, repo, number string) ([]provider.Review, error) {
	return giteafamily.ListReviews(ctx, p.family, p.ports, owner, repo, number)
}

// GetReview implements provider.ReviewManager.
func (p *Provider) GetReview(ctx context.Context, owner, repo, number string, reviewID int64) (*provider.Review, error) {
	return giteafamily.GetReview(ctx, p.family, p.ports, owner, repo, number, reviewID)
}

// CreateReview implements provider.ReviewManager.
func (p *Provider) CreateReview(ctx context.Context, owner, repo, number string, opts provider.CreateReviewOptions) (*provider.ReviewResult, error) {
	return giteafamily.CreateReview(ctx, p.family, p.ports, owner, repo, number, opts)
}

// RequestReviewers implements provider.ReviewManager.
func (p *Provider) RequestReviewers(ctx context.Context, owner, repo, number string, reviewers []string) error {
	return giteafamily.RequestReviewers(ctx, p.family, p.ports, owner, repo, number, reviewers)
}

// DismissReview implements provider.ReviewManager.
func (p *Provider) DismissReview(ctx context.Context, owner, repo, number string, reviewID int64, message string) error {
	return giteafamily.DismissReview(ctx, p.family, p.ports, owner, repo, number, reviewID, message)
}

// RequestTeamReviewers implements provider.TeamReviewManager (issue #79):
// the family's review-request wire carries an independent team_reviewers
// channel.
func (p *Provider) RequestTeamReviewers(ctx context.Context, owner, repo, number string, teams []string) error {
	return giteafamily.RequestTeamReviewers(ctx, p.family, p.ports, owner, repo, number, teams)
}

var (
	_ provider.ReviewManager     = (*Provider)(nil)
	_ provider.TeamReviewManager = (*Provider)(nil)
)
