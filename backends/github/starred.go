package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/google/go-github/v92/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListStarred implements provider.StarredManager.
//
// GET /user/starred 有两种响应形状:Accept 带 star+json 时返回
// [{starred_at, repo:{...}}] 包裹形状,否则返回裸 repo 数组。SDK 的
// Activity.ListStarred 固定请求包裹形状——服务端一旦返回裸列表(代理改写
// Accept、GHES 版本差异),SDK 会把每项解成空 Repository。因此这里自行
// 构造请求并逐元素探测两种形状,保证兼容;Accept 仍请求 star+json,与
// 消费方(下游 org_mirror)的线上行为一致。
func (p *Provider) ListStarred(ctx context.Context, page, perPage int) ([]*provider.PlatformRepo, error) {
	page, perPage = provider.NormalizePageOpts(page, perPage)
	u := fmt.Sprintf("user/starred?page=%d&per_page=%d", page, perPage)
	req, err := p.client.NewRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListStarred", err)
	}
	req.Header.Set("Accept", "application/vnd.github.star+json")

	var entries []json.RawMessage
	if _, err := p.client.Do(req, &entries); err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListStarred", err)
	}
	result := make([]*provider.PlatformRepo, 0, len(entries))
	for _, e := range entries {
		repo, err := decodeStarredEntry(e)
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "ListStarred", err)
		}
		result = append(result, convertRepo(repo))
	}
	return result, nil
}

// decodeStarredEntry 先按包裹形状 {repo:{...}} 解码,解不出 repo 字段时
// 回退为裸 Repository——两种形状都归一到 *github.Repository,复用
// convertRepo 保证 Owner/Stars/Language/Archived/Fork 等元数据填全。
func decodeStarredEntry(entry json.RawMessage) (*github.Repository, error) {
	var wrapped struct {
		Repo *github.Repository `json:"repo"`
	}
	if err := json.Unmarshal(entry, &wrapped); err == nil && wrapped.Repo != nil {
		return wrapped.Repo, nil
	}
	var bare github.Repository
	if err := json.Unmarshal(entry, &bare); err != nil {
		return nil, oekiterrors.Wrap(err, "decode starred entry")
	}
	return &bare, nil
}

// compile-time guard
var _ provider.StarredManager = (*Provider)(nil)
