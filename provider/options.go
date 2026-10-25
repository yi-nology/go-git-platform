package provider

import "time"

// This file consolidates every option/result type used by the Provider
// sub-interfaces. The split between "options" (what callers pass in) and
// "result types" (what platforms return) is intentional so consumers can
// scan a single file to see the cross-platform API surface.
//
// Option field conventions:
//   - Update*Options (e.g. UpdateIssueOptions, UpdateCROptions): zero-value
//     fields (empty string, false bool) are treated as "leave unchanged".
//     There is no way to explicitly set a field to its zero value.
//   - Update*Options with pointer fields (e.g. UpdateLabelOptions,
//     UpdateMilestoneOptions): nil means "leave unchanged", non-nil means
//     "set to this value" (including zero values like "" or false).
//   - Create*Options: all fields are used as-is; zero values mean "use
//     platform default".

// --- Listing / pagination ---

// ListRepoOptions contains options for listing repositories on a platform.
type ListRepoOptions struct {
	Owner   string `json:"owner,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

// ListCommitsOptions contains options for listing commits in a repository.
type ListCommitsOptions struct {
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Since   string `json:"since,omitempty"` // RFC3339
	Until   string `json:"until,omitempty"` // RFC3339
}

// --- Change requests (PRs / MRs) ---

// CreateCROptions contains options for creating a change request.
type CreateCROptions struct {
	Owner              string   `json:"owner"`
	Repo               string   `json:"repo"`
	Title              string   `json:"title"`
	Description        string   `json:"description,omitempty"`
	SourceBranch       string   `json:"source_branch"`
	TargetBranch       string   `json:"target_branch"`
	Labels             []string `json:"labels,omitempty"`
	RemoveSourceBranch bool     `json:"remove_source_branch,omitempty"`
}

// ListCROptions contains options for listing change requests.
type ListCROptions struct {
	Owner        string  `json:"owner"`
	Repo         string  `json:"repo"`
	State        CRState `json:"state,omitempty"`
	SourceBranch string  `json:"source_branch,omitempty"`
	TargetBranch string  `json:"target_branch,omitempty"`
	Page         int     `json:"page,omitempty"`
	PerPage      int     `json:"per_page,omitempty"`
}

// MergeCROptions contains options for merging a change request.
type MergeCROptions struct {
	MergeCommitMessage string `json:"merge_commit_message,omitempty"`
	Squash             bool   `json:"squash,omitempty"`
	RemoveSourceBranch bool   `json:"remove_source_branch,omitempty"`
}

// UpdateCROptions contains options for updating a change request.
type UpdateCROptions struct {
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`
}

// DiscussionOptions contains options for creating a discussion comment.
type DiscussionOptions struct {
	Body         string `json:"body"`
	FilePath     string `json:"file_path,omitempty"`
	NewLine      int    `json:"new_line,omitempty"`
	OldLine      int    `json:"old_line,omitempty"`
	StartNewLine int    `json:"start_new_line,omitempty"`
	BaseSHA      string `json:"base_sha,omitempty"`
	StartSHA     string `json:"start_sha,omitempty"`
	HeadSHA      string `json:"head_sha,omitempty"`
}

// CreateReviewOptions contains options for creating a review.
type CreateReviewOptions struct {
	CommitID string          `json:"commit_id,omitempty"`
	Event    string          `json:"event,omitempty"`
	Body     string          `json:"body,omitempty"`
	Comments []ReviewComment `json:"comments,omitempty"`
}

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

// CommitStatusOptions contains options for creating a commit status.
type CommitStatusOptions struct {
	State       string `json:"state"`
	Context     string `json:"context"`
	Description string `json:"description,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
}

// --- Repositories ---

// CreateRepoOptions contains options for creating a repository.
type CreateRepoOptions struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Private       bool   `json:"private,omitempty"`
	AutoInit      bool   `json:"auto_init,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

// UpdateRepoOptions contains options for updating a repository.
type UpdateRepoOptions struct {
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Private       *bool  `json:"private,omitempty"`
}

// ForkRepoOptions contains options for forking a repository.
type ForkRepoOptions struct {
	Organization string `json:"organization,omitempty"`
	Name         string `json:"name,omitempty"`
}

// --- Files ---

// FileOptions contains options for creating or updating a file.
type FileOptions struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Message string `json:"message"`
	Branch  string `json:"branch,omitempty"`
	SHA     string `json:"sha,omitempty"`
	Author  string `json:"author,omitempty"`
	Email   string `json:"email,omitempty"`
}

// FileDeleteOptions contains options for deleting a file.
type FileDeleteOptions struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Branch  string `json:"branch,omitempty"`
	SHA     string `json:"sha,omitempty"`
	Author  string `json:"author,omitempty"`
	Email   string `json:"email,omitempty"`
}

