package tencentcode

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

	gongfeng "github.com/studyzy/gongfeng-sdk-go"
	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*provider.PlatformWebhook, error) {
	pid := opts.Owner + "/" + opts.Repo
	addOpts := &gongfeng.AddWebhookOptions{
		URL:        gongfeng.Ptr(opts.URL),
		PushEvents: gongfeng.Ptr(true),
	}
	if len(opts.Events) > 0 {
		em := map[string]bool{}
		for _, e := range opts.Events {
			em[e] = true
		}
		if v, ok := em["push"]; ok {
			addOpts.PushEvents = gongfeng.Ptr(v)
		}
		mrEvents := em["merge_request"] || em["merge_requests"] || em["pull_request"] || em["cr"]
		addOpts.MergeRequestsEvents = gongfeng.Ptr(mrEvents)
		tagEvents := em["tag_push"] || em["tag"]
		addOpts.TagPushEvents = gongfeng.Ptr(tagEvents)
	}
	hook, _, err := p.client.Webhooks.AddWebhook(ctx, pid, addOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateWebhook", err)
	}
	return convertWebhook(hook), nil
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	pid := owner + "/" + repo
	_, err := p.client.Webhooks.DeleteWebhook(ctx, pid, int(webhookID))
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteWebhook", err)
	}
	return nil
}

