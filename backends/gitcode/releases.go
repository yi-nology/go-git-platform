package gitcode

import (
	"context"

	oekiterrors "github.com/oekit/pkg/errors"
	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListTags implements provider.ReleaseManager. The endpoint has no
// caller-facing pagination knobs, so every page is fetched via AllPages
// (GitCode's page-size ceiling is 100) through the SDK's paginated
// ListTagsWithOptions rather than the hard-coded per_page=100 single-shot
// ListTags helper.
func (p *Provider) ListTags(ctx context.Context, owner, repo string) ([]*provider.TagInfo, error) {
	tags, err := backendutil.AllPages(func(page int) ([]*gitcode.Tag, error) {
		return p.client.ListTagsWithOptions(ctx, owner, repo, gitcode.ListOptions{Page: page, PerPage: 100})
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListTags", err)
	}
	result := make([]*provider.TagInfo, 0, len(tags))
	for _, t := range tags {
		// Tag.Commit is a value type in the SDK; no nil check needed.
		result = append(result, &provider.TagInfo{Name: t.Name, Commit: t.Commit.SHA})
	}
	return result, nil
}

// ListReleases implements provider.ReleaseManager. The SDK's ListReleases
// endpoint exposes no page parameter (it hard-codes per_page=100), so the
// call stays single-shot; GitCode's page-size ceiling bounds what one call
// can return.
func (p *Provider) ListReleases(ctx context.Context, owner, repo string) ([]*provider.ReleaseInfo, error) {
	releases, err := p.client.ListReleases(ctx, owner, repo)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListReleases", err)
	}
	result := make([]*provider.ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		result = append(result, convertRelease(r))
	}
	return result, nil
}

// CreateRelease implements provider.ReleaseManager.
func (p *Provider) CreateRelease(ctx context.Context, owner, repo string, opts provider.CreateReleaseOptions) (*provider.ReleaseInfo, error) {
	r, err := p.client.CreateRelease(ctx, owner, repo, gitcode.CreateReleaseOptions{
		TagName: opts.TagName, Target: opts.Target, Title: opts.Title,
		Body: opts.Body, Draft: opts.Draft, Prerelease: opts.Prerelease,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateRelease", err)
	}
	return convertRelease(r), nil
}

// GetReleaseByTag implements provider.ReleaseManager. GitCode exposes a
// dedicated tag-addressed endpoint.
func (p *Provider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*provider.ReleaseInfo, error) {
	r, err := p.client.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetReleaseByTag", err)
	}
	return convertRelease(r), nil
}

// UpdateRelease implements provider.ReleaseManager. The update endpoint is
// id-addressed, so the tag is resolved through the by-tag endpoint first
// (exact lookup, no list window). The SDK's UpdateReleaseOptions carries
// plain-string name/body fields that would clobber the release when sent
// empty, so the current values from the resolution fetch backfill every
// option the caller left nil; draft/prerelease are pointers that pass
// through untouched.
func (p *Provider) UpdateRelease(ctx context.Context, owner, repo, tag string, opts provider.UpdateReleaseOptions) (*provider.ReleaseInfo, error) {
	cur, err := p.client.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateRelease", err)
	}
	name, body := cur.Name, cur.Body
	if opts.Name != nil {
		name = *opts.Name
	}
	if opts.Body != nil {
		body = *opts.Body
	}
	r, err := p.client.UpdateRelease(ctx, owner, repo, cur.ID, gitcode.UpdateReleaseOptions{
		TagName:    cur.TagName,
		Name:       name,
		Body:       body,
		Draft:      opts.Draft,
		Prerelease: opts.Prerelease,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateRelease", err)
	}
	return convertRelease(r), nil
}

// DeleteRelease implements provider.ReleaseManager. The delete endpoint is
// id-addressed, so the tag is resolved through the by-tag endpoint first
// (exact lookup, no list window) and the release is deleted by ID. The
// SDK's tag-addressed DeleteRelease is deliberately NOT used: it falls
// back to deleting the tag itself on any release-delete error — transient
// 500/401/429 included — which would silently destroy the git tag; the
// release-object-only semantics every other platform honors rule that out.
func (p *Provider) DeleteRelease(ctx context.Context, owner, repo, tag string) error {
	cur, err := p.client.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteRelease", err)
	}
	if err := p.client.DeleteReleaseByID(ctx, owner, repo, cur.ID); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteRelease", err)
	}
	return nil
}

// GetArchive implements provider.ReleaseManager. GitCode serves no archive
// endpoint — the documented-shaped routes (/repos/{o}/{r}/archive,
// /archive/{ref.ext}, /archive/status) all 404 live (probe round
// 2026-11-03; see gitcode_api docs/wire-provenance.md). The method stays
// for ReleaseManager completeness and fails with ErrNotImplemented so
// callers can classify; use ListTags/CreateRelease instead.
func (p *Provider) GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error) {
	return nil, provider.Wrap(provider.PlatformGitCode, "GetArchive",
		oekiterrors.Wrap(provider.ErrNotImplemented, "GitCode serves no archive endpoint (live-verified 2026-11-03)"))
}

var _ provider.ReleaseManager = (*Provider)(nil)
