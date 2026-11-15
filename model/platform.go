package model

// Platform represents a Git hosting platform.
type Platform string

const (
	PlatformGitLab      Platform = "gitlab"
	PlatformGitHub      Platform = "github"
	PlatformGitea       Platform = "gitea"
	PlatformGitee       Platform = "gitee"
	PlatformForgejo     Platform = "forgejo"
	PlatformTencentCode Platform = "tencent_code"
	PlatformGitCode     Platform = "gitcode"
)
