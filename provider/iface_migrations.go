package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// CreateMigrationOptions 发起归档迁移的选项。json tag 与 GitHub 迁移 API
// 请求体字段一致,便于后端直接序列化上送。
type CreateMigrationOptions struct {
	// LockRepositories 迁移期间锁定仓库(防止导出过程中数据变动)。
	LockRepositories bool `json:"lock_repositories"`
	// ExcludeMetadata 跳过 issues/PR 等元数据、只导出仓库内容。
	ExcludeMetadata bool `json:"exclude_metadata"`
}

// MigrationManager 提供平台原生的「一键全量归档导出」(GitHub Migrations
// API)。可选能力接口:使用前先看 Provider.Capabilities().Migrations
// (或类型断言)。迁移是异步的:CreateMigration 立即返回任务 ID,消费方
// 轮询 GetMigration 直到 state 进入终态。
type MigrationManager interface {
	// CreateMigration 发起迁移导出。org == "" 表示用户级迁移
	// (POST /user/migrations),非空表示组织级(POST /orgs/{org}/migrations)
	// ——同一语义贯穿本接口全部方法。
	CreateMigration(ctx context.Context, org string, opts CreateMigrationOptions) (*model.MigrationInfo, error)
	// GetMigration 查询迁移状态,org 语义与 CreateMigration 一致。
	GetMigration(ctx context.Context, org string, id int64) (*model.MigrationInfo, error)
}
