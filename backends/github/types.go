package github

import (
	"strconv"
	"time"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// Internal type aliases for clarity in conversion code. The actual JSON
// shapes are unchanged from the upstream go-github package.

type (
	ghRepo    = github.Repository
	ghPR      = github.PullRequest
	ghBranch  = github.Branch
	ghHook    = github.Hook
	ghCommit  = github.RepositoryCommit
	ghRelease = github.RepositoryRelease
	ghUser    = github.User
)

// convertRepo maps a go-github Repository to the provider-neutral type.
func convertRepo(r *ghRepo) *model.PlatformRepo {
	if r == nil {
		return nil
	}
	owner, _ := provider.SplitFullName(r.GetFullName())
	return &model.PlatformRepo{
		ID:            r.GetID(),
		FullName:      r.GetFullName(),
		Name:          r.GetName(),
		Owner:         owner,
		Description:   r.GetDescription(),
		CloneURL:      r.GetCloneURL(),
		SSHURL:        r.GetSSHURL(),
		DefaultBranch: r.GetDefaultBranch(),
		Private:       r.GetPrivate(),
		Platform:      model.PlatformGitHub,
		Archived:      r.GetArchived(),
		Fork:          r.GetFork(),
		Stars:         r.GetStargazersCount(),
		Language:      r.GetLanguage(),
	}
}

// convertPR maps a go-github PullRequest to the provider-neutral type.
func convertPR(pr *ghPR) *model.ChangeRequest {
	if pr == nil {
		return nil
	}
	state := model.CRStateOpened
	if pr.GetState() == "closed" {
		if pr.GetMerged() {
			state = model.CRStateMerged
		} else {
			state = model.CRStateClosed
		}
	}
	mergeStatus := "unknown"
	if pr.Mergeable != nil {
		if *pr.Mergeable {
			mergeStatus = "mergeable"
		} else {
			mergeStatus = "conflicting"
		}
	}
	author := &model.CRUser{}
	if pr.GetUser() != nil {
		author = &model.CRUser{
			ID:        pr.GetUser().GetID(),
			Username:  pr.GetUser().GetLogin(),
			AvatarURL: pr.GetUser().GetAvatarURL(),
		}
	}
	var reviewers []*model.CRUser
	for _, r := range pr.RequestedReviewers {
		reviewers = append(reviewers, &model.CRUser{
			ID:        r.GetID(),
			Username:  r.GetLogin(),
			AvatarURL: r.GetAvatarURL(),
		})
	}
	var assignees []*model.CRUser
	for _, a := range pr.Assignees {
		assignees = append(assignees, &model.CRUser{
			ID:        a.GetID(),
			Username:  a.GetLogin(),
			AvatarURL: a.GetAvatarURL(),
		})
	}
	var labels []string
	for _, l := range pr.Labels {
		if l != nil {
			labels = append(labels, l.GetName())
		}
	}
	// GitHub exposes no distinct merge-base in webhook payloads; base.sha is the
	// target-branch tip, which serves as both BaseSHA and StartSHA.
	baseSHA := pr.GetBase().GetSHA()
	return &model.ChangeRequest{
		ID:           int64(pr.GetNumber()),
		Number:       strconv.Itoa(pr.GetNumber()),
		Title:        pr.GetTitle(),
		Description:  pr.GetBody(),
		State:        state,
		Draft:        pr.GetDraft(),
		SourceBranch: pr.GetHead().GetRef(),
		TargetBranch: pr.GetBase().GetRef(),
		HeadSHA:      pr.GetHead().GetSHA(),
		BaseSHA:      baseSHA,
		StartSHA:     baseSHA,
		Author:       author,
		Assignees:    assignees,
		Reviewers:    reviewers,
		Labels:       labels,
		MergeStatus:  mergeStatus,
		WebURL:       pr.GetHTMLURL(),
		CreatedAt:    tsOrZero(pr.GetCreatedAt()),
		UpdatedAt:    tsOrZero(pr.GetUpdatedAt()),
	}
}

// convertBranch maps a go-github Branch.
func convertBranch(b *ghBranch) *model.PlatformBranch {
	if b == nil {
		return nil
	}
	return &model.PlatformBranch{Name: b.GetName()}
}

// convertHook maps a go-github Hook.
func convertHook(h *ghHook) *model.PlatformWebhook {
	if h == nil {
		return nil
	}
	return &model.PlatformWebhook{
		ID:     h.GetID(),
		URL:    h.GetURL(),
		Events: h.Events,
	}
}

// convertCommit maps a go-github RepositoryCommit.
func convertCommit(c *ghCommit) *model.CommitInfo {
	if c == nil {
		return nil
	}
	ci := &model.CommitInfo{
		SHA:     c.GetSHA(),
		Message: c.GetCommit().GetMessage(),
	}
	if c.GetCommit() != nil && c.GetCommit().GetAuthor() != nil {
		ci.CreatedAt = c.GetCommit().GetAuthor().GetDate().Time
	}
	if c.GetAuthor() != nil {
		ci.Author = &model.CRUser{ID: c.GetAuthor().GetID(), Username: c.GetAuthor().GetLogin(), AvatarURL: c.GetAuthor().GetAvatarURL()}
	}
	if c.GetCommitter() != nil {
		ci.Committer = &model.CRUser{ID: c.GetCommitter().GetID(), Username: c.GetCommitter().GetLogin()}
	}
	return ci
}

// convertRelease maps a go-github RepositoryRelease.
func convertRelease(r *ghRelease) *model.ReleaseInfo {
	if r == nil {
		return nil
	}
	return &model.ReleaseInfo{
		ID:          r.GetID(),
		TagName:     r.GetTagName(),
		Title:       r.GetName(),
		Body:        r.GetBody(),
		URL:         r.GetHTMLURL(),
		Draft:       r.GetDraft(),
		Prerelease:  r.GetPrerelease(),
		CreatedAt:   tsOrZero(r.GetCreatedAt()),
		PublishedAt: tsOrZero(r.GetPublishedAt()),
		Assets:      convertReleaseAssets(r.Assets),
	}
}

// convertReleaseAssets maps go-github release assets. SDK 的 Size 是 *int
// (GitHub 附件体积在 int 范围内),provider 侧统一放大为 int64。
func convertReleaseAssets(assets []*github.ReleaseAsset) []*model.ReleaseAsset {
	if len(assets) == 0 {
		return nil
	}
	out := make([]*model.ReleaseAsset, 0, len(assets))
	for _, a := range assets {
		if a == nil {
			continue
		}
		out = append(out, &model.ReleaseAsset{
			ID:                 a.GetID(),
			Name:               a.GetName(),
			Size:               int64(a.GetSize()),
			ContentType:        a.GetContentType(),
			BrowserDownloadURL: a.GetBrowserDownloadURL(),
			URL:                a.GetURL(),
		})
	}
	return out
}

// convertUser maps a go-github User.
func convertUser(u *ghUser) *model.CRUser {
	if u == nil {
		return nil
	}
	return &model.CRUser{
		ID:        u.GetID(),
		Username:  u.GetLogin(),
		Name:      u.GetName(),
		AvatarURL: u.GetAvatarURL(),
	}
}

// tsOrZero takes a value-type github.Timestamp and returns the underlying
// time.Time. The upstream SDK returns Timestamp by value from its Get*
// helpers, so the by-reference form is rarely useful in practice.
func tsOrZero(ts github.Timestamp) time.Time {
	if ts.IsZero() {
		return time.Time{}
	}
	return ts.Time
}

// mapCRState maps a model.CRState to the GitHub API string used in list
// filters ("open"/"closed"/"all"). Mapping CRStateMerged to "closed" is
// intentional: GitHub treats merged PRs as closed; consumers can inspect the
// merged boolean on returned PRs to disambiguate.
func mapCRState(state model.CRState) string {
	switch state {
	case model.CRStateOpened:
		return "open"
	case model.CRStateClosed:
		return "closed"
	case model.CRStateMerged:
		return "closed"
	default:
		return "all"
	}
}
