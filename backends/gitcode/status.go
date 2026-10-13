package gitcode

import (
	"errors"

	"github.com/yi-nology/go-git-platform/provider"
	gitcode "github.com/yi-nology/go-gitcode"
)

// init registers GitCode's SDK error extractor. go-gitcode's *APIError
// carries StatusCode directly, and each specialized wrapper
// (NotFound/Unauthorized/Forbidden/RateLimit/Conflict/Validation) embeds
// *APIError without an Unwrap method — so errors.As must target every
// wrapper type individually (the status field is promoted from the embedded
// pointer, nil-guarded here).
func init() {
	provider.RegisterStatusExtractor(provider.PlatformGitCode, func(err error) (int, bool) {
		var nf *gitcode.NotFoundError
		if errors.As(err, &nf) && nf.APIError != nil {
			return nf.APIError.StatusCode, true
		}
		var ua *gitcode.UnauthorizedError
		if errors.As(err, &ua) && ua.APIError != nil {
			return ua.APIError.StatusCode, true
		}
		var fb *gitcode.ForbiddenError
		if errors.As(err, &fb) && fb.APIError != nil {
			return fb.APIError.StatusCode, true
		}
		var rl *gitcode.RateLimitError
		if errors.As(err, &rl) && rl.APIError != nil {
			return rl.APIError.StatusCode, true
		}
		var cf *gitcode.ConflictError
		if errors.As(err, &cf) && cf.APIError != nil {
			return cf.APIError.StatusCode, true
		}
		var vd *gitcode.ValidationError
		if errors.As(err, &vd) && vd.APIError != nil {
			return vd.APIError.StatusCode, true
		}
		var ae *gitcode.APIError
		if errors.As(err, &ae) {
			return ae.StatusCode, true
		}
		return 0, false
	})
}
