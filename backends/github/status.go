package github

import (
	"errors"

	"github.com/google/go-github/v92/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// init registers GitHub's SDK error extractor: go-github's five status-
// carrying error types all expose the status through Response *http.Response
// (RedirectionError also carries StatusCode directly). Replaces the old
// reflection-over-fields fallback with explicit errors.As checks.
func init() {
	provider.RegisterStatusExtractor(provider.PlatformGitHub, func(err error) (int, bool) {
		var er *github.ErrorResponse
		if errors.As(err, &er) && er.Response != nil {
			return er.Response.StatusCode, true
		}
		var rl *github.RateLimitError
		if errors.As(err, &rl) && rl.Response != nil {
			return rl.Response.StatusCode, true
		}
		var ab *github.AbuseRateLimitError
		if errors.As(err, &ab) && ab.Response != nil {
			return ab.Response.StatusCode, true
		}
		var tf *github.TwoFactorAuthError
		if errors.As(err, &tf) && tf.Response != nil {
			return tf.Response.StatusCode, true
		}
		var rd *github.RedirectionError
		if errors.As(err, &rd) {
			if rd.StatusCode > 0 {
				return rd.StatusCode, true
			}
			if rd.Response != nil {
				return rd.Response.StatusCode, true
			}
		}
		return 0, false
	})
}
