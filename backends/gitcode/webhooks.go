package gitcode

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

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*provider.PlatformWebhook, error) {
	events := opts.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request"}
	}
	hook, err := p.client.CreateWebhook(ctx, opts.Owner, opts.Repo, gitcode.CreateWebhookOptions{
		URL: opts.URL, Secret: opts.Secret, Events: events,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateWebhook", err)
	}
	return &provider.PlatformWebhook{ID: hook.ID, URL: hook.URL, Events: hook.Events}, nil
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	err := p.client.DeleteWebhook(ctx, owner, repo, webhookID)
	if err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteWebhook", err)
	}
	return nil
}

// ListWebhooks implements provider.WebhookManager. The SDK's hooks endpoint
// exposes no pagination parameters, so the call stays single-shot.
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*provider.PlatformWebhook, error) {
	hooks, err := p.client.ListWebhooks(ctx, owner, repo)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListWebhooks", err)
	}
	result := make([]*provider.PlatformWebhook, 0, len(hooks))
	for _, h := range hooks {
		result = append(result, &provider.PlatformWebhook{ID: h.ID, URL: h.URL, Events: h.Events})
	}
	return result, nil
}

// ValidateWebhookSignature implements provider.WebhookManager. It
// delegates to the platform's registered validator so the signature
// scheme has exactly one implementation (shared with contracttest);
// notably an empty secret is rejected rather than silently accepted.
func (p *Provider) ValidateWebhookSignature(r *http.Request, secret string) error {
	if err := provider.ValidateWebhookWithRegistry(provider.PlatformGitCode, r, secret); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "ValidateWebhookSignature", err)
	}
	return nil
}

// ParseWebhookEvent implements provider.WebhookManager.
func (p *Provider) ParseWebhookEvent(r *http.Request, secret string) (*provider.NormalizedEvent, error) {
	if err := p.ValidateWebhookSignature(r, secret); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ParseWebhookEvent", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	eventType := r.Header.Get("X-Gitea-Event")
	if eventType == "" {
		eventType = r.Header.Get("X-GitCode-Event")
	}

	ne := &provider.NormalizedEvent{
		Source:     p.Platform(),
		Timestamp:  time.Now(),
		RawPayload: json.RawMessage(body),
	}

	switch eventType {
	case "pull_request":
		prEvent, err := p.client.ParsePullRequestEvent(body)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitCode, "ParseWebhookEvent", err)
		}
		action := provider.NormalizeCRAction(prEvent.Action, prEvent.PullRequest != nil && prEvent.PullRequest.Merged)
		ne.Type = provider.EventTypeCR + action
		ne.Action = action
		if prEvent.Sender != nil {
			senderID, _ := parseGitCodeID(prEvent.Sender.ID)
			ne.Actor = &provider.CRUser{
				ID: senderID, Username: prEvent.Sender.Login, AvatarURL: prEvent.Sender.AvatarURL,
			}
		}
		if prEvent.Repository != nil {
			ne.Repo = provider.BuildEventRepo(prEvent.Repository.FullName)
		}
		if prEvent.PullRequest != nil {
			ne.CR = convertPullRequest(prEvent.PullRequest)
			if prEvent.PullRequest.Head != nil {
				ne.CommitSHA = prEvent.PullRequest.Head.SHA
			}
		}
	case "push":
		pushEvent, err := p.client.ParsePushEvent(body)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitCode, "ParseWebhookEvent", err)
		}
		ne.Type = "push"
		ne.Branch = strings.TrimPrefix(pushEvent.Ref, "refs/heads/")
		ne.CommitSHA = pushEvent.After
		if pushEvent.Sender != nil {
			senderID, _ := parseGitCodeID(pushEvent.Sender.ID)
			ne.Actor = &provider.CRUser{
				ID: senderID, Username: pushEvent.Sender.Login, AvatarURL: pushEvent.Sender.AvatarURL,
			}
		}
		if pushEvent.Repository != nil {
			ne.Repo = provider.BuildEventRepo(pushEvent.Repository.FullName)
		}
	case "tag_push":
		tagEvent, err := p.client.ParseTagPushEvent(body)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitCode, "ParseWebhookEvent", err)
		}
		ne.Type = provider.EventTypeTag + "created"
		ne.Action = "created"
		ne.Tag = strings.TrimPrefix(tagEvent.Ref, "refs/tags/")
		ne.CommitSHA = tagEvent.After
		if tagEvent.Sender != nil {
			senderID, _ := parseGitCodeID(tagEvent.Sender.ID)
			ne.Actor = &provider.CRUser{
				ID: senderID, Username: tagEvent.Sender.Login, AvatarURL: tagEvent.Sender.AvatarURL,
			}
		}
		if tagEvent.Repository != nil {
			ne.Repo = provider.BuildEventRepo(tagEvent.Repository.FullName)
		}
	case "create":
		var createEvent struct {
			Ref        string              `json:"ref"`
			RefType    string              `json:"ref_type"`
			Sender     *gitcode.User       `json:"sender"`
			Repository *gitcode.Repository `json:"repository"`
		}
		if err := json.Unmarshal(body, &createEvent); err != nil {
			return nil, provider.Wrap(provider.PlatformGitCode, "ParseWebhookEvent", err)
		}
		if createEvent.RefType == "tag" {
			ne.Type = provider.EventTypeTag + "created"
			ne.Action = "created"
			ne.Tag = createEvent.Ref
		} else {
			ne.Type = provider.EventTypeBranch + "created"
			ne.Action = "created"
			ne.Branch = createEvent.Ref
		}
		if createEvent.Sender != nil {
			senderID, _ := parseGitCodeID(createEvent.Sender.ID)
			ne.Actor = &provider.CRUser{
				ID: senderID, Username: createEvent.Sender.Login, AvatarURL: createEvent.Sender.AvatarURL,
			}
		}
		if createEvent.Repository != nil {
			ne.Repo = provider.BuildEventRepo(createEvent.Repository.FullName)
		}
	case "delete":
		var deleteEvent struct {
			Ref        string              `json:"ref"`
			RefType    string              `json:"ref_type"`
			Sender     *gitcode.User       `json:"sender"`
			Repository *gitcode.Repository `json:"repository"`
		}
		if err := json.Unmarshal(body, &deleteEvent); err != nil {
			return nil, provider.Wrap(provider.PlatformGitCode, "ParseWebhookEvent", err)
		}
		if deleteEvent.RefType == "tag" {
			ne.Type = provider.EventTypeTag + "deleted"
			ne.Action = "deleted"
			ne.Tag = deleteEvent.Ref
		} else {
			ne.Type = provider.EventTypeBranch + "deleted"
			ne.Action = "deleted"
			ne.Branch = deleteEvent.Ref
		}
		if deleteEvent.Sender != nil {
			senderID, _ := parseGitCodeID(deleteEvent.Sender.ID)
			ne.Actor = &provider.CRUser{
				ID: senderID, Username: deleteEvent.Sender.Login, AvatarURL: deleteEvent.Sender.AvatarURL,
			}
		}
		if deleteEvent.Repository != nil {
			ne.Repo = provider.BuildEventRepo(deleteEvent.Repository.FullName)
		}
	case "issues":
		var issueEvent struct {
			Action     string               `json:"action"`
			Issue      *gitcodeIssuePayload `json:"issue"`
			Sender     *gitcode.User        `json:"sender"`
			Repository *gitcode.Repository  `json:"repository"`
		}
		if err := json.Unmarshal(body, &issueEvent); err != nil {
			return nil, provider.Wrap(provider.PlatformGitCode, "ParseWebhookEvent", err)
		}
		action := provider.NormalizeIssueAction(issueEvent.Action)
		ne.Type = provider.EventTypeIssue + action
		ne.Action = action
		attachGitCodeIssue(ne, issueEvent.Issue, issueEvent.Sender, issueEvent.Repository)
	case "issue_comment":
		var commentEvent struct {
			Action  string               `json:"action"`
			Issue   *gitcodeIssuePayload `json:"issue"`
			Comment *struct {
				Body string `json:"body"`
			} `json:"comment"`
			Sender     *gitcode.User       `json:"sender"`
			Repository *gitcode.Repository `json:"repository"`
		}
		if err := json.Unmarshal(body, &commentEvent); err != nil {
			return nil, provider.Wrap(provider.PlatformGitCode, "ParseWebhookEvent", err)
		}
		ne.Type = provider.EventTypeComment + provider.CommentActionCreated
		ne.Action = provider.CommentActionCreated
		if commentEvent.Comment != nil && commentEvent.Sender != nil {
			senderID, _ := parseGitCodeID(commentEvent.Sender.ID)
			ne.Comment = &provider.IssueComment{
				Body:   commentEvent.Comment.Body,
				Author: &provider.CRUser{ID: senderID, Username: commentEvent.Sender.Login, AvatarURL: commentEvent.Sender.AvatarURL},
			}
		}
		attachGitCodeIssue(ne, commentEvent.Issue, commentEvent.Sender, commentEvent.Repository)
	default:
		ne.Type = eventType
	}
	return ne, nil
}

