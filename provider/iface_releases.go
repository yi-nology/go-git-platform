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
	// 非 2xx 响应、传输错误或响应体为空（实际写入 0 字节）时返回错误；
	// 空体错误链可经 errors.Is(err, ErrEmptyResponse) 判定（issue #164）。
	// 注意：合法的 0 字节附件同样命中该错误——需要放行的调用方可先按
	// ReleaseAsset.Size 判定资产本身是否为空。
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
