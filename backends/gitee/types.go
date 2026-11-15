package gitee

import (
	"strconv"
	"strings"
	"time"

	gitee "github.com/next-bin/go-gitee/gitee"
	"github.com/oekit/pkg/timex"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// deref safely dereferences a pointer, returning the zero value when nil.
func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// parseGiteeTime parses the RFC3339 timestamp strings the Gitee API uses.
// Unparseable/empty values stay zero.
func parseGiteeTime(s *string) time.Time {
	if s == nil || *s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, *s); err == nil {
		return t
	}
	return time.Time{}
}

// convertProject translates the SDK Project model into a PlatformRepo.
func convertProject(p *gitee.Project) *model.PlatformRepo {
	if p == nil {
		return nil
	}
	out := &model.PlatformRepo{
		ID:            int64(deref(p.ID)),
		FullName:      deref(p.FullName),
		Name:          deref(p.Name),
		Description:   deref(p.Description),
		SSHURL:        deref(p.SshURL),
		DefaultBranch: deref(p.DefaultBranch),
		Private:       deref(p.Private),
		Platform:      model.PlatformGitee,
	}
	if p.Owner != nil {
		out.Owner = deref(p.Owner.Login)
	}
	return out
}

// convertPullRequest translates the SDK PullRequest model into a
// model.ChangeRequest.
func convertPullRequest(pr *gitee.PullRequest) *model.ChangeRequest {
	if pr == nil {
		return nil
	}
	state := provider.MapBoolStateToCR(deref(pr.State), pr.MergedAt != nil && *pr.MergedAt != "")
	var labels []string
	if pr.Labels != nil {
		for _, l := range *pr.Labels {
			labels = append(labels, deref(l.Name))
		}
	}
	var assignees []*model.CRUser
	if pr.Assignees != nil {
		for _, a := range *pr.Assignees {
			assignees = append(assignees, &model.CRUser{
				ID:       int64(deref(a.ID)),
				Username: deref(a.Login),
			})
		}
	}
	mergeStatus := "conflicting"
	if deref(pr.Mergeable) {
		mergeStatus = "mergeable"
	}
	out := &model.ChangeRequest{
		ID:          int64(deref(pr.ID)),
		Number:      strconv.Itoa(deref(pr.Number)),
		Title:       deref(pr.Title),
		Description: deref(pr.Body),
		State:       state,
		Assignees:   assignees,
		Reviewers:   assignees, // Gitee has no separate RequestedReviewers
		Labels:      labels,
		MergeStatus: mergeStatus,
		WebURL:      deref(pr.HTMLURL),
		Draft:       deref(pr.Draft),
		CreatedAt:   parseGiteeTime(pr.CreatedAt),
		UpdatedAt:   parseGiteeTime(pr.UpdatedAt),
	}
	if pr.Head != nil {
		out.SourceBranch = deref(pr.Head.Ref)
		if pr.Head.Sha != nil {
			out.HeadSHA = *pr.Head.Sha
		}
	}
	if pr.Base != nil {
		out.TargetBranch = deref(pr.Base.Ref)
		if pr.Base.Sha != nil {
			out.BaseSHA = *pr.Base.Sha
		}
	}
	if pr.User != nil {
		out.Author = &model.CRUser{
			ID:       int64(deref(pr.User.ID)),
			Username: deref(pr.User.Login),
			Name:     deref(pr.User.Name),
		}
	}
	return out
}

// convertPRComment translates the SDK PullRequestComments model into a
// model.CRComment.
func convertPRComment(c *gitee.PullRequestComments) *model.CRComment {
	if c == nil {
		return nil
	}
	out := &model.CRComment{
		ID:        int64(deref(c.ID)),
		Body:      deref(c.Body),
		CreatedAt: parseGiteeTime(c.CreatedAt),
		UpdatedAt: parseGiteeTime(c.UpdatedAt),
	}
	if c.User != nil {
		out.Author = &model.CRUser{
			ID:       int64(deref(c.User.ID)),
			Username: deref(c.User.Login),
			Name:     deref(c.User.Name),
		}
	}
	return out
}

