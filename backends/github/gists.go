package github

import (
	"context"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListMyGists implements provider.GistManager. user 传空字符串时 go-github
// 走 GET /gists——已认证调用返回的是 token 用户自己的 gist 列表(而非全站
// 公开 gist),正是备份场景要的「token 用户可见列表」。
func (p *Provider) ListMyGists(ctx context.Context, page, perPage int) ([]*provider.Gist, error) {
	page, perPage = provider.NormalizePageOpts(page, perPage)
	gists, _, err := p.client.Gists.List(ctx, "", &github.GistListOptions{
		ListOptions: github.ListOptions{Page: page, PerPage: perPage},
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListMyGists", err)
	}
	result := make([]*provider.Gist, 0, len(gists))
	for _, g := range gists {
		result = append(result, convertGist(g))
	}
	return result, nil
}

// convertGist maps a go-github Gist to the provider-neutral type.
func convertGist(g *github.Gist) *provider.Gist {
	if g == nil {
		return nil
	}
	out := &provider.Gist{
		ID:          g.GetID(),
		Description: g.GetDescription(),
		Public:      g.GetPublic(),
		HTMLURL:     g.GetHTMLURL(),
		CreatedAt:   tsOrZero(g.GetCreatedAt()),
		UpdatedAt:   tsOrZero(g.GetUpdatedAt()),
	}
	if len(g.Files) > 0 {
		out.Files = make(map[string]provider.GistFile, len(g.Files))
		for name, f := range g.Files {
			out.Files[string(name)] = provider.GistFile{
				Filename: f.GetFilename(),
				Language: f.GetLanguage(),
				RawURL:   f.GetRawURL(),
				Size:     int64(f.GetSize()),
				Content:  f.GetContent(),
			}
		}
	}
	return out
}

// compile-time guard
var _ provider.GistManager = (*Provider)(nil)
