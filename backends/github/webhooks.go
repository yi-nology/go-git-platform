package github

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/go-github/v92/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*provider.PlatformWebhook, error) {
	events := opts.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request"}
	}
	hook := &github.Hook{
		Name:   new("web"),
		Events: events,
		Config: &github.HookConfig{
			URL:    new(opts.URL),
			Secret: new(opts.Secret),
		},
		Active: new(true),
	}
	h, _, err := p.client.Repositories.CreateHook(ctx, opts.Owner, opts.Repo, hook)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateWebhook", err)
	}
	return convertHook(h), nil
}

// GetWebhook implements provider.WebhookConfigManager.
func (p *Provider) GetWebhook(ctx context.Context, owner, repo string, webhookID int64) (*provider.PlatformWebhook, error) {
	h, _, err := p.client.Repositories.GetHook(ctx, owner, repo, webhookID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetWebhook", err)
	}
	return convertHook(h), nil
}

// UpdateWebhook implements provider.WebhookConfigManager. GitHub's PATCH
// semantics keep unprovided settings; the config object is only attached
// when at least one of URL/Secret is being changed so the remaining config
// keys stay server-side.
func (p *Provider) UpdateWebhook(ctx context.Context, owner, repo string, webhookID int64, opts provider.UpdateWebhookOptions) (*provider.PlatformWebhook, error) {
	hook := &github.Hook{}
	if opts.Events != nil {
		hook.Events = opts.Events
	}
	if opts.URL != nil || opts.Secret != nil {
		cfg := &github.HookConfig{}
		if opts.URL != nil {
			cfg.URL = opts.URL
		}
		if opts.Secret != nil {
			cfg.Secret = opts.Secret
		}
		hook.Config = cfg
	}
	h, _, err := p.client.Repositories.EditHook(ctx, owner, repo, webhookID, hook)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateWebhook", err)
	}
	return convertHook(h), nil
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	_, err := p.client.Repositories.DeleteHook(ctx, owner, repo, webhookID)
	if err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DeleteWebhook", err)
	}
	return nil
}