// convertPRCommit translates the SDK PullRequestCommits model into a
// model.CRCommit.
func convertPRCommit(c *gitee.PullRequestCommits) *model.CRCommit {
	if c == nil {
		return nil
	}
	cc := &model.CRCommit{SHA: deref(c.Sha)}
	if c.Commit != nil {
		cc.Message = deref(c.Commit.Message)
		if c.Commit.Author != nil {
			cc.CreatedAt = parseGiteeTime(c.Commit.Author.Date)
		}
	}
	if c.Author != nil && deref(c.Author.ID) > 0 {
		cc.Author = &model.CRUser{
			ID:       int64(deref(c.Author.ID)),
			Username: deref(c.Author.Login),
		}
	} else if c.Commit != nil && c.Commit.Author != nil && c.Commit.Author.Name != nil {
		cc.Author = &model.CRUser{Name: deref(c.Commit.Author.Name)}
	}
	return cc
}

// convertPRFile translates the SDK PullRequestFiles model into a
// model.ChangedFile.
func convertPRFile(f *gitee.PullRequestFiles) *model.ChangedFile {
	if f == nil {
		return nil
	}
	filename := deref(f.Filename)
	out := &model.ChangedFile{
		OldPath: filename,
		NewPath: filename,
	}
	if f.Patch != nil {
		out.OldPath = orDefault(deref(f.Patch.OldPath), filename)
		out.NewPath = orDefault(deref(f.Patch.NewPath), filename)
		out.Diff = deref(f.Patch.Diff)
		out.IsNew = deref(f.Patch.NewFile)
		out.IsDeleted = deref(f.Patch.DeletedFile)
		out.IsRenamed = deref(f.Patch.RenamedFile)
	}
	add, del := provider.CountDiffLines(out.Diff)
	if n, err := strconv.Atoi(deref(f.Additions)); err == nil && n > 0 {
		add = n
	}
	if n, err := strconv.Atoi(deref(f.Deletions)); err == nil && n > 0 {
		del = n
	}
	out.Additions = add
	out.Deletions = del
	return out
}

