package github

import "github.com/yi-nology/go-git-platform/provider"

// divergences is the GitHub divergence ledger: the registered places where
// this backend's behavior departs from the unified provider semantics.
// See provider.Divergence and docs/divergence-ledger.md.
var divergences = []provider.Divergence{
	{Capability: "ChangeRequestManager", Method: "GetCR", Field: "BaseSHA", Kind: provider.DivergenceMapping,
		Reason: "GitHub payloads expose no merge base; BaseSHA carries the target-branch tip instead (StartSHA equals BaseSHA), as does every other method returning a change request."},
	{Capability: "ChangeRequestManager", Method: "ListCRs", Field: "BaseSHA", Kind: provider.DivergenceMapping,
		Reason: "See GetCR."},
	{Capability: "ReleaseManager", Method: "GetArchive", Kind: provider.DivergenceDetour,
		Reason: "Archive downloads stream through the raw transport client rather than go-github."},
	{Capability: "WebhookConfigManager", Method: "UpdateWebhook", Field: "opts.Events (empty)", Kind: provider.DivergenceIgnore,
		Reason: "go-github marshals Hook.Events with omitempty, so an empty slice is indistinguishable from \"unchanged\" on the wire — clearing every event is not expressible here (other platforms send explicit empty event sets)."},
}

// Divergences returns the registered divergence ledger for the GitHub backend.
func Divergences() []provider.Divergence { return divergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return divergences }
