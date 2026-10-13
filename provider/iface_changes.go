package provider

import "context"

// ChangeRequestManager handles pull request / merge request lifecycle.
// Change request numbers are strings (same addressing scheme as
// IssueManager); numeric platforms parse with strconv and fail with a
// wrapped "invalid pull request number" error.
type ChangeRequestManager interface {
	CreateCR(ctx context.Context, opts CreateCROptions) (*ChangeRequest, error)
	GetCR(ctx context.Context, owner, repo, number string) (*ChangeRequest, error)
	// ListCRs lists change requests. total is nil when the platform
	// provided no authoritative total for the page (an unknown count is
	// reported as unknown, never as the page length); a non-nil total is
	// the platform's own metadata (exact or header-derived).
	ListCRs(ctx context.Context, opts ListCROptions) ([]*ChangeRequest, *int, error)
	MergeCR(ctx context.Context, owner, repo, number string, opts MergeCROptions) (*ChangeRequest, error)
	CloseCR(ctx context.Context, owner, repo, number string) (*ChangeRequest, error)
	ReopenCR(ctx context.Context, owner, repo, number string) (*ChangeRequest, error)
	UpdateCR(ctx context.Context, owner, repo, number string, opts UpdateCROptions) (*ChangeRequest, error)
	UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error
	ListCRComments(ctx context.Context, owner, repo, number string) ([]*CRComment, error)
	ListCRCommits(ctx context.Context, owner, repo, number string) ([]*CRCommit, error)
}
