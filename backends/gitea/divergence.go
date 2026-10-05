package gitea

import "github.com/yi-nology/go-git-platform/provider"

// Divergences returns the registered divergence ledger for the Gitea backend.
func Divergences() []provider.Divergence { return giteafamilyDivergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return giteafamilyDivergences }
