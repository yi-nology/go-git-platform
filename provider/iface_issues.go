package provider

import "context"

// IssueManager provides issue CRUD, comments, and label management. Issue
// numbers are strings: every platform address is representable as a string,
// and Gitee natively uses alphanumeric identifiers (e.g. "IAINVA"). Backends
// on numeric platforms parse with strconv.Atoi and fail with a wrapped
// "invalid issue number" error.
type IssueManager interface {
	ListIssues(ctx context.Context, opts ListIssuesOptions) ([]*Issue, int, error)
	GetIssue(ctx context.Context, owner, repo, number string) (*Issue, error)
	CreateIssue(ctx context.Context, opts CreateIssueOptions) (*Issue, error)
	UpdateIssue(ctx context.Context, owner, repo, number string, opts UpdateIssueOptions) (*Issue, error)
	CloseIssue(ctx context.Context, owner, repo, number string) (*Issue, error)
	ReopenIssue(ctx context.Context, owner, repo, number string) (*Issue, error)
	// ListIssueComments returns the issue's comments in full, exhausting
	// the platform's pagination — the result is the complete comment list,
	// not a single page, in the platform's default order.
	ListIssueComments(ctx context.Context, owner, repo, number string) ([]*IssueComment, error)
	CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*IssueComment, error)
	// UpdateIssueComment replaces a comment's body wholesale: every
	// platform's edit surface is a body-only PATCH/PUT with no
	// partial-update semantics, mirroring CreateIssueComment. The platform
	// enforces authorship — only the comment's author (typically the token
	// identity, e.g. a review bot updating its own comment) may edit — so
	// the call fails for anyone else's comment. commentID is the
	// IssueComment.ID from CreateIssueComment or ListIssueComments. number
	// is ignored on platforms whose edit endpoint addresses the comment
	// directly (GitHub, Gitea, Forgejo, GitCode, Gitee); GitLab and Tencent
	// 工蜂 route through the issue, so it must carry that issue's number.
	UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*IssueComment, error)
	// DeleteIssueComment removes a comment. commentID is the
	// IssueComment.ID from CreateIssueComment or ListIssueComments, the
	// same address space UpdateIssueComment writes to; the platform
	// enforces authorship — only the comment's author (typically the token
	// identity rotating its own report comment) may delete — so the call
	// fails for anyone else's comment. number is ignored on platforms
	// whose delete endpoint addresses the comment directly (GitHub, Gitea,
	// Forgejo, GitCode, Gitee); GitLab and Tencent 工蜂 route through the
	// issue, so it must carry that issue's number. Deletion is permanent:
	// callers wanting "edit instead" semantics (summary-comment rotation
	// that preserves history) should prefer UpdateIssueComment.
	DeleteIssueComment(ctx context.Context, owner, repo, number string, commentID int64) error
	ListIssueLabels(ctx context.Context, owner, repo string) ([]*IssueLabel, error)
	AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error
	RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error
}
