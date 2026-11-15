package runtime

import (
	"context"
	"errors"
	"slices"

	"github.com/oekit/pkg/slicex"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// MutationOutcome 是统一变更结果（2B spec 2026-11-14-phase2b）：自动化按
// outcome 分支，不必解析错误文本或比对返回对象。
type MutationOutcome string

const (
	OutcomeCreated   MutationOutcome = "created"   // 资源原本不存在，已创建
	OutcomeUpdated   MutationOutcome = "updated"   // 存在且发生了写入
	OutcomeUnchanged MutationOutcome = "unchanged" // 已达期望态，零写入
	// OutcomeConflict 是唯一同时携带 outcome 与非 nil error 的分支
	// （并发冲突 409，err 经 provider.IsConflict 可判）；其余 outcome 错误一律
	// ("", nil, err)。实体是写前快照（pre-write snapshot），冲突浮出时
	// 可能已被并发改动/删除——以 err 为准。
	OutcomeConflict MutationOutcome = "conflict"
)

// EnsureAction 是 MutationOutcome 的既有名——类型别名，源码完全兼容；
// 既有 EnsureCreated/EnsureUpdated/EnsureUnchanged 常量保留（新代码
// 优先用 Outcome* 名）。
type EnsureAction = MutationOutcome

// Ensure* outcomes (aliases of the Outcome* names, kept for source
// compatibility).
const (
	// EnsureCreated: the resource did not exist and was created.
	EnsureCreated = OutcomeCreated
	// EnsureUpdated: the resource existed with different content and was
	// updated (for webhooks and deploy keys: replaced, see below).
	EnsureUpdated = OutcomeUpdated
	// EnsureUnchanged: the resource already matched the desired state.
	EnsureUnchanged = OutcomeUnchanged
)

// EnsureWebhook makes the repository carry exactly one webhook for
// opts.URL with opts.Events, creating or replacing as needed, and returns
// the resulting webhook plus the action taken. It is the idempotent
// building block automation wants: "make this hook exist, whatever it
// takes", safe to re-run.
//
// Semantics:
//
//   - No webhook with that URL → created.
//   - Same URL, same event set → unchanged. Secrets are write-only on
//     every platform (list responses never return them), so an existing
//     hook's secret is not comparable; re-running with a different secret
//     reports unchanged and keeps the old secret. Set
//     provider.CreateWebhookOptions.EnsureSecretMode to provider.EnsureSecretAlwaysUpdate
//     to rewrite the secret in that case instead (rotation convergence).
//   - Same URL, different event set → updated. The provider.WebhookManager
//     interface has no update method, so the hook is deleted and
//     recreated; its platform ID changes as a result.
//   - Empty opts.Events defaults to {push, pull_request}, matching
//     CreateWebhook's platform-side default.
//
// Multiple existing hooks with the same URL are all replaced by one.
func EnsureWebhook(ctx context.Context, p provider.WebhookManager, opts provider.CreateWebhookOptions) (EnsureAction, *model.PlatformWebhook, error) {
	events := opts.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request"}
	}
	desired := canonicalWebhookEvents(events)

	existing, err := p.ListWebhooks(ctx, opts.Owner, opts.Repo)
	if err != nil {
		return "", nil, err
	}

	matches := slicex.Filter(existing, func(h *model.PlatformWebhook) bool {
		return h != nil && h.URL == opts.URL
	})

	if len(matches) == 1 && equalEvents(canonicalWebhookEvents(matches[0].Events), desired) {
		// Secret write-only: the match says nothing about the stored
		// secret. always-update converges a rotated secret in place
		// instead of trusting the match.
		if opts.EnsureSecretMode == provider.EnsureSecretAlwaysUpdate && opts.Secret != "" {
			return ensureWebhookSecret(ctx, p, opts, matches[0])
		}
		return EnsureUnchanged, matches[0], nil
	}

	// Drift (or duplicates): replace every matching hook with one fresh one.
	for _, h := range matches {
		if err := p.DeleteWebhook(ctx, opts.Owner, opts.Repo, h.ID); err != nil {
			if provider.IsConflict(err) {
				return OutcomeConflict, matches[0], err
			}
			return "", nil, err
		}
	}
	hook, err := p.CreateWebhook(ctx, provider.CreateWebhookOptions{
		Owner: opts.Owner, Repo: opts.Repo, URL: opts.URL,
		Secret: opts.Secret, Events: events,
	})
	if err != nil {
		if provider.IsConflict(err) {
			var prior *model.PlatformWebhook
			if len(matches) > 0 {
				prior = matches[0]
			}
			return OutcomeConflict, prior, err
		}
		return "", nil, err
	}
	if len(matches) == 0 {
		return EnsureCreated, hook, nil
	}
	return EnsureUpdated, hook, nil
}

// webhookEventCanonical 把各后端 webhook 事件名归一到比对用的 canonical
// 族（issue #40）。写侧与读侧词表不同源是 GitLab 家族的结构：Create/Update
// 接受 pull_request/cr 等输入名并落到 merge_requests_events 布尔位，而
// List/Get 的 convertHook 把同一布尔位读回 merge_request——归一不做，
// EnsureWebhook 的「同 URL 同事件集 ⇒ unchanged」永不成立，每次调用都走
// delete+recreate（钩子 ID 翻动、删除与重建之间丢投递、重建失败即裸奔）。
// 比对两侧（desired 与 list 输出）都过这一张表；未登记名原样透传
// （平台特定事件不误判漂移）。
var webhookEventCanonical = map[string]string{
	"push":           "push",
	"merge_request":  "pull_request",
	"merge_requests": "pull_request",
	"pull_request":   "pull_request",
	"cr":             "pull_request",
	"tag_push":       "tag_push",
	"tag":            "tag_push",
	"note":           "note",
	"comment":        "note",
	"issue_comment":  "note",
	"issues":         "issue",
	"issue":          "issue",
}

