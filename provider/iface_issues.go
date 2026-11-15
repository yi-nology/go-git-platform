package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// IssueManager provides issue CRUD, comments, and label management. Issue
// numbers are strings: every platform address is representable as a string,
// and Gitee natively uses alphanumeric identifiers (e.g. "IAINVA"). Backends
// on numeric platforms parse with strconv.Atoi and fail with a wrapped
// "invalid issue number" error.
type IssueManager interface {
	// ListIssues lists a repository's issues under the caller-paged
	// pagination contract (issue #51 — see NormalizePageOpts, which anchors
	// the SDK-wide Page semantics): opts.Page == 0 (the zero value, the
	// default) serves the platform's FIRST page at the normalized page
	// size — one request, no walk; opts.Page > 0 returns exactly that
	// page; callers page through explicitly until a short page comes back.
	// The int return is uniformly len(issues), the served rows' count —
	// NOT a platform total. Callers that need the total should page until
	// a short page and count, or read it off the platform's own headers
	// via a raw client.
	ListIssues(ctx context.Context, opts ListIssuesOptions) ([]*model.Issue, int, error)
	GetIssue(ctx context.Context, owner, repo, number string) (*model.Issue, error)
	CreateIssue(ctx context.Context, opts CreateIssueOptions) (*model.Issue, error)
	UpdateIssue(ctx context.Context, owner, repo, number string, opts UpdateIssueOptions) (*model.Issue, error)
	CloseIssue(ctx context.Context, owner, repo, number string) (*model.Issue, error)
	ReopenIssue(ctx context.Context, owner, repo, number string) (*model.Issue, error)
	// ListIssueComments returns the issue's comments in full, exhausting
	// the platform's pagination — the result is the complete comment list,
	// not a single page, in the platform's default order.
	ListIssueComments(ctx context.Context, owner, repo, number string) ([]*model.IssueComment, error)
	CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*model.IssueComment, error)
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
	UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*model.IssueComment, error)
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
	ListIssueLabels(ctx context.Context, owner, repo string) ([]*model.IssueLabel, error)
	AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error
	RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error
	// RemoveIssueLabels 移除多个标签（AddIssueLabels 的对称面，#163）：
	// 保序处理、首错即停——已移除的不回滚，部分失败语义在本方法单点收口，
	// 消费方不再各自循环 N 次 HTTP。labels 为空返回 nil、零请求。
	// 无原生 bulk-remove-some 的平台在后端内部循环保序收口
	// （GitHub/Gitea/Forgejo/GitCode/Gitee）；GitLab 走 remove_labels
	// 单请求、TencentCode 走读改写单次 PUT（其整集写限制见 divergence 台账）。
	RemoveIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error
}
