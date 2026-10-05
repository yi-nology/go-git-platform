// Package gitee implements the Gitee Provider for the go-git-platform.
//
// The backend builds on the hand-written go-gitee SDK
// (github.com/next-bin/go-gitee) which follows go-github patterns:
// pointer-based model types, functional options for client construction,
// per-service method groups, and automatic access_token injection via
// a tokenTransport round-tripper.
//
// All Provider methods are split across the per-responsibility files in
// this package:
//
//   - gitee.go:   constructor + identity (Platform, TestConnection, Capabilities)
//   - init.go:    provider registration with the global registry
//   - methods.go: esc/escPath path escaping helpers
//   - repos.go:   ListRepos, GetRepo, CreateRepo, DeleteRepo, UpdateRepo, ForkRepo
//   - crs.go:     Change requests (PRs): Create/Get/List/Merge/Close/Reopen/Update/UpdateLabels/Comments/Commits
//   - webhooks.go: webhook CRUD + signature validation + event parsing
//   - branches.go: ListBranches, CreateBranch, DeleteBranch
//   - diffs.go:   GetCRDiff, GetCRFiles, CreateNote/DeleteNote, CreateDiscussion
//   - commits.go: GetCommit, ListCommits, CompareCommits
//   - files.go:   GetFileContent, CreateFile, UpdateFile, DeleteFile
//   - releases.go: ListTags, ListReleases, CreateRelease, GetArchive
//   - labels.go:  repository label CRUD (LabelManager)
//   - issues.go:  issue CRUD, comments, and issue labels (IssueManager)
//   - milestones.go: repository milestone CRUD (MilestoneManager)
//   - search.go:    global repo/issue/user search (SearchManager)
//   - types.go:   SDK model conversions and deref helpers
package gitee

import (
	"context"
	"strings"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// Provider is the Gitee implementation of provider.Provider.
type Provider struct {
	client  *gitee.Client
	baseURL string
	logger  provider.Logger
}

// New builds a Gitee Provider from the given config.
func New(cfg provider.Config) (provider.Provider, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = provider.NewNoopLogger()
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://gitee.com/api/v5"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if !strings.Contains(baseURL, "/api/v5") {
		baseURL += "/api/v5"
	}
	// The new SDK expects a trailing slash on the base URL.
	sdkBaseURL := baseURL + "/"

	transportClient := backendutil.NewTransportClient(cfg, baseURL, transport.AuthStyleBearer)
	httpClient := backendutil.SDKHTTPClient(transportClient, cfg.SkipTLS)

	// Build the new SDK client with functional options. The tokenTransport
	// inside NewClient injects access_token as a query param automatically.
	opts := []gitee.ClientOptionsFunc{
		gitee.WithHTTPClient(httpClient),
		gitee.WithBaseURL(sdkBaseURL),
	}
	if cfg.Token != "" {
		opts = append(opts, gitee.WithToken(cfg.Token))
	}

	client, err := gitee.NewClient(opts...)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "New", err)
	}

	return &Provider{
		client:  client,
		baseURL: baseURL,
		logger:  logger,
	}, nil
}

// Platform implements provider.Provider.
func (p *Provider) Platform() provider.Platform { return provider.PlatformGitee }

// Capabilities implements provider.Provider.
func (p *Provider) Capabilities() provider.CapabilitySet {
	return provider.CapabilitySet{
		Labels:            true,
		Issues:            true,
		Milestones:        true,
		Search:            true,
		Notifications:     true,
		BranchProtections: true,
		Collaborators:     true,
		DeployKeys:        true,
		CommitStatuses:    true,
		RepoStats:         true,
	}
}

// TestConnection implements provider.Provider.
func (p *Provider) TestConnection(ctx context.Context) (*provider.TestConnectionResult, error) {
	user, _, err := p.client.Users.GetAuthenticated(ctx)
	if err != nil {
		return &provider.TestConnectionResult{Connected: false, Message: err.Error()}, nil
	}
	result := &provider.TestConnectionResult{
		Connected: true,
		Platform:  string(p.Platform()),
		UserName:  deref(user.Login),
	}
	_, err = p.ListRepos(ctx, provider.ListRepoOptions{Page: 1, PerPage: 1})
	result.CanListRepos = err == nil
	result.CanReadCR = result.CanListRepos
	result.CanWriteCR = result.CanListRepos
	result.CanWebhook = result.CanListRepos
	return result, nil
}
