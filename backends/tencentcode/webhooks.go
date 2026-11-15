package tencentcode

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	gongfeng "github.com/yi-nology/go-gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/runtime"
)

// applyWebhookEvents maps the unified event vocabulary onto 工蜂's boolean
// webhook flags (issue #91). Every flag is set explicitly — true only for
// the families the caller asked for, false for the rest: the edit endpoint
// treats omitted flags as "unchanged", so a partial flag set can neither
// express removal nor shrink a subscription. The vocabulary mirrors
// convertWebhook's read side exactly (push / merge_request / tag_push /
// issues / note) — a smaller write side made EnsureWebhook diverge forever
// (listed "issues" vs desired "push"-only leak). issue_comment/comment ride
// the platform's note family.
func applyWebhookEvents(em map[string]bool, push, issues, mrs, tags, notes **bool) {
	*push = gongfeng.Ptr(em["push"])
	*issues = gongfeng.Ptr(em["issues"] || em["issue"])
	*mrs = gongfeng.Ptr(em["merge_request"] || em["merge_requests"] || em["pull_request"] || em["cr"])
	*tags = gongfeng.Ptr(em["tag_push"] || em["tag"])
	*notes = gongfeng.Ptr(em["note"] || em["issue_comment"] || em["comment"])
}

func webhookEventSet(events []string) map[string]bool {
	em := make(map[string]bool, len(events))
	for _, e := range events {
		em[e] = true
	}
	return em
}

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*model.PlatformWebhook, error) {
	pid := opts.Owner + "/" + opts.Repo
	addOpts := &gongfeng.AddWebhookOptions{
		URL: gongfeng.Ptr(opts.URL),
	}
	events := opts.Events
	if len(events) == 0 {
		// historical default: a push subscription
		events = []string{"push"}
	}
	applyWebhookEvents(webhookEventSet(events),
		&addOpts.PushEvents, &addOpts.IssuesEvents, &addOpts.MergeRequestsEvents,
		&addOpts.TagPushEvents, &addOpts.NoteEvents)
	hook, _, err := p.client.Webhooks.AddWebhook(ctx, pid, addOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "CreateWebhook", err)
	}
	return convertWebhook(hook), nil
}

// GetWebhook implements provider.WebhookConfigManager.
func (p *Provider) GetWebhook(ctx context.Context, owner, repo string, webhookID int64) (*model.PlatformWebhook, error) {
	hook, _, err := p.client.Webhooks.GetWebhook(ctx, owner+"/"+repo, int(webhookID))
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "GetWebhook", err)
	}
	return convertWebhook(hook), nil
}

// UpdateWebhook implements provider.WebhookConfigManager. 工蜂's edit
// endpoint is boolean-flag based: omitted flags mean "unchanged", so a
// non-nil Events slice must set every derived flag explicitly (true for
// members, false for the rest) to express removal — mirroring the create
// mapping's full scope (issue #91: issues/note included, or hooks created
// for them keep their stale flags through every update).
func (p *Provider) UpdateWebhook(ctx context.Context, owner, repo string, webhookID int64, opts provider.UpdateWebhookOptions) (*model.PlatformWebhook, error) {
	editOpts := &gongfeng.EditWebhookOptions{}
	if opts.URL != nil {
		editOpts.URL = opts.URL
	}
	if opts.Secret != nil {
		editOpts.Token = opts.Secret
	}
	if opts.Events != nil {
		applyWebhookEvents(webhookEventSet(opts.Events),
			&editOpts.PushEvents, &editOpts.IssuesEvents, &editOpts.MergeRequestsEvents,
			&editOpts.TagPushEvents, &editOpts.NoteEvents)
	}
	hook, _, err := p.client.Webhooks.EditWebhook(ctx, owner+"/"+repo, int(webhookID), editOpts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "UpdateWebhook", err)
	}
	return convertWebhook(hook), nil
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	pid := owner + "/" + repo
	_, err := p.client.Webhooks.DeleteWebhook(ctx, pid, int(webhookID))
	if err != nil {
		return provider.Wrap(model.PlatformTencentCode, "DeleteWebhook", err)
	}
	return nil
}

// ListWebhooks implements provider.WebhookManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*model.PlatformWebhook, error) {
	pid := owner + "/" + repo
	hooks, err := backendutil.AllPages(func(page int) ([]*gongfeng.Webhook, error) {
		list, _, err := p.client.Webhooks.ListWebhooks(ctx, pid,
			&gongfeng.ListWebhooksOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "ListWebhooks", err)
	}
	result := make([]*model.PlatformWebhook, 0, len(hooks))
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
	if err := runtime.ValidateWebhookWithRegistry(model.PlatformTencentCode, r, secret); err != nil {
		return provider.Wrap(model.PlatformTencentCode, "ValidateWebhookSignature", err)
	}
	return nil
}

