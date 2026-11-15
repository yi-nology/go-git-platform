package forgejo

import (
	"context"
	"net/http"

	"github.com/yi-nology/go-git-platform/adapter/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/runtime"
)

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*model.PlatformWebhook, error) {
	return giteafamily.CreateWebhook(ctx, p.family, p.ports, opts)
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	return giteafamily.DeleteWebhook(ctx, p.family, p.ports, owner, repo, webhookID)
}

// ListWebhooks implements provider.WebhookManager.
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*model.PlatformWebhook, error) {
	return giteafamily.ListWebhooks(ctx, p.family, p.ports, owner, repo)
}

// GetWebhook implements provider.WebhookConfigManager.
func (p *Provider) GetWebhook(ctx context.Context, owner, repo string, webhookID int64) (*model.PlatformWebhook, error) {
	return giteafamily.GetWebhook(ctx, p.family, p.ports, owner, repo, webhookID)
}

// UpdateWebhook implements provider.WebhookConfigManager.
func (p *Provider) UpdateWebhook(ctx context.Context, owner, repo string, webhookID int64, opts provider.UpdateWebhookOptions) (*model.PlatformWebhook, error) {
	return giteafamily.UpdateWebhook(ctx, p.family, p.ports, owner, repo, webhookID, opts)
}

// ValidateWebhookSignature implements provider.WebhookManager. It
// delegates to the platform's registered validator so the signature
// scheme has exactly one implementation (shared with contracttest);
// notably an empty secret is rejected rather than silently accepted.
func (p *Provider) ValidateWebhookSignature(r *http.Request, secret string) error {
	if err := runtime.ValidateWebhookWithRegistry(model.PlatformForgejo, r, secret); err != nil {
		return provider.Wrap(model.PlatformForgejo, "ValidateWebhookSignature", err)
	}
	return nil
}

// ParseWebhookEvent implements provider.WebhookManager.
func (p *Provider) ParseWebhookEvent(r *http.Request, secret string) (*model.NormalizedEvent, error) {
	return giteafamily.ParseWebhookEvent(p.family, r, secret)
}

var _ provider.WebhookManager = (*Provider)(nil)
var _ provider.WebhookConfigManager = (*Provider)(nil)
