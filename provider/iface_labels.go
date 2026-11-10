package provider

import "context"

// LabelManager provides repository-level label CRUD. It is an optional
// capability interface: consumers should gate on Provider.Capabilities()
// (or type-assert) before use. Labels are addressed by name; backends whose
// platform API addresses labels by numeric ID (GitLab, Gitea, Forgejo)
// resolve the name internally. Such backends scan labels with server-side
// pagination (100 per page, bounded to 50 pages); beyond that bound a label
// may be reported as not found by UpdateLabel/DeleteLabel even though it
// exists.
//
// The issue-scoped operations (ListIssueLabels, AddIssueLabels,
// RemoveIssueLabel) remain on IssueManager because they operate on an issue,
// not on the repository's label set.
type LabelManager interface {
	// ListLabels lists the repository's labels. Page semantics: exhaustive
	// family (issue #60, see NormalizePageOpts) — Page == 0 walks every
	// page and returns all labels; Page > 0 returns exactly that page.
	ListLabels(ctx context.Context, owner, repo string, opts ListLabelsOptions) ([]*Label, error)
	// CreateLabel creates a repository label.
	CreateLabel(ctx context.Context, owner, repo string, opts CreateLabelOptions) (*Label, error)
	// UpdateLabel updates the label with the given name. Nil fields in opts
	// are left unchanged.
	UpdateLabel(ctx context.Context, owner, repo, name string, opts UpdateLabelOptions) (*Label, error)
	// DeleteLabel deletes the label with the given name.
	DeleteLabel(ctx context.Context, owner, repo, name string) error
}
