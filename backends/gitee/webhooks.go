package gitee

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/provider"
)

// giteeHookEventFlags maps the platform-neutral webhook event names onto
// Gitee's per-event boolean fields.
var giteeHookEventFlags = map[string]string{
	"push":         "push_events",
	"tag_push":     "tag_push_events",
	"issues":       "issues_events",
	"note":         "note_events",
	"comment":      "note_events",
	"pull_request": "merge_requests_events",
}

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*provider.PlatformWebhook, error) {
	events := opts.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request"}
	}
	createOpts := &gitee.CreateWebhookOptions{
		URL:      gitee.String(opts.URL),
		Password: gitee.String(opts.Secret),
	}
	for _, e := range events {
		switch giteeHookEventFlags[e] {
		case "push_events":
			createOpts.PushEvents = gitee.Bool(true)
		case "tag_push_events":
			createOpts.TagPushEvents = gitee.Bool(true)
		case "issues_events":
			createOpts.IssuesEvents = gitee.Bool(true)
		case "note_events":
			createOpts.NoteEvents = gitee.Bool(true)
		case "merge_requests_events":
			createOpts.MergeRequestsEvents = gitee.Bool(true)
		}
	}
	hook, _, err := p.client.Webhooks.Create(ctx, esc(opts.Owner), esc(opts.Repo), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateWebhook", err)
	}
	return convertHook(hook), nil
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	_, err := p.client.Webhooks.Delete(ctx, esc(owner), esc(repo), webhookID)
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "DeleteWebhook", err)
	}
	return nil
}

// ListWebhooks implements provider.WebhookManager.
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*provider.PlatformWebhook, error) {
	hooks, _, err := p.client.Webhooks.List(ctx, esc(owner), esc(repo), &gitee.ListOptions{})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListWebhooks", err)
	}
	result := make([]*provider.PlatformWebhook, 0, len(hooks))
	for _, h := range hooks {
		result = append(result, convertHook(h))
	}
	return result, nil
}

// ValidateWebhookSignature implements provider.WebhookManager. It
// delegates to the platform's registered validator so the signature
// scheme has exactly one implementation (shared with contracttest);
// notably an empty secret is rejected rather than silently accepted.
func (p *Provider) ValidateWebhookSignature(r *http.Request, secret string) error {
	if err := provider.ValidateWebhookWithRegistry(provider.PlatformGitee, r, secret); err != nil {
		return provider.Wrap(provider.PlatformGitee, "ValidateWebhookSignature", err)
	}
	return nil
}

// ParseWebhookEvent implements provider.WebhookManager.
func (p *Provider) ParseWebhookEvent(r *http.Request, secret string) (*provider.NormalizedEvent, error) {
	if err := p.ValidateWebhookSignature(r, secret); err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ParseWebhookEvent", readErr)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	var pl struct {
		Action       string `json:"action"`
		ActionDesc   string `json:"action_desc"`
		Number       int    `json:"number"`
		Title        string `json:"title"`
		Body         string `json:"body"`
		State        string `json:"state"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		HTMLURL      string `json:"html_url"`
		User         struct {
			ID    int    `json:"id"`
			Login string `json:"login"`
			Name  string `json:"name"`
		} `json:"user"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Ref       string    `json:"ref"`
		After     string    `json:"after"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		// note hooks: the comment body and its target kind.
		Note         string `json:"note"`
		NoteableType string `json:"noteable_type"` //nolint:misspell // 平台 webhook 原始字段名
		// issue hooks: the issue object (Gitee numbers are alphanumeric).
		Issue *struct {
			Number  string `json:"number"`
			Title   string `json:"title"`
			Body    string `json:"body"`
			State   string `json:"state"`
			HTMLURL string `json:"html_url"`
			User    struct {
				ID    int    `json:"id"`
				Login string `json:"login"`
			} `json:"user"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(body, &pl); err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ParseWebhookEvent", err)
	}

	hookName := r.Header.Get("X-Gitee-Event")
	er := provider.BuildEventRepo(pl.Repository.FullName)
	actor := &provider.CRUser{ID: int64(pl.User.ID), Username: pl.User.Login, Name: pl.User.Name}

	event := &provider.NormalizedEvent{
		ID:         fmt.Sprintf("ge-%d-%d", time.Now().UnixNano(), pl.Number),
		Source:     p.Platform(),
		Timestamp:  time.Now(),
		Actor:      actor,
		Repo:       er,
		RawPayload: json.RawMessage(body),
	}

	switch hookName {
	case "pull_request":
		action := provider.NormalizeCRAction(pl.Action, pl.State == "merged")
		event.Type = provider.EventTypeCR + action
		event.Action = action
		event.CR = &provider.ChangeRequest{
			Number:       strconv.Itoa(pl.Number),
			Title:        pl.Title,
			Description:  pl.Body,
			State:        provider.MapBoolStateToCR(pl.State, pl.State == "merged"),
			SourceBranch: pl.SourceBranch,
			TargetBranch: pl.TargetBranch,
			WebURL:       pl.HTMLURL,
			Author:       actor,
			CreatedAt:    pl.CreatedAt,
			UpdatedAt:    pl.UpdatedAt,
		}
	case "push":
		event.Type = "push"
		event.Action = "push"
		event.Branch = strings.TrimPrefix(pl.Ref, "refs/heads/")
		event.CommitSHA = pl.After
	case "tag_push":
		event.Type = provider.EventTypeTag + "created"
		event.Action = "created"
		event.Tag = strings.TrimPrefix(pl.Ref, "refs/tags/")
		event.CommitSHA = pl.After
	case "note", "comment":
		event.Type = provider.EventTypeComment + provider.CommentActionCreated
		event.Action = provider.CommentActionCreated
		event.Comment = &provider.IssueComment{Body: pl.Note, Author: actor}
		if pl.NoteableType == "PullRequest" && pl.Number != 0 {
			event.CR = &provider.ChangeRequest{Number: strconv.Itoa(pl.Number)}
		}
		if pl.Issue != nil {
			event.Issue = &provider.Issue{
				Number: pl.Issue.Number, Title: pl.Issue.Title, Body: pl.Issue.Body,
				Author: &provider.CRUser{ID: int64(pl.Issue.User.ID), Username: pl.Issue.User.Login},
				WebURL: pl.Issue.HTMLURL, CreatedAt: pl.Issue.CreatedAt, UpdatedAt: pl.Issue.UpdatedAt,
			}
		}
	case "Issue Hook":
		action := provider.NormalizeIssueAction(pl.Action)
		event.Type = provider.EventTypeIssue + action
		event.Action = action
		if pl.Issue != nil {
			state := provider.IssueStateOpen
			if strings.HasPrefix(pl.Issue.State, "closed") || strings.HasPrefix(pl.Issue.State, "reject") {
				state = provider.IssueStateClosed
			}
			event.Issue = &provider.Issue{
				Number: pl.Issue.Number, Title: pl.Issue.Title, Body: pl.Issue.Body,
				State:  state,
				Author: &provider.CRUser{ID: int64(pl.Issue.User.ID), Username: pl.Issue.User.Login},
				WebURL: pl.Issue.HTMLURL, CreatedAt: pl.Issue.CreatedAt, UpdatedAt: pl.Issue.UpdatedAt,
			}
		}
	}
	return event, nil
}

var _ provider.WebhookManager = (*Provider)(nil)
