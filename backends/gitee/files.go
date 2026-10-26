package gitee

import (
	"context"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/provider"
)

// GetFileContent implements provider.FileManager.
func (p *Provider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	opts := &gitee.GetContentOptions{}
	if ref != "" {
		opts.Ref = gitee.String(ref)
	}
	contents, _, err := p.client.Repositories.GetContents(ctx, esc(owner), esc(repo), escPath(path), opts)
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitee, "GetFileContent", err)
	}
	if len(contents) == 0 {
		return "", provider.Wrapf(provider.PlatformGitee, "GetFileContent", "no content returned for %s", path)
	}
	// **必须按 Encoding 解码**:Gitee 的内容端点返回 base64,
	// 以前原样返回 ⇒ 调用方拿到的是 base64 串(读文件全错)。
	return provider.DecodeFileContent(provider.PlatformGitee, deref(contents[0].Encoding), deref(contents[0].Content))
}

// CreateFile implements provider.FileManager.
func (p *Provider) CreateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	createOpts := &gitee.CreateContentOptions{
		Content: gitee.String(opts.Content),
		Message: gitee.String(opts.Message),
	}
	if opts.Branch != "" {
		createOpts.Branch = gitee.String(opts.Branch)
	}
	if opts.Author != "" {
		createOpts.AuthorName = gitee.String(opts.Author)
	}
	if opts.Email != "" {
		createOpts.AuthorEmail = gitee.String(opts.Email)
	}
	cc, _, err := p.client.Repositories.CreateFile(ctx, esc(owner), esc(repo), escPath(opts.Path), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateFile", err)
	}
	result := &provider.FileResult{}
	if cc.Content != nil {
		result.SHA = deref(cc.Content.Sha)
	}
	if cc.Commit != nil {
		result.CommitSHA = deref(cc.Commit.Sha)
	}
	return result, nil
}

// UpdateFile implements provider.FileManager.
func (p *Provider) UpdateFile(ctx context.Context, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	updateOpts := &gitee.UpdateContentOptions{
		Content: gitee.String(opts.Content),
		Message: gitee.String(opts.Message),
	}
	if opts.SHA != "" {
		updateOpts.SHA = gitee.String(opts.SHA)
	}
	if opts.Branch != "" {
		updateOpts.Branch = gitee.String(opts.Branch)
	}
	if opts.Author != "" {
		updateOpts.AuthorName = gitee.String(opts.Author)
	}
	if opts.Email != "" {
		updateOpts.AuthorEmail = gitee.String(opts.Email)
	}
	cc, _, err := p.client.Repositories.UpdateFile(ctx, esc(owner), esc(repo), escPath(opts.Path), updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "UpdateFile", err)
	}
	result := &provider.FileResult{}
	if cc.Content != nil {
		result.SHA = deref(cc.Content.Sha)
	}
	if cc.Commit != nil {
		result.CommitSHA = deref(cc.Commit.Sha)
	}
	return result, nil
}

// DeleteFile implements provider.FileManager.
func (p *Provider) DeleteFile(ctx context.Context, owner, repo string, opts provider.FileDeleteOptions) (*provider.FileResult, error) {
	deleteOpts := &gitee.DeleteContentOptions{
		Message: gitee.String(opts.Message),
	}
	if opts.SHA != "" {
		deleteOpts.SHA = gitee.String(opts.SHA)
	}
	if opts.Branch != "" {
		deleteOpts.Branch = gitee.String(opts.Branch)
	}
	if opts.Author != "" {
		deleteOpts.AuthorName = gitee.String(opts.Author)
	}
	if opts.Email != "" {
		deleteOpts.AuthorEmail = gitee.String(opts.Email)
	}
	cc, _, err := p.client.Repositories.DeleteFile(ctx, esc(owner), esc(repo), escPath(opts.Path), deleteOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "DeleteFile", err)
	}
	result := &provider.FileResult{}
	if cc.Commit != nil {
		result.CommitSHA = deref(cc.Commit.Sha)
	}
	return result, nil
}

var _ provider.FileManager = (*Provider)(nil)
