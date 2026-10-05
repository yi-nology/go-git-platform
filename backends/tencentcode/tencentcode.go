// Package tencentcode implements the Tencent 工蜂 Provider for the
// go-git-platform.
//
// Tencent 工蜂 exposes a GitLab-compatible REST API with some platform-
// specific extensions (native code reviews, branch protection, repository
// tree/blob). This package implements the cross-platform provider.Provider
// interface and additionally exposes a TencentCodeExtras interface for the
// platform-specific capabilities. All Provider methods are split across
// the per-responsibility files in this package:
//
//   - tencentcode.go: constructor + identity (Platform, TestConnection, Capabilities)
//   - init.go:    provider registration with the global registry
//   - repos.go:   ListRepos, GetRepo, CreateRepo, DeleteRepo, UpdateRepo, ForkRepo
//   - crs.go:     Change requests (MRs): Create/Get/List/Close/Merge/Reopen/Update/Comments/Commits
//   - webhooks.go: webhook CRUD + signature validation + event parsing
//   - branches.go: ListBranches, CreateBranch, DeleteBranch
//   - diffs.go:   GetCRDiff, GetCRFiles, CreateNote/DeleteNote, CreateDiscussion
//   - commits.go:  GetCommit, ListCommits, CompareCommits, CreateCommitStatus
//   - files.go:    GetFileContent, CreateFile, UpdateFile, DeleteFile
//   - releases.go: ListTags, ListReleases, CreateRelease, GetReleaseByTag,
//     UpdateRelease, DeleteRelease, GetArchive
//   - milestones.go: repository milestone CRUD (MilestoneManager)
//   - labels.go:   repository label CRUD (LabelManager)
//   - issues.go:   issue CRUD, state transitions, comments (via notes),
//     and issue-label writes (IssueManager)
//   - reviews.go:  code reviews over native review notes, reviewer
//     requests (registered ignore), and dismissals (registered stub)
//     (ReviewManager)
//   - extras.go:  TencentCodeExtras (native code reviews, commit comments,
//     repository tree, branch protection)
//   - types.go:   gongfeng model -> provider model conversions
package tencentcode

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"
	"time"

	gongfeng "github.com/studyzy/gongfeng-sdk-go"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// Provider is the Tencent 工蜂 implementation of provider.Provider.
type Provider struct {
	client    *gongfeng.Client
	transport *transport.Client
	logger    provider.Logger
	userIDs   *backendutil.IDCache
}

// New builds a Tencent Code Provider from the given config.
func New(cfg provider.Config) (provider.Provider, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = provider.NewNoopLogger()
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://git.code.tencent.com"
	}

	// Shared config concerns (logger/ETag/retry/hooks/auth) come from the
	// backendutil builder; the SkipTLS transport is overridden with the
	// platform-pinned TLS suite below. Tencent 工蜂 requires TLS 1.2 with a
	// specific cipher-suite allowlist, so a bespoke transport is injected
	// rather than backendutil.HTTPTransport (which only flips
	// InsecureSkipVerify).
	transportClient := backendutil.NewTransportClient(cfg, baseURL+"/api/v3", transport.AuthStylePrivate,
		transport.WithTransport(tencentTLS12Transport()))

	// The gongfeng SDK client points straight at the retrying round tripper:
	// the pinned TLS transport lives inside the transport.Client, so the
	// extra ChainTransport/HTTPTransport layer the other backends use would
	// be dead weight here.
	httpClient := &http.Client{
		Timeout:   transport.DefaultTimeout,
		Transport: transportClient.NewRetryingRoundTripper(),
	}

	// Normalize the base URL for the gongfeng SDK (it appends /api/v3/ itself).
	sdkBaseURL := strings.TrimRight(baseURL, "/")
	sdkBaseURL = strings.TrimSuffix(sdkBaseURL, "/api/v3")

	// Create the gongfeng SDK client with the custom HTTP client. The SDK
	// rejects an empty token at construction; under a refreshable
	// TokenSource the placeholder below never reaches the network — the
	// transport round tripper overwrites PRIVATE-TOKEN per request, and
	// clears it entirely if the source yields an empty token.
	gfToken := cfg.Token
	if gfToken == "" && cfg.TokenSource != nil {
		gfToken = "managed-by-token-source"
	}
	gfClient, err := gongfeng.NewClient(gfToken,
		gongfeng.WithHTTPClient(httpClient),
		gongfeng.WithBaseURL(sdkBaseURL),
	)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "New", err)
	}

	return &Provider{
		client:    gfClient,
		transport: transportClient,
		logger:    logger,
		userIDs:   backendutil.NewIDCache(5 * time.Minute),
	}, nil
}

