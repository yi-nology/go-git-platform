package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// FileManager handles file CRUD operations on repositories.
//
// Content crosses this seam as plaintext: GetFileContent returns the decoded
// bytes and FileOptions.Content is what should land in the repository.
// Adapters apply platform wire encodings internally — the gitea/forgejo
// family base64-encodes content at its port boundary because both SDKs
// require it, GitHub's SDK base64-encodes []byte, and GitLab takes the
// string verbatim. A caller writes one code path and never branches on the
// platform's encoding (issue #169).
type FileManager interface {
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)
	CreateFile(ctx context.Context, owner, repo string, opts FileOptions) (*model.FileResult, error)
	UpdateFile(ctx context.Context, owner, repo string, opts FileOptions) (*model.FileResult, error)
	DeleteFile(ctx context.Context, owner, repo string, opts FileDeleteOptions) (*model.FileResult, error)
}
