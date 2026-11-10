package provider

import "context"

// ChangeRequestManager handles pull request / merge request lifecycle.
// Change request numbers are strings (same addressing scheme as
// IssueManager); numeric platforms parse with strconv and fail with a
// wrapped "invalid pull request number" error.
type ChangeRequestManager interface {
	CreateCR(ctx context.Context, opts CreateCROptions) (*ChangeRequest, error)
	GetCR(ctx context.Context, owner, repo, number string) (*ChangeRequest, error)
	// ListCRs lists change requests. Page semantics: caller-paged family
	// (issue #60, see NormalizePageOpts) — Page == 0 returns the platform's
	// first page in one request; Page > 0 returns exactly that page.
	//
	// total carries the platform's own metadata for the served page: exact
	// (X-Total / X-Total-Count / response payload) or header-derived
	// (GitHub's Link header yields LastPage*PerPage — an upper bound for
	// the served page, registered as a mapping divergence on the GitHub
	// backend). total is nil when the platform provided no authoritative
	// count — an unknown is reported as unknown, never as the page length.
	ListCRs(ctx context.Context, opts ListCROptions) ([]*ChangeRequest, *int, error)
	MergeCR(ctx context.Context, owner, repo, number string, opts MergeCROptions) (*ChangeRequest, error)
	CloseCR(ctx context.Context, owner, repo, number string) (*ChangeRequest, error)
	ReopenCR(ctx context.Context, owner, repo, number string) (*ChangeRequest, error)
	UpdateCR(ctx context.Context, owner, repo, number string, opts UpdateCROptions) (*ChangeRequest, error)
	UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error
	ListCRComments(ctx context.Context, owner, repo, number string) ([]*CRComment, error)
	ListCRCommits(ctx context.Context, owner, repo, number string) ([]*CRCommit, error)
}
