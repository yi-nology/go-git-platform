package gitee

import (
	"errors"

	gitee "github.com/next-bin/go-gitee/gitee"
	"github.com/yi-nology/go-git-platform/provider"
)

// init registers Gitee's SDK error extractor: go-gitee's three status-
// carrying types (ErrorResponse/GiteeError/RateLimitError) all expose the
// status only through Response *http.Response.
func init() {
	provider.RegisterStatusExtractor(provider.PlatformGitee, func(err error) (int, bool) {
		var er *gitee.ErrorResponse
		if errors.As(err, &er) && er.Response != nil {
			return er.Response.StatusCode, true
		}
		var ge *gitee.GiteeError
		if errors.As(err, &ge) && ge.Response != nil {
			return ge.Response.StatusCode, true
		}
		var rl *gitee.RateLimitError
		if errors.As(err, &rl) && rl.Response != nil {
			return rl.Response.StatusCode, true
		}
		return 0, false
	})
}
