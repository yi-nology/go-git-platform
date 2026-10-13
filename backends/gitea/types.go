package gitea

import (
	gitea "gitea.dev/sdk"

	"github.com/yi-nology/go-git-platform/provider"
)

func convertRelease(r *gitea.Release) *provider.ReleaseInfo {
	if r == nil {
		return nil
	}
	return &provider.ReleaseInfo{
		ID:          r.ID,
		TagName:     r.TagName,
		Title:       r.Title,
		Body:        r.Note,
		URL:         r.URL,
		Draft:       r.IsDraft,
		Prerelease:  r.IsPrerelease,
		CreatedAt:   r.CreatedAt,
		PublishedAt: r.PublishedAt,
	}
}

// parseTotalCount extracts the X-Total-Count header from a gitea response.
