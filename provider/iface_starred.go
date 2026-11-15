package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// StarredManager 列出认证用户 star 过的仓库。可选能力接口:使用前先看
// Provider.Capabilities().Starred(或类型断言)。
//
// 直接复用 PlatformRepo 而非再造摘要类型:下游导入/org 镜像场景只消费
// FullName/Name/Owner/CloneURL/Description/Archived/Fork/Stars/Language/
// Private 这些现成字段,再造一套会引入无谓的字段映射漂移。GitHub 专属
// (无跨平台抽象价值),故仅 github 后端实现。
type StarredManager interface {
	// ListStarred 分页列出认证用户 star 的仓库(GET /user/starred)。
	// page/perPage 语义与其他分页方法一致,非法值经 NormalizePageOpts 归一。
	ListStarred(ctx context.Context, page, perPage int) ([]*model.PlatformRepo, error)
}
