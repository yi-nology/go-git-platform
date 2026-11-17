package provider

import (
	"time"

	"github.com/yi-nology/go-git-platform/model"
)

// This file consolidates every option type used by the Provider
// sub-interfaces. The result/record types platforms return live in
// package model; this file keeps the "options" half — what callers pass
// in — so consumers can scan a single file for the cross-platform
// parameter surface.
//
// Option field conventions:
//   - Update*Options（e.g. UpdateIssueOptions, UpdateCROptions）：旧字段
//     的零值（空串、false）仍表示 "leave unchanged"。2A 起配套 XxxPatch
//     指针字段提供三态：nil = 不动、非 nil = 全量替换（空值 = 显式清空），
//     双字段同设时 patch 优先——见 provider/patch.go ResolvePatch。
//   - Update*Options with pointer fields (e.g. UpdateLabelOptions,
//     UpdateMilestoneOptions): nil means "leave unchanged", non-nil means
//     "set to this value" (including zero values like "" or false).
//   - Create*Options: all fields are used as-is; zero values mean "use
//     platform default".

// --- Listing / pagination ---
//
// Page semantics across the list managers (issue #60): every Page option
// field follows one of two families — the "exhaustive" family (bounded
// sets: Page == 0 walks every page and returns all items; Page > 0 returns
// exactly that page) and the "caller-paged" family (unbounded sets:
// Page == 0 returns the platform's first page, one request, no walk;
// Page > 0 returns exactly that page). The full contract is documented on
// NormalizePageOpts in pagination.go — the single anchor every Page field
// points at. Per-backend departures are machine-readable in the backends'
// divergence ledgers (Provider.Divergences / docs/divergence-ledger.md).

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
	// Draft 以草稿/work-in-progress 形态创建 CR（#160，读侧 ChangeRequest.Draft
	// 对齐）。缺省 false = 现行为，wire 逐字不变。true 仅平台创建面可表达时
	// 下发（GitHub/Gitee/GitCode）；gitlab/gitea/forgejo/tencentcode 的
	// SDK/API 创建面无该参数——显式返回 ErrDraftNotSupported（台账
	// Field:"opts.Draft" 登记），绝不静默产出非 draft CR。
	Draft bool `json:"draft,omitempty"`
}

// ListCROptions contains options for listing change requests.
type ListCROptions struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	// State 过滤状态。**空 = 不过滤(全部)**。
	//
	// 各后端的原生词表不同(open/closed/merged 与 opened/closed)⇒ 平台层的
	// "空" 语义必须统一,否则同一个调用在有的平台只拿到 open、在别的平台拿到全部
	// —— 调用方无从察觉 closed/merged 的那部分**静默丢失**。
	// (GitHub / GitLab / 工蜂 本来就是"空=不过滤";Gitee / Gitea / Forgejo /
	//  GitCode 曾把空值映射成 open ⇒ 已对齐。)
	State model.CRState `json:"state,omitempty"`
	// SourceBranch filters by the source branch. The cross-backend contract
	// (issue #78): a pure branch name ("feature-x") matches every change
	// request whose head branch is named so - including fork-sourced ones
	// (the unified ChangeRequest.SourceBranch is the namespace-free head
	// ref everywhere). An "owner:branch" form is honored on GitHub's
	// namespace-pinned server filter as an exact match; on GitLab-family
	// backends the same string filters server-side by branch name.
	SourceBranch string `json:"source_branch,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`
	// Page semantics: caller-paged family (issue #60 — see
	// NormalizePageOpts). Page == 0 returns the platform's first page in a
	// single request; Page > 0 returns exactly that page. Tencent Code is
	// the registered departure: its Page == 0 walks every page (divergence
	// ledger).
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

// MergeCROptions contains options for merging a change request.
type MergeCROptions struct {
	MergeCommitMessage string `json:"merge_commit_message,omitempty"`
	Squash             bool   `json:"squash,omitempty"`
	RemoveSourceBranch bool   `json:"remove_source_branch,omitempty"`
}

