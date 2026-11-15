package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// RepoStatsManager provides read-only repository statistics: forks,
// stargazers, and contributors. It is an optional capability interface:
// consumers should gate on Provider.Capabilities().RepoStats (or
// type-assert) before use.
//
// Not every backend implements every method; backends that lack a
// specific method return ErrNotImplemented and register the gap in
// their divergence ledger. A backend declares RepoStats=true if it
// implements at least one method.
type RepoStatsManager interface {
	// ListForks returns all forks of the repository.
	ListForks(ctx context.Context, owner, repo string) ([]*model.PlatformRepo, error)
	// ListStargazers returns users who have starred the repository.
	ListStargazers(ctx context.Context, owner, repo string) ([]*model.CRUser, error)
	// ListContributors returns contributors ranked by commit count.
	ListContributors(ctx context.Context, owner, repo string) ([]*model.Contributor, error)
}
