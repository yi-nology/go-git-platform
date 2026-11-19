package model

import (
	"time"
)

// PlatformBranch represents a branch on a platform.
type PlatformBranch struct {
	Name string `json:"name"`
}

// PlatformRepo represents a repository on a platform.
type PlatformRepo struct {
	ID            int64    `json:"id"`
	FullName      string   `json:"full_name"`
	Name          string   `json:"name"`
	Owner         string   `json:"owner"`
	Description   string   `json:"description"`
	CloneURL      string   `json:"clone_url"`
	SSHURL        string   `json:"ssh_url"`
	DefaultBranch string   `json:"default_branch"`
	Private       bool     `json:"private"`
	Platform      Platform `json:"platform"`
	// 仓库元数据(用于导入过滤:排除 archived/fork、按 star/语言过滤)
	Archived bool   `json:"archived,omitempty"`
	Fork     bool   `json:"fork,omitempty"`
	Stars    int    `json:"stars,omitempty"`
	Language string `json:"language,omitempty"`
}

// CommitInfo represents a commit.
type CommitInfo struct {
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	Author    *CRUser   `json:"author"`
	Committer *CRUser   `json:"committer"`
	CreatedAt time.Time `json:"created_at"`
	Additions int       `json:"additions"`
	Deletions int       `json:"deletions"`
}

// CompareResult represents the result of comparing two commits.
type CompareResult struct {
	Commits      []*CommitInfo  `json:"commits"`
	Files        []*ChangedFile `json:"files"`
	TotalCommits int            `json:"total_commits"`
	AheadBy      int            `json:"ahead_by"`
	BehindBy     int            `json:"behind_by"`
	// FilesTruncated reports that the Files list is KNOWN to be incomplete
	// because the platform caps the compare-diff file list (issue #77:
	// GitHub serves at most 300 changed files per comparison, first page
	// only, with no pagination line to exhaust). A compare-based audit or
	// sync must treat Files as a prefix, never as the full changeset, when
	// this is true. Backends without a platform cap (or that paginate the
	// file list fully) always leave it false.
	FilesTruncated bool `json:"files_truncated,omitempty"`
}

// TagInfo represents a tag.
type TagInfo struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// ReleaseInfo represents a release.
type ReleaseInfo struct {
	ID          int64     `json:"id"`
	TagName     string    `json:"tag_name"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	URL         string    `json:"url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
	// Assets 发布附件元数据(仅声明 ReleaseAssets 能力的后端填充)。
	Assets []*ReleaseAsset `json:"assets,omitempty"`
}

// BranchProtection represents a branch protection rule.
type BranchProtection struct {
	BranchName               string `json:"branch_name"`
	RequiredApprovingReviews int    `json:"required_approving_reviews"`
	RequiredStatusChecks     bool   `json:"required_status_checks"`
	AllowForcePushes         bool   `json:"allow_force_pushes"`
	AllowDeletions           bool   `json:"allow_deletions"`
	// Contexts 是 required status checks 的上下文列表(issue #171)。
	// 平台未返回该面时保持 nil。
	Contexts []string `json:"contexts,omitempty"`
	// RequireCodeOwnerReviews / EnforceAdmins 指针承载「平台未返回」
	// (nil) 与显式 false 的区分(issue #171)——值语义 bool 会把
	// 「没这字段」误读成「关了」。
	RequireCodeOwnerReviews *bool `json:"require_code_owner_reviews,omitempty"`
	EnforceAdmins           *bool `json:"enforce_admins,omitempty"`
}

// Collaborator represents a repository collaborator.
type Collaborator struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username"`
	Permission string  `json:"permission,omitempty"`
	User       *CRUser `json:"user,omitempty"`
}

// DeployKey represents a deploy key on a repository.
type DeployKey struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Key      string `json:"key"`
	ReadOnly bool   `json:"read_only"`
}

// Contributor represents a repository contributor.
type Contributor struct {
	Username      string `json:"username"`
	Contributions int    `json:"contributions"`
}
