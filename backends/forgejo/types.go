package forgejo

import (
	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/provider"
)

func convertRelease(r *forgejo.Release) *provider.ReleaseInfo {
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

// parseTotalCount extracts the X-Total-Count header from a forgejo response.
