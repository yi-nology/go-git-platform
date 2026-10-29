package tencentcode

import (
	"bytes"
	"context"

	gongfeng "github.com/yi-nology/go-git-platform/backends/tencentcode/internal/gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// maxReleaseScanPages caps the list walk behind the by-tag release
// resolution, mirroring the backendutil.AllPages budget: the scan stops at
// the first empty page or the cap, whichever comes first.
const maxReleaseScanPages = 50

// ListTags implements provider.ReleaseManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListTags(ctx context.Context, owner, repo string) ([]*provider.TagInfo, error) {
	pid := owner + "/" + repo
	tags, err := backendutil.AllPages(func(page int) ([]*gongfeng.Tag, error) {
		list, _, err := p.client.Tags.ListTags(ctx, pid,
			&gongfeng.ListTagsOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListTags", err)
	}
	result := make([]*provider.TagInfo, 0, len(tags))
	for _, t := range tags {
		result = append(result, convertTag(t))
	}
	return result, nil
}

// ListReleases implements provider.ReleaseManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListReleases(ctx context.Context, owner, repo string) ([]*provider.ReleaseInfo, error) {
	pid := owner + "/" + repo
	releases, err := backendutil.AllPages(func(page int) ([]*gongfeng.Release, error) {
		list, _, err := p.client.Releases.ListReleases(ctx, pid,
			&gongfeng.ListReleasesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListReleases", err)
	}
	result := make([]*provider.ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		result = append(result, convertRelease(r))
	}
	return result, nil
}

// CreateRelease implements provider.ReleaseManager. Registration: the
// create endpoint carries only tag/title/description, so opts.Target and
// the draft/prerelease flags have no equivalent here (registered ignores).
func (p *Provider) CreateRelease(ctx context.Context, owner, repo string, opts provider.CreateReleaseOptions) (*provider.ReleaseInfo, error) {
	pid := owner + "/" + repo
	createOpts := &gongfeng.CreateReleaseOptions{
		Tag:         gongfeng.Ptr(opts.TagName),
		Title:       gongfeng.Ptr(opts.Title),
		Description: gongfeng.Ptr(opts.Body),
	}
	release, _, err := p.client.Releases.CreateRelease(ctx, pid, createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateRelease", err)
	}
	return convertRelease(release), nil
}

// findReleaseByTag resolves a release by its tag. 工蜂's release endpoints
// are ID-addressed natively and expose no by-tag fetch (registered detour),
// so tag-addressed lookups scan the release list. The scan stops at the
// page containing the match.
func (p *Provider) findReleaseByTag(ctx context.Context, op, owner, repo, tag string) (*gongfeng.Release, error) {
	pid := owner + "/" + repo
	var found *gongfeng.Release
	for page := 1; page <= maxReleaseScanPages && found == nil; page++ {
		list, _, err := p.client.Releases.ListReleases(ctx, pid,
			&gongfeng.ListReleasesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformTencentCode, op, err)
		}
		for _, r := range list {
			if r != nil && r.Tag == tag {
				found = r
				break
			}
		}
		if len(list) == 0 {
			break
		}
	}
	if found == nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, op, provider.ErrNotFound)
	}
	return found, nil
}

// GetReleaseByTag implements provider.ReleaseManager via the registered
// list-scan detour (see findReleaseByTag).
func (p *Provider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*provider.ReleaseInfo, error) {
	release, err := p.findReleaseByTag(ctx, "GetReleaseByTag", owner, repo, tag)
	if err != nil {
		return nil, err
	}
	return convertRelease(release), nil
}

// UpdateRelease implements provider.ReleaseManager via the registered
// list-scan detour: resolve the release ID by tag, then PUT. Registration:
// the platform's update surface accepts only a description, so
// UpdateReleaseOptions.Name, .Draft, and .Prerelease are ignored on this
// platform — only Body is carried. An update carrying nothing (Body nil,
// the only field forwarded) short-circuits to the GetReleaseByTag result
// instead of PUTting an empty update body.
func (p *Provider) UpdateRelease(ctx context.Context, owner, repo, tag string, opts provider.UpdateReleaseOptions) (*provider.ReleaseInfo, error) {
	if opts.Body == nil {
		return p.GetReleaseByTag(ctx, owner, repo, tag)
	}
	release, err := p.findReleaseByTag(ctx, "UpdateRelease", owner, repo, tag)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	updateOpts := &gongfeng.UpdateReleaseOptions{
		Description: gongfeng.Ptr(*opts.Body),
	}
	updated, _, err := p.client.Releases.UpdateRelease(ctx, pid, release.ID, updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "UpdateRelease", err)
	}
	return convertRelease(updated), nil
}

// DeleteRelease implements provider.ReleaseManager via the registered
// list-scan detour: resolve the release ID by tag, then DELETE.
func (p *Provider) DeleteRelease(ctx context.Context, owner, repo, tag string) error {
	release, err := p.findReleaseByTag(ctx, "DeleteRelease", owner, repo, tag)
	if err != nil {
		return err
	}
	pid := owner + "/" + repo
	_, err = p.client.Releases.DeleteRelease(ctx, pid, release.ID)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteRelease", err)
	}
	return nil
}

// GetArchive implements provider.ReleaseManager.
func (p *Provider) GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error) {
	pid := owner + "/" + repo
	var buf bytes.Buffer
	opts := &gongfeng.ArchiveOptions{}
	if ref != "" {
		opts.SHA = gongfeng.Ptr(ref)
	}
	_, err := p.client.Repositories.Archive(ctx, pid, &buf, opts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetArchive", err)
	}
	return buf.Bytes(), nil
}

var _ provider.ReleaseManager = (*Provider)(nil)
