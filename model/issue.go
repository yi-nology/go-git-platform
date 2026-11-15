package model

import (
	"time"
)

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

// Label represents a repository label. Color is canonicalized to 6-digit hex
// without a leading '#' (e.g. "ff0000"); backends add the '#' when a platform
// requires it (GitLab, Gitea, Forgejo) and strip it on the way in.
type Label struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

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
