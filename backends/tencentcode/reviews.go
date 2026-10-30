package tencentcode

import (
	"context"
	"strconv"

	gongfeng "github.com/yi-nology/go-gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements provider.ReviewManager over the gongfeng SDK's
// NotesService: 工蜂's native reviews are expressed as merge-request notes
// carrying an optional reviewer_state verdict, so CreateReview posts an MR
// note, ListReviews lists the MR's notes, and GetReview fetches a single
// note by ID — the same collection end to end. Four registered limitations
// apply:
//
//   - state on reads: the gongfeng Note model has no state field (the
//     reviewer_state verdict travels on writes only and never comes back),
//     so every review read normalizes to provider.ReviewStateCommented.
//   - verdict vocabulary: only APPROVE and REQUEST_CHANGES map to 工蜂's
//     reviewer_state verbs (approved / change_required); comment-style
//     events post a plain note with no verdict. 工蜂's third verb
//     (change_denied) has no SDK-side event and is never produced.
//   - inline comments and commit pinning: opts.Comments and opts.CommitID
//     have no review-note equivalent — a note carries at most one inline
//     position (path/line), and posting one note per comment would fabricate
//     separate review records — so they are ignored.
//   - dismissal: 工蜂 exposes no review-dismissal surface at all (see
//     DismissReview's registered stub).

// CreateReview implements provider.ReviewManager by posting a merge-request
// note: the body becomes the note text and the event maps to the note's
// reviewer_state verdict verb (same vocabulary the reviews/{id} commit-note
// surface uses — the gongfeng SDK's bundled docs/api/notes.md). See the file doc for the registered
// unmapped fields (Comments, CommitID) and the verdict vocabulary.
func (p *Provider) CreateReview(ctx context.Context, owner, repo, number string, opts provider.CreateReviewOptions) (*provider.ReviewResult, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "CreateReview", number)
	if err != nil {
		return nil, err
	}
	createOpts := &gongfeng.CreateMergeRequestNoteOptions{Body: gongfeng.Ptr(opts.Body)}
	if state := reviewNoteState(opts.Event); state != "" {
		createOpts.ReviewerState = gongfeng.Ptr(state)
	}
	note, _, err := p.client.Notes.CreateMergeRequestNote(ctx, pid(owner, repo), n, createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateReview", err)
	}
	return convertReviewNoteResult(note), nil
}

// ListReviews implements provider.ReviewManager by listing the change
// request's merge-request notes. The collection can mix in 工蜂's system
// bookkeeping notes ("milestone removed" and the like); those are not
// reviews and are filtered out. The endpoint has no caller-facing
// pagination knobs, so every page is fetched via AllPages (工蜂's
// page-size ceiling is 100) — the same fetch ListCRComments uses.
func (p *Provider) ListReviews(ctx context.Context, owner, repo, number string) ([]provider.Review, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "ListReviews", number)
	if err != nil {
		return nil, err
	}
	notes, err := backendutil.AllPages(func(page int) ([]*gongfeng.Note, error) {
		list, _, err := p.client.Notes.ListMergeRequestNotes(ctx, pid(owner, repo), n,
			&gongfeng.ListMergeRequestNotesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListReviews", err)
	}
	reviews := make([]provider.Review, 0, len(notes))
	for _, note := range notes {
		if note == nil || note.System {
			continue
		}
		reviews = append(reviews, convertReviewNote(note))
	}
	return reviews, nil
}

// GetReview implements provider.ReviewManager by fetching the single
// merge-request note backing the review: reviewID is the note ID — the same
// identifier CreateReview returns as ReviewResult.ID, so created reviews
// round-trip through this call.
func (p *Provider) GetReview(ctx context.Context, owner, repo, number string, reviewID int64) (*provider.Review, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "GetReview", number)
	if err != nil {
		return nil, err
	}
	note, _, err := p.client.Notes.GetMergeRequestNote(ctx, pid(owner, repo), n, int(reviewID))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetReview", err)
	}
	review := convertReviewNote(note)
	return &review, nil
}

// RequestReviewers implements provider.ReviewManager as a registered ignore.
//
// Registered mapping: IGNORED. 工蜂's merge-request update surface cannot
// add reviewers (gongfeng's UpdateMergeRequestOptions carries no reviewer
// field), and the native invite endpoint addresses reviewers by numeric user
// ID (InviteMRReviewer's reviewer_id) while the SDK's callers supply
// usernames — the Users API offers no by-username lookup to bridge them
// (the same class of limitation as CreateIssue's ignored Assignees). The
// call therefore succeeds without effect and touches no wire.
func (p *Provider) RequestReviewers(ctx context.Context, owner, repo, number string, reviewers []string) error {
	return nil
}

// DismissReview implements provider.ReviewManager as a registered stub —
// the only registered stub among the optional-capability interfaces (the
// SDK's other stubs, e.g. ChangeRequestManager.UpdateCRLabels, live on core
// interfaces).
//
// Registered mapping: STUB. 工蜂's review surface has no dismissal verb:
// review notes can be created, listed, fetched, and edited
// (UpdateReviewNote rewrites the body or reviewer_state), but no endpoint
// removes or voids a review, and the dismissal message has no 工蜂
// equivalent. The call fails fast with a platform error wrapping
// provider.ErrNotImplemented.
func (p *Provider) DismissReview(ctx context.Context, owner, repo, number string, reviewID int64, message string) error {
	return provider.Wrap(provider.PlatformTencentCode, "DismissReview", provider.ErrNotImplemented)
}

// reviewNoteState maps the SDK's review-event vocabulary to 工蜂's
// reviewer_state verbs. Comment-style events carry no verdict (registered;
// see the file doc).
func reviewNoteState(event string) string {
	switch event {
	case "APPROVE":
		return "approved"
	case "REQUEST_CHANGES":
		return "change_required"
	default:
		return ""
	}
}

// convertReviewNote maps a gongfeng merge-request note to a
// provider.Review. The note model carries no verdict field, so State is
// always commented (registered limitation); SubmittedAt carries the note's
// creation time.
func convertReviewNote(n *gongfeng.Note) provider.Review {
	if n == nil {
		return provider.Review{}
	}
	review := provider.Review{
		ID:          int64(n.ID),
		State:       provider.ReviewStateCommented,
		Body:        n.Body,
		SubmittedAt: n.CreatedAt.Time,
	}
	if n.Author != nil {
		review.User = n.Author.Username
	}
	return review
}

// convertReviewNoteResult maps the created merge-request note to a
// provider.ReviewResult; ID carries the note ID, the identifier GetReview
// addresses the review by.
func convertReviewNoteResult(n *gongfeng.Note) *provider.ReviewResult {
	if n == nil {
		return nil
	}
	return &provider.ReviewResult{
		ID:   strconv.Itoa(n.ID),
		Body: n.Body,
		User: convertUser(n.Author),
	}
}

var _ provider.ReviewManager = (*Provider)(nil)
