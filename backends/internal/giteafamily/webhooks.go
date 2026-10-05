// Package giteafamily hosts the single implementation of the capability
// logic shared by the Gitea and Forgejo backends. Forgejo is a Gitea fork:
// after normalizing the two SDK identifiers the backend packages were ~95%
// identical, differing only in the SDK call shape (the Forgejo SDK drops
// ctx and flattens the method namespace) and a handful of converter deltas.
//
// Architecture (ports & adapters): the engine owns every behavioral concern
// — option building, pagination, provider-type conversion, webhook payload
// parsing, error wrapping — against its own minimal wire-shaped types. Each
// backend supplies a small adapter implementing the port interfaces over
// its SDK; gitea's adapter delegates almost directly, forgejo's converts
// its fork-identical types and drops ctx. Behavior is pinned on both sides
// by the identical contracttest suite and webhook golden corpus.
//
// Capabilities migrate here incrementally; types with large SDK surfaces
// (PullRequest, Issue, ...) each require an exhaustive field-by-field
// conversion check before their port is cut, so they land capability by
// capability (see docs/superpowers/specs/2026-10-06-cohesion-patterns-
// refactor-design.md for the ledger).
package giteafamily

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// Family carries the per-platform bits of one Gitea-family backend.
type Family struct {
	// Platform identifies the backend (provider.PlatformGitea / Forgejo).
	Platform provider.Platform
	// EventIDPrefix prefixes generated webhook event IDs ("gt-" for both
	// platforms today, kept explicit so the dialect stays declarative).
	EventIDPrefix string
	// EventHeader is the event-type header of the family webhook dialect
	// ("X-Gitea-Event"; Forgejo brands it "X-Forgejo-Event").
	EventHeader string
	// PageSize is the per-page size for paginated family list fetches.
	PageSize int
}

// --- family wire types (the engine's only vocabulary) ---

// ListOptions is the shared pagination struct of the family SDKs.
type ListOptions struct {
	Page     int
	PageSize int
}

// CreateHookOption mirrors the family's POST webhook body.
type CreateHookOption struct {
	Type   string
	Config map[string]string
	Events []string
	Active bool
}

// ListHooksOptions mirrors the family's webhook list query.
type ListHooksOptions struct {
	ListOptions ListOptions
}

// Hook is the slice of the family's webhook object the engine consumes.
type Hook struct {
	ID     int64
	Config map[string]string
	Events []string
}

// HookPort is the webhook endpoint of the family SDK surface. Implementations
// adapt a concrete SDK (gitea: near-direct delegation; forgejo: fork types
// converted, ctx dropped).
type HookPort interface {
	CreateRepoHook(ctx context.Context, owner, repo string, opt CreateHookOption) (*Hook, error)
	DeleteRepoHook(ctx context.Context, owner, repo string, id int64) error
	ListRepoHooks(ctx context.Context, owner, repo string, opt ListHooksOptions) ([]*Hook, error)
}

// --- engine: webhook CRUD ---

// CreateWebhook creates a family webhook, defaulting the event list to
// push+pull_request when unset.
func CreateWebhook(ctx context.Context, f Family, c HookPort, opts provider.CreateWebhookOptions) (*provider.PlatformWebhook, error) {
	events := opts.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request"}
	}
	hook, err := c.CreateRepoHook(ctx, opts.Owner, opts.Repo, CreateHookOption{
		Type:   "gitea", // the family wire value; Forgejo accepts it unchanged
		Config: map[string]string{"url": opts.URL, "content_type": "json", "secret": opts.Secret},
		Events: events,
		Active: true,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateWebhook", err)
	}
	return convertHook(hook), nil
}

// DeleteWebhook removes one family webhook.
func DeleteWebhook(ctx context.Context, f Family, c HookPort, owner, repo string, webhookID int64) error {
	if err := c.DeleteRepoHook(ctx, owner, repo, webhookID); err != nil {
		return provider.Wrap(f.Platform, "DeleteWebhook", err)
	}
	return nil
}

