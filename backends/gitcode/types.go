package gitcode

import (
	"strconv"
	"strings"
	"time"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/provider"
)

func convertUser(u *gitcode.User) *provider.CRUser {
	if u == nil {
		return nil
	}
	id, _ := parseGitCodeID(u.ID)
	return &provider.CRUser{
		ID:        id,
		Username:  u.Login,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
	}
}

func convertIssue(i *gitcode.Issue) *provider.Issue {
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
	var milestone *provider.MilestoneRef
	if i.Milestone != nil {
		milestone = &provider.MilestoneRef{Number: strconv.FormatInt(milestoneAddress(i.Milestone), 10), Title: i.Milestone.Title}
	}
	return &provider.Issue{
		ID:        i.ID,
		Number:    strconv.Itoa(int(i.Number)),
		Title:     i.Title,
		Body:      i.Body,
		State:     provider.IssueState(i.State),
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

func convertIssueComment(c *gitcode.IssueComment) *provider.IssueComment {
	if c == nil {
		return nil
	}
	author := convertUser(c.Author)
	if author == nil {
		author = convertUser(c.User)
	}
	return &provider.IssueComment{
		ID:        c.ID,
		Body:      c.Body,
		Author:    author,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func convertPullRequest(pr *gitcode.PullRequest) *provider.ChangeRequest {
	if pr == nil {
		return nil
	}
	state := provider.CRStateOpened
	if pr.State == gitcode.PullRequestStateClosed {
		if pr.Merged {
			state = provider.CRStateMerged
		} else {
			state = provider.CRStateClosed
		}
	}
	cr := &provider.ChangeRequest{
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
		cr.Author = &provider.CRUser{
			ID: authorID, Username: pr.Author.Login, AvatarURL: pr.Author.AvatarURL,
		}
	}
	cr.Draft = pr.Draft
	return cr
}

func convertCommit(c *gitcode.Commit) *provider.CommitInfo {
	if c == nil {
		return nil
	}
	ci := &provider.CommitInfo{SHA: c.SHA, Message: c.Message, CreatedAt: c.CreatedAt}
	if c.Author != nil {
		authorID, _ := parseGitCodeID(c.Author.ID)
		ci.Author = &provider.CRUser{
			ID: authorID, Username: c.Author.Login, AvatarURL: c.Author.AvatarURL,
		}
	}
	return ci
}

func convertLabel(l *gitcode.Label) *provider.Label {
	if l == nil {
		return nil
	}
	return &provider.Label{
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
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

func convertMilestone(m *gitcode.Milestone) provider.Milestone {
	var ms provider.Milestone
	if m == nil {
		return ms
	}
	ms = provider.Milestone{
		Number:      strconv.FormatInt(milestoneAddress(m), 10),
		Title:       m.Title,
		Description: m.Description,
		State:       provider.MilestoneState(m.State),
		DueOn:       parseGitCodeDueOn(m.DueOn),
	}
	return ms
}

func convertRelease(r *gitcode.Release) *provider.ReleaseInfo {
	if r == nil {
		return nil
	}
	return &provider.ReleaseInfo{
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

func convertNotification(t *gitcode.NotificationThread) *provider.Notification {
	n := &provider.Notification{
		ID:     strconv.FormatInt(t.ID, 10),
		Unread: t.Unread,
	}
	if t.Subject != nil {
		n.Subject = provider.NotificationSubject{
			Title: t.Subject.Title,
			Type:  t.Subject.Type,
			URL:   t.Subject.URL,
		}
	}
	if t.Repository != nil {
		n.Repo = &provider.EventRepo{
			ID:       t.Repository.ID,
			FullName: t.Repository.FullName,
		}
		owner, name := provider.SplitFullName(t.Repository.FullName)
		n.Repo.Owner = owner
		n.Repo.Name = name
	}
	n.UpdatedAt = t.UpdatedAt
	return n
}

func convertCollaborator(c *gitcode.Collaborator) *provider.Collaborator {
	if c == nil {
		return nil
	}
	return &provider.Collaborator{
		ID:         c.ID,
		Username:   c.Login,
		Permission: c.Permission,
	}
}

func convertGitcodeRepo(r *gitcode.Repository) *provider.PlatformRepo {
	if r == nil {
		return nil
	}
	owner := ""
	if r.Owner != nil {
		owner = r.Owner.Login
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
		Platform:      provider.PlatformGitCode,
	}
}

func convertReview(r *gitcode.PullRequestReview) provider.Review {
	var review provider.Review
	if r == nil {
		return review
	}
	review = provider.Review{
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
		review.State = provider.ReviewStateApproved
	case "CHANGES_REQUESTED":
		review.State = provider.ReviewStateChangesRequested
	case "COMMENTED":
		review.State = provider.ReviewStateCommented
	case "PENDING":
		review.State = provider.ReviewStatePending
	default:
		if r.State != "" {
			review.State = provider.ReviewState(strings.ToLower(r.State))
		}
	}
	if !r.CreatedAt.IsZero() {
		review.SubmittedAt = r.CreatedAt
	}
	return review
}