// FileResult is the result of a file operation.
type FileResult struct {
	SHA       string `json:"sha,omitempty"`
	CommitSHA string `json:"commit_sha,omitempty"`
}

// --- Webhooks ---

// CreateWebhookOptions contains options for creating a webhook.
type CreateWebhookOptions struct {
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events,omitempty"`
}

// --- Releases ---

// CreateReleaseOptions contains options for creating a release.
type CreateReleaseOptions struct {
	TagName    string `json:"tag_name"`
	Target     string `json:"target,omitempty"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	Draft      bool   `json:"draft,omitempty"`
	Prerelease bool   `json:"prerelease,omitempty"`
}

// UpdateReleaseOptions contains options for updating a release addressed by
// tag. Nil fields are left unchanged.
type UpdateReleaseOptions struct {
	Name       *string `json:"name,omitempty"`
	Body       *string `json:"body,omitempty"`
	Draft      *bool   `json:"draft,omitempty"`
	Prerelease *bool   `json:"prerelease,omitempty"`
}

// --- Issues ---

// IssueState represents the state of an issue.
type IssueState string

const (
	IssueStateOpen   IssueState = "open"
	IssueStateClosed IssueState = "closed"
)

// Issue represents an issue on a platform. Number is the platform's issue
// identifier as a string (numeric on every current platform except Gitee,
// whose identifiers are alphanumeric).
type Issue struct {
	ID        int64         `json:"id"`
	Number    string        `json:"number"`
	Title     string        `json:"title"`
	Body      string        `json:"body"`
	State     IssueState    `json:"state"`
	Author    *CRUser       `json:"author,omitempty"`
	Labels    []string      `json:"labels,omitempty"`
	Assignees []string      `json:"assignees,omitempty"`
	Milestone *MilestoneRef `json:"milestone,omitempty"`
	WebURL    string        `json:"web_url,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	ClosedAt  *time.Time    `json:"closed_at,omitempty"`
	// IsPullRequest 标记这条"issue"其实是 PR。
	//
	// **GitHub 的 issue 列表端点会把 PR 一起返回**(同一条记录既是 issue 也是 PR),
	// 不标记的话消费方没法把两者分开 ⇒ 同一个 PR 既进 issues 快照又进
	// pull_requests 快照,而且**占掉 issue 的数量预算**(默认 500)
	// ⇒ PR 多的仓里真实 issue 可能整批落在预算之外,快照缺项且无任何告警。
	//
	// Gitea/Forgejo 家族的列表端点有 type=issues 过滤(本来就不含 PR),
	// GitLab 的 issues 端点也不含 MR ⇒ 只有 GitHub 需要这个标记。
	IsPullRequest bool `json:"is_pull_request,omitempty"`
}

// MilestoneRef references a milestone from an issue. Number carries the
// platform's milestone addressing identifier as a string: the milestone
// *number* on GitHub, the platform milestone *ID* on GitLab, Gitea,
// Forgejo, GitCode, and Tencent Code (whose write endpoints take exactly
// that identifier, so per-platform round-trips hold), and Gitee's
// milestone *serial number* (the "number" field of Gitee's milestone
// payload — the identifier Gitee's own issue and milestone write endpoints
// take; the SDK model exposes no id). This is the same identifier
// Milestone.Number exposes and MilestoneManager methods accept, so refs
// round-trip through the milestone manager on the platform they came from.
type MilestoneRef struct {
	Number string `json:"number"`
	Title  string `json:"title,omitempty"`
}

// IssueComment represents a comment on an issue.
type IssueComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	Author    *CRUser   `json:"author,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Mentions returns the deduplicated @usernames found in the comment body.
func (c *IssueComment) Mentions() []string { return ExtractMentions(c.Body) }

// IssueLabel represents a label on a repository.
type IssueLabel struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// ListIssuesOptions contains options for listing issues.
type ListIssuesOptions struct {
	Owner    string     `json:"owner"`
	Repo     string     `json:"repo"`
	State    IssueState `json:"state,omitempty"`
	Assignee string     `json:"assignee,omitempty"`
	Labels   string     `json:"labels,omitempty"`
	Page     int        `json:"page,omitempty"`
	PerPage  int        `json:"per_page,omitempty"`
	// Sort 排序字段("created"|"updated"|"comments"),空 = 平台默认。
	//
	// **增量备份必须用它**:平台默认按 created 倒序 ⇒ "上次备份后被改动的
	// **老** issue"正好被挤在第一页之外,客户端再按 updated 过滤也捞不到
	// (捞到的永远是"最新创建"的那批)。
	//
	// 支持情况:GitHub 支持(sort=updated);Gitea/Forgejo 家族当前 SDK 不暴露该
	// 参数 ⇒ 传入被忽略(调用方需自己兜底,见 core 的增量采集)。
	Sort string `json:"sort,omitempty"`
	// Direction 排序方向("asc"|"desc"),空 = 平台默认。
	Direction string `json:"direction,omitempty"`
}

// CreateIssueOptions contains options for creating an issue.
type CreateIssueOptions struct {
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	Milestone string   `json:"milestone,omitempty"` // milestone number/ID as a string; "" = do not set
}

// UpdateIssueOptions contains options for updating an issue.
type UpdateIssueOptions struct {
	Title     string     `json:"title,omitempty"`
	Body      string     `json:"body,omitempty"`
	State     IssueState `json:"state,omitempty"`
	Assignees []string   `json:"assignees,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	Milestone string     `json:"milestone,omitempty"` // milestone number/ID as a string; "" = leave unchanged
}