// ListWebhooks returns every family webhook. The provider surface carries no
// pagination parameters, so the full hook list is fetched by exhausting the
// endpoint's pagination (backendutil.AllPages).
func ListWebhooks(ctx context.Context, f Family, c HookPort, owner, repo string) ([]*provider.PlatformWebhook, error) {
	hooks, err := backendutil.AllPages(func(page int) ([]*Hook, error) {
		return c.ListRepoHooks(ctx, owner, repo, ListHooksOptions{
			ListOptions: ListOptions{Page: page, PageSize: f.PageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListWebhooks", err)
	}
	result := make([]*provider.PlatformWebhook, 0, len(hooks))
	for _, h := range hooks {
		result = append(result, convertHook(h))
	}
	return result, nil
}

// convertHook projects a family Hook onto the provider webhook shape.
func convertHook(h *Hook) *provider.PlatformWebhook {
	if h == nil {
		return nil
	}
	return &provider.PlatformWebhook{
		ID:     h.ID,
		URL:    h.Config["url"],
		Events: h.Events,
	}
}

// --- engine: webhook event parsing ---

// ParseWebhookEvent validates the signature via the platform's registered
// validator and maps the (byte-identical across the family) Gitea-style
// payload onto a NormalizedEvent.
func ParseWebhookEvent(f Family, r *http.Request, secret string) (*provider.NormalizedEvent, error) {
	if err := provider.ValidateWebhookWithRegistry(f.Platform, r, secret); err != nil {
		// Wrapped under the ValidateWebhookSignature op: the backends'
		// ParseWebhookEvent has always run signature validation first.
		return nil, provider.Wrap(f.Platform, "ValidateWebhookSignature", err)
	}
	body, readErr := provider.ReadAndRestoreBody(r)
	if readErr != nil {
		return nil, provider.Wrap(f.Platform, "ParseWebhookEvent", readErr)
	}

	eventType := r.Header.Get(f.EventHeader)
	var pl struct {
		Action string `json:"action"`
		Sender struct {
			ID    int    `json:"id"`
			Login string `json:"login"`
		} `json:"sender"`
		Repository struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
		} `json:"repository"`
		PullRequest *struct {
			ID     int    `json:"id"`
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			State  string `json:"state"`
			Head   struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"base"`
			Draft   bool   `json:"draft"`
			Merged  bool   `json:"merged"`
			HTMLURL string `json:"html_url"`
			User    struct {
				ID    int    `json:"id"`
				Login string `json:"login"`
			} `json:"user"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"pull_request"`
		Number int    `json:"number"`
		Ref    string `json:"ref"`
		After  string `json:"after"`
		// create/delete hooks carry the ref kind ("branch" or "tag").
		RefType string `json:"ref_type"`
		// issues/issue_comment hooks carry the issue object; a PR-shaped
		// issue additionally carries a pull_request key, and issue_comment
		// hooks carry the comment itself.
		Issue   *webhookIssuePayload   `json:"issue"`
		Comment *webhookCommentPayload `json:"comment"`
	}
	if err := json.Unmarshal(body, &pl); err != nil {
		return nil, provider.Wrap(f.Platform, "ParseWebhookEvent", err)
	}

	er := provider.BuildEventRepo(pl.Repository.FullName)
	er.ID = pl.Repository.ID
	actor := &provider.CRUser{ID: int64(pl.Sender.ID), Username: pl.Sender.Login}

	event := &provider.NormalizedEvent{
		ID:         fmt.Sprintf("%s-%d-%d", f.EventIDPrefix, time.Now().UnixNano(), pl.Number),
		Source:     f.Platform,
		Timestamp:  time.Now(),
		Actor:      actor,
		Repo:       er,
		RawPayload: json.RawMessage(body),
	}

	switch eventType {
	case "pull_request":
		action := provider.NormalizeCRAction(pl.Action, pl.PullRequest != nil && pl.PullRequest.Merged)
		event.Type = "cr." + action
		event.Action = action
		if pl.PullRequest != nil {
			event.CommitSHA = pl.PullRequest.Head.SHA
			event.CR = &provider.ChangeRequest{
				ID:           int64(pl.PullRequest.Number),
				Number:       strconv.Itoa(pl.PullRequest.Number),
				Title:        pl.PullRequest.Title,
				Description:  pl.PullRequest.Body,
				State:        MapState(pl.PullRequest.State, pl.PullRequest.Merged),
				Draft:        pl.PullRequest.Draft,
				SourceBranch: pl.PullRequest.Head.Ref,
				TargetBranch: pl.PullRequest.Base.Ref,
				HeadSHA:      pl.PullRequest.Head.SHA,
				BaseSHA:      pl.PullRequest.Base.SHA,
				// The family exposes no distinct merge-base; base.sha is the
				// target tip.
				StartSHA:  pl.PullRequest.Base.SHA,
				WebURL:    pl.PullRequest.HTMLURL,
				Author:    &provider.CRUser{ID: int64(pl.PullRequest.User.ID), Username: pl.PullRequest.User.Login},
				CreatedAt: pl.PullRequest.CreatedAt,
				UpdatedAt: pl.PullRequest.UpdatedAt,
			}
		}
	case "push":
		event.Type = "push"
		event.Action = "push"
		event.Branch = strings.TrimPrefix(pl.Ref, "refs/heads/")
		event.CommitSHA = pl.After
	case "tag_push":
		event.Type = "tag.created"
		event.Tag = strings.TrimPrefix(pl.Ref, "refs/tags/")
		event.CommitSHA = pl.After
	case "create":
		if pl.RefType == "tag" {
			event.Type = provider.EventTypeTag + "created"
			event.Action = "created"
			event.Tag = pl.Ref
		} else {
			event.Type = provider.EventTypeBranch + "created"
			event.Action = "created"
			event.Branch = pl.Ref
		}
	case "delete":
		if pl.RefType == "tag" {
			event.Type = provider.EventTypeTag + "deleted"
			event.Action = "deleted"
			event.Tag = pl.Ref
		} else {
			event.Type = provider.EventTypeBranch + "deleted"
			event.Action = "deleted"
			event.Branch = pl.Ref
		}
	case "issues":
		action := provider.NormalizeIssueAction(pl.Action)
		event.Type = provider.EventTypeIssue + action
		event.Action = action
		attachWebhookIssue(event, pl.Issue)
	case "issue_comment":
		event.Type = provider.EventTypeComment + provider.CommentActionCreated
		event.Action = provider.CommentActionCreated
		if pl.Comment != nil {
			author := &provider.CRUser{ID: int64(pl.Comment.User.ID), Username: pl.Comment.User.Login}
			if author.ID == 0 && author.Username == "" {
				author = actor
			}
			event.Comment = &provider.IssueComment{
				ID:        pl.Comment.ID,
				Body:      pl.Comment.Body,
				Author:    author,
				CreatedAt: pl.Comment.CreatedAt,
				UpdatedAt: pl.Comment.UpdatedAt,
			}
		}
		attachWebhookIssue(event, pl.Issue)
	}
	return event, nil
}

// webhookIssuePayload is the issue object carried by issues and
// issue_comment hooks.
type webhookIssuePayload struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	User    struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
	} `json:"user"`
	PullRequest *struct {
		Merged bool `json:"merged"`
	} `json:"pull_request"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// webhookCommentPayload is the comment object carried by issue_comment
// hooks.
type webhookCommentPayload struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
	} `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// attachWebhookIssue populates event.Issue (and, for PR-shaped issues,
// event.CR) from a hook's issue payload.
func attachWebhookIssue(event *provider.NormalizedEvent, issue *webhookIssuePayload) {
	if issue == nil {
		return
	}
	state := provider.IssueStateOpen
	if issue.State == "closed" {
		state = provider.IssueStateClosed
	}
	event.Issue = &provider.Issue{
		ID:        int64(issue.Number),
		Number:    strconv.Itoa(issue.Number),
		Title:     issue.Title,
		Body:      issue.Body,
		State:     state,
		Author:    &provider.CRUser{ID: int64(issue.User.ID), Username: issue.User.Login},
		WebURL:    issue.HTMLURL,
		CreatedAt: issue.CreatedAt,
		UpdatedAt: issue.UpdatedAt,
	}
	if issue.PullRequest != nil {
		event.CR = &provider.ChangeRequest{
			ID: int64(issue.Number), Number: strconv.Itoa(issue.Number), Title: issue.Title,
		}
	}
}

// MapState maps a family issue/PR state string onto the provider CR state;
// exported because the family backends' converters share it.
func MapState(state string, merged bool) provider.CRState {
	return provider.MapBoolStateToCR(state, merged)
}
