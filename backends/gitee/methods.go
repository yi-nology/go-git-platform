package gitee

import (
	"net/url"
	"strings"
)

// esc escapes a single URL path segment. Variable segments interpolated into
// REST paths (owner, repo, branch, label name, sha, ...) must pass through
// this, since the SDK interpolates path parameters without escaping them.
// esc was a one-line alias for backendutil.Esc; use the shared helper.

// escPath escapes a multi-segment path (e.g. a file path), preserving the
// "/" separators: each segment is percent-encoded, the separators are not.
func escPath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