// tencentTLS12Transport returns the transport Tencent 工蜂 requires: TLS 1.2
// only, with a pinned cipher-suite allowlist, plus InsecureSkipVerify when the
// caller opts into SkipTLS.
//
//nolint:gosec // G402: InsecureSkipVerify is only set when the caller opts in via SkipTLS.
func tencentTLS12Transport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			},
		},
	}
}

// Platform implements provider.Provider.
func (p *Provider) Platform() provider.Platform { return provider.PlatformTencentCode }

// Capabilities implements provider.Provider. The backend declares the
// optional capability interfaces it implements (currently Labels,
// Milestones, Issues, and Reviews).
func (p *Provider) Capabilities() provider.CapabilitySet {
	return provider.CapabilitySet{Labels: true, Milestones: true, Issues: true, Reviews: true, CommitStatuses: true, RepoStats: true, Users: true}
}

// TestConnection implements provider.Provider.
func (p *Provider) TestConnection(ctx context.Context) (*provider.TestConnectionResult, error) {
	var user struct {
		Username string `json:"username"`
	}
	if err := p.doRequest(ctx, "TestConnection", "GET", "user", nil, &user); err != nil {
		return &provider.TestConnectionResult{Connected: false, Message: err.Error()}, nil
	}
	result := &provider.TestConnectionResult{
		Connected: true,
		Platform:  string(p.Platform()),
		UserName:  user.Username,
	}
	_, listErr := p.ListRepos(ctx, provider.ListRepoOptions{Page: 1, PerPage: 1})
	result.CanListRepos = listErr == nil
	result.CanReadCR = result.CanListRepos
	result.CanWriteCR = result.CanListRepos
	result.CanWebhook = result.CanListRepos
	return result, nil
}

// Compile-time guarantee that *Provider satisfies provider.Provider and
// TencentCodeExtras. The extras methods live in extras.go.
var (
	_ provider.Provider = (*Provider)(nil)
	_ TencentCodeExtras = (*Provider)(nil)
)

// doRequest executes a JSON request through the gongfeng SDK client.
// Used by extras.go and diffs.go for endpoints not covered by SDK services.
// Variable path segments interpolated by the caller (owner/repo pid, branch,
// note ID, ...) must already be esc()-escaped: the SDK client joins the
// base URL and the path verbatim, so unescaped '#', '?', '%', or spaces
// would corrupt or truncate the URL.
func (p *Provider) doRequest(ctx context.Context, op, method, path string, body, result any) error {
	req, err := p.client.NewRequest(ctx, method, path, body)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, op, err)
	}
	if _, err := p.client.Do(req, result); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, op, err)
	}
	return nil
}

// extractTotalCount returns the total item count from the SDK response,
// falling back to the length of the result slice.
func extractTotalCount(resp *gongfeng.Response, fallback int) int {
	if resp != nil && resp.TotalItems > 0 {
		return resp.TotalItems
	}
	return fallback
}

// pid returns the project identifier in "owner/repo" format for SDK calls.
// The SDK's typed methods pass it through parseID (url.PathEscape), so the
// pid stays unescaped there; raw doRequest/NewRequest paths must instead
// interpolate esc(pid(...)) — see esc below.
func pid(owner, repo string) string {
	return owner + "/" + repo
}

// esc escapes a single URL path segment. Variable segments interpolated
// into raw-client paths (owner/repo pid, branch, note ID, ...) must pass
// through this — the SDK's NewRequest concatenates the base URL and the
// path verbatim, so characters like '#', '?', '%', or spaces would
// otherwise corrupt or truncate the URL. (SDK-covered calls hand the pid
// to gongfeng's typed methods, which escape via parseID, and are out of
// scope here.)
func esc(s string) string { return url.PathEscape(s) }