func canonicalWebhookEvents(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if c, ok := webhookEventCanonical[n]; ok {
			out = append(out, c)
		} else {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	// Aliases collapse onto one canonical name ("note"+"comment" → "note"):
	// dedupe after the sort so equalEvents compares sets, not multisets —
	// a backend reading one flag back under two names (issue #85) must not
	// look like drift against the single-name desired set.
	return slices.Compact(out)
}

func equalEvents(a, sortedB []string) bool {
	sa := slices.Clone(a)
	slices.Sort(sa)
	return slices.Equal(sa, sortedB)
}

// ensureWebhookSecret is the provider.EnsureSecretAlwaysUpdate convergence path for
// a hook that already matches URL + events: patch the secret in place via
// provider.WebhookConfigManager (keeps the platform ID, one API round-trip). A
// manager without the config surface is an error — silently keeping a
// possibly-stale secret would defeat the mode.
func ensureWebhookSecret(ctx context.Context, p provider.WebhookManager, opts provider.CreateWebhookOptions, hook *model.PlatformWebhook) (EnsureAction, *model.PlatformWebhook, error) {
	wcm, ok := p.(provider.WebhookConfigManager)
	if !ok {
		return "", nil, errors.New("provider: EnsureSecretAlwaysUpdate requires the manager to also implement WebhookConfigManager")
	}
	secret := opts.Secret
	out, err := wcm.UpdateWebhook(ctx, opts.Owner, opts.Repo, hook.ID, provider.UpdateWebhookOptions{Secret: &secret})
	if err != nil {
		if provider.IsConflict(err) {
			return OutcomeConflict, hook, err
		}
		return "", nil, err
	}
	return EnsureUpdated, out, nil
}

// EnsureBranchProtection converges the branch protection rule for
// opts.BranchName onto opts, creating, updating, or leaving it untouched
// as required, and returns the resulting rule plus the action taken.
// It targets the provider.BranchProtectionManager directly so it works with any
// provider.Provider (type-assert it at the call site) or test double.
// 契约防御：Get 违约返回 (nil, nil)（iface 契约禁止）时视为缺席走 Create
// 而非 Update——见 iface_branch_protection.go Get 的契约注释。
func EnsureBranchProtection(ctx context.Context, bpm provider.BranchProtectionManager, owner, repo string, opts provider.CreateBranchProtectionOptions) (EnsureAction, *model.BranchProtection, error) {
	cur, err := bpm.GetBranchProtection(ctx, owner, repo, opts.BranchName)
	if err == nil && cur != nil {
		if cur.RequiredApprovingReviews == opts.RequiredApprovingReviews &&
			cur.RequiredStatusChecks == opts.RequiredStatusChecks &&
			cur.AllowForcePushes == opts.AllowForcePushes &&
			cur.AllowDeletions == opts.AllowDeletions {
			return EnsureUnchanged, cur, nil
		}
		out, err := bpm.UpdateBranchProtection(ctx, owner, repo, opts.BranchName, provider.UpdateBranchProtectionOptions{
			RequiredApprovingReviews: &opts.RequiredApprovingReviews,
			RequiredStatusChecks:     &opts.RequiredStatusChecks,
			AllowForcePushes:         &opts.AllowForcePushes,
			AllowDeletions:           &opts.AllowDeletions,
		})
		if err != nil {
			if provider.IsConflict(err) {
				return OutcomeConflict, cur, err
			}
			return "", nil, err
		}
		return EnsureUpdated, out, nil
	}
	if err != nil && !provider.IsNotFound(err) {
		return "", nil, err
	}
	// 缺席：provider.IsNotFound，或违约的 (nil, nil)——统一 Create。
	out, err := bpm.CreateBranchProtection(ctx, owner, repo, opts)
	if err != nil {
		if provider.IsConflict(err) {
			return OutcomeConflict, nil, err
		}
		return "", nil, err
	}
	return EnsureCreated, out, nil
}

// EnsureDeployKey converges the repository's deploy key titled
// opts.Title onto opts, creating, replacing, or leaving it untouched as
// required, and returns the resulting key plus the action taken.
//
// Existing keys are matched by title; a title collision with different
// key material or read-only flag replaces the key (the manager interface
// has no update: delete + re-add), which changes its platform ID.
func EnsureDeployKey(ctx context.Context, dkm provider.DeploymentKeyManager, owner, repo string, opts provider.AddDeployKeyOptions) (EnsureAction, *model.DeployKey, error) {
	existing, err := dkm.ListDeployKeys(ctx, owner, repo)
	if err != nil {
		return "", nil, err
	}
	for _, k := range existing {
		if k == nil || k.Title != opts.Title {
			continue
		}
		if k.Key == opts.Key && k.ReadOnly == opts.ReadOnly {
			return EnsureUnchanged, k, nil
		}
		if err := dkm.DeleteDeployKey(ctx, owner, repo, k.ID); err != nil {
			if provider.IsConflict(err) {
				return OutcomeConflict, k, err
			}
			return "", nil, err
		}
		out, err := dkm.AddDeployKey(ctx, owner, repo, opts)
		if err != nil {
			if provider.IsConflict(err) {
				return OutcomeConflict, k, err
			}
			return "", nil, err
		}
		return EnsureUpdated, out, nil
	}
	out, err := dkm.AddDeployKey(ctx, owner, repo, opts)
	if err != nil {
		if provider.IsConflict(err) {
			return OutcomeConflict, nil, err
		}
		return "", nil, err
	}
	return EnsureCreated, out, nil
}
