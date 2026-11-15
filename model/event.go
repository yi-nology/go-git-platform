package model

import (
	"encoding/json"
	"time"
)

// PlatformWebhook represents a webhook on a platform.
type PlatformWebhook struct {
	ID     int64    `json:"id"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// NormalizedEvent represents a normalized webhook event from any platform.
type NormalizedEvent struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Source    Platform       `json:"source"`
	Timestamp time.Time      `json:"timestamp"`
	Actor     *CRUser        `json:"actor"`
	Repo      *EventRepo     `json:"repo"`
	CR        *ChangeRequest `json:"cr,omitempty"`
	// Issue is set for issue.* events and for comments made on issues;
	// Comment carries the comment body for comment.* events. A comment on
	// a change request populates CR instead of (or alongside) nothing.
	Issue      *Issue          `json:"issue,omitempty"`
	Comment    *IssueComment   `json:"comment,omitempty"`
	Branch     string          `json:"branch,omitempty"`
	Tag        string          `json:"tag,omitempty"`
	CommitSHA  string          `json:"commit_sha,omitempty"`
	Action     string          `json:"action,omitempty"`
	RawPayload json.RawMessage `json:"raw_payload"`
}

// EventRepo represents the repository in a webhook event.
type EventRepo struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
}

// TestConnectionResult contains the result of a connection test.
type TestConnectionResult struct {
	Connected    bool   `json:"connected"`
	Platform     string `json:"platform"`
	UserName     string `json:"user_name"`
	Message      string `json:"message,omitempty"`
	CanListRepos bool   `json:"can_list_repos"`
	CanReadCR    bool   `json:"can_read_cr"`
	CanWriteCR   bool   `json:"can_write_cr"`
	CanWebhook   bool   `json:"can_webhook"`
}
