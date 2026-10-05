// Package gitlab implements the GitLab Provider for the go-git-platform.
//
// It builds on top of the official gitlab-org/api/client-go SDK and adds
// transport-layer cross-cutting behavior (auth, retry, hooks, logging)
// provided by the parent project's transport package. All Provider methods
// are split across the per-responsibility files in this package:
//
//   - gitlab.go:  constructor + identity (Platform, TestConnection, Capabilities)
//   - init.go:    provider registration with the global registry
//   - repos.go:   ListRepos, GetRepo, CreateRepo, DeleteRepo, UpdateRepo, ForkRepo
//   - crs.go:     Change requests (MRs): Create/Get/List/Close/Merge/Reopen/Update/Comments/Commits
//   - webhooks.go: webhook CRUD + signature validation + event parsing
//   - branches.go: ListBranches, CreateBranch, DeleteBranch
//   - diffs.go:    GetCRDiff, GetCRFiles, CreateNote/DeleteNote, CreateDiscussion
//   - commits.go:  GetCommit, ListCommits, CompareCommits, CreateCommitStatus
//   - files.go:    GetFileContent, CreateFile, UpdateFile, DeleteFile
//   - releases.go: ListTags, ListReleases, CreateRelease, GetArchive
//   - labels.go:   repository label CRUD (LabelManager)
//   - issues.go:   issue CRUD, comments (notes), and issue labels (IssueManager)
//   - reviews.go:  code reviews via approvals mappings (ReviewManager)
//   - milestones.go: repository milestone CRUD (MilestoneManager)
//   - search.go:    global repo/issue/user search (SearchManager)
//   - types.go:    internal GitLab-API types and conversion helpers
package gitlab

import (
	"context"
	"fmt"

	"time"

	"golang.org/x/oauth2"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// Provider is the GitLab implementation of provider.Provider.
type Provider struct {
	client   *gitlab.Client
	logger   provider.Logger
	labelIDs *backendutil.IDCache
	userIDs  *backendutil.IDCache
}

// New builds a GitLab Provider from the given config.
func New(cfg provider.Config) (provider.Provider, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = provider.NewNoopLogger()
	}

	// Select auth strategy: "bearer" → OAuth Bearer, otherwise → PRIVATE-TOKEN (GitLab default).
	style := transport.AuthStylePrivate
	if cfg.TokenStyle == "bearer" {
		style = transport.AuthStyleBearer
	}
	transportClient := backendutil.NewTransportClient(cfg, backendutil.DefaultBaseURL(cfg.BaseURL, "https://gitlab.com/api/v4"), style)
	httpClient := backendutil.SDKHTTPClient(transportClient, cfg.SkipTLS)

	opts := []gitlab.ClientOptionFunc{gitlab.WithHTTPClient(httpClient)}
	if cfg.BaseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(cfg.BaseURL))
	}
	var client *gitlab.Client
	var err error
	if cfg.TokenStyle == "bearer" {
		// Bearer auth (e.g. GitLab CI_JOB_TOKEN). The deprecated
		// NewOAuthClient is replaced by NewAuthSourceClient per the
		// client-go v2.60 guidance.
		var ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.Token})
		if cfg.TokenSource != nil {
			ts = providerTokenSource{src: cfg.TokenSource}
		}
		client, err = gitlab.NewAuthSourceClient(gitlab.OAuthTokenSource{TokenSource: ts}, opts...)
	} else {
		// With a refreshable TokenSource the static token may be empty;
		// the transport round tripper injects the (fresh) PRIVATE-TOKEN
		// header on every request, overwriting whatever the SDK set.
		client, err = gitlab.NewClient(cfg.Token, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("gitlab: failed to create client: %w", err)
	}
	return &Provider{
		client:   client,
		logger:   logger,
		labelIDs: backendutil.NewIDCache(5 * time.Minute),
		userIDs:  backendutil.NewIDCache(5 * time.Minute),
	}, nil
}

// providerTokenSource adapts provider.TokenSource to the context-free
// oauth2.TokenSource shape gitlab's OAuthTokenSource expects. Refresh
// coordination loses the request context on this legacy interface; the
// transport-layer auth still refreshes with full context on every request
// and its header wins, so this adapter only feeds the SDK's own header
// fallback.
type providerTokenSource struct{ src provider.TokenSource }

func (p providerTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token(context.Background())
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{AccessToken: tok}, nil
}

// Platform implements provider.Provider.
func (p *Provider) Platform() provider.Platform { return provider.PlatformGitLab }

// Capabilities implements provider.Provider. GitLab implements the optional
// LabelManager (see labels.go), IssueManager (see issues.go),
// ReviewManager (see reviews.go), and SearchManager (see search.go)
// interfaces.
func (p *Provider) Capabilities() provider.CapabilitySet {
	return provider.CapabilitySet{Labels: true, Issues: true, Reviews: true, Milestones: true, Search: true, CommitStatuses: true, Notifications: true, Reactions: true, BranchProtections: true, DeployKeys: true, RepoStats: true, Users: true}
}

// TestConnection implements provider.Provider.
func (p *Provider) TestConnection(ctx context.Context) (*provider.TestConnectionResult, error) {
	user, _, err := p.client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return &provider.TestConnectionResult{Connected: false, Message: err.Error()}, nil
	}
	result := &provider.TestConnectionResult{
		Connected: true,
		Platform:  string(p.Platform()),
		UserName:  user.Username,
	}
	_, err = p.ListRepos(ctx, provider.ListRepoOptions{Page: 1, PerPage: 1})
	result.CanListRepos = err == nil
	result.CanReadCR = result.CanListRepos
	result.CanWriteCR = result.CanListRepos
	result.CanWebhook = result.CanListRepos
	return result, nil
}