func orDefault(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// convertDiffFile translates the SDK DiffFile (from compare endpoint) into a
// model.ChangedFile.
func convertDiffFile(f *gitee.DiffFile) *model.ChangedFile {
	if f == nil {
		return nil
	}
	filename := deref(f.Filename)
	patch := deref(f.Patch)
	add, del := provider.CountDiffLines(patch)
	if deref(f.Additions) > 0 {
		add = deref(f.Additions)
	}
	if deref(f.Deletions) > 0 {
		del = deref(f.Deletions)
	}
	status := deref(f.Status)
	return &model.ChangedFile{
		OldPath:   filename,
		NewPath:   filename,
		Diff:      patch,
		Additions: add,
		Deletions: del,
		IsNew:     status == "added",
		IsDeleted: status == "removed",
		IsRenamed: status == "renamed",
	}
}

// convertRepoCommit translates the SDK RepoCommit into a model.CommitInfo.
func convertRepoCommit(c *gitee.RepoCommit) *model.CommitInfo {
	if c == nil {
		return nil
	}
	ci := &model.CommitInfo{
		SHA: deref(c.Sha),
	}
	if c.Commit != nil {
		ci.Message = deref(c.Commit.Message)
		if c.Commit.Author != nil {
			ci.CreatedAt = parseGiteeTime(c.Commit.Author.Date)
		}
	}
	if c.Author != nil && deref(c.Author.ID) > 0 {
		ci.Author = &model.CRUser{
			ID:       int64(deref(c.Author.ID)),
			Username: deref(c.Author.Login),
		}
	} else if c.Commit != nil && c.Commit.Author != nil && c.Commit.Author.Name != nil {
		ci.Author = &model.CRUser{Name: deref(c.Commit.Author.Name)}
	}
	if c.Committer != nil && deref(c.Committer.ID) > 0 {
		ci.Committer = &model.CRUser{
			ID:       int64(deref(c.Committer.ID)),
			Username: deref(c.Committer.Login),
		}
	}
	return ci
}

// convertRepoCommitWithFiles translates the SDK RepoCommitWithFiles (from
// GetCommit) into a model.CommitInfo with stats.
func convertRepoCommitWithFiles(c *gitee.RepoCommitWithFiles) *model.CommitInfo {
	if c == nil {
		return nil
	}
	ci := &model.CommitInfo{
		SHA: deref(c.Sha),
	}
	if c.Commit != nil {
		ci.Message = deref(c.Commit.Message)
		if c.Commit.Author != nil {
			ci.CreatedAt = parseGiteeTime(c.Commit.Author.Date)
		}
	}
	ci.Author = convertCommitUser(c.Author, c.Commit, true)
	ci.Committer = convertCommitUser(c.Committer, c.Commit, false)
	if c.Stats != nil {
		ci.Additions = deref(c.Stats.Additions)
		ci.Deletions = deref(c.Stats.Deletions)
	}
	return ci
}

// convertCommitUser builds a CRUser from a top-level user (with ID/Login)
// enriched with the commit-level name when available.
func convertCommitUser(u *gitee.UserBasic, commit *gitee.CommitDetail, isAuthor bool) *model.CRUser {
	var cr *model.CRUser
	if u != nil && deref(u.ID) > 0 {
		cr = &model.CRUser{
			ID:       int64(deref(u.ID)),
			Username: deref(u.Login),
		}
	}
	if commit != nil {
		var gitUser *gitee.GitUser
		if isAuthor {
			gitUser = commit.Author
		} else {
			gitUser = commit.Committer
		}
		if gitUser != nil && gitUser.Name != nil {
			if cr == nil {
				cr = &model.CRUser{}
			}
			cr.Name = deref(gitUser.Name)
		}
	}
	return cr
}

// convertHook translates the SDK Hook model into a model.PlatformWebhook.
func convertHook(h *gitee.Hook) *model.PlatformWebhook {
	if h == nil {
		return nil
	}
	var events []string
	if deref(h.PushEvents) {
		events = append(events, "push")
	}
	if deref(h.TagPushEvents) {
		events = append(events, "tag_push")
	}
	if deref(h.IssuesEvents) {
		events = append(events, "issues")
	}
	if deref(h.NoteEvents) {
		events = append(events, "note")
	}
	if deref(h.MergeRequestsEvents) {
		events = append(events, "pull_request")
	}
	return &model.PlatformWebhook{
		ID:     int64(deref(h.ID)),
		URL:    deref(h.URL),
		Events: events,
	}
}

// convertBranch translates the SDK Branch model into a PlatformBranch.
func convertBranch(b *gitee.Branch) *model.PlatformBranch {
	if b == nil {
		return nil
	}
	return &model.PlatformBranch{Name: deref(b.Name)}
}

// convertRelease translates the SDK Release model into a model.ReleaseInfo.
func convertRelease(r *gitee.Release) *model.ReleaseInfo {
	if r == nil {
		return nil
	}
	ri := &model.ReleaseInfo{
		ID:         int64(deref(r.ID)),
		TagName:    deref(r.TagName),
		Title:      deref(r.Name),
		Body:       deref(r.Body),
		Draft:      false, // Gitee releases have no draft field
		Prerelease: deref(r.Prerelease),
		CreatedAt:  parseGiteeTime(r.CreatedAt),
	}
	return ri
}

// convertTag translates the SDK Tag model into a model.TagInfo.
func convertTag(t *gitee.Tag) *model.TagInfo {
	if t == nil {
		return nil
	}
	ti := &model.TagInfo{Name: deref(t.Name)}
	if t.Commit != nil {
		ti.Commit = deref(t.Commit.Sha)
	}
	return ti
}

// convertContributor translates the SDK Contributor model into a
// model.Contributor.
func convertContributor(c *gitee.Contributor) *model.Contributor {
	if c == nil {
		return nil
	}
	return &model.Contributor{
		Username:      deref(c.Name),
		Contributions: deref(c.Contributions),
	}
}

// convertCollaborator translates the SDK ProjectMember model into a
// model.Collaborator.
func convertCollaborator(m *gitee.ProjectMember) *model.Collaborator {
	if m == nil {
		return nil
	}
	return &model.Collaborator{
		ID:       int64(deref(m.ID)),
		Username: deref(m.Login),
	}
}

// convertLabel maps the SDK Label to a model.Label. Gitee colors are 6-digit
// hex without '#'.
func convertLabel(l *gitee.Label) *model.Label {
	if l == nil {
		return nil
	}
	return &model.Label{
		ID:    int64(deref(l.ID)),
		Name:  deref(l.Name),
		Color: strings.TrimPrefix(deref(l.Color), "#"),
	}
}

// convertIssue maps the SDK Issue model to a model.Issue. Gitee issue
// numbers are alphanumeric strings, carried as-is.
func convertIssue(i *gitee.Issue) *model.Issue {
	if i == nil {
		return nil
	}
	issue := &model.Issue{
		Number:    deref(i.Number),
		Title:     deref(i.Title),
		Body:      deref(i.Body),
		State:     model.IssueState(deref(i.State)),
		WebURL:    deref(i.HTMLURL),
		CreatedAt: parseGiteeTime(i.CreatedAt),
		UpdatedAt: parseGiteeTime(i.UpdatedAt),
	}
	if i.User != nil {
		issue.Author = &model.CRUser{
			ID:       int64(deref(i.User.ID)),
			Username: deref(i.User.Login),
			Name:     deref(i.User.Name),
		}
	}
	if i.Labels != nil {
		for _, l := range *i.Labels {
			issue.Labels = append(issue.Labels, deref(l.Name))
		}
	}
	if i.Assignee != nil {
		issue.Assignees = append(issue.Assignees, deref(i.Assignee.Login))
	}
	if i.Collaborators != nil {
		for _, c := range *i.Collaborators {
			issue.Assignees = append(issue.Assignees, deref(c.Login))
		}
	}
	if i.Milestone != nil {
		issue.Milestone = &model.MilestoneRef{
			Number: strconv.Itoa(deref(i.Milestone.Number)),
			Title:  deref(i.Milestone.Title),
		}
	}
	return issue
}

// convertIssueComment maps the SDK Note model to a model.IssueComment.
func convertIssueComment(n *gitee.Note) *model.IssueComment {
	if n == nil {
		return nil
	}
	ic := &model.IssueComment{
		ID:        int64(deref(n.ID)),
		Body:      deref(n.Body),
		CreatedAt: parseGiteeTime(n.CreatedAt),
		UpdatedAt: parseGiteeTime(n.UpdatedAt),
	}
	if n.User != nil {
		ic.Author = &model.CRUser{
			ID:       int64(deref(n.User.ID)),
			Username: deref(n.User.Login),
			Name:     deref(n.User.Name),
		}
	}
	return ic
}

// convertMilestone maps the SDK Milestone to a model.Milestone.
func convertMilestone(m *gitee.Milestone) model.Milestone {
	if m == nil {
		return model.Milestone{}
	}
	ms := model.Milestone{
		Number:      strconv.Itoa(deref(m.Number)),
		Title:       deref(m.Title),
		Description: deref(m.Description),
		State:       model.MilestoneState(deref(m.State)),
	}
	if due, ok := parseGiteeDueOn(deref(m.DueOn)); ok {
		ms.DueOn = &due
	}
	return ms
}

// giteeServerTZ 是 Gitee 服务端产出无时区字段（"2006-01-02 15:04:05"）
// 的时区（CST/UTC+8）：按 UTC 解析会把 due_on 偏 8 小时（pkg#271 的
// 下游实证场景）。
var giteeServerTZ = time.FixedZone("CST", 8*3600)

// parseGiteeDueOn 经 oekit timex.ParseFirstInLocation（pkg#271 落地）多
// layout first-match：RFC3339 自带时区（loc 不生效）/ "2006-01-02
// 15:04:05" 按 CST / "2006-01-02" 午夜按 CST。
func parseGiteeDueOn(s string) (time.Time, bool) {
	return timex.ParseFirstInLocation(s, giteeServerTZ, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02")
}

func formatGiteeDueOn(t time.Time) string { return t.Format(time.RFC3339) }