// UpdateCROptions contains options for updating a change request.
type UpdateCROptions struct {
	Title string `json:"title,omitempty"`
	// DescriptionPatch 三态描述（2A）：nil = 不动；非 nil = 全量替换，"" = 清空。
	// 与 Description 同设时本字段优先。平台无法表达清空（A/B 矩阵 B 级）时
	// UpdateCR 返回 ErrClearNotSupported。
	DescriptionPatch *string `json:"description_patch,omitempty"`
	// AssigneesPatch 三态指派人：nil = 不动；非 nil = 全量替换，
	// 空切片 = 清空。语义同 DescriptionPatch。
	AssigneesPatch *[]string `json:"assignees_patch,omitempty"`
	//
	// 三态（2A 起）：清空/置空用 DescriptionPatch（见上）；本字段语义不变
	// （空 = 不动），保留为兼容路径——MCP 工具「参数未提供 = 不变」
	// 即此语义。
	Description  string `json:"description,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`
	// Assignees 全量替换 CR 指派人（logins；后端按平台词表解析为 ID/单值线）。
	// 空（nil 或空切片）= 指派人不变。**清空不可表达**——与
	// UpdateIssueOptions.Assignees 同款口径（len>0 才落线）。
	//
	// 三态（2A 起）：清空/置空用 AssigneesPatch（见上）；本字段语义不变
	// （空 = 不动），保留为兼容路径——MCP 工具「参数未提供 = 不变」
	// 即此语义。
	Assignees []string `json:"assignees,omitempty"`
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
	CommitID string                `json:"commit_id,omitempty"`
	Event    string                `json:"event,omitempty"`
	Body     string                `json:"body,omitempty"`
	Comments []model.ReviewComment `json:"comments,omitempty"`
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
//
// Content is PLAINTEXT — the bytes as they should land in the repository.
// The wire form differs by platform: the gitea/forgejo family's SDKs require
// their content field base64-encoded (gitea.dev/sdk and forgejo-sdk:
// "content must be base64 encoded"; the family adapter applies that codec),
// GitHub's SDK encodes []byte internally, and GitLab takes the string
// verbatim. Callers must NOT pre-encode: the adapter that needs a codec
// applies it, and pre-encoding would make the base64 text itself the file on
// the platforms that take plaintext (issue #169).
type FileOptions struct {
	Path string `json:"path"`
	// Content is the file's plaintext content, never base64: the adapter
	// applies the platform's wire encoding (gitea/forgejo family: base64).
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

// --- Webhooks ---

// EnsureSecretMode controls how EnsureWebhook treats a matching hook's
// secret. Platform webhook secrets are write-only on every platform (list
// responses never return them), so a hook that already matches URL + events
// cannot be verified to carry the desired secret — it may be stale after a
// host-side rotation, and a stale secret makes every delivery fail
// signature validation (fail-closed, silently).
//
// 位置裁定（S2 runtime 剖离）：本类型是 CreateWebhookOptions 的字段类型
// （契约参数面，C5 留 provider），而 runtime/ensure.go 依赖 provider ——
// 若随 ensure.go 迁 runtime 即成 provider→runtime 反向边与 import 环，
// 故与所属字段同留 provider；EnsureWebhook（runtime）单向引用它。
type EnsureSecretMode string

const (
	// EnsureSecretKeep (default, also the zero value) trusts the match:
	// a hook that already matches URL + events is left untouched even if
	// opts.Secret differs from what the platform may hold.
	EnsureSecretKeep EnsureSecretMode = ""
	// EnsureSecretAlwaysUpdate rewrites the secret whenever the hook
	// matches and opts.Secret is non-empty, converging rotations in the
	// same ensure call: the in-place patch goes through
	// WebhookConfigManager.UpdateWebhook (implemented by every backend),
	// keeps the hook's platform ID, and reports EnsureUpdated. A manager
	// that does not also implement WebhookConfigManager is an error —
	// silently keeping a possibly-stale secret would defeat the mode.
	EnsureSecretAlwaysUpdate EnsureSecretMode = "always-update"
)

// CreateWebhookOptions contains options for creating a webhook.
type CreateWebhookOptions struct {
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events,omitempty"`
	// EnsureSecretMode is consumed only by EnsureWebhook (CreateWebhook
	// ignores it): EnsureSecretKeep trusts a URL+events match and keeps
	// the platform-held secret; EnsureSecretAlwaysUpdate rewrites the
	// secret on a match, converging host-side rotations. See
	// EnsureSecretMode for the write-only rationale.
	EnsureSecretMode EnsureSecretMode `json:"ensure_secret_mode,omitempty"`
}

// UpdateWebhookOptions contains options for updating a webhook (patch-style:
// nil fields are left unchanged). Events is the exception — see
// WebhookConfigManager for the full-replacement semantics.
type UpdateWebhookOptions struct {
	URL    *string  `json:"url,omitempty"`
	Secret *string  `json:"secret,omitempty"`
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

// ListIssuesOptions contains options for listing issues.
type ListIssuesOptions struct {
	Owner    string           `json:"owner"`
	Repo     string           `json:"repo"`
	State    model.IssueState `json:"state,omitempty"`
	Assignee string           `json:"assignee,omitempty"`
	Labels   string           `json:"labels,omitempty"`
	Page     int              `json:"page,omitempty"`
	PerPage  int              `json:"per_page,omitempty"`
	// Sort 排序字段("created"|"updated"|"comments"),空 = 平台默认。
	//
	// **增量备份必须用它**:平台默认按 created 倒序 ⇒ "上次备份后被改动的
	// **老** issue"正好被挤在第一页之外,客户端再按 updated 过滤也捞不到
	// (捞到的永远是"最新创建"的那批)。
	//
	// 支持情况(逐后端登记在各自的 divergence 账本里):
	//   - GitHub:支持(sort + direction);
	//   - GitLab:支持(order_by=created_at|updated_at + sort=asc|desc);
	//   - Gitee / Gitea / Forgejo / GitCode / 工蜂:当前 SDK 不暴露(或词表无法确证)
	//     ⇒ 传入被忽略(调用方需自己兜底,见 core 的增量采集)。
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
	Title string `json:"title,omitempty"`
	// BodyPatch 三态正文（2A）：nil = 不动；非 nil = 全量替换，"" = 清空。
	// 与 Body 同设时本字段优先。平台无法表达清空（A/B 矩阵 B 级）时
	// UpdateIssue 返回 ErrClearNotSupported。
	BodyPatch *string `json:"body_patch,omitempty"`
	// AssigneesPatch 三态指派人：nil = 不动；非 nil = 全量替换，
	// 空切片 = 清空。语义同 BodyPatch。
	AssigneesPatch *[]string `json:"assignees_patch,omitempty"`
	// LabelsPatch 三态标签：nil = 不动；非 nil = 全量替换，空 = 清空。
	LabelsPatch *[]string `json:"labels_patch,omitempty"`
	// MilestonePatch 三态里程碑：nil = 不动；非 nil = 全量替换，
	// "" = 清空（各平台映射 null/0，见能力矩阵）。
	MilestonePatch *string `json:"milestone_patch,omitempty"`
	//
	// 三态（2A 起）：清空/置空用 BodyPatch（见上）；本字段语义不变
	// （空 = 不动），保留为兼容路径——MCP 工具「参数未提供 = 不变」
	// 即此语义。
	Body  string           `json:"body,omitempty"`
	State model.IssueState `json:"state,omitempty"`
	//
	// 三态（2A 起）：清空/置空用 AssigneesPatch（见上）；本字段语义不变
	// （空 = 不动），保留为兼容路径——MCP 工具「参数未提供 = 不变」
	// 即此语义。
	Assignees []string `json:"assignees,omitempty"`
	//
	// 三态（2A 起）：清空/置空用 LabelsPatch（见上）；本字段语义不变
	// （空 = 不动），保留为兼容路径——MCP 工具「参数未提供 = 不变」
	// 即此语义。
	Labels []string `json:"labels,omitempty"`
	// milestone number/ID as a string; "" = leave unchanged
	//
	// 三态（2A 起）：清空/置空用 MilestonePatch（见上）；本字段语义不变
	// （空 = 不动），保留为兼容路径——MCP 工具「参数未提供 = 不变」
	// 即此语义。
	Milestone string `json:"milestone,omitempty"`
}

// --- Labels ---

// ListLabelsOptions contains options for listing repository labels.
//
// Page semantics: exhaustive family (issue #60 — see NormalizePageOpts).
// Page == 0 walks every page and returns all labels; Page > 0 returns
// exactly that page.
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

// ListMilestonesOptions contains options for listing repository
// milestones.
//
// State contract (issue #61): "open" and "closed" filter; an empty State
// lists ALL milestones. Every backend that can express the "all" value on
// the wire sends it explicitly — the pre-contract behavior (forward the
// zero value, let the platform default) silently dropped closed milestones
// on the platforms whose own default is open-only. Tencent Code's list
// endpoint exposes no state filter at all: explicit states are dropped
// there (registered divergence), and its unfiltered output already matches
// the empty-State contract.
//
// Page semantics: exhaustive family (issue #60 — see NormalizePageOpts).
// Page == 0 walks every page and returns all milestones; Page > 0 returns
// exactly that page.
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
	Title       *string              `json:"title,omitempty"`
	Description *string              `json:"description,omitempty"`
	State       model.MilestoneState `json:"state,omitempty"`
	DueOn       *time.Time           `json:"due_on,omitempty"`
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

// --- Notifications ---

// ListNotificationsOptions contains options for listing notifications.
//
// Page semantics: exhaustive family (issue #60 — see NormalizePageOpts).
// Page == 0 walks every page and returns all notifications — matching the
// interface's "exhausting the platform's pagination" promise; Page > 0
// returns exactly that page.
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

// --- Branch protection ---

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

// AddCollaboratorOptions contains options for adding a collaborator.
type AddCollaboratorOptions struct {
	Permission string `json:"permission,omitempty"` // "read", "write", "admin"
}

// --- Deploy keys ---

// AddDeployKeyOptions contains options for adding a deploy key.
type AddDeployKeyOptions struct {
	Title    string `json:"title"`
	Key      string `json:"key"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// --- Repo stats ---
