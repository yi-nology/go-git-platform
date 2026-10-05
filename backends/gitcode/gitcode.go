// Package gitcode implements the GitCode Provider for the go-git-platform.
//
// It builds on top of the yi-nology/go-gitcode client SDK and adds
// transport-layer cross-cutting behavior (auth, retry, hooks, logging)
// provided by the parent project's transport package. All Provider methods
// are split across the per-responsibility files in this package:
//
//   - gitcode.go:  constructor + identity (Platform, TestConnection, Capabilities)
//   - init.go:     provider registration with the global registry
//   - repos.go:    ListRepos, GetRepo, CreateRepo, DeleteRepo, UpdateRepo, ForkRepo
//   - crs.go:      Change requests (PRs): Create/Get/List/Merge/Close/Reopen/Update/UpdateLabels/Comments/Commits
//   - webhooks.go: webhook CRUD + signature validation + event parsing
//   - branches.go: ListBranches, CreateBranch, DeleteBranch
//   - diffs.go:    GetCRDiff, GetCRFiles, CreateNote/DeleteNote, CreateDiscussion
//   - commits.go:  GetCommit, ListCommits, CompareCommits, CreateCommitStatus
//   - files.go:    GetFileContent, CreateFile, UpdateFile, DeleteFile
//   - releases.go: ListTags, ListReleases, CreateRelease, GetArchive
//   - issues.go:   IssueManager: issue CRUD, comments, issue-scoped label ops
//   - search.go:   SearchManager: SearchRepos, SearchIssues, SearchUsers
//   - labels.go:   repository label CRUD (LabelManager)
//   - reviews.go:  ReviewManager: list/get/create/request/dismiss PR reviews
//   - milestones.go: repository milestone CRUD (MilestoneManager)
package gitcode

import (
	"context"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// Provider is the GitCode implementation of provider.Provider.
type Provider struct {
	client *gitcode.Client
	// rawClient serves the registered raw detours — surfaces where the
	// SDK's option types cannot express the honest wire body (currently
	// milestone create/update; see milestones.go).
	rawClient *transport.Client
	logger    provider.Logger
}

// New builds a GitCode Provider from the given config.
func New(cfg provider.Config) (provider.Provider, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = provider.NewNoopLogger()
	}

	baseURL := backendutil.DefaultBaseURL(cfg.BaseURL, "https://api.gitcode.com/api/v5")
	transportClient := backendutil.NewTransportClient(cfg, baseURL, transport.AuthStyleBearer)
	httpClient := backendutil.SDKHTTPClient(transportClient, cfg.SkipTLS)

	client := gitcode.NewClientWithBaseURL(baseURL, cfg.Token)
	client.SetHTTPClient(httpClient)

	return &Provider{client: client, rawClient: transportClient, logger: logger}, nil
}

// Platform implements provider.Provider.
func (p *Provider) Platform() provider.Platform { return provider.PlatformGitCode }

// Capabilities implements provider.Provider. GitCode implements the optional
// IssueManager, SearchManager, LabelManager, ReviewManager, and
// MilestoneManager interfaces (see issues.go, search.go, labels.go,
// reviews.go, milestones.go).
func (p *Provider) Capabilities() provider.CapabilitySet {
	return provider.CapabilitySet{Issues: true, Search: true, Labels: true, Reviews: true, Milestones: true, CommitStatuses: true, Notifications: true, Reactions: true, BranchProtections: true, Collaborators: true, DeployKeys: true, RepoStats: true}
}

// TestConnection implements provider.Provider.
func (p *Provider) TestConnection(ctx context.Context) (*provider.TestConnectionResult, error) {
	user, err := p.client.GetCurrentUser(ctx)
	if err != nil {
		return &provider.TestConnectionResult{Connected: false, Message: err.Error()}, nil
	}
	result := &provider.TestConnectionResult{
		Connected: true,
		Platform:  string(p.Platform()),
		UserName:  user.Login,
	}
	_, err = p.client.ListRepositories(ctx, gitcode.ListRepositoriesOptions{
		ListOptions: gitcode.ListOptions{Page: 1, PerPage: 1},
	})
	result.CanListRepos = err == nil
	result.CanReadCR = result.CanListRepos
	result.CanWriteCR = result.CanListRepos
	result.CanWebhook = result.CanListRepos
	return result, nil
}
