package giteafamily

import (
	"github.com/yi-nology/go-git-platform/provider"
)

// familyError folds the HTTP status the adapters thread out of the family
// SDK's *Response into the classified provider error. The family SDKs
// flatten every non-2xx into a plain-text error (statusCodeToErr), so the
// response status is the only classification source (issue #55): a 4xx/5xx
// status is attached to the error chain via provider.WrapStatusError, and
// provider.Wrap then resolves the sentinel (404→ErrNotFound, 429→
// ErrRateLimited, 403→ErrForbidden, 409→ErrConflict...) exactly like the
// SDKs that carry status on their error values. Ports that return no
// status keep the plain Wrap path (their adapters already wrap the status
// into the error chain themselves — see ports.go withStatus).
func familyError(f Family, op string, status int, err error) error {
	if err == nil {
		return nil
	}
	if status >= 400 {
		return provider.Wrap(f.Platform, op, provider.WrapStatusError(err, status))
	}
	return provider.Wrap(f.Platform, op, err)
}
