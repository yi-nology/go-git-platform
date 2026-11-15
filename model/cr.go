package model

import (
	"time"
)

// CRState represents the state of a change request.
type CRState string

const (
	CRStateOpened CRState = "opened"
	CRStateMerged CRState = "merged"
	CRStateClosed CRState = "closed"
)

// ChangeRequest represents a pull request or merge request. Number is the
// platform's change-request identifier as a string (numeric on every
// current platform), mirroring Issue.Number.
type ChangeRequest struct {
	ID           int64   `json:"id"`
	Number       string  `json:"number"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	State        CRState `json:"state"`
	SourceBranch string  `json:"source_branch"`
	TargetBranch string  `json:"target_branch"`
	// HeadSHA is the SHA of the source-branch tip.
	// BaseSHA is the diff base: the merge-base (common ancestor) where GitLab/TencentCode
	// expose it via diff_refs.base_sha. GitHub/Gitea do not expose a distinct merge base in
	// webhook payloads, so BaseSHA there is the target-branch tip (equivalent to StartSHA).
	// StartSHA is the SHA of the target-branch tip at event time (GitLab diff_refs.start_sha).
	// On GitHub/Gitea it equals BaseSHA since no separate value is exposed.
	HeadSHA  string `json:"head_sha,omitempty"`
	BaseSHA  string `json:"base_sha,omitempty"`
	StartSHA string `json:"start_sha,omitempty"`
	// Draft reports the work-in-progress / draft state uniformly across platforms
	// (GitHub pr.draft, Gitea draft, GitLab/TencentCode work_in_progress).
	Draft       bool      `json:"draft"`
	Author      *CRUser   `json:"author"`
	Assignees   []*CRUser `json:"assignees,omitempty"`
	Reviewers   []*CRUser `json:"reviewers"`
	Labels      []string  `json:"labels"`
	MergeStatus string    `json:"merge_status"`
	WebURL      string    `json:"web_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CRUser represents a user on a platform.
type CRUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// CRComment represents a comment on a change request.
type CRComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	Author    *CRUser   `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Mentions returns the deduplicated @usernames found in the comment body.
func (c *CRComment) Mentions() []string { return ExtractMentions(c.Body) }

// CRCommit represents a commit in a change request.
type CRCommit struct {
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	Author    *CRUser   `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// MergeDiff represents the diff of a change request.
type MergeDiff struct {
	Files    []*ChangedFile
	TotalAdd int
	TotalDel int
	RawDiff  string
}

// ChangedFile represents a file changed in a change request.
type ChangedFile struct {
	OldPath   string `json:"old_path"`
	NewPath   string `json:"new_path"`
	Diff      string `json:"diff"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	IsNew     bool   `json:"new_file"`
	IsDeleted bool   `json:"deleted_file"`
	IsRenamed bool   `json:"renamed_file"`
	IsBinary  bool   `json:"binary"`
}
