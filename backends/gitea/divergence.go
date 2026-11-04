package gitea

import (
	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// giteaDivergences extends the family ledger with Gitea-only entries.
// Forgejo shares the family ledger but has no Actions surface at all, so
// CILogManager entries must stay backend-local — a family-level entry would
// register a divergence for a method forgejo does not even implement.
var giteaDivergences = append(giteafamily.Divergences("Gitea"),
	provider.Divergence{
		Capability: "CILogManager", Method: "ListFailedJobLogs", Field: "", Kind: provider.DivergenceMapping,
		Reason: "A commit can fan out to multiple Actions runs (one per workflow file path); failed jobs are collected from the latest run per workflow path (runs return newest-first, first sighting wins) instead of a single pipeline.",
	},
)

// Divergences returns the registered divergence ledger for the Gitea backend.
func Divergences() []provider.Divergence { return giteaDivergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return giteaDivergences }
