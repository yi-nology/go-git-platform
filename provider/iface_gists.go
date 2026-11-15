package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// GistManager 提供认证用户自己的 gist 列表。可选能力接口:使用前先看
// Provider.Capabilities().Gists(或类型断言)。目前只读——下游备份场景
// 只需要分页列出,创建/修改 gist 不在统一接口范围内。
type GistManager interface {
	// ListMyGists 分页列出 token 用户可见的 gist(GET /gists:已认证调用
	// 返回当前用户的 gist 列表)。page/perPage 语义与其他分页方法一致,
	// 非法值经 NormalizePageOpts 归一。
	ListMyGists(ctx context.Context, page, perPage int) ([]*model.Gist, error)
}
