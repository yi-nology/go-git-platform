package gitee

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListTags implements provider.ReleaseManager.
func (p *Provider) ListTags(ctx context.Context, owner, repo string) ([]*provider.TagInfo, error) {
	tags, err := backendutil.AllPages(func(page int) ([]*gitee.Tag, error) {
		opts := &gitee.ListTagsOptions{
			Page:    gitee.Int(page),
			PerPage: gitee.Int(backendutil.IssueCommentPageSize),
		}
		batch, _, err := p.client.Repositories.ListTags(ctx, esc(owner), esc(repo), opts)
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListTags", err)
	}
	result := make([]*provider.TagInfo, 0, len(tags))
	for _, t := range tags {
		result = append(result, convertTag(t))
	}
	return result, nil
}

// ListReleases implements provider.ReleaseManager.
func (p *Provider) ListReleases(ctx context.Context, owner, repo string) ([]*provider.ReleaseInfo, error) {
	releases, err := backendutil.AllPages(func(page int) ([]*gitee.Release, error) {
		opts := &gitee.ListReleasesOptions{
			Page:    gitee.Int(page),
			PerPage: gitee.Int(backendutil.IssueCommentPageSize),
		}
		batch, _, err := p.client.Repositories.ListReleases(ctx, esc(owner), esc(repo), opts)
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "ListReleases", err)
	}
	result := make([]*provider.ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		result = append(result, convertRelease(r))
	}
	return result, nil
}

// CreateRelease implements provider.ReleaseManager.
func (p *Provider) CreateRelease(ctx context.Context, owner, repo string, opts provider.CreateReleaseOptions) (*provider.ReleaseInfo, error) {
	createOpts := &gitee.CreateReleaseOptions{
		TagName: gitee.String(opts.TagName),
		Name:    gitee.String(opts.Title),
	}
	if opts.Target != "" {
		createOpts.TargetCommitish = gitee.String(opts.Target)
	}
	if opts.Body != "" {
		createOpts.Body = gitee.String(opts.Body)
	}
	if opts.Prerelease {
		createOpts.Prerelease = gitee.Bool(true)
	}
	r, _, err := p.client.Repositories.CreateRelease(ctx, esc(owner), esc(repo), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateRelease", err)
	}
	return convertRelease(r), nil
}

// GetReleaseByTag implements provider.ReleaseManager.
func (p *Provider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*provider.ReleaseInfo, error) {
	r, _, err := p.client.Repositories.GetReleaseByTag(ctx, esc(owner), esc(repo), esc(tag))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetReleaseByTag", err)
	}
	return convertRelease(r), nil
}

// UpdateRelease implements provider.ReleaseManager.
func (p *Provider) UpdateRelease(ctx context.Context, owner, repo, tag string, opts provider.UpdateReleaseOptions) (*provider.ReleaseInfo, error) {
	// Resolve the release ID by tag first.
	r, _, err := p.client.Repositories.GetReleaseByTag(ctx, esc(owner), esc(repo), esc(tag))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "UpdateRelease", err)
	}
	updateOpts := &gitee.UpdateReleaseOptions{}
	if opts.Name != nil {
		updateOpts.Name = gitee.String(*opts.Name)
	}
	if opts.Body != nil {
		updateOpts.Body = gitee.String(*opts.Body)
	}
	if opts.Prerelease != nil {
		updateOpts.Prerelease = gitee.Bool(*opts.Prerelease)
	}
	updated, _, err := p.client.Repositories.UpdateRelease(ctx, esc(owner), esc(repo), int64(deref(r.ID)), updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "UpdateRelease", err)
	}
	return convertRelease(updated), nil
}

// DeleteRelease implements provider.ReleaseManager.
func (p *Provider) DeleteRelease(ctx context.Context, owner, repo, tag string) error {
	// Resolve the release ID by tag first.
	r, _, err := p.client.Repositories.GetReleaseByTag(ctx, esc(owner), esc(repo), esc(tag))
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "DeleteRelease", err)
	}
	_, err = p.client.Repositories.DeleteRelease(ctx, esc(owner), esc(repo), int64(deref(r.ID)))
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "DeleteRelease", err)
	}
	return nil
}

// GetArchive implements provider.ReleaseManager.
//
// **不能走 SDK 的 DownloadTarball/DownloadZipball**:go-gitee 的 Client.Do
// 内部是 `defer resp.Body.Close()` ⇒ 它返回时 body **已经关闭**,
// 之后再读只会拿到 "http: read on closed response body"
// ⇒ 归档下载**必然失败**(不是偶发,是 100% 失败)。
//
// 这里改用 SDK 同一个 http.Client 直接发请求 —— 认证、重试、重定向策略
// 与 SDK 完全一致(同一个 client、同一个 Transport 链),区别只是
// body 的生命周期由本方法自己持有。
func (p *Provider) GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error) {
	kind := "zipball"
	if format == "tar.gz" {
		kind = "tarball"
	}
	u := p.baseURL + "/repos/" + esc(owner) + "/" + esc(repo) + "/" + kind
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetArchive", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetArchive", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, provider.Wrap(provider.PlatformGitee, "GetArchive",
			fmt.Errorf("gitee archive: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail))))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetArchive", err)
	}
	return data, nil
}

var _ provider.ReleaseManager = (*Provider)(nil)
