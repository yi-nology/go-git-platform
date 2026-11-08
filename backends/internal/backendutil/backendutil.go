// Package backendutil holds the shared plumbing used by every platform backend
// under backends/. It exists to eliminate the copy-pasted HTTP-client, hook,
// retry, and base-URL construction that was previously duplicated (often
// byte-for-byte) across the seven backend packages.
//
// These helpers are intentionally internal: they are an implementation detail
// of the backends and must not become part of the SDK's public API.
package backendutil

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// ConvertHooks adapts provider.Hooks into transport.Hooks. The request-hook
// signatures differ across the two packages (provider.RequestHook returns a
// context; transport.RequestHook returns an error), so each request hook is
// wrapped to discard the returned context. Identical across all backends.
func ConvertHooks(h *provider.Hooks) *transport.Hooks {
	if h == nil {
		return nil
	}
	out := &transport.Hooks{}
	for _, rh := range h.Request {
		if rh == nil {
			continue
		}
		rhCopy := rh
		out.AddRequest(func(ctx context.Context, req *http.Request) error {
			_ = rhCopy(ctx, req)
			return nil
		})
	}
	for _, rh := range h.Response {
		if rh == nil {
			continue
		}
		rhCopy := rh
		out.AddResponse(func(ctx context.Context, req *http.Request, resp *http.Response, d time.Duration, err error) {
			rhCopy(ctx, req, resp, d, err)
		})
	}
	return out
}

// MapRetryConfig converts a provider.RetryConfig (which counts retries, i.e.
// attempts beyond the first) into a transport.RetryConfig (which counts total
// attempts), hence the +1. Returns nil for a nil input so callers can assign
// the result directly.
func MapRetryConfig(rc *provider.RetryConfig) *transport.RetryConfig {
	if rc == nil {
		return nil
	}
	return &transport.RetryConfig{
		MaxAttempts: rc.MaxRetries + 1,
		BaseDelay:   rc.BaseDelay,
		MaxDelay:    rc.MaxDelay,
		Statuses:    rc.RetryOn,
	}
}

// HTTPTransport returns an http.RoundTripper honouring the skipTLS flag: the
// default transport when verification is enabled, or a transport that disables
// certificate verification when the caller has explicitly opted into SkipTLS.
//
//nolint:gosec // G402: InsecureSkipVerify is only set when the caller opts in via SkipTLS.
func HTTPTransport(skipTLS bool) http.RoundTripper {
	if !skipTLS {
		return http.DefaultTransport
	}
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

// ChainTransport chains two round trippers. The outer tripper is invoked first;
// if it returns a non-nil response or a non-nil error that result is returned,
// otherwise the inner transport (the underlying connection) handles the
// request.
func ChainTransport(inner, outer http.RoundTripper) http.RoundTripper {
	return &chainedTransport{inner: inner, outer: outer}
}

type chainedTransport struct {
	inner http.RoundTripper
	outer http.RoundTripper
}

func (c *chainedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.outer != nil {
		if resp, err := c.outer.RoundTrip(req); resp != nil || err != nil {
			return resp, err
		}
	}
	return c.inner.RoundTrip(req)
}

// ToTransportLogger adapts a provider.Logger to a transport.Logger.
func ToTransportLogger(l provider.Logger) transport.Logger {
	if l == nil {
		return nil
	}
	return transport.LoggerFunc{
		DebugFunc: func(msg string, kv ...any) { l.Debug(msg, kv...) },
		InfoFunc:  func(msg string, kv ...any) { l.Info(msg, kv...) },
		WarnFunc:  func(msg string, kv ...any) { l.Warn(msg, kv...) },
		ErrorFunc: func(msg string, kv ...any) { l.Error(msg, kv...) },
	}
}

// NormalizeBaseURL returns base with a single trailing slash removed and a
// trailing "/api/v1" suffix removed. Used by Gitea/Forgejo-style backends
// whose SDKs add the version prefix themselves.
func NormalizeBaseURL(base string) string {
	base = strings.TrimSuffix(base, "/")
	base = strings.TrimSuffix(base, "/api/v1")
	return base
}

// DefaultBaseURL returns base if non-empty, otherwise def.
func DefaultBaseURL(base, def string) string {
	if base == "" {
		return def
	}
	return base
}

// --- Shared pagination constants ---

// IssueCommentPageSize is the per-page value for paginated issue-comment
// fetches across all backends.
const IssueCommentPageSize = 100

// GitlabDiffPageSize 是 GitLab diff 列表端点的 per_page 取值（issue #44）：
// REST 上限 100，显式拉满——服务端缺省 20 会把大 MR 静默截到 20 个文件。
const GitlabDiffPageSize = 100

// LabelPageSize is the per-page value for paginated label-list fetches
// across all backends.
const LabelPageSize = 100

// LabelScanMaxPages is the maximum number of pages to scan when resolving
// label names to IDs. At LabelPageSize items per page, this caps the scan
// at 5000 labels.
const LabelScanMaxPages = 50

// --- Shared number parsing ---

// ParseIssueNumber parses a string issue number into the platform's native
// integer type. The op and platform arguments are forwarded to the standard
// error wrapping so callers produce identical error messages.
func ParseIssueNumber(platform provider.Platform, op, number string) (int, error) {
	n, err := strconv.Atoi(number)
	if err != nil {
		return 0, provider.Wrapf(platform, op, "invalid issue number %q", number)
	}
	return n, nil
}

// ParseIssueNumber64 is like ParseIssueNumber but returns int64 for platforms
// whose SDK uses int64 IDs (Gitea, Forgejo, GitLab).
func ParseIssueNumber64(platform provider.Platform, op, number string) (int64, error) {
	n, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return 0, provider.Wrapf(platform, op, "invalid issue number %q", number)
	}
	return n, nil
}

// ParsePRNumber parses a string PR/MR number into int.
func ParsePRNumber(platform provider.Platform, op, number string) (int, error) {
	n, err := strconv.Atoi(number)
	if err != nil {
		return 0, provider.Wrapf(platform, op, "invalid pull request number %q", number)
	}
	return n, nil
}

// ParsePRNumber64 is like ParsePRNumber but returns int64.
func ParsePRNumber64(platform provider.Platform, op, number string) (int64, error) {
	n, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return 0, provider.Wrapf(platform, op, "invalid pull request number %q", number)
	}
	return n, nil
}

// ParseMilestoneNumber parses a string milestone number/ID into int64.
func ParseMilestoneNumber(platform provider.Platform, op, number string) (int64, error) {
	n, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return 0, provider.Wrapf(platform, op, "invalid milestone number %q", number)
	}
	return n, nil
}

// Esc is a shared path-escaping helper used by backends whose platform
// API paths contain owner/repo segments that may need URL encoding.
func Esc(s string) string { return url.PathEscape(s) }
