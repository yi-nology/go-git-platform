package forgejo

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

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*provider.PlatformWebhook, error) {
	events := opts.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request"}
	}
	hook, _, err := p.client.CreateRepoHook(opts.Owner, opts.Repo, forgejo.CreateHookOption{
		Type:   forgejo.HookTypeForgejo,
		Config: map[string]string{"url": opts.URL, "content_type": "json", "secret": opts.Secret},
		Events: events,
		Active: true,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "CreateWebhook", err)
	}
	return convertHook(hook), nil
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	_, err := p.client.DeleteRepoHook(owner, repo, webhookID)
	if err != nil {
		return provider.Wrap(provider.PlatformForgejo, "DeleteWebhook", err)
	}
	return nil
}

// ListWebhooks implements provider.WebhookManager. The provider surface
// carries no pagination parameters, so the full hook list is fetched by
// exhausting the endpoint's pagination (backendutil.AllPages).
//
// The forgejo SDK accepts no context parameter (registered platform
// limitation), so ctx is unused beyond signature conformance.
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*provider.PlatformWebhook, error) {
	hooks, err := backendutil.AllPages(func(page int) ([]*forgejo.Hook, error) {
		list, _, err := p.client.ListRepoHooks(owner, repo, forgejo.ListHooksOptions{
			ListOptions: forgejo.ListOptions{Page: page, PageSize: listPageSize},
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformForgejo, "ListWebhooks", err)
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
	if err := provider.ValidateWebhookWithRegistry(provider.PlatformForgejo, r, secret); err != nil {
		return provider.Wrap(provider.PlatformForgejo, "ValidateWebhookSignature", err)
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
		return nil, provider.Wrap(provider.PlatformForgejo, "ParseWebhookEvent", readErr)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	eventType := r.Header.Get("X-Forgejo-Event")
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
		return nil, provider.Wrap(provider.PlatformForgejo, "ParseWebhookEvent", err)
	}

	er := provider.BuildEventRepo(pl.Repository.FullName)
	er.ID = pl.Repository.ID
	actor := &provider.CRUser{ID: int64(pl.Sender.ID), Username: pl.Sender.Login}

	event := &provider.NormalizedEvent{
		ID:         fmt.Sprintf("gt-%d-%d", time.Now().UnixNano(), pl.Number),
		Source:     p.Platform(),
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
				State:        mapState(pl.PullRequest.State, pl.PullRequest.Merged),
				Draft:        pl.PullRequest.Draft,
				SourceBranch: pl.PullRequest.Head.Ref,
				TargetBranch: pl.PullRequest.Base.Ref,
				HeadSHA:      pl.PullRequest.Head.SHA,
				BaseSHA:      pl.PullRequest.Base.SHA,
				// Forgejo exposes no distinct merge-base; base.sha is the target tip.
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

var _ provider.WebhookManager = (*Provider)(nil)