// ParseWebhookEvent implements provider.WebhookManager.
// tencentEnvelope is TencentCODE's webhook dialect for the shared envelope
// steps (dispatch rides the payload's object_kind, not an event header).
var tencentEnvelope = runtime.WebhookEnvelope{
	Platform: model.PlatformTencentCode,
	IDPrefix: "tc",
	// 投递 ID 头（issue #48）：工蜂是 GitLab-v3 衍生，投递带 per-webhook
	// UUID（X-Gitlab-Event-UUID 家族）；此前不声明 IDHeaders ⇒ 每次投递
	// （含重投）都本地合成新 ID，按 NormalizedEvent.ID 去重的宿主对重投
	// 事件重复处理。头缺席时回落合成 ID，行为不回退。
	IDHeaders: []string{"X-Gitlab-Event-UUID", "X-Request-ID"},
}

func (p *Provider) ParseWebhookEvent(r *http.Request, secret string) (*model.NormalizedEvent, error) {
	_, body, err := tencentEnvelope.Read(r, secret)
	if err != nil {
		return nil, err
	}

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
		return nil, provider.Wrap(model.PlatformTencentCode, "ParseWebhookEvent", err)
	}

	repoName := pl.Project.PathWithNS
	if repoName == "" {
		repoName = pl.Repository.Name
	}
	er := runtime.BuildEventRepo(repoName)
	er.ID = pl.Project.ID
	actor := &model.CRUser{ID: int64(pl.User.ID), Username: pl.User.Username, Name: pl.User.Name}

	event := tencentEnvelope.NewEvent(int64(pl.ObjectAttributes.IID), body)
	event.Actor = actor
	event.Repo = er

	switch pl.ObjectKind {
	case "merge_request":
		state := mapState(pl.ObjectAttributes.State)
		action := runtime.NormalizeCRAction(pl.ObjectAttributes.Action, state == model.CRStateMerged)
		event.Type = "cr." + action
		event.Action = action
		event.CommitSHA = pl.ObjectAttributes.LastCommit.ID
		headSHA, baseSHA, startSHA := runtime.ResolveMRSHAs(
			pl.ObjectAttributes.DiffRefs.HeadSHA,
			pl.ObjectAttributes.DiffRefs.BaseSHA,
			pl.ObjectAttributes.DiffRefs.StartSHA,
			pl.ObjectAttributes.MergeCommitSHA,
			pl.ObjectAttributes.LastCommit.ID,
		)
		event.CR = &model.ChangeRequest{
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
		// 删除归一（issue #67，同族 #47 的 push 案）：工蜂家族没有独立
		// delete 事件，分支删除以 push + 全零 after 投递——此前一律归一成
		// push，消费方 switch branch.deleted 静默漏掉。before 非零确认真删除。
		if runtime.IsWebhookDeletionSHA(pl.After) && !runtime.IsWebhookDeletionSHA(pl.Before) {
			event.Type = runtime.EventTypeBranch + "deleted"
			event.Action = "deleted"
			// 全零 after 是删除标记而非提交,不设 commit_sha。
		} else {
			event.Type = "push"
			event.CommitSHA = pl.After
		}
		event.Branch = strings.TrimPrefix(pl.Ref, "refs/heads/")
	case "tag_push":
		// 删除归一（issue #47，同 GitLab 家族——全零 after 即删除投递）
		if runtime.IsWebhookDeletionSHA(pl.After) && !runtime.IsWebhookDeletionSHA(pl.Before) {
			event.Type = "tag.deleted"
			event.Action = "deleted"
		} else {
			event.Type = "tag.created"
			event.Action = "created"
		}
		event.Tag = strings.TrimPrefix(pl.Ref, "refs/tags/")
	case "issue":
		action := runtime.NormalizeIssueAction(pl.ObjectAttributes.Action)
		event.Type = runtime.EventTypeIssue + action
		event.Action = action
		event.Issue = &model.Issue{
			ID:     int64(pl.ObjectAttributes.IID),
			Number: strconv.Itoa(pl.ObjectAttributes.IID),
			Title:  pl.ObjectAttributes.Title,
			Body:   pl.ObjectAttributes.Description,
			State:  mapGongfengIssueState(pl.ObjectAttributes.State),
			Author: actor,
		}
	case "note":
		event.Type = runtime.EventTypeComment + runtime.CommentActionCreated
		event.Action = runtime.CommentActionCreated
		event.Comment = &model.IssueComment{Body: pl.ObjectAttributes.Note, Author: actor}
		if pl.MergeRequest.IID != 0 {
			event.CR = &model.ChangeRequest{
				ID: int64(pl.MergeRequest.IID), Number: strconv.Itoa(pl.MergeRequest.IID),
			}
		}
		if pl.Issue.IID != 0 {
			event.Issue = &model.Issue{
				ID: int64(pl.Issue.IID), Number: strconv.Itoa(pl.Issue.IID), Title: pl.Issue.Name, Author: actor,
			}
		}
	}
	return event, nil
}

// mapGongfengIssueState maps gongfeng (GitLab-v3-flavored) issue states
// onto the unified IssueState vocabulary.
func mapGongfengIssueState(state string) model.IssueState {
	switch state {
	case "closed":
		return model.IssueStateClosed
	case "opened", "reopened", "":
		return model.IssueStateOpen
	default:
		return model.IssueState(state)
	}
}

var _ provider.WebhookManager = (*Provider)(nil)
var _ provider.WebhookConfigManager = (*Provider)(nil)
