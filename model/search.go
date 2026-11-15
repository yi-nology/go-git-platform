package model

import (
	"time"
)

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