// ListWebhooks implements provider.WebhookManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*provider.PlatformWebhook, error) {
	pid := owner + "/" + repo
	hooks, err := backendutil.AllPages(func(page int) ([]*gongfeng.Webhook, error) {
		list, _, err := p.client.Webhooks.ListWebhooks(ctx, pid,
			&gongfeng.ListWebhooksOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListWebhooks", err)
	}
	result := make([]*provider.PlatformWebhook, 0, len(hooks))
	for _, wh := range hooks {
		result = append(result, convertWebhook(wh))
	}
	return result, nil
}

// ValidateWebhookSignature implements provider.WebhookManager. It
// delegates to the platform's registered validator so the signature
// scheme has exactly one implementation (shared with contracttest);
// notably an empty secret is rejected rather than silently accepted.
func (p *Provider) ValidateWebhookSignature(r *http.Request, secret string) error {
	if err := provider.ValidateWebhookWithRegistry(provider.PlatformTencentCode, r, secret); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "ValidateWebhookSignature", err)
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
		return nil, provider.Wrap(provider.PlatformTencentCode, "ParseWebhookEvent", readErr)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	var pl struct {
		ObjectKind string `json:"object_kind"`
		User       struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
		Repository struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"repository"`
		Project struct {
			ID         int64  `json:"id"`
			PathWithNS string `json:"path_with_namespace"`
		} `json:"project"`
		ObjectAttributes struct {
			IID            int    `json:"iid"`
			Title          string `json:"title"`
			Description    string `json:"description"`
			State          string `json:"state"`
			SourceBranch   string `json:"source_branch"`
			TargetBranch   string `json:"target_branch"`
			Action         string `json:"action"`
			MergeStatus    string `json:"merge_status"`
			URL            string `json:"url"`
			MergeCommitSHA string `json:"merge_commit_sha"`
			WorkInProgress bool   `json:"work_in_progress"`
			LastCommit     struct {
				ID string `json:"id"`
			} `json:"last_commit"`
			DiffRefs struct {
				BaseSHA  string `json:"base_sha"`
				StartSHA string `json:"start_sha"`
				HeadSHA  string `json:"head_sha"`
			} `json:"diff_refs"`
			CreatedAt gongfeng.Time `json:"created_at"`
			UpdatedAt gongfeng.Time `json:"updated_at"`
			// note hooks carry the comment body in object_attributes (the
			// triggering action field above serves issue hooks).
			Note string `json:"note"`
		} `json:"object_attributes"`
		// note hooks carry their target as a top-level issue or
		// merge_request object.
		Issue struct {
			IID  int    `json:"iid"`
			Name string `json:"title"`
		} `json:"issue"`
		MergeRequest struct {
			IID int `json:"iid"`
		} `json:"merge_request"`
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
		TotalCount int    `json:"total_commits_count"`
		Commits    []struct {
			ID string `json:"id"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(body, &pl); err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ParseWebhookEvent", err)
	}

	repoName := pl.Project.PathWithNS
	if repoName == "" {
		repoName = pl.Repository.Name
	}
	er := provider.BuildEventRepo(repoName)
	er.ID = pl.Project.ID
	actor := &provider.CRUser{ID: int64(pl.User.ID), Username: pl.User.Username, Name: pl.User.Name}

	event := &provider.NormalizedEvent{
		ID:         fmt.Sprintf("tc-%d-%d", time.Now().UnixNano(), pl.ObjectAttributes.IID),
		Source:     p.Platform(),
		Timestamp:  time.Now(),
		Actor:      actor,
		Repo:       er,
		RawPayload: json.RawMessage(body),
	}

	switch pl.ObjectKind {
	case "merge_request":
		state := mapState(pl.ObjectAttributes.State)
		action := provider.NormalizeCRAction(pl.ObjectAttributes.Action, state == provider.CRStateMerged)
		event.Type = "cr." + action
		event.Action = action
		event.CommitSHA = pl.ObjectAttributes.LastCommit.ID
		headSHA, baseSHA, startSHA := provider.ResolveMRSHAs(
			pl.ObjectAttributes.DiffRefs.HeadSHA,
			pl.ObjectAttributes.DiffRefs.BaseSHA,
			pl.ObjectAttributes.DiffRefs.StartSHA,
			pl.ObjectAttributes.MergeCommitSHA,
			pl.ObjectAttributes.LastCommit.ID,
		)
		event.CR = &provider.ChangeRequest{
			ID:           int64(pl.ObjectAttributes.IID),
			Number:       strconv.Itoa(pl.ObjectAttributes.IID),
			Title:        pl.ObjectAttributes.Title,
			Description:  pl.ObjectAttributes.Description,
			State:        state,
			Draft:        pl.ObjectAttributes.WorkInProgress,
			SourceBranch: pl.ObjectAttributes.SourceBranch,
			TargetBranch: pl.ObjectAttributes.TargetBranch,
			HeadSHA:      headSHA,
			BaseSHA:      baseSHA,
			StartSHA:     startSHA,
			MergeStatus:  pl.ObjectAttributes.MergeStatus,
			WebURL:       pl.ObjectAttributes.URL,
			Author:       actor,
			CreatedAt:    pl.ObjectAttributes.CreatedAt.Time,
			UpdatedAt:    pl.ObjectAttributes.UpdatedAt.Time,
		}
	case "push":
		event.Type = "push"
		event.Branch = strings.TrimPrefix(pl.Ref, "refs/heads/")
		event.CommitSHA = pl.After
	case "tag_push":
		event.Type = "tag.created"
		event.Tag = strings.TrimPrefix(pl.Ref, "refs/tags/")
	case "issue":
		action := provider.NormalizeIssueAction(pl.ObjectAttributes.Action)
		event.Type = provider.EventTypeIssue + action
		event.Action = action
		event.Issue = &provider.Issue{
			ID:     int64(pl.ObjectAttributes.IID),
			Number: strconv.Itoa(pl.ObjectAttributes.IID),
			Title:  pl.ObjectAttributes.Title,
			Body:   pl.ObjectAttributes.Description,
			State:  mapGongfengIssueState(pl.ObjectAttributes.State),
			Author: actor,
		}
	case "note":
		event.Type = provider.EventTypeComment + provider.CommentActionCreated
		event.Action = provider.CommentActionCreated
		event.Comment = &provider.IssueComment{Body: pl.ObjectAttributes.Note, Author: actor}
		if pl.MergeRequest.IID != 0 {
			event.CR = &provider.ChangeRequest{
				ID: int64(pl.MergeRequest.IID), Number: strconv.Itoa(pl.MergeRequest.IID),
			}
		}
		if pl.Issue.IID != 0 {
			event.Issue = &provider.Issue{
				ID: int64(pl.Issue.IID), Number: strconv.Itoa(pl.Issue.IID), Title: pl.Issue.Name, Author: actor,
			}
		}
	}
	return event, nil
}

// mapGongfengIssueState maps gongfeng (GitLab-v3-flavored) issue states
// onto the unified IssueState vocabulary.
func mapGongfengIssueState(state string) provider.IssueState {
	switch state {
	case "closed":
		return provider.IssueStateClosed
	case "opened", "reopened", "":
		return provider.IssueStateOpen
	default:
		return provider.IssueState(state)
	}
}

var _ provider.WebhookManager = (*Provider)(nil)
