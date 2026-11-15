package github

import (
	"context"
	"strconv"
	"strings"

	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListReviews implements provider.ReviewManager. The provider interface
// exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListReviews(ctx context.Context, owner, repo, number string) ([]model.Review, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "ListReviews", number)
	if err != nil {
		return nil, err
	}
	reviews, err := backendutil.AllPages(func(page int) ([]*github.PullRequestReview, error) {
		list, _, err := p.client.PullRequests.ListReviews(ctx, owner, repo, n, &github.ListOptions{
			Page: page, PerPage: 100,
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "ListReviews", err)
	}
	result := make([]model.Review, 0, len(reviews))
	for _, r := range reviews {
		result = append(result, convertReview(r))
	}
	return result, nil
}

// GetReview implements provider.ReviewManager.
func (p *Provider) GetReview(ctx context.Context, owner, repo, number string, reviewID int64) (*model.Review, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "GetReview", number)
	if err != nil {
		return nil, err
	}
	review, _, err := p.client.PullRequests.GetReview(ctx, owner, repo, n, reviewID)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "GetReview", err)
	}
	r := convertReview(review)
	return &r, nil
}

// CreateReview implements provider.ReviewManager. It moved here from
// DiffManager; the change request number is now addressed as a string.
func (p *Provider) CreateReview(ctx context.Context, owner, repo, number string, opts provider.CreateReviewOptions) (*model.ReviewResult, error) {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "CreateReview", number)
	if err != nil {
		return nil, err
	}
	reviewRequest := &github.PullRequestReviewRequest{
		CommitID: new(opts.CommitID),
		Body:     new(opts.Body),
		Event:    new(opts.Event),
	}
	for _, c := range opts.Comments {
		rc := &github.DraftReviewComment{
			Path: new(c.Path),
			Body: new(c.Body),
		}
		if c.StartLine > 0 && c.EndLine > c.StartLine {
			rc.StartLine = new(c.StartLine)
			rc.Line = new(c.EndLine)
			if c.Side != "" {
				rc.Side = new(c.Side)
			} else {
				rc.Side = new("RIGHT")
			}
			if c.StartLine != c.EndLine {
				rc.StartSide = new("RIGHT")
			}
		} else if c.Line > 0 {
			rc.Line = new(c.Line)
			if c.Side != "" {
				rc.Side = new(c.Side)
			} else {
				rc.Side = new("RIGHT")
			}
		}
		reviewRequest.Comments = append(reviewRequest.Comments, rc)
	}
	review, _, err := p.client.PullRequests.CreateReview(ctx, owner, repo, n, reviewRequest)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "CreateReview", err)
	}
	result := &model.ReviewResult{
		ID: strconv.FormatInt(review.GetID(), 10),
	}
	if review.GetHTMLURL() != "" {
		result.HTMLURL = review.GetHTMLURL()
	}
	if review.GetUser() != nil {
		result.User = convertUser(review.GetUser())
	}
	return result, nil
}

// RequestReviewers implements provider.ReviewManager.
func (p *Provider) RequestReviewers(ctx context.Context, owner, repo, number string, reviewers []string) error {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "RequestReviewers", number)
	if err != nil {
		return err
	}
	req := github.ReviewersRequest{Reviewers: reviewers}
	if _, _, err := p.client.PullRequests.RequestReviewers(ctx, owner, repo, n, req); err != nil {
		return provider.Wrap(model.PlatformGitHub, "RequestReviewers", err)
	}
	return nil
}

// RequestTeamReviewers implements provider.TeamReviewManager (issue #79):
// GitHub's review-request endpoint carries an independent team_reviewers
// channel next to the user reviewers; the unified ReviewManager only
// expresses the user half.
func (p *Provider) RequestTeamReviewers(ctx context.Context, owner, repo, number string, teams []string) error {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "RequestTeamReviewers", number)
	if err != nil {
		return err
	}
	req := github.ReviewersRequest{TeamReviewers: teams}
	if _, _, err := p.client.PullRequests.RequestReviewers(ctx, owner, repo, n, req); err != nil {
		return provider.Wrap(model.PlatformGitHub, "RequestTeamReviewers", err)
	}
	return nil
}

// DismissReview implements provider.ReviewManager.
func (p *Provider) DismissReview(ctx context.Context, owner, repo, number string, reviewID int64, message string) error {
	n, err := backendutil.ParsePRNumber(model.PlatformGitHub, "DismissReview", number)
	if err != nil {
		return err
	}
	if _, _, err := p.client.PullRequests.DismissReview(ctx, owner, repo, n, reviewID, github.PullRequestDismissReviewRequest{
		Message: message,
	}); err != nil {
		return provider.Wrap(model.PlatformGitHub, "DismissReview", err)
	}
	return nil
}

// convertReview maps a github.PullRequestReview to a model.Review,
// normalizing the UPPERCASE wire states to the SDK's lowercase ReviewState
// constants. Unknown states pass through lowercased rather than being
// silently dropped.
func convertReview(r *github.PullRequestReview) model.Review {
	var review model.Review
	if r == nil {
		return review
	}
	review = model.Review{
		ID:   r.GetID(),
		User: r.GetUser().GetLogin(),
		Body: r.GetBody(),
	}
	switch r.GetState() {
	case "APPROVED":
		review.State = model.ReviewStateApproved
	case "CHANGES_REQUESTED":
		review.State = model.ReviewStateChangesRequested
	case "COMMENTED":
		review.State = model.ReviewStateCommented
	case "PENDING":
		review.State = model.ReviewStatePending
	default:
		if s := r.GetState(); s != "" {
			review.State = model.ReviewState(strings.ToLower(s))
		}
	}
	if t := r.GetSubmittedAt(); !t.IsZero() {
		review.SubmittedAt = t.Time
	}
	return review
}

var (
	_ provider.ReviewManager     = (*Provider)(nil)
	_ provider.TeamReviewManager = (*Provider)(nil)
)