// --- Labels ---

// Label represents a repository label. Color is canonicalized to 6-digit hex
// without a leading '#' (e.g. "ff0000"); backends add the '#' when a platform
// requires it (GitLab, Gitea, Forgejo) and strip it on the way in.
type Label struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

// ListLabelsOptions contains options for listing repository labels.
type ListLabelsOptions struct {
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

// CreateLabelOptions contains options for creating a repository label.
// Color uses the canonical 6-digit hex form without '#' (e.g. "ff0000").
type CreateLabelOptions struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
}

// UpdateLabelOptions contains options for updating a repository label.
// Nil fields are left unchanged.
type UpdateLabelOptions struct {
	NewName     *string `json:"new_name,omitempty"`
	Color       *string `json:"color,omitempty"`
	Description *string `json:"description,omitempty"`
}

// --- Milestones ---

// MilestoneState represents the state of a milestone.
type MilestoneState string

const (
	MilestoneStateOpen   MilestoneState = "open"
	MilestoneStateClosed MilestoneState = "closed"
)

// Milestone represents a repository milestone. Number carries the
// platform's milestone addressing identifier as a string — the same value
// MilestoneRef.Number uses and MilestoneManager methods accept: the
// milestone number on GitHub, the platform milestone ID on GitLab, Gitea,
// Forgejo, GitCode, and Tencent Code, and the milestone serial number on
// Gitee (see MilestoneManager for the per-platform truth).
type Milestone struct {
	Number      string         `json:"number"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	State       MilestoneState `json:"state"`
	DueOn       *time.Time     `json:"due_on,omitempty"`
}

// ListMilestonesOptions contains options for listing repository
// milestones. State filters by "open" or "closed"; an empty State lists
// whatever the platform defaults to (GitHub/Gitea/Forgejo/Gitee default to
// open, GitLab to all). Tencent Code ignores State entirely — gongfeng's
// list options expose pagination only, so all states are listed.
type ListMilestonesOptions struct {
	State   string `json:"state,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

// CreateMilestoneOptions contains options for creating a repository
// milestone.
type CreateMilestoneOptions struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	DueOn       *time.Time `json:"due_on,omitempty"`
}

// UpdateMilestoneOptions contains options for updating a repository
// milestone. Nil fields are left unchanged.
type UpdateMilestoneOptions struct {
	Title       *string        `json:"title,omitempty"`
	Description *string        `json:"description,omitempty"`
	State       MilestoneState `json:"state,omitempty"`
	DueOn       *time.Time     `json:"due_on,omitempty"`
}

// --- Search ---

