package gitcode

import (
	"context"
	"strconv"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListReviews implements provider.ReviewManager via
// ListPullRequestReviews (GET .../pulls/{n}/reviews).
func (p *Provider) ListReviews(ctx context.Context, owner, repo, number string) ([]provider.Review, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "ListReviews", number)
	if err != nil {
		return nil, err
	}
	reviews, err := p.client.ListPullRequestReviews(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListReviews", err)
	}
	result := make([]provider.Review, 0, len(reviews))
	for _, r := range reviews {
		result = append(result, convertReview(r))
	}
	return result, nil
}

// GetReview implements provider.ReviewManager via
// GetPullRequestReview (GET .../pulls/{n}/reviews/{id}).
func (p *Provider) GetReview(ctx context.Context, owner, repo, number string, reviewID int64) (*provider.Review, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "GetReview", number)
	if err != nil {
		return nil, err
	}
	review, err := p.client.GetPullRequestReview(ctx, owner, repo, n, reviewID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetReview", err)
	}
	r := convertReview(review)
	return &r, nil
}

// CreateReview implements provider.ReviewManager via
// CreatePullRequestReview (POST .../pulls/{n}/reviews with body+event). It
// moved here from DiffManager when the review capability was split out.
//
// The gitcode API's review create accepts only a summary body and an event,
// so inline comments are posted individually after the review itself; a
// failed inline comment is logged but does not fail the review. If the
// review endpoint itself fails, the pre-P3 fallback path posts the inline
// comments and a plain note instead of giving up.
func (p *Provider) CreateReview(ctx context.Context, owner, repo, number string, opts provider.CreateReviewOptions) (*provider.ReviewResult, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "CreateReview", number)
	if err != nil {
		return nil, err
	}
	review, err := p.client.CreatePullRequestReview(ctx, owner, repo, n, opts.Body, opts.Event)
	if err != nil {
		return p.createReviewFallback(ctx, owner, repo, number, opts)
	}
	result := &provider.ReviewResult{ID: strconv.FormatInt(review.ID, 10)}
	user := review.User
	if user == nil {
		user = review.Author
	}
	if user != nil {
		authorID, _ := parseGitCodeID(user.ID)
		result.User = &provider.CRUser{
			ID: authorID, Username: user.Login, AvatarURL: user.AvatarURL,
		}
	}
	for _, c := range opts.Comments {
		if err := p.createInlineComment(ctx, owner, repo, n, c, opts.CommitID); err != nil && p.logger != nil {
			p.logger.Warn("inline comment failed", "path", c.Path, "line", c.Line, "error", err)
		}
	}
	return result, nil
}

// createReviewFallback posts the review as individual inline comments plus a
// top-level note when the dedicated review endpoint rejects the request.
func (p *Provider) createReviewFallback(ctx context.Context, owner, repo, number string, opts provider.CreateReviewOptions) (*provider.ReviewResult, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "CreateReview", number)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, c := range opts.Comments {
		if err := p.createInlineComment(ctx, owner, repo, n, c, opts.CommitID); err != nil {
			lastErr = err
		}
	}
	if opts.Body != "" {
		if _, err := p.CreateNote(ctx, owner, repo, number, opts.Body); err != nil {
			lastErr = err
		}
	}
	if lastErr != nil && len(opts.Comments) == 0 {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateReview", lastErr)
	}
	return &provider.ReviewResult{}, nil
}

// createInlineComment posts one inline (file/line-anchored) review comment.
func (p *Provider) createInlineComment(ctx context.Context, owner, repo string, number int, comment provider.ReviewComment, commitID string) error {
	side := comment.Side
	if side == "" {
		side = "RIGHT"
	}
	_, err := p.client.CreatePullRequestInlineComment(ctx, owner, repo, number, gitcode.CreatePullRequestInlineCommentOptions{
		Body: comment.Body, Path: comment.Path, Line: comment.Line, Side: side, CommitID: commitID,
	})
	if err != nil {
		return provider.Wrap(provider.PlatformGitCode, "createInlineComment", err)
	}
	return nil
}

// RequestReviewers implements provider.ReviewManager via
// RequestPullRequestReviewers (POST .../pulls/{n}/requested_reviewers with
// the reviewer logins under the same "reviewers" wire key as GitHub).
func (p *Provider) RequestReviewers(ctx context.Context, owner, repo, number string, reviewers []string) error {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "RequestReviewers", number)
	if err != nil {
		return err
	}
	if err := p.client.RequestPullRequestReviewers(ctx, owner, repo, n, gitcode.PullRequestReviewRequest{
		Reviewers: reviewers,
	}); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "RequestReviewers", err)
	}
	return nil
}

// DismissReview implements provider.ReviewManager via
// DismissPullRequestReview (PUT .../pulls/{n}/reviews/{id}/dismissals with
// the dismissal message).
func (p *Provider) DismissReview(ctx context.Context, owner, repo, number string, reviewID int64, message string) error {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "DismissReview", number)
	if err != nil {
		return err
	}
	if err := p.client.DismissPullRequestReview(ctx, owner, repo, n, reviewID, message); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DismissReview", err)
	}
	return nil
}

var _ provider.ReviewManager = (*Provider)(nil)