// ListWebhooks implements provider.WebhookManager. The provider interface
// exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*provider.PlatformWebhook, error) {
	hooks, err := backendutil.AllPages(func(page int) ([]*github.Hook, error) {
		list, _, err := p.client.Repositories.ListHooks(ctx, owner, repo, &github.ListOptions{
			Page: page, PerPage: 100,
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListWebhooks", err)
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
	if err := provider.ValidateWebhookWithRegistry(provider.PlatformGitHub, r, secret); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "ValidateWebhookSignature", err)
	}
	return nil
}

// ParseWebhookEvent implements provider.WebhookManager.
// githubEnvelope is GitHub's webhook dialect for the shared envelope steps.
// The event header and payload decoding stay on the go-github library's
// path; GitHub never set an event ID, so the prefix stays empty.
var githubEnvelope = provider.WebhookEnvelope{
	Platform: provider.PlatformGitHub,
	// X-GitHub-Delivery:GitHub 每次投递都带(重投时值不变)⇒ 真正的幂等键。
	// 以前 GitHub 的事件 ID 恒为空串 ⇒ 上层按 event_id 去重/唯一索引时,
	// 第 2 条起的事件被静默丢弃(入站同步从第 2 条起完全失效)。
	IDHeaders: []string{"X-GitHub-Delivery"},
}

func (p *Provider) ParseWebhookEvent(r *http.Request, secret string) (*provider.NormalizedEvent, error) {
	payload, err := github.ValidatePayload(r, []byte(secret))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ParseWebhookEvent", err)
	}
	eventType := github.WebHookType(r)
	event, err := github.ParseWebHook(eventType, payload)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ParseWebhookEvent", err)
	}

	ne := githubEnvelope.NewEventWithID(r, 0, payload)

	switch e := event.(type) {
	case *github.PullRequestEvent:
		ne.Type = provider.EventTypeCR + provider.NormalizeCRAction(e.GetAction(), e.GetPullRequest().GetMerged())
		ne.Actor = convertUser(e.GetSender())
		if e.GetRepo() != nil {
			ne.Repo = provider.BuildEventRepo(e.GetRepo().GetFullName())
			ne.Repo.ID = e.GetRepo().GetID()
		}
		if e.GetPullRequest() != nil {
			ne.CR = convertPR(e.GetPullRequest())
			ne.CommitSHA = e.GetPullRequest().GetHead().GetSHA()
		}
	case *github.PushEvent:
		ne.Type = "push"
		ne.Branch = strings.TrimPrefix(e.GetRef(), "refs/heads/")
		ne.CommitSHA = e.GetAfter()
		ne.Actor = convertUser(e.GetSender())
		if e.GetRepo() != nil {
			ne.Repo = provider.BuildEventRepo(e.GetRepo().GetFullName())
			ne.Repo.ID = e.GetRepo().GetID()
		}
	case *github.IssuesEvent:
		action := provider.NormalizeIssueAction(e.GetAction())
		ne.Type = provider.EventTypeIssue + action
		ne.Action = action
		ne.Actor = convertUser(e.GetSender())
		if e.GetRepo() != nil {
			ne.Repo = provider.BuildEventRepo(e.GetRepo().GetFullName())
			ne.Repo.ID = e.GetRepo().GetID()
		}
		if e.GetIssue() != nil {
			ne.Issue = convertIssue(e.GetIssue())
		}
	case *github.IssueCommentEvent:
		// Comments on both plain issues and PRs arrive here; a PR comment
		// also carries the pull request, so CR is populated alongside Issue.
		// GitHub sends action created|edited|deleted — derived, not assumed.
		action := normalizeCommentAction(e.GetAction())
		ne.Type = provider.EventTypeComment + action
		ne.Action = action
		ne.Actor = convertUser(e.GetSender())
		if e.GetRepo() != nil {
			ne.Repo = provider.BuildEventRepo(e.GetRepo().GetFullName())
			ne.Repo.ID = e.GetRepo().GetID()
		}
		if e.GetIssue() != nil {
			ne.Issue = convertIssue(e.GetIssue())
		}
		if pr := e.GetIssue().GetPullRequestLinks(); pr != nil {
			ne.CR = &provider.ChangeRequest{Number: ne.Issue.Number}
		}
	case *github.PullRequestReviewCommentEvent:
		action := normalizeCommentAction(e.GetAction())
		ne.Type = provider.EventTypeComment + action
		ne.Action = action
		ne.Actor = convertUser(e.GetSender())
		if e.GetRepo() != nil {
			ne.Repo = provider.BuildEventRepo(e.GetRepo().GetFullName())
			ne.Repo.ID = e.GetRepo().GetID()
		}
		if e.GetPullRequest() != nil {
			ne.CR = convertPR(e.GetPullRequest())
		}
		if c := e.GetComment(); c != nil {
			// PullRequestComment is a different SDK type from IssueComment
			// but carries the same normalized fields.
			ne.Comment = &provider.IssueComment{
				ID:     c.GetID(),
				Body:   c.GetBody(),
				Author: convertUser(c.GetUser()),
			}
		}
	case *github.CreateEvent:
		// ref_type distinguishes tags from branches.
		ne.Actor = convertUser(e.GetSender())
		if e.GetRepo() != nil {
			ne.Repo = provider.BuildEventRepo(e.GetRepo().GetFullName())
			ne.Repo.ID = e.GetRepo().GetID()
		}
		if e.GetRefType() == "tag" {
			ne.Type = provider.EventTypeTag + "created"
			ne.Action = "created"
			ne.Tag = e.GetRef()
		} else {
			ne.Type = provider.EventTypeBranch + "created"
			ne.Action = "created"
			ne.Branch = e.GetRef()
		}
	case *github.DeleteEvent:
		ne.Actor = convertUser(e.GetSender())
		if e.GetRepo() != nil {
			ne.Repo = provider.BuildEventRepo(e.GetRepo().GetFullName())
			ne.Repo.ID = e.GetRepo().GetID()
		}
		if e.GetRefType() == "tag" {
			ne.Type = provider.EventTypeTag + "deleted"
			ne.Action = "deleted"
			ne.Tag = e.GetRef()
		} else {
			ne.Type = provider.EventTypeBranch + "deleted"
			ne.Action = "deleted"
			ne.Branch = e.GetRef()
		}
	default:
		ne.Type = eventType
	}
	return ne, nil
}

// normalizeCommentAction maps a GitHub comment action onto the canonical
// vocabulary; unknown values pass through so new GitHub actions surface
// verbatim instead of masquerading as creations.
func normalizeCommentAction(action string) string {
	switch action {
	case "created", "edited", "deleted":
		return action
	case "":
		return provider.CommentActionCreated
	default:
		return action
	}
}

// compile-time guard
var _ provider.WebhookManager = (*Provider)(nil)
var _ provider.WebhookConfigManager = (*Provider)(nil)
