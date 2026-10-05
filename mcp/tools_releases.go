package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- releases: tags and releases ---

type listReleasesIn ownerRepo

type getReleaseIn struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Tag   string `json:"tag" jsonschema:"release tag name"`
}

type createReleaseIn struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	TagName    string `json:"tag_name" jsonschema:"tag to release"`
	Title      string `json:"title" jsonschema:"release title"`
	Body       string `json:"body,omitempty" jsonschema:"release notes"`
	Target     string `json:"target,omitempty" jsonschema:"commitish the tag points at; empty = default branch"`
	Draft      bool   `json:"draft,omitempty"`
	Prerelease bool   `json:"prerelease,omitempty"`
}

func registerReleases(s *mcp.Server, st *state) {
	add(s, st, "list_tags", "List tags", "List a repository's tags.", false, func(ctx context.Context, in ownerRepo) ([]*provider.TagInfo, error) {
		return st.p.ListTags(ctx, in.Owner, in.Repo)
	})

	add(s, st, "list_releases", "List releases", "List a repository's published releases.", false, func(ctx context.Context, in listReleasesIn) ([]*provider.ReleaseInfo, error) {
		return st.p.ListReleases(ctx, in.Owner, in.Repo)
	})

	add(s, st, "get_release", "Get release by tag", "Fetch one release addressed by its tag name.", false, func(ctx context.Context, in getReleaseIn) (*provider.ReleaseInfo, error) {
		return st.p.GetReleaseByTag(ctx, in.Owner, in.Repo, in.Tag)
	})

	add(s, st, "create_release", "Create release", "Publish a release for a tag.", true, func(ctx context.Context, in createReleaseIn) (*provider.ReleaseInfo, error) {
		return st.p.CreateRelease(ctx, in.Owner, in.Repo, provider.CreateReleaseOptions{
			TagName: in.TagName, Target: in.Target, Title: in.Title,
			Body: in.Body, Draft: in.Draft, Prerelease: in.Prerelease,
		})
	})
}
