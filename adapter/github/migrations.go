package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateMigration implements provider.MigrationManager. go-github 的
// MigrationService 只覆盖组织级端点(orgs/{org}/migrations 且请求体不含
// exclude_metadata),而消费方需要 user/org 两条路径共用同一请求体
// (lock_repositories + exclude_metadata),因此自行构造请求。
// org == "" → POST /user/migrations(用户级),非空 → POST /orgs/{org}/migrations。
func (p *Provider) CreateMigration(ctx context.Context, org string, opts provider.CreateMigrationOptions) (*model.MigrationInfo, error) {
	u := "user/migrations"
	if org != "" {
		u = fmt.Sprintf("orgs/%s/migrations", org)
	}
	req, err := p.client.NewRequest(ctx, http.MethodPost, u, opts)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "CreateMigration", err)
	}
	// 与消费方线上验证过的 Accept 保持一致(迁移 API 已 GA,无需 preview)。
	req.Header.Set("Accept", "application/vnd.github+json")

	var info model.MigrationInfo
	if _, err := p.client.Do(req, &info); err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "CreateMigration", err)
	}
	return &info, nil
}

// GetMigration implements provider.MigrationManager. org 语义与
// CreateMigration 一致:"" 走 GET /user/migrations/{id},非空走
// GET /orgs/{org}/migrations/{id}(消费方轮询到 exported/failed 终态)。
func (p *Provider) GetMigration(ctx context.Context, org string, id int64) (*model.MigrationInfo, error) {
	u := fmt.Sprintf("user/migrations/%d", id)
	if org != "" {
		u = fmt.Sprintf("orgs/%s/migrations/%d", org, id)
	}
	req, err := p.client.NewRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "GetMigration", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	var info model.MigrationInfo
	if _, err := p.client.Do(req, &info); err != nil {
		return nil, provider.Wrap(model.PlatformGitHub, "GetMigration", err)
	}
	return &info, nil
}

// compile-time guard
var _ provider.MigrationManager = (*Provider)(nil)
