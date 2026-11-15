package provider

import (
	"context"
	"io"

	"github.com/yi-nology/go-git-platform/model"
)

// ReleaseAssetManager 下载 Release 附件。可选能力接口:使用前先看
// Provider.Capabilities().ReleaseAssets(或类型断言)。附件可能很大,
// 因此是流式接口而非返回 []byte,避免整读内存。
type ReleaseAssetManager interface {
	// DownloadReleaseAsset 下载 assetID 对应附件并流式写入 w。
	// 非 2xx 响应或无响应体时返回错误。
	DownloadReleaseAsset(ctx context.Context, owner, repo string, assetID int64, w io.Writer) error
}

// ReleaseManager handles tags, releases, and archives. Releases are
// addressed by tag name across every method: tag names are stable and
// human-addressable, while the underlying numeric release IDs are not.
type ReleaseManager interface {
	ListTags(ctx context.Context, owner, repo string) ([]*model.TagInfo, error)
	ListReleases(ctx context.Context, owner, repo string) ([]*model.ReleaseInfo, error)
	CreateRelease(ctx context.Context, owner, repo string, opts CreateReleaseOptions) (*model.ReleaseInfo, error)
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*model.ReleaseInfo, error)
	UpdateRelease(ctx context.Context, owner, repo, tag string, opts UpdateReleaseOptions) (*model.ReleaseInfo, error)
	DeleteRelease(ctx context.Context, owner, repo, tag string) error
	GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error)
}
