package gitlab

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListTags implements provider.ReleaseManager.
func (p *Provider) ListTags(ctx context.Context, owner, repo string) ([]*model.TagInfo, error) {
	tags, err := backendutil.AllPages(func(page int) ([]*gitlab.Tag, error) {
		list, _, err := p.client.Tags.ListTags(pidOf(owner, repo), &gitlab.ListTagsOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListTags", err)
	}
	result := make([]*model.TagInfo, 0, len(tags))
	for _, t := range tags {
		ti := &model.TagInfo{Name: t.Name}
		if t.Commit != nil {
			ti.Commit = t.Commit.ID
		}
		result = append(result, ti)
	}
	return result, nil
}

// ListReleases implements provider.ReleaseManager.
func (p *Provider) ListReleases(ctx context.Context, owner, repo string) ([]*model.ReleaseInfo, error) {
	releases, err := backendutil.AllPages(func(page int) ([]*gitlab.Release, error) {
		list, _, err := p.client.Releases.ListReleases(pidOf(owner, repo), &gitlab.ListReleasesOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListReleases", err)
	}
	result := make([]*model.ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		ri := &model.ReleaseInfo{
			TagName:   r.TagName,
			Title:     r.Name,
			Body:      r.Description,
			URL:       r.Links.Self,
			CreatedAt: timeOrZero(r.CreatedAt),
		}
		if r.ReleasedAt != nil {
			ri.PublishedAt = *r.ReleasedAt
		}
		result = append(result, ri)
	}
	return result, nil
}

// CreateRelease implements provider.ReleaseManager.
func (p *Provider) CreateRelease(ctx context.Context, owner, repo string, opts provider.CreateReleaseOptions) (*model.ReleaseInfo, error) {
	r, _, err := p.client.Releases.CreateRelease(pidOf(owner, repo), &gitlab.CreateReleaseOptions{
		TagName:     new(opts.TagName),
		Ref:         new(opts.Target),
		Name:        new(opts.Title),
		Description: new(opts.Body),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "CreateRelease", err)
	}
	ri := &model.ReleaseInfo{
		TagName:   r.TagName,
		Title:     r.Name,
		Body:      r.Description,
		URL:       r.Links.Self,
		CreatedAt: timeOrZero(r.CreatedAt),
	}
	if r.ReleasedAt != nil {
		ri.PublishedAt = *r.ReleasedAt
	}
	return ri, nil
}

// GetReleaseByTag implements provider.ReleaseManager. GitLab's release
// endpoints are tag-addressed natively.
func (p *Provider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*model.ReleaseInfo, error) {
	r, _, err := p.client.Releases.GetRelease(pidOf(owner, repo), tag, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "GetReleaseByTag", err)
	}
	return convertGitLabRelease(r), nil
}

// UpdateRelease implements provider.ReleaseManager via GitLab's
// tag-addressed update. Registration: GitLab releases have no draft or
// prerelease concepts (a release is created from a tag and published
// immediately), so UpdateReleaseOptions.Draft and .Prerelease are ignored
// on this platform — only Name and Body are carried.
func (p *Provider) UpdateRelease(ctx context.Context, owner, repo, tag string, opts provider.UpdateReleaseOptions) (*model.ReleaseInfo, error) {
	r, _, err := p.client.Releases.UpdateRelease(pidOf(owner, repo), tag, &gitlab.UpdateReleaseOptions{
		Name:        opts.Name,
		Description: opts.Body,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "UpdateRelease", err)
	}
	return convertGitLabRelease(r), nil
}

// DeleteRelease implements provider.ReleaseManager via GitLab's
// tag-addressed delete. The release's tag is kept; only the release object
// is removed.
func (p *Provider) DeleteRelease(ctx context.Context, owner, repo, tag string) error {
	_, _, err := p.client.Releases.DeleteRelease(pidOf(owner, repo), tag, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(model.PlatformGitLab, "DeleteRelease", err)
	}
	return nil
}

// convertGitLabRelease maps a gitlab.Release to a model.ReleaseInfo
// (description is the release body; released_at is the publish timestamp).
func convertGitLabRelease(r *gitlab.Release) *model.ReleaseInfo {
	if r == nil {
		return nil
	}
	ri := &model.ReleaseInfo{
		TagName:   r.TagName,
		Title:     r.Name,
		Body:      r.Description,
		URL:       r.Links.Self,
		CreatedAt: timeOrZero(r.CreatedAt),
	}
	if r.ReleasedAt != nil {
		ri.PublishedAt = *r.ReleasedAt
	}
	return ri
}

// GetArchive implements provider.ReleaseManager.
func (p *Provider) GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error) {
	fmtVal := "tar.gz"
	if format == "zip" {
		fmtVal = "zip"
	}
	data, _, err := p.client.Repositories.Archive(pidOf(owner, repo),
		&gitlab.ArchiveOptions{Format: new(fmtVal), SHA: new(ref)},
		gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "GetArchive", err)
	}
	return data, nil
}

var _ provider.ReleaseManager = (*Provider)(nil)
