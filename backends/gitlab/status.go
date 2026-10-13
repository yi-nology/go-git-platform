package gitlab

import (
	"errors"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/provider"
)

// init registers GitLab's SDK error extractor: client-go's single
// *ErrorResponse carries StatusCode as a first-class field (its 404 sentinel
// ErrNotFound is also an *ErrorResponse with StatusCode 404).
func init() {
	provider.RegisterStatusExtractor(provider.PlatformGitLab, func(err error) (int, bool) {
		var er *gitlab.ErrorResponse
		if errors.As(err, &er) && er.StatusCode > 0 {
			return er.StatusCode, true
		}
		return 0, false
	})
}
