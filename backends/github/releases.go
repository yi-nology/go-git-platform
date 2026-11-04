package github

import (
	"context"
	"io"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListTags implements provider.ReleaseManager. The provider interface
// exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListTags(ctx context.Context, owner, repo string) ([]*provider.TagInfo, error) {
	tags, err := backendutil.AllPages(func(page int) ([]*github.RepositoryTag, error) {
		list, _, err := p.client.Repositories.ListTags(ctx, owner, repo, &github.ListOptions{
			Page: page, PerPage: 100,
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListTags", err)
	}
	result := make([]*provider.TagInfo, 0, len(tags))
	for _, t := range tags {
		result = append(result, &provider.TagInfo{Name: t.GetName(), Commit: t.GetCommit().GetSHA()})
	}
	return result, nil
}

// ListReleases implements provider.ReleaseManager. The provider interface
// exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListReleases(ctx context.Context, owner, repo string) ([]*provider.ReleaseInfo, error) {
	releases, err := backendutil.AllPages(func(page int) ([]*github.RepositoryRelease, error) {
		list, _, err := p.client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{
			Page: page, PerPage: 100,
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListReleases", err)
	}
	result := make([]*provider.ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		result = append(result, convertRelease(r))
	}
	return result, nil
}

// CreateRelease implements provider.ReleaseManager.
func (p *Provider) CreateRelease(ctx context.Context, owner, repo string, opts provider.CreateReleaseOptions) (*provider.ReleaseInfo, error) {
	r, _, err := p.client.Repositories.CreateRelease(ctx, owner, repo, github.CreateReleaseRequest{
		TagName:         opts.TagName,
		TargetCommitish: new(opts.Target),
		Name:            new(opts.Title),
		Body:            new(opts.Body),
		Draft:           new(opts.Draft),
		Prerelease:      new(opts.Prerelease),
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateRelease", err)
	}
	return convertRelease(r), nil
}

// GetReleaseByTag implements provider.ReleaseManager. GitHub exposes a
// dedicated tag-addressed endpoint, so no id resolution is needed.
func (p *Provider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*provider.ReleaseInfo, error) {
	r, _, err := p.client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetReleaseByTag", err)
	}
	return convertRelease(r), nil
}

// UpdateRelease implements provider.ReleaseManager. The update endpoint is
// id-addressed, so the tag is resolved through the by-tag endpoint first
// (exact lookup, no list window). go-github's UpdateReleaseRequest carries
// pointer fields, and GitHub's PATCH leaves omitted fields
// unchanged, so nil options pass through untouched.
func (p *Provider) UpdateRelease(ctx context.Context, owner, repo, tag string, opts provider.UpdateReleaseOptions) (*provider.ReleaseInfo, error) {
	id, err := p.resolveReleaseID(ctx, "UpdateRelease", owner, repo, tag)
	if err != nil {
		return nil, err
	}
	r, _, err := p.client.Repositories.UpdateRelease(ctx, owner, repo, id, github.UpdateReleaseRequest{
		Name:       opts.Name,
		Body:       opts.Body,
		Draft:      opts.Draft,
		Prerelease: opts.Prerelease,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateRelease", err)
	}
	return convertRelease(r), nil
}

// DeleteRelease implements provider.ReleaseManager. The delete endpoint is
// id-addressed; the tag is resolved through the by-tag endpoint first.
func (p *Provider) DeleteRelease(ctx context.Context, owner, repo, tag string) error {
	id, err := p.resolveReleaseID(ctx, "DeleteRelease", owner, repo, tag)
	if err != nil {
		return err
	}
	if _, err := p.client.Repositories.DeleteRelease(ctx, owner, repo, id); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DeleteRelease", err)
	}
	return nil
}

// resolveReleaseID finds the numeric ID of the release addressed by tag.
// Unlike label resolution (name→id via paginated list scans), GitHub offers
// a dedicated single-release-by-tag endpoint, so the lookup is exact with
// no pagination window to bound; a miss returns a wrapped NotFound. op is
// the public operation the resolution serves; failures surface under that
// op rather than under this unexported helper's name.
func (p *Provider) resolveReleaseID(ctx context.Context, op, owner, repo, tag string) (int64, error) {
	r, _, err := p.client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return 0, provider.Wrap(provider.PlatformGitHub, op, err)
	}
	return r.GetID(), nil
}

// GetArchive implements provider.ReleaseManager.
func (p *Provider) GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error) {
	url, _, err := p.client.Repositories.GetArchiveLink(ctx, owner, repo, github.ArchiveFormat(format), &github.RepositoryContentGetOptions{Ref: ref}, 1)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetArchive", err)
	}
	resp, err := p.client.Client().Get(url.String())
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetArchive", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "GetArchive", err)
	}
	return data, nil
}

// compile-time guard
var _ provider.ReleaseManager = (*Provider)(nil)
