package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// DiffManager handles diff and discussion operations. Review operations
// (CreateReview and friends) live on the optional ReviewManager capability
// interface; DiffManager itself carries five methods. Change request
// numbers are strings (same addressing scheme as IssueManager); numeric
// platforms parse with strconv and fail with a wrapped "invalid pull
// request number" error.
type DiffManager interface {
	GetCRDiff(ctx context.Context, owner, repo, number string) (*model.MergeDiff, error)
	GetCRFiles(ctx context.Context, owner, repo, number string) ([]*model.ChangedFile, error)
	CreateNote(ctx context.Context, owner, repo, number, body string) (string, error)
	DeleteNote(ctx context.Context, owner, repo, number string, noteID string) error
	CreateDiscussion(ctx context.Context, owner, repo, number string, opts DiscussionOptions) (string, error)
}

// NoteManager provides targeted update and full pagination over CR (MR/PR)
// comments — an optional capability (v0.67.2). Semantics are anchored to the
// CR comment thread: on GitLab these hit the MergeRequest Notes API, because
// IssueManager's comment methods go through the Issues Notes API and always
// 404 for merge requests (MRs and issues live in separate iid namespaces).
// Platforms where an issue IS the PR (GitHub, Gitea, Gitee) are already
// covered by IssueManager and need not implement this interface.
type NoteManager interface {
	// UpdateNote rewrites the body of one CR comment.
	UpdateNote(ctx context.Context, owner, repo, number, noteID, body string) (*model.IssueComment, error)
	// ListNotes returns every comment on the CR, oldest first (all pages).
	ListNotes(ctx context.Context, owner, repo, number string) ([]*model.IssueComment, error)
}
