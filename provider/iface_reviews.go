package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// ReviewManager provides code-review operations on change requests. Change
// request numbers are strings (same addressing scheme as IssueManager);
// individual reviews are addressed by their numeric platform ID. It is an
// optional capability: consumers should check Capabilities().Reviews before
// type-asserting.
type ReviewManager interface {
	CreateReview(ctx context.Context, owner, repo, number string, opts CreateReviewOptions) (*model.ReviewResult, error)
	ListReviews(ctx context.Context, owner, repo, number string) ([]model.Review, error)
	GetReview(ctx context.Context, owner, repo, number string, reviewID int64) (*model.Review, error)
	RequestReviewers(ctx context.Context, owner, repo, number string, reviewers []string) error
	DismissReview(ctx context.Context, owner, repo, number string, reviewID int64, message string) error
}
