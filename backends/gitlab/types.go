package gitlab

import (
	"strconv"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/runtime"
)

// convertProject maps a gitlab.Project to the provider-neutral type.
func convertProject(p *gitlab.Project) *model.PlatformRepo {
	if p == nil {
		return nil
	}
	owner, _ := runtime.SplitFullName(p.PathWithNamespace)
	return &model.PlatformRepo{
		ID:            p.ID,
		FullName:      p.PathWithNamespace,
		Name:          p.Name,
		Owner:         owner,
		Description:   p.Description,
		CloneURL:      p.HTTPURLToRepo,
		SSHURL:        p.SSHURLToRepo,
		DefaultBranch: p.DefaultBranch,
		Private:       p.Visibility != "public",
		Platform:      model.PlatformGitLab,
		Archived:      p.Archived,
		Fork:          p.ForkedFromProject != nil,
		Stars:         int(p.StarCount),
		Language:      "",
	}
}

// convertMR maps a gitlab.MergeRequest to the provider-neutral type.
func convertMR(mr *gitlab.MergeRequest) *model.ChangeRequest {
	if mr == nil {
		return nil
	}
	var author *model.CRUser
	if mr.Author != nil {
		author = &model.CRUser{ID: mr.Author.ID, Username: mr.Author.Username, Name: mr.Author.Name, AvatarURL: mr.Author.AvatarURL}
	}
	var reviewers []*model.CRUser
	for _, r := range mr.Reviewers {
		reviewers = append(reviewers, &model.CRUser{ID: r.ID, Username: r.Username, Name: r.Name, AvatarURL: r.AvatarURL})
	}
	var assignees []*model.CRUser
	for _, a := range mr.Assignees {
		assignees = append(assignees, &model.CRUser{ID: a.ID, Username: a.Username, Name: a.Name, AvatarURL: a.AvatarURL})
	}
	return &model.ChangeRequest{
		ID:           mr.IID,
		Number:       strconv.FormatInt(mr.IID, 10),
		Title:        mr.Title,
		Description:  mr.Description,
		State:        mapGLState(mr.State),
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		Author:       author,
		Assignees:    assignees,
		Reviewers:    reviewers,
		Labels:       mr.Labels,
		MergeStatus:  mr.DetailedMergeStatus,
		WebURL:       mr.WebURL,
		HeadSHA:      mrHeadSHA(mr),
		Draft:        mr.Draft,
		BaseSHA:      mr.DiffRefs.BaseSha,
		StartSHA:     mr.DiffRefs.StartSha,
		CreatedAt:    timeOrZero(mr.CreatedAt),
		UpdatedAt:    timeOrZero(mr.UpdatedAt),
	}
}

func mrHeadSHA(mr *gitlab.MergeRequest) string {
	if mr.DiffRefs.HeadSha != "" {
		return mr.DiffRefs.HeadSha
	}
	return mr.SHA
}

// convertBasicMR maps the lighter-weight gitlab.BasicMergeRequest (returned
// by list endpoints) to the provider-neutral type.
func convertBasicMR(mr *gitlab.BasicMergeRequest) *model.ChangeRequest {
	if mr == nil {
		return nil
	}
	var author *model.CRUser
	if mr.Author != nil {
		author = &model.CRUser{ID: mr.Author.ID, Username: mr.Author.Username, Name: mr.Author.Name, AvatarURL: mr.Author.AvatarURL}
	}
	var reviewers []*model.CRUser
	for _, r := range mr.Reviewers {
		reviewers = append(reviewers, &model.CRUser{ID: r.ID, Username: r.Username, Name: r.Name, AvatarURL: r.AvatarURL})
	}
	var assignees []*model.CRUser
	for _, a := range mr.Assignees {
		assignees = append(assignees, &model.CRUser{ID: a.ID, Username: a.Username, Name: a.Name, AvatarURL: a.AvatarURL})
	}
	return &model.ChangeRequest{
		ID:          mr.IID,
		Number:      strconv.FormatInt(mr.IID, 10),
		Title:       mr.Title,
		Description: mr.Description,
		State:       mapGLState(mr.State),
		// Draft/HeadSHA 必须映射（v0.67.1 修复）：poller 依赖 HeadSHA 判空跳过、
		// 双路径依赖 Draft 做 skip-draft——此前列表路径两者恒零值，poller 对
		// GitLab 所有 MR 静默跳过（兜底路径从未真正建过任务，215 实测实锤）。
		Draft:        mr.Draft,
		HeadSHA:      mr.SHA,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		Author:       author,
		Assignees:    assignees,
		Reviewers:    reviewers,
		Labels:       mr.Labels,
		MergeStatus:  mr.DetailedMergeStatus,
		WebURL:       mr.WebURL,
		CreatedAt:    timeOrZero(mr.CreatedAt),
		UpdatedAt:    timeOrZero(mr.UpdatedAt),
	}
}

func convertBranch(b *gitlab.Branch) *model.PlatformBranch {
	if b == nil {
		return nil
	}
	return &model.PlatformBranch{Name: b.Name}
}

func convertHook(h *gitlab.ProjectHook) *model.PlatformWebhook {
	if h == nil {
		return nil
	}
	var events []string
	if h.PushEvents {
		events = append(events, "push")
	}
	if h.MergeRequestsEvents {
		events = append(events, "merge_request")
	}
	if h.TagPushEvents {
		events = append(events, "tag_push")
	}
	if h.NoteEvents {
		events = append(events, "note")
	}
	return &model.PlatformWebhook{ID: h.ID, URL: h.URL, Events: events}
}

func convertCommit(c *gitlab.Commit) *model.CommitInfo {
	if c == nil {
		return nil
	}
	ci := &model.CommitInfo{SHA: c.ID, Message: c.Message}
	if c.Stats != nil {
		ci.Additions = int(c.Stats.Additions)
		ci.Deletions = int(c.Stats.Deletions)
	}
	ci.CreatedAt = timeOrZero(c.CreatedAt)
	if c.AuthorName != "" || c.AuthorEmail != "" {
		ci.Author = &model.CRUser{Name: c.AuthorName}
	}
	return ci
}

// timeOrZero dereferences a *time.Time, returning the zero value when nil.
func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func mapGLState(state string) model.CRState {
	return provider.MapMRStateToCR(state)
}

func mapCommitState(state string) gitlab.BuildStateValue {
	switch state {
	case "success":
		return gitlab.Success
	case "failed":
		return gitlab.Failed
	case "pending":
		return gitlab.Pending
	case "running":
		return gitlab.Running
	default:
		return gitlab.Pending
	}
}
