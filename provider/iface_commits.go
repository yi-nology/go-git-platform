package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// CommitManager handles commit operations.
//
// Commit statuses are NOT part of CommitManager: they are a CI reporting
// concern that not every platform exposes (Gitee's public REST API has no
// commit-status endpoint). See the optional CommitStatusManager capability
// interface and CapabilitySet.CommitStatuses.
type CommitManager interface {
	GetCommit(ctx context.Context, owner, repo, sha string) (*model.CommitInfo, error)
	ListCommits(ctx context.Context, owner, repo string, opts ListCommitsOptions) ([]*model.CommitInfo, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string) (*model.CompareResult, error)
}
