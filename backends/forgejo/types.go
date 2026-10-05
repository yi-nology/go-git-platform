package forgejo

import (
	"strconv"
	"time"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/provider"
)

func convertRepo(r *forgejo.Repository) *provider.PlatformRepo {
	if r == nil {
		return nil
	}
	owner := ""
	if r.Owner != nil {
		owner = r.Owner.UserName
	}
	return &provider.PlatformRepo{
		ID:            r.ID,
		FullName:      r.FullName,
		Name:          r.Name,
		Owner:         owner,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Archived:      r.Archived,
		Fork:          r.Fork,
		Stars:         r.Stars,
		Platform:      provider.PlatformForgejo,
	}
}

func convertPR(pr *forgejo.PullRequest) *provider.ChangeRequest {
	if pr == nil {
		return nil
	}
	var author *provider.CRUser
	if pr.Poster != nil {
		author = &provider.CRUser{ID: pr.Poster.ID, Username: pr.Poster.UserName, AvatarURL: pr.Poster.AvatarURL}
	}
	var labels []string
	for _, l := range pr.Labels {
		if l != nil {
			labels = append(labels, l.Name)
		}
	}
	var assignees []*provider.CRUser
	for _, r := range pr.Assignees {
		if r != nil {
			assignees = append(assignees, &provider.CRUser{ID: r.ID, Username: r.UserName, AvatarURL: r.AvatarURL})
		}
	}
	return &provider.ChangeRequest{
		ID:           pr.ID,
		Number:       strconv.FormatInt(pr.Index, 10),
		Title:        pr.Title,
		Description:  pr.Body,
		State:        mapState(string(pr.State), pr.HasMerged),
		SourceBranch: pr.Head.Ref,
		TargetBranch: pr.Base.Ref,
		HeadSHA:      pr.Head.Sha,
		BaseSHA:      pr.Base.Sha,
		Author:       author,
		Assignees:    assignees,
		Reviewers:    assignees, // Forgejo has no separate RequestedReviewers
		Labels:       labels,
		WebURL:       pr.HTMLURL,
		CreatedAt:    timeOrZero(pr.Created),
		UpdatedAt:    timeOrZero(pr.Updated),
	}
}

func convertBranch(b *forgejo.Branch) *provider.PlatformBranch {
	if b == nil {
		return nil
	}
	return &provider.PlatformBranch{Name: b.Name}
}

func convertCommit(c *forgejo.Commit) *provider.CommitInfo {
	if c == nil {
		return nil
	}
	sha := ""
	if c.CommitMeta != nil {
		sha = c.CommitMeta.SHA
	}
	ci := &provider.CommitInfo{SHA: sha}
	if c.RepoCommit != nil {
		ci.Message = c.RepoCommit.Message
		if c.RepoCommit.Author != nil {
			ci.Author = &provider.CRUser{Name: c.RepoCommit.Author.Name}
		}
	}
	if c.CommitMeta != nil {
		ci.CreatedAt = c.CommitMeta.Created
	}
	return ci
}

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

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func mapState(state string, merged bool) provider.CRState {
	return provider.MapBoolStateToCR(state, merged)
}

// parseTotalCount extracts the X-Total-Count header from a forgejo response.
func parseTotalCount(resp *forgejo.Response) int {
	if resp == nil {
		return 0
	}
	return provider.ParseTotalCountHeader(resp.Header, 0)
}
