// Package github implements the GitHub Provider for the go-git-platform.
//
// It builds on top of the official google/go-github SDK and adds the
// transport-layer cross-cutting behavior (auth, retry, hooks, logging)
// provided by the parent project's transport package. All Provider methods
// are split across the per-responsibility files in this package:
//
//   - github.go:  constructor, registration, identity (Platform, TestConnection)
//   - repos.go:   ListRepos, GetRepo, CreateRepo, DeleteRepo, UpdateRepo, ForkRepo
//   - crs.go:     Change requests (PRs): Create/Get/List/Close/Merge/Reopen/Update/Comments/Commits
//   - webhooks.go: webhook CRUD + signature validation + event parsing
//   - branches.go: ListBranches, CreateBranch, DeleteBranch
//   - diffs.go:    GetCRDiff, GetCRFiles, CreateNote/DeleteNote, CreateDiscussion
//   - commits.go:  GetCommit, ListCommits, CompareCommits, CreateCommitStatus
//   - files.go:    GetFileContent, CreateFile, UpdateFile, DeleteFile
//   - releases.go: ListTags, ListReleases, CreateRelease, GetArchive
//   - labels.go:   repository label CRUD (LabelManager)
//   - issues.go:   issue CRUD, comments, and issue labels (IssueManager)
//   - reviews.go:  PR review list/get/create/dismiss/request-reviewers (ReviewManager)
//   - milestones.go: repository milestone CRUD (MilestoneManager)
//   - search.go:    global repo/issue/user search (SearchManager)
//   - gists.go:      ListMyGists, token 用户的 gist 列表 (GistManager)
//   - starred.go:    ListStarred, 认证用户 star 的仓库 (StarredManager)
//   - migrations.go: CreateMigration/GetMigration, user/org 归档导出 (MigrationManager)
//   - release_assets.go: DownloadReleaseAsset 流式下载附件 (ReleaseAssetManager)
//   - types.go:    internal GitHub-API types and conversion helpers
package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v92/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// Provider is the GitHub implementation of provider.Provider. It embeds the
// official go-github client and an auxiliary transport.Client used for
// endpoints not covered by the SDK (e.g. archive downloads).
type Provider struct {
	client *github.Client
	logger provider.Logger
	// followClient redirects to signed URLs (release-asset downloads). It
	// carries no credentials and must honour SkipTLS so GHES self-signed
	// deployments do not fail x509 on the second hop.
	followClient *http.Client
}

// New builds a GitHub Provider from the given config. It registers itself
// with provider.Register so provider.NewProvider(PlatformGitHub, ...) returns
// an instance of this Provider.
func New(cfg provider.Config) (provider.Provider, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = provider.NewNoopLogger()
	}

	transportClient := backendutil.NewTransportClient(cfg, backendutil.DefaultBaseURL(cfg.BaseURL, "https://api.github.com"), transport.AuthStyleBearer)
	httpClient := backendutil.SDKHTTPClient(transportClient, cfg.SkipTLS)

	var ghClient *github.Client
	if cfg.BaseURL == "" {
		c, err := github.NewClient(github.WithHTTPClient(httpClient))
		if err != nil {
			return nil, fmt.Errorf("github: failed to create client: %w", err)
		}
		ghClient = c
	} else {
		base := cfg.BaseURL
		// go-github v91's WithEnterpriseURLs rejects an empty upload URL (v72's
		// NewEnterpriseClient silently accepted it); the upload URL is unused
		// by this backend, so pass the API base for it.
		c, err := github.NewClient(github.WithHTTPClient(httpClient), github.WithEnterpriseURLs(base, base))
		if err != nil {
			return nil, fmt.Errorf("github: failed to create enterprise client for %s: %w", base, err)
		}
		ghClient = c
	}

	return &Provider{
		client: ghClient,
		logger: logger,
		// Bare client on purpose (signed URLs must not see the API token),
		// but built on the same TLS policy as everything else so SkipTLS
		// holds on the redirect hop too.
		followClient: &http.Client{Transport: backendutil.HTTPTransport(cfg.SkipTLS)},
	}, nil
}

// Platform implements provider.Provider.
func (p *Provider) Platform() provider.Platform { return provider.PlatformGitHub }

// Capabilities implements provider.Provider. GitHub implements the optional
// LabelManager interface (see labels.go), the IssueManager interface
// (see issues.go), the ReviewManager interface (see reviews.go), the
// SearchManager interface (see search.go), and the GitHub-only capabilities
// GistManager (gists.go), StarredManager (starred.go), MigrationManager
// (migrations.go), and ReleaseAssetManager (release_assets.go).
func (p *Provider) Capabilities() provider.CapabilitySet {
	return provider.CapabilitySet{Labels: true, Issues: true, Reviews: true, Milestones: true, Search: true, CommitStatuses: true, Notifications: true, Reactions: true, BranchProtections: true, Collaborators: true, DeployKeys: true, RepoStats: true, Gists: true, Starred: true, Migrations: true, ReleaseAssets: true}
}

// TestConnection implements provider.Provider.
func (p *Provider) TestConnection(ctx context.Context) (*provider.TestConnectionResult, error) {
	user, _, err := p.client.Users.Get(ctx, "")
	if err != nil {
		return &provider.TestConnectionResult{Connected: false, Message: err.Error()}, nil
	}
	result := &provider.TestConnectionResult{
		Connected: true,
		Platform:  string(p.Platform()),
		UserName:  user.GetLogin(),
	}
	_, err = p.ListRepos(ctx, provider.ListRepoOptions{Page: 1, PerPage: 1})
	result.CanListRepos = err == nil
	result.CanReadCR = result.CanListRepos
	result.CanWriteCR = result.CanListRepos
	result.CanWebhook = result.CanListRepos
	return result, nil
}
