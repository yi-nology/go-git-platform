package github

import (
	"errors"
	"time"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// init registers GitHub's SDK error extractor: go-github's five status-
// carrying error types all expose the status through Response *http.Response
// (RedirectionError also carries StatusCode directly). Replaces the old
// reflection-over-fields fallback with explicit errors.As checks.
//
// The throttle classifier (issue #50) gives Wrap the authoritative verdict
// for the two SDK types that ARE throttling: *RateLimitError (primary quota
// exhausted, always 403) and *AbuseRateLimitError (secondary limit, also
// always 403). Both implement neither StatusCode() int nor the transport's
// rateLimitedMarker, so the bare 403 classification used to drop GitHub's
// most common throttle signal into ErrForbidden — callers could not back
// off, and RateLimitRecovery found no window. The classifier restores the
// verdict plus the recovery hints (rate reset / RetryAfter).
func init() {
	provider.RegisterStatusExtractor(provider.PlatformGitHub, githubStatusExtractor)
	provider.RegisterThrottleClassifier(provider.PlatformGitHub, githubThrottleClassifier)
}

// githubStatusExtractor reads the HTTP status out of go-github's five
// status-carrying error types via explicit errors.As checks.
func githubStatusExtractor(err error) (int, bool) {
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
}

// githubThrottleClassifier gives Wrap the authoritative throttle verdict for
// the two SDK types that ARE throttling, plus their recovery hints.
func githubThrottleClassifier(err error) (time.Duration, time.Time, bool) {
	var rl *github.RateLimitError
	if errors.As(err, &rl) {
		// Primary quota exhaustion: the Rate snapshot carries the reset
		// time the platform sent (zero when absent).
		return 0, rl.Rate.Reset.Time, true
	}
	var ab *github.AbuseRateLimitError
	if errors.As(err, &ab) {
		// Secondary limit: RetryAfter when the platform advised one.
		if ab.RetryAfter != nil && *ab.RetryAfter > 0 {
			return *ab.RetryAfter, time.Time{}, true
		}
		return 0, time.Time{}, true
	}
	return 0, time.Time{}, false
}
