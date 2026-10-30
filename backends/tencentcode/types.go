package tencentcode

import (
	"strconv"
	"strings"

	"github.com/yi-nology/go-git-platform/provider"
	gongfeng "github.com/yi-nology/go-gongfeng"
)

func mapState(state string) provider.CRState {
	return provider.MapMRStateToCR(state)
}

// convertProject maps a gongfeng.Project to a provider.PlatformRepo.
func convertProject(p *gongfeng.Project) *provider.PlatformRepo {
	parts := strings.SplitN(p.PathWithNamespace, "/", 2)
	owner := ""
	if len(parts) == 2 {
		owner = parts[0]
	}
	return &provider.PlatformRepo{
		ID:            int64(p.ID),
		FullName:      p.PathWithNamespace,
		Name:          p.Name,
		Owner:         owner,
		Description:   p.Description,
		CloneURL:      p.HTTPURLToRepo,
		SSHURL:        p.SSHURLToRepo,
		DefaultBranch: p.DefaultBranch,
		Private:       p.VisibilityLevel == 0,
		Platform:      provider.PlatformTencentCode,
	}
}

// convertBranch maps a gongfeng.Branch to a provider.PlatformBranch.
func convertBranch(b *gongfeng.Branch) *provider.PlatformBranch {
	return &provider.PlatformBranch{Name: b.Name}
}

// convertCommit maps a gongfeng.Commit to a provider.CommitInfo.
func convertCommit(c *gongfeng.Commit) *provider.CommitInfo {
	info := &provider.CommitInfo{
		SHA:     c.ID,
		Message: c.Message,
		Author: &provider.CRUser{
			Name: c.AuthorName,
		},
		CreatedAt: c.AuthoredDate.Time,
	}
	if c.CommitterName != "" {
		info.Committer = &provider.CRUser{Name: c.CommitterName}
	}
	return info
}

// convertMR maps a gongfeng.MergeRequest to a provider.ChangeRequest.
func convertMR(mr *gongfeng.MergeRequest) *provider.ChangeRequest {
	cr := &provider.ChangeRequest{
		ID:           int64(mr.IID),
		Number:       strconv.Itoa(mr.IID),
		Title:        mr.Title,
		Description:  mr.Description,
		State:        mapState(mr.State),
		Draft:        mr.WorkInProgress,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		Labels:       mr.Labels,
		CreatedAt:    mr.CreatedAt.Time,
		UpdatedAt:    mr.UpdatedAt.Time,
	}
	if mr.Author != nil {
		cr.Author = convertUser(mr.Author)
	}
	return cr
}

// convertDiff maps a gongfeng.Diff to a provider.ChangedFile.
func convertDiff(d *gongfeng.Diff) *provider.ChangedFile {
	return &provider.ChangedFile{
		OldPath:   d.OldPath,
		NewPath:   d.NewPath,
		Diff:      d.Diff,
		Additions: d.Additions,
		Deletions: d.Deletions,
		IsNew:     d.NewFile,
		IsDeleted: d.DeletedFile,
		IsRenamed: d.RenamedFile,
	}
}

// convertTag maps a gongfeng.Tag to a provider.TagInfo.
func convertTag(t *gongfeng.Tag) *provider.TagInfo {
	commitSHA := ""
	if t.Commit != nil {
		commitSHA = t.Commit.ID
	}
	return &provider.TagInfo{
		Name:   t.Name,
		Commit: commitSHA,
	}
}

// convertRelease maps a gongfeng.Release to a provider.ReleaseInfo. The
// gongfeng release object carries no draft/prerelease flags or web URL,
// so those fields stay zero.
func convertRelease(r *gongfeng.Release) *provider.ReleaseInfo {
	return &provider.ReleaseInfo{
		ID:        int64(r.ID),
		TagName:   r.Tag,
		Title:     r.Title,
		Body:      r.Description,
		CreatedAt: r.CreatedAt.Time,
	}
}

// convertWebhook maps a gongfeng.Webhook to a provider.PlatformWebhook.
func convertWebhook(w *gongfeng.Webhook) *provider.PlatformWebhook {
	events := []string{}
	if w.PushEvents {
		events = append(events, "push")
	}
	if w.MergeRequestsEvents {
		events = append(events, "merge_request")
	}
	if w.TagPushEvents {
		events = append(events, "tag_push")
	}
	if w.IssuesEvents {
		events = append(events, "issues")
	}
	if w.NoteEvents {
		events = append(events, "note")
	}
	return &provider.PlatformWebhook{
		ID:     int64(w.ID),
		URL:    w.URL,
		Events: events,
	}
}

// convertUser maps a gongfeng.User to a provider.CRUser.
func convertUser(u *gongfeng.User) *provider.CRUser {
	if u == nil {
		return nil
	}
	return &provider.CRUser{
		ID:        int64(u.ID),
		Username:  u.Username,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
	}
}
