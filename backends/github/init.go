package github

import (
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

func init() {
	provider.Register(model.PlatformGitHub, func(cfg provider.Config) (provider.Provider, error) {
		return New(cfg)
	})
}
