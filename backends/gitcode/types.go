package gitcode

import (
	"strconv"
	"strings"
	"time"

	"github.com/oekit/pkg/timex"
	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/runtime"
)

func convertUser(u *gitcode.User) *model.CRUser {
	if u == nil {
		return nil
	}
	id, _ := parseGitCodeID(u.ID)
	return &model.CRUser{
		ID:        id,
		Username:  u.Login,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
	}
}

func convertIssue(i *gitcode.Issue) *model.Issue {
	if i == nil {
		return nil
	}
	labels := make([]string, 0, len(i.Labels))
	for _, l := range i.Labels {
		labels = append(labels, l.Name)
	}
	assignees := make([]string, 0, len(i.Assignees))
	for _, a := range i.Assignees {
		assignees = append(assignees, a.Login)
	}
	author := convertUser(i.Author)
	if author == nil {
		author = convertUser(i.User)
	}
	var milestone *model.MilestoneRef
	if i.Milestone != nil {
		milestone = &model.MilestoneRef{Number: strconv.FormatInt(milestoneAddress(i.Milestone), 10), Title: i.Milestone.Title}
	}
	return &model.Issue{
		ID:        i.ID,
		Number:    strconv.Itoa(int(i.Number)),
		Title:     i.Title,
		Body:      i.Body,
		State:     model.IssueState(i.State),
		Author:    author,
		Labels:    labels,
		Assignees: assignees,
		Milestone: milestone,
		WebURL:    i.HTMLURL,
		CreatedAt: i.CreatedAt,
		UpdatedAt: i.UpdatedAt,
		ClosedAt:  i.ClosedAt,
	}
}

