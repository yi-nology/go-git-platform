package forgejo

import (
	"context"
	"net/http"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateWebhook implements provider.WebhookManager.
func (p *Provider) CreateWebhook(ctx context.Context, opts provider.CreateWebhookOptions) (*provider.PlatformWebhook, error) {
	return giteafamily.CreateWebhook(ctx, p.family, p.ports, opts)
}

// DeleteWebhook implements provider.WebhookManager.
func (p *Provider) DeleteWebhook(ctx context.Context, owner, repo string, webhookID int64) error {
	return giteafamily.DeleteWebhook(ctx, p.family, p.ports, owner, repo, webhookID)
}

// ListWebhooks implements provider.WebhookManager.
func (p *Provider) ListWebhooks(ctx context.Context, owner, repo string) ([]*provider.PlatformWebhook, error) {
	return giteafamily.ListWebhooks(ctx, p.family, p.ports, owner, repo)
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
	return giteafamily.ParseWebhookEvent(p.family, r, secret)
}

var _ provider.WebhookManager = (*Provider)(nil)
