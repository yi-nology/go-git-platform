package gitcode

import (
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

func init() {
	provider.Register(model.PlatformGitCode, func(cfg provider.Config) (provider.Provider, error) {
		return New(cfg)
	})
}