func convertIssueComment(c *gitcode.IssueComment) *model.IssueComment {
	if c == nil {
		return nil
	}
	author := convertUser(c.Author)
	if author == nil {
		author = convertUser(c.User)
	}
	return &model.IssueComment{
		ID:        c.ID,
		Body:      c.Body,
		Author:    author,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func convertPullRequest(pr *gitcode.PullRequest) *model.ChangeRequest {
	if pr == nil {
		return nil
	}
	state := model.CRStateOpened
	if pr.State == gitcode.PullRequestStateClosed {
		if pr.Merged {
			state = model.CRStateMerged
		} else {
			state = model.CRStateClosed
		}
	}
	cr := &model.ChangeRequest{
		ID:          pr.ID,
		Number:      strconv.Itoa(pr.Number),
		Title:       pr.Title,
		Description: pr.Body,
		State:       state,
		WebURL:      pr.HTMLURL,
		CreatedAt:   pr.CreatedAt,
		UpdatedAt:   pr.UpdatedAt,
	}
	if pr.Head != nil {
		cr.SourceBranch = pr.Head.Ref
	}
	if pr.Base != nil {
		cr.TargetBranch = pr.Base.Ref
	}
	if pr.Author != nil {
		authorID, _ := strconv.ParseInt(string(pr.Author.ID), 10, 64)
		cr.Author = &model.CRUser{
			ID: authorID, Username: pr.Author.Login, AvatarURL: pr.Author.AvatarURL,
		}
	}
	cr.Draft = pr.Draft
	return cr
}

func convertCommit(c *gitcode.Commit) *model.CommitInfo {
	if c == nil {
		return nil
	}
	ci := &model.CommitInfo{SHA: c.SHA, Message: c.Message, CreatedAt: c.CreatedAt}
	if c.Author != nil {
		authorID, _ := parseGitCodeID(c.Author.ID)
		ci.Author = &model.CRUser{
			ID: authorID, Username: c.Author.Login, AvatarURL: c.Author.AvatarURL,
		}
	}
	return ci
}

func convertLabel(l *gitcode.Label) *model.Label {
	if l == nil {
		return nil
	}
	return &model.Label{
		ID:    l.ID,
		Name:  l.Name,
		Color: strings.TrimPrefix(l.Color, "#"),
	}
}

// milestoneAddress resolves the milestone addressing identifier: real
// GitCode deployments key milestones by the wire's "id" (the value the
// Get/Update/Delete path parameter expects), while the documented v5
// shape carries only "number" — prefer id, fall back to number.
func milestoneAddress(m *gitcode.Milestone) int64 {
	if m == nil {
		return 0
	}
	if m.ID != 0 {
		return m.ID
	}
	return m.Number
}

// parseGitCodeDueOn parses the milestone due date as served: GitCode's
// documented due_on is a date-only string ("2026-01-31"), while instances
// that echo back what was sent return RFC3339 timestamps. Empty or
// unparseable values yield nil rather than failing the conversion.
func parseGitCodeDueOn(s string) *time.Time {
	// oekit timex.ParseFirst（pkg#197 落地）：多 layout first-match 原语，
	// 替换手写循环。空/不可解析 → nil（转换不因 due_on 失败）。
	if t, ok := timex.ParseFirst(s, time.RFC3339, "2006-01-02"); ok {
		return &t
	}
	return nil
}

func convertMilestone(m *gitcode.Milestone) model.Milestone {
	var ms model.Milestone
	if m == nil {
		return ms
	}
	ms = model.Milestone{
		Number:      strconv.FormatInt(milestoneAddress(m), 10),
		Title:       m.Title,
		Description: m.Description,
		State:       model.MilestoneState(m.State),
		DueOn:       parseGitCodeDueOn(m.DueOn),
	}
	return ms
}

func convertRelease(r *gitcode.Release) *model.ReleaseInfo {
	if r == nil {
		return nil
	}
	return &model.ReleaseInfo{
		ID:          r.ID,
		TagName:     r.TagName,
		Title:       r.Name,
		Body:        r.Body,
		URL:         r.HTMLURL,
		Draft:       r.Draft,
		Prerelease:  r.Prerelease,
		CreatedAt:   r.CreatedAt,
		PublishedAt: r.PublishedAt,
	}
}

func convertNotification(t *gitcode.NotificationThread) *model.Notification {
	n := &model.Notification{
		ID:     strconv.FormatInt(t.ID, 10),
		Unread: t.Unread,
	}
	if t.Subject != nil {
		n.Subject = model.NotificationSubject{
			Title: t.Subject.Title,
			Type:  t.Subject.Type,
			URL:   t.Subject.URL,
		}
	}
	if t.Repository != nil {
		n.Repo = &model.EventRepo{
			ID:       t.Repository.ID,
			FullName: t.Repository.FullName,
		}
		owner, name := runtime.SplitFullName(t.Repository.FullName)
		n.Repo.Owner = owner
		n.Repo.Name = name
	}
	n.UpdatedAt = t.UpdatedAt
	return n
}

func convertCollaborator(c *gitcode.Collaborator) *model.Collaborator {
	if c == nil {
		return nil
	}
	return &model.Collaborator{
		ID:         c.ID,
		Username:   c.Login,
		Permission: c.Permission,
	}
}

func convertGitcodeRepo(r *gitcode.Repository) *model.PlatformRepo {
	if r == nil {
		return nil
	}
	owner := ""
	if r.Owner != nil {
		owner = r.Owner.Login
	}
	return &model.PlatformRepo{
		ID:            r.ID,
		FullName:      r.FullName,
		Name:          r.Name,
		Owner:         owner,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Platform:      model.PlatformGitCode,
	}
}

func convertReview(r *gitcode.PullRequestReview) model.Review {
	var review model.Review
	if r == nil {
		return review
	}
	review = model.Review{
		ID:   r.ID,
		Body: r.Body,
	}
	user := r.User
	if user == nil {
		user = r.Author
	}
	if user != nil {
		review.User = user.Login
	}
	switch r.State {
	case "APPROVED":
		review.State = model.ReviewStateApproved
	case "CHANGES_REQUESTED":
		review.State = model.ReviewStateChangesRequested
	case "COMMENTED":
		review.State = model.ReviewStateCommented
	case "PENDING":
		review.State = model.ReviewStatePending
	default:
		if r.State != "" {
			review.State = model.ReviewState(strings.ToLower(r.State))
		}
	}
	if !r.CreatedAt.IsZero() {
		review.SubmittedAt = r.CreatedAt
	}
	return review
}
