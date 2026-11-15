package backendutil

import (
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// Auth builds the transport auth strategy for cfg. When cfg.TokenSource is
// set it returns a per-request refreshable TokenSourceAuth in the given
// style (provider.TokenSource and transport.TokenSource share one method
// set, so the value forwards unchanged); otherwise it returns the static
// strategy equivalent for that style. style is one of the
// transport.AuthStyle* constants.
func Auth(cfg provider.Config, style string) transport.AuthStrategy {
	if cfg.TokenSource != nil {
		return transport.TokenSourceAuth{Source: cfg.TokenSource, Style: style}
	}
	switch style {
	case transport.AuthStylePrivate:
		return transport.PrivateToken{Token: cfg.Token}
	case transport.AuthStyleToken:
		return transport.TokenHeader{Token: cfg.Token}
	default:
		return transport.BearerToken{Token: cfg.Token}
	}
}

// ConditionalCache returns the ETag cache shared by a backend's transport
// client and round tripper when cfg.ConditionalRequests is enabled, nil
// otherwise. Assign straight to transport.Client.ETag.
func ConditionalCache(cfg provider.Config) *transport.ETagCache {
	if !cfg.ConditionalRequests {
		return nil
	}
	return transport.NewETagCache(0)
}
