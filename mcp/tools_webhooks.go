package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- webhooks (toolset "webhooks") ---
//
// WebhookManager is a core Provider interface (every backend implements
// create/list/delete), so the toolset mounts unconditionally; the single-
// webhook get/update pair additionally requires WebhookConfigManager and is
// only registered when the concrete provider implements it.

type createWebhookIn struct {
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	URL    string   `json:"url" jsonschema:"payload delivery URL"`
	Secret string   `json:"secret,omitempty" jsonschema:"shared secret for signature validation"`
	Events []string `json:"events,omitempty" jsonschema:"event names to subscribe (platform vocabulary, e.g. push, pull_request); empty = platform default"`
}

func registerWebhooks(s *mcp.Server, st *state) {
	wm, ok := st.p.(provider.WebhookManager)
	if !ok {
		return
	}

	add(s, st, "list_webhooks", "List webhooks", "List the webhooks of a repository with their IDs, delivery URLs and subscribed events.", false, func(ctx context.Context, in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}) ([]*provider.PlatformWebhook, error) {
		return wm.ListWebhooks(ctx, in.Owner, in.Repo)
	})

	add(s, st, "create_webhook", "Create webhook", "Create a repository webhook that delivers platform events to the given URL.", true, func(ctx context.Context, in createWebhookIn) (*provider.PlatformWebhook, error) {
		return wm.CreateWebhook(ctx, provider.CreateWebhookOptions{
			Owner: in.Owner, Repo: in.Repo, URL: in.URL, Secret: in.Secret, Events: in.Events,
		})
	})

	add(s, st, "delete_webhook", "Delete webhook", "Delete a repository webhook by its platform ID.", true, func(ctx context.Context, in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		ID    int64  `json:"id" jsonschema:"webhook ID (platform-addressed)"`
	}) (map[string]any, error) {
		if err := wm.DeleteWebhook(ctx, in.Owner, in.Repo, in.ID); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	// Single-webhook get/update is the optional WebhookConfigManager
	// capability — register the pair only where the concrete provider
	// implements it (today: all seven backends).
	cm, ok := st.p.(provider.WebhookConfigManager)
	if !ok {
		return
	}

	add(s, st, "get_webhook", "Get webhook", "Fetch one webhook by its platform ID.", false, func(ctx context.Context, in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		ID    int64  `json:"id" jsonschema:"webhook ID (platform-addressed)"`
	}) (*provider.PlatformWebhook, error) {
		return cm.GetWebhook(ctx, in.Owner, in.Repo, in.ID)
	})

	type updateWebhookIn struct {
		Owner  string   `json:"owner"`
		Repo   string   `json:"repo"`
		ID     int64    `json:"id" jsonschema:"webhook ID (platform-addressed)"`
		URL    *string  `json:"url,omitempty" jsonschema:"new payload delivery URL; omitted = unchanged"`
		Secret *string  `json:"secret,omitempty" jsonschema:"new shared secret; omitted = unchanged"`
		Events []string `json:"events,omitempty" jsonschema:"event names; the full replacement set (an empty array clears every event where the platform supports it); omitted = unchanged"`
	}
	add(s, st, "update_webhook", "Update webhook", "Patch a webhook: nil fields are left unchanged; a non-nil events array replaces the whole event set.", true, func(ctx context.Context, in updateWebhookIn) (*provider.PlatformWebhook, error) {
		if in.URL == nil && in.Secret == nil && in.Events == nil {
			return nil, fmt.Errorf("update_webhook needs at least one of url, secret, events")
		}
		return cm.UpdateWebhook(ctx, in.Owner, in.Repo, in.ID, provider.UpdateWebhookOptions{
			URL: in.URL, Secret: in.Secret, Events: in.Events,
		})
	})
}
