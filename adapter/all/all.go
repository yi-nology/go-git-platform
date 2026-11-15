// Package all imports every platform backend that ships with the SDK. Import
// it with a blank identifier once at the start of your program to register
// all platforms with provider:
//
//	import _ "github.com/yi-nology/go-git-platform/adapter/all"
//
// After that, provider.NewProvider accepts any platform configured via
// provider.Config.
package all

import (
	// Each backend self-registers via its own init().
	_ "github.com/yi-nology/go-git-platform/adapter/forgejo"
	_ "github.com/yi-nology/go-git-platform/adapter/gitcode"
	_ "github.com/yi-nology/go-git-platform/adapter/gitea"
	_ "github.com/yi-nology/go-git-platform/adapter/gitee"
	_ "github.com/yi-nology/go-git-platform/adapter/github"
	_ "github.com/yi-nology/go-git-platform/adapter/gitlab"
	_ "github.com/yi-nology/go-git-platform/adapter/tencentcode"
)
