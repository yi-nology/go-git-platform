package gitlab

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

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*provider.PlatformWebhook, error) {
	pid := pidOf(opts.Owner, opts.Repo)
	hookOpts := &gitlab.AddProjectHookOptions{
		URL:   new(opts.URL),
		Token: new(opts.Secret),
	}
	hookOpts.PushEvents = new(true)
	if len(opts.Events) > 0 {
		em := map[string]bool{}
		for _, e := range opts.Events {
			em[e] = true
		}
		if v, ok := em["push"]; ok {
			hookOpts.PushEvents = new(v)
		}
		hookOpts.MergeRequestsEvents = new(em["merge_request"] || em["merge_requests"] || em["pull_request"] || em["cr"])
		hookOpts.TagPushEvents = new(em["tag_push"] || em["tag"])
	}
	hook, _, err := p.client.Projects.AddProjectHook(pid, hookOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "CreateWebhook", err)
	}
	return convertHook(hook), nil
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	_, err := p.client.Projects.DeleteProjectHook(pidOf(owner, repo), webhookID, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(provider.PlatformGitLab, "DeleteWebhook", err)
	}
	return nil
}

// ListWebhooks implements provider.WebhookManager.
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*provider.PlatformWebhook, error) {
	hooks, err := backendutil.AllPages(func(page int) ([]*gitlab.ProjectHook, error) {
		list, _, err := p.client.Projects.ListProjectHooks(pidOf(owner, repo), &gitlab.ListProjectHooksOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListWebhooks", err)
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
	if err := provider.ValidateWebhookWithRegistry(provider.PlatformGitLab, r, secret); err != nil {
		return provider.Wrap(provider.PlatformGitLab, "ValidateWebhookSignature", err)
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
		return nil, provider.Wrap(provider.PlatformGitLab, "ParseWebhookEvent", readErr)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	var pl struct {
		ObjectKind string `json:"object_kind"`
		User       struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
		Project struct {
			ID         int64  `json:"id"`
			PathWithNS string `json:"path_with_namespace"`
		} `json:"project"`
		ObjectAttributes struct {
			IID            int64  `json:"iid"`
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
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			SHA       string    `json:"sha"`
			Status    string    `json:"status"`
			// Note hook: object_attributes is the note itself.
			Note         string `json:"note"`
			NoteableType string `json:"noteable_type"` //nolint:misspell // 平台 webhook 原始字段名
		} `json:"object_attributes"`
		MergeRequest struct {
			IID int64 `json:"iid"`
		} `json:"merge_request"`
		Issue struct {
			IID         int64     `json:"iid"`
			Title       string    `json:"title"`
			Description string    `json:"description"`
			State       string    `json:"state"`
			CreatedAt   time.Time `json:"created_at"`
			UpdatedAt   time.Time `json:"updated_at"`
		} `json:"issue"`
		Ref   string `json:"ref"`
		After string `json:"after"`
	}
	if err := json.Unmarshal(body, &pl); err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ParseWebhookEvent", err)
	}

	er := provider.BuildEventRepo(pl.Project.PathWithNS)
	er.ID = pl.Project.ID
	actor := &provider.CRUser{ID: pl.User.ID, Username: pl.User.Username, Name: pl.User.Name}

	event := &provider.NormalizedEvent{
		ID:         fmt.Sprintf("gl-%d-%d", time.Now().UnixNano(), pl.ObjectAttributes.IID),
		RawPayload: json.RawMessage(body),
		Source:     p.Platform(),
		Timestamp:  time.Now(),
		Actor:      actor,
		Repo:       er,
	}

	switch pl.ObjectKind {
	case "merge_request":
		state := mapGLState(pl.ObjectAttributes.State)
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
			ID:           pl.ObjectAttributes.IID,
			Number:       strconv.FormatInt(pl.ObjectAttributes.IID, 10),
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
			CreatedAt:    pl.ObjectAttributes.CreatedAt,
			UpdatedAt:    pl.ObjectAttributes.UpdatedAt,
		}
	case "push":
		event.Type = "push"
		event.Action = "push"
		event.Branch = strings.TrimPrefix(pl.Ref, "refs/heads/")
		event.CommitSHA = pl.After
	case "tag_push":
		event.Type = "tag.created"
		event.Tag = strings.TrimPrefix(pl.Ref, "refs/tags/")
	case "pipeline":
		// Pipeline Hook（v0.67.0）：只把失败终态送进事件流——成功/运行中对
		// CI 失败归因场景是噪声。pipeline 的 sha 取 object_attributes.sha
		// （merge_request 关联字段仅在 MR hook 出现，pipeline hook 用
		// merge_request.iid 若有则带出，供评论回帖定位）。
		status := pl.ObjectAttributes.Status
		if status != "failed" && status != "canceled" {
			return nil, nil
		}
		event.Type = "pipeline." + status
		event.Action = status
		event.CommitSHA = pl.ObjectAttributes.SHA
		if pl.MergeRequest.IID != 0 {
			event.CR = &provider.ChangeRequest{
				ID:     pl.MergeRequest.IID,
				Number: strconv.FormatInt(pl.MergeRequest.IID, 10),
			}
		}
	case "note":
		// Note hook: object_attributes is the note; the referenced target
		// rides along as a top-level issue or merge_request object. The
		// canonical type is comment.created regardless of the target —
		// consumers branch on which of CR/Issue is populated.
		event.Type = provider.EventTypeComment + provider.CommentActionCreated
		event.Action = provider.CommentActionCreated
		// GitLab note hooks carry no comment timestamps; leave them zero
		// rather than fabricating the parse time.
		event.Comment = &provider.IssueComment{
			Body: pl.ObjectAttributes.Note, Author: actor,
		}
		if pl.MergeRequest.IID != 0 {
			event.CR = &provider.ChangeRequest{
				ID: pl.MergeRequest.IID, Number: strconv.FormatInt(pl.MergeRequest.IID, 10),
			}
		}
		if pl.Issue.IID != 0 {
			event.Issue = &provider.Issue{
				ID: pl.Issue.IID, Number: strconv.FormatInt(pl.Issue.IID, 10),
				Title:     pl.Issue.Title,
				State:     mapGLIssueState(pl.Issue.State),
				Author:    actor,
				CreatedAt: pl.Issue.CreatedAt, UpdatedAt: pl.Issue.UpdatedAt,
			}
		}
	case "issue":
		// Issue hook: object_attributes carries the issue (iid, title,
		// description, state) plus the triggering action.
		action := provider.NormalizeIssueAction(pl.ObjectAttributes.Action)
		event.Type = provider.EventTypeIssue + action
		event.Action = action
		event.Issue = &provider.Issue{
			ID: pl.ObjectAttributes.IID, Number: strconv.FormatInt(pl.ObjectAttributes.IID, 10),
			Title: pl.ObjectAttributes.Title, Body: pl.ObjectAttributes.Description,
			State:     mapGLIssueState(pl.ObjectAttributes.State),
			Author:    actor,
			CreatedAt: pl.ObjectAttributes.CreatedAt, UpdatedAt: pl.ObjectAttributes.UpdatedAt,
		}
	}
	return event, nil
}

// mapGLIssueState maps GitLab issue states ("opened"/"reopened"/"closed")
// onto the unified IssueState vocabulary.
func mapGLIssueState(state string) provider.IssueState {
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
