package forgejo

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetCommit implements provider.CommitManager.
func (p *Provider) GetCommit(ctx context.Context, owner, repo, sha string) (*provider.CommitInfo, error) {
	return giteafamily.GetCommit(ctx, p.family, p.ports, owner, repo, sha)
}

// ListCommits implements provider.CommitManager.
func (p *Provider) ListCommits(ctx context.Context, owner, repo string, opts provider.ListCommitsOptions) ([]*provider.CommitInfo, error) {
	return giteafamily.ListCommits(ctx, p.family, p.ports, owner, repo, opts)
}

// CompareCommits implements provider.CommitManager.
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*provider.CompareResult, error) {
	return giteafamily.CompareCommits(ctx, p.family, p.ports, owner, repo, base, head)
}

// CreateCommitStatus implements provider.CommitStatusManager.
func (p *Provider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, opts provider.CommitStatusOptions) error {
	return giteafamily.CreateCommitStatus(ctx, p.family, p.ports, owner, repo, sha, opts)
}

// ListCommitStatuses implements provider.CommitStatusManager.
func (p *Provider) ListCommitStatuses(ctx context.Context, owner, repo, sha string) ([]provider.CommitStatus, error) {
	return giteafamily.ListCommitStatuses(ctx, p.family, p.ports, owner, repo, sha)
}

var (
	_ provider.CommitManager       = (*Provider)(nil)
	_ provider.CommitStatusManager = (*Provider)(nil)
)
