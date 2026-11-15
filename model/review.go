package model

import (
	"time"
)

// ReviewComment is a single inline comment in a code review.
type ReviewComment struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	Line      int    `json:"line,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Side      string `json:"side,omitempty"`
}

// Mentions returns the deduplicated @usernames found in the comment body.
func (c *ReviewComment) Mentions() []string { return ExtractMentions(c.Body) }

// ReviewResult is the result of a CreateReview call.
type ReviewResult struct {
	ID       string                `json:"id"`
	Body     string                `json:"body,omitempty"`
	HTMLURL  string                `json:"html_url,omitempty"`
	User     *CRUser               `json:"user,omitempty"`
	Comments []ReviewCommentResult `json:"comments,omitempty"`
}

// ReviewCommentResult is the result of posting a single inline comment.
type ReviewCommentResult struct {
	Path       string `json:"path,omitempty"`
	Line       int    `json:"line,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ReviewState is the normalized state of a code review.
type ReviewState string

const (
	ReviewStateApproved         ReviewState = "approved"
	ReviewStateChangesRequested ReviewState = "changes_requested"
	ReviewStateCommented        ReviewState = "commented"
	ReviewStatePending          ReviewState = "pending"
)

// Review represents a code review on a change request (the ReviewManager
// view; ReviewResult above is the create-call response).
type Review struct {
	ID          int64       `json:"id"`
	User        string      `json:"user"`
	State       ReviewState `json:"state"`
	Body        string      `json:"body"`
	SubmittedAt time.Time   `json:"submitted_at"`
}
