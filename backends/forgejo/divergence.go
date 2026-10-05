package forgejo

import "github.com/yi-nology/go-git-platform/provider"

// Divergences returns the registered divergence ledger for the Forgejo backend.
func Divergences() []provider.Divergence { return forgejoDivergences }

// Divergences implements provider.Provider.
func (p *Provider) Divergences() []provider.Divergence { return forgejoDivergences }