// gitcodeIssuePayload is the issue object carried by GitCode's
// gitea-shaped issues/issue_comment hooks.
type gitcodeIssuePayload struct {
	Number  int           `json:"number"`
	Title   string        `json:"title"`
	Body    string        `json:"body"`
	State   string        `json:"state"`
	HTMLURL string        `json:"html_url"`
	User    *gitcode.User `json:"user"`
	// PullRequest is present when the "issue" is a pull request.
	PullRequest *struct {
		Merged bool `json:"merged"`
	} `json:"pull_request"`
}

func attachGitCodeIssue(ne *provider.NormalizedEvent, issue *gitcodeIssuePayload, sender *gitcode.User, repo *gitcode.Repository) {
	if sender != nil {
		senderID, _ := parseGitCodeID(sender.ID)
		ne.Actor = &provider.CRUser{ID: senderID, Username: sender.Login, AvatarURL: sender.AvatarURL}
	}
	if repo != nil {
		ne.Repo = provider.BuildEventRepo(repo.FullName)
	}
	if issue == nil {
		return
	}
	state := provider.IssueStateOpen
	if issue.State == "closed" {
		state = provider.IssueStateClosed
	}
	var author *provider.CRUser
	if issue.User != nil {
		userID, _ := parseGitCodeID(issue.User.ID)
		author = &provider.CRUser{ID: userID, Username: issue.User.Login, AvatarURL: issue.User.AvatarURL}
	}
	ne.Issue = &provider.Issue{
		ID:     int64(issue.Number),
		Number: strconv.Itoa(issue.Number),
		Title:  issue.Title,
		Body:   issue.Body,
		State:  state,
		Author: author,
		WebURL: issue.HTMLURL,
	}
	if issue.PullRequest != nil {
		ne.CR = &provider.ChangeRequest{
			ID: int64(issue.Number), Number: strconv.Itoa(issue.Number), Title: issue.Title,
		}
	}
}

// parseGitCodeID converts an SDK gitcode.FlexString (an alias for a string)
// into an int64. Returns 0 when the value is empty or non-numeric.
func parseGitCodeID(id gitcode.FlexString) (int64, error) {
	return parseInt64(string(id))
}

func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer %q", s)
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

var _ provider.WebhookManager = (*Provider)(nil)