// SearchReposOptions contains options for searching repositories.
//
// Sort and Order are platform-dependent: each backend forwards them to its
// platform's own vocabulary (e.g. GitHub's stars/forks/updated with
// asc/desc), so values valid on one platform may be ignored or rejected on
// another (gitea/forgejo reject unknown sort/order values with HTTP 422;
// gitlab's search API exposes no sort/order at all — a registered ignore).
// Consult the target platform's search documentation for the accepted
// values.
type SearchReposOptions struct {
	Query   string `json:"q"`
	Sort    string `json:"sort,omitempty"`
	Order   string `json:"order,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

// SearchIssuesOptions contains options for searching issues.
type SearchIssuesOptions struct {
	Query   string `json:"q"`
	Repo    string `json:"repo,omitempty"`
	State   string `json:"state,omitempty"`
	Sort    string `json:"sort,omitempty"`
	Order   string `json:"order,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

// SearchUsersOptions contains options for searching users.
type SearchUsersOptions struct {
	Query   string `json:"q"`
	Sort    string `json:"sort,omitempty"`
	Order   string `json:"order,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

// SearchRepoResult is a single result from a repository search.
type SearchRepoResult struct {
	FullName      string `json:"full_name"`
	Description   string `json:"description,omitempty"`
	WebURL        string `json:"web_url,omitempty"`
	Stars         int    `json:"stars,omitempty"`
	Forks         int    `json:"forks,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Private       bool   `json:"private,omitempty"`
}

// SearchIssueResult is a single result from an issue search. Number is the
// platform's issue addressing identifier as a string, so results feed
// GetIssue(number string) directly (numeric platforms return "1", Gitee's
// alphanumeric identifiers return e.g. "IAINVA").
type SearchIssueResult struct {
	Number    string     `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	State     IssueState `json:"state"`
	WebURL    string     `json:"web_url,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	Comments  int        `json:"comments,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// SearchUserResult is a single result from a user search.
type SearchUserResult struct {
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// --- Notifications ---

// Notification represents a single notification from a user's inbox.
type Notification struct {
	ID        string              `json:"id"`
	Unread    bool                `json:"unread"`
	Reason    string              `json:"reason"`
	Subject   NotificationSubject `json:"subject"`
	Repo      *EventRepo          `json:"repo,omitempty"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// NotificationSubject describes the resource that triggered the notification.
type NotificationSubject struct {
	Title string `json:"title"`
	Type  string `json:"type"` // "Issue", "PullRequest", "Commit", etc.
	URL   string `json:"url"`
}

// ListNotificationsOptions contains options for listing notifications.
type ListNotificationsOptions struct {
	All     bool   `json:"all,omitempty"`   // include already-read notifications
	Since   string `json:"since,omitempty"` // RFC3339 timestamp
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

// MarkNotificationsOptions contains options for marking notifications as read.
type MarkNotificationsOptions struct {
	LastReadAt string `json:"last_read_at,omitempty"` // RFC3339; empty = mark all
}

// --- Reactions ---

// Standard emoji identifiers used across all platforms. GitLab maps these
// to its award-emoji names internally (e.g. +1 ↔ thumbsup).
const (
	ReactionPlusOne  = "+1"
	ReactionMinusOne = "-1"
	ReactionLaugh    = "laugh"
	ReactionConfused = "confused"
	ReactionHeart    = "heart"
	ReactionHooray   = "hooray"
	ReactionRocket   = "rocket"
	ReactionEyes     = "eyes"
)

// Reaction represents an emoji reaction on an issue or comment.
type Reaction struct {
	ID    int64   `json:"id"`
	Emoji string  `json:"emoji"`
	User  *CRUser `json:"user,omitempty"`
}

// --- Branch protection ---

// BranchProtection represents a branch protection rule.
type BranchProtection struct {
	BranchName               string `json:"branch_name"`
	RequiredApprovingReviews int    `json:"required_approving_reviews"`
	RequiredStatusChecks     bool   `json:"required_status_checks"`
	AllowForcePushes         bool   `json:"allow_force_pushes"`
	AllowDeletions           bool   `json:"allow_deletions"`
}

// CreateBranchProtectionOptions contains options for creating a branch
// protection rule.
type CreateBranchProtectionOptions struct {
	BranchName               string `json:"branch_name"`
	RequiredApprovingReviews int    `json:"required_approving_reviews,omitempty"`
	RequiredStatusChecks     bool   `json:"required_status_checks,omitempty"`
	AllowForcePushes         bool   `json:"allow_force_pushes,omitempty"`
	AllowDeletions           bool   `json:"allow_deletions,omitempty"`
}

// UpdateBranchProtectionOptions contains options for updating a branch
// protection rule. Nil fields are left unchanged.
type UpdateBranchProtectionOptions struct {
	RequiredApprovingReviews *int  `json:"required_approving_reviews,omitempty"`
	RequiredStatusChecks     *bool `json:"required_status_checks,omitempty"`
	AllowForcePushes         *bool `json:"allow_force_pushes,omitempty"`
	AllowDeletions           *bool `json:"allow_deletions,omitempty"`
}

// --- Collaborators ---

// Collaborator represents a repository collaborator.
type Collaborator struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username"`
	Permission string  `json:"permission,omitempty"`
	User       *CRUser `json:"user,omitempty"`
}

// AddCollaboratorOptions contains options for adding a collaborator.
type AddCollaboratorOptions struct {
	Permission string `json:"permission,omitempty"` // "read", "write", "admin"
}

// --- Deploy keys ---

// DeployKey represents a deploy key on a repository.
type DeployKey struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Key      string `json:"key"`
	ReadOnly bool   `json:"read_only"`
}

// AddDeployKeyOptions contains options for adding a deploy key.
type AddDeployKeyOptions struct {
	Title    string `json:"title"`
	Key      string `json:"key"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// --- Repo stats ---

// Contributor represents a repository contributor.
type Contributor struct {
	Username      string `json:"username"`
	Contributions int    `json:"contributions"`
}
