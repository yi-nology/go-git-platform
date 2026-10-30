package provider

import (
	"context"
	"net/http"
)

// WebhookManager handles webhook CRUD and event parsing.
type WebhookManager interface {
	CreateWebhook(ctx context.Context, opts CreateWebhookOptions) (*PlatformWebhook, error)
	DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error
	ListWebhooks(ctx context.Context, owner, repo string) ([]*PlatformWebhook, error)
	ParseWebhookEvent(r *http.Request, secret string) (*NormalizedEvent, error)
	ValidateWebhookSignature(r *http.Request, secret string) error
}

// WebhookConfigManager provides single-webhook read and update on top of the
// core WebhookManager. It is an optional capability interface: consumers
// should gate on Provider.Capabilities().WebhookConfig (or type-assert)
// before use.
//
// Platform support: all seven backends (GitHub, GitLab, Gitee, GitCode,
// Gitea, Forgejo, TencentCode). Update is patch-style — nil fields are left
// unchanged — with one documented exception: a non-nil Events slice always
// replaces the full event set (an empty slice clears every event), because
// event membership cannot be expressed incrementally on the platforms whose
// webhooks are boolean-flag based (GitLab/Gitee/TencentCode). The secret is
// write-only on every platform: GetWebhook never returns it.
type WebhookConfigManager interface {
	// GetWebhook returns one webhook by its platform ID.
	GetWebhook(ctx context.Context, owner, repo string, webhookID int64) (*PlatformWebhook, error)
	// UpdateWebhook patches one webhook. Nil URL/Secret leave the
	// corresponding setting untouched; a nil Events slice leaves the event
	// set untouched while a non-nil one replaces it wholesale.
	UpdateWebhook(ctx context.Context, owner, repo string, webhookID int64, opts UpdateWebhookOptions) (*PlatformWebhook, error)
}
