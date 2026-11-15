package model

import (
	"time"
)

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
