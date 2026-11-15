package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// FileManager handles file CRUD operations on repositories.
type FileManager interface {
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)
	CreateFile(ctx context.Context, owner, repo string, opts FileOptions) (*model.FileResult, error)
	UpdateFile(ctx context.Context, owner, repo string, opts FileOptions) (*model.FileResult, error)
	DeleteFile(ctx context.Context, owner, repo string, opts FileDeleteOptions) (*model.FileResult, error)
}
