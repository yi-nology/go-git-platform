package forgejo

import (
	"context"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/oekit/pkg/stringx"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// forgejoDivergences is the Forgejo view of the family divergence ledger.
var forgejoDivergences = giteafamily.Divergences("Forgejo")

// sdkPorts adapts the forgejo SDK to the family engine's ports. The Forgejo
// SDK accepts no context parameter (registered platform limitation) and
// carries fork-identical wire types, so every adapter drops ctx and
// converts the structs field by field.
type sdkPorts struct {
	c *forgejo.Client
}

// --- giteafamily.HookPort ---

func (s sdkPorts) CreateRepoHook(ctx context.Context, owner, repo string, opt giteafamily.CreateHookOption) (*giteafamily.Hook, error) {
	_ = ctx // forgejo SDK takes no context
	h, _, err := s.c.CreateRepoHook(owner, repo, forgejo.CreateHookOption{
		Type:   forgejo.HookType(opt.Type),
		Config: opt.Config,
		Events: opt.Events,
		Active: opt.Active,
	})
	return familyHook(h), err
}

func (s sdkPorts) DeleteRepoHook(ctx context.Context, owner, repo string, id int64) error {
	_ = ctx // forgejo SDK takes no context
	_, err := s.c.DeleteRepoHook(owner, repo, id)
	return err
}

func (s sdkPorts) ListRepoHooks(ctx context.Context, owner, repo string, opt giteafamily.ListHooksOptions) ([]*giteafamily.Hook, error) {
	_ = ctx // forgejo SDK takes no context
	list, _, err := s.c.ListRepoHooks(owner, repo, forgejo.ListHooksOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyHooks(list), err
}

func familyHook(h *forgejo.Hook) *giteafamily.Hook {
	if h == nil {
		return nil
	}
	return &giteafamily.Hook{ID: h.ID, Config: h.Config, Events: h.Events}
}

func familyHooks(list []*forgejo.Hook) []*giteafamily.Hook {
	out := make([]*giteafamily.Hook, 0, len(list))
	for _, h := range list {
		out = append(out, familyHook(h))
	}
	return out
}

// --- giteafamily.DeployKeyPort ---

func (s sdkPorts) ListDeployKeys(ctx context.Context, owner, repo string, opt giteafamily.ListDeployKeysOptions) ([]*giteafamily.DeployKey, error) {
	_ = ctx // forgejo SDK takes no context
	list, _, err := s.c.ListDeployKeys(owner, repo, forgejo.ListDeployKeysOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyDeployKeys(list), err
}

func (s sdkPorts) CreateDeployKey(ctx context.Context, owner, repo string, opt giteafamily.CreateKeyOption) (*giteafamily.DeployKey, error) {
	_ = ctx // forgejo SDK takes no context
	k, _, err := s.c.CreateDeployKey(owner, repo, forgejo.CreateKeyOption{
		Title:    opt.Title,
		Key:      opt.Key,
		ReadOnly: opt.ReadOnly,
	})
	return familyDeployKey(k), err
}

func (s sdkPorts) DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error {
	_ = ctx // forgejo SDK takes no context
	_, err := s.c.DeleteDeployKey(owner, repo, keyID)
	return err
}

func familyDeployKey(k *forgejo.DeployKey) *giteafamily.DeployKey {
	if k == nil {
		return nil
	}
	return &giteafamily.DeployKey{ID: k.ID, Title: k.Title, Key: k.Key, ReadOnly: k.ReadOnly}
}

func familyDeployKeys(list []*forgejo.DeployKey) []*giteafamily.DeployKey {
	out := make([]*giteafamily.DeployKey, 0, len(list))
	for _, k := range list {
		out = append(out, familyDeployKey(k))
	}
	return out
}

// --- giteafamily.MilestonePort ---

func (s sdkPorts) ListMilestones(ctx context.Context, owner, repo string, opt giteafamily.ListMilestoneOption) ([]*giteafamily.Milestone, error) {
	_ = ctx // forgejo SDK takes no context
	listOpts := forgejo.ListMilestoneOption{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		State:       forgejo.StateType(opt.State),
	}
	list, _, err := s.c.ListRepoMilestones(owner, repo, listOpts)
	return familyMilestones(list), err
}

func (s sdkPorts) GetMilestone(ctx context.Context, owner, repo string, id int64) (*giteafamily.Milestone, error) {
	_ = ctx // forgejo SDK takes no context
	m, _, err := s.c.GetMilestone(owner, repo, id)
	return familyMilestone(m), err
}

func (s sdkPorts) CreateMilestone(ctx context.Context, owner, repo string, opt giteafamily.CreateMilestoneOption) (*giteafamily.Milestone, error) {
	_ = ctx // forgejo SDK takes no context
	m, _, err := s.c.CreateMilestone(owner, repo, forgejo.CreateMilestoneOption{
		Title:       opt.Title,
		Description: opt.Description,
		Deadline:    opt.Deadline,
	})
	return familyMilestone(m), err
}

func (s sdkPorts) EditMilestone(ctx context.Context, owner, repo string, id int64, opt giteafamily.EditMilestoneOption) (*giteafamily.Milestone, error) {
	_ = ctx // forgejo SDK takes no context
	editOpts := forgejo.EditMilestoneOption{
		Title:       opt.Title,
		Description: opt.Description,
		Deadline:    opt.Deadline,
	}
	if opt.State != nil {
		state := forgejo.StateType(*opt.State)
		editOpts.State = &state
	}
	m, _, err := s.c.EditMilestone(owner, repo, id, editOpts)
	return familyMilestone(m), err
}

func (s sdkPorts) DeleteMilestone(ctx context.Context, owner, repo string, id int64) error {
	_ = ctx // forgejo SDK takes no context
	_, err := s.c.DeleteMilestone(owner, repo, id)
	return err
}

func familyMilestone(m *forgejo.Milestone) *giteafamily.Milestone {
	if m == nil {
		return nil
	}
	return &giteafamily.Milestone{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		State:       string(m.State),
		Deadline:    m.Deadline,
	}
}

func familyMilestones(list []*forgejo.Milestone) []*giteafamily.Milestone {
	out := make([]*giteafamily.Milestone, 0, len(list))
	for _, m := range list {
		out = append(out, familyMilestone(m))
	}
	return out
}

// --- giteafamily.LabelPort ---

func (s sdkPorts) ListRepoLabels(ctx context.Context, owner, repo string, opt giteafamily.ListLabelsOptions) ([]*giteafamily.Label, error) {
	_ = ctx
	list, _, err := s.c.ListRepoLabels(owner, repo, forgejo.ListLabelsOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyLabels(list), err
}

func (s sdkPorts) CreateLabel(ctx context.Context, owner, repo string, opt giteafamily.CreateLabelOption) (*giteafamily.Label, error) {
	_ = ctx
	l, _, err := s.c.CreateLabel(owner, repo, forgejo.CreateLabelOption{
		Name:        opt.Name,
		Color:       opt.Color,
		Description: opt.Description,
	})
	return familyLabel(l), err
}

func (s sdkPorts) EditLabel(ctx context.Context, owner, repo string, id int64, opt giteafamily.EditLabelOption) (*giteafamily.Label, error) {
	_ = ctx
	l, _, err := s.c.EditLabel(owner, repo, id, forgejo.EditLabelOption{
		Name:        opt.Name,
		Color:       opt.Color,
		Description: opt.Description,
	})
	return familyLabel(l), err
}

func (s sdkPorts) DeleteLabel(ctx context.Context, owner, repo string, id int64) error {
	_ = ctx
	_, err := s.c.DeleteLabel(owner, repo, id)
	return err
}

func familyLabel(l *forgejo.Label) *giteafamily.Label {
	if l == nil {
		return nil
	}
	return &giteafamily.Label{ID: l.ID, Name: l.Name, Color: l.Color, Description: l.Description}
}

func familyLabels(list []*forgejo.Label) []*giteafamily.Label {
	out := make([]*giteafamily.Label, 0, len(list))
	for _, l := range list {
		out = append(out, familyLabel(l))
	}
	return out
}

// --- giteafamily.BranchPort ---

func (s sdkPorts) ListRepoBranches(ctx context.Context, owner, repo string, opt giteafamily.ListRepoBranchesOptions) ([]*giteafamily.Branch, error) {
	_ = ctx
	list, _, err := s.c.ListRepoBranches(owner, repo, forgejo.ListRepoBranchesOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.Branch, 0, len(list))
	for _, b := range list {
		if b != nil {
			out = append(out, &giteafamily.Branch{Name: b.Name})
		}
	}
	return out, err
}

func (s sdkPorts) CreateBranch(ctx context.Context, owner, repo string, opt giteafamily.CreateBranchOption) (*giteafamily.Branch, error) {
	_ = ctx
	b, _, err := s.c.CreateBranch(owner, repo, forgejo.CreateBranchOption{
		BranchName:    opt.BranchName,
		OldBranchName: opt.OldBranchName,
	})
	if b == nil {
		return nil, err
	}
	return &giteafamily.Branch{Name: b.Name}, err
}

func (s sdkPorts) DeleteRepoBranch(ctx context.Context, owner, repo, branch string) error {
	_ = ctx
	_, _, err := s.c.DeleteRepoBranch(owner, repo, branch)
	return err
}

// --- giteafamily.CollaboratorPort ---

func (s sdkPorts) ListCollaborators(ctx context.Context, owner, repo string, opt giteafamily.ListCollaboratorsOptions) ([]*giteafamily.User, error) {
	_ = ctx
	list, _, err := s.c.ListCollaborators(owner, repo, forgejo.ListCollaboratorsOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyUsers(list), err
}

func (s sdkPorts) CollaboratorPermission(ctx context.Context, owner, repo, username string) (string, error) {
	_ = ctx
	perm, _, err := s.c.CollaboratorPermission(owner, repo, username)
	if perm == nil {
		return "", err
	}
	return string(perm.Permission), err
}

func (s sdkPorts) AddCollaborator(ctx context.Context, owner, repo, username string, opt giteafamily.AddCollaboratorOption) error {
	_ = ctx
	aco := forgejo.AddCollaboratorOption{}
	if opt.Permission != nil {
		perm := forgejo.AccessMode(*opt.Permission)
		aco.Permission = &perm
	}
	_, err := s.c.AddCollaborator(owner, repo, username, aco)
	return err
}

func (s sdkPorts) DeleteCollaborator(ctx context.Context, owner, repo, username string) error {
	_ = ctx
	_, err := s.c.DeleteCollaborator(owner, repo, username)
	return err
}

func familyUsers(list []*forgejo.User) []*giteafamily.User {
	out := make([]*giteafamily.User, 0, len(list))
	for _, u := range list {
		if u != nil {
			out = append(out, familyUser(u))
		}
	}
	return out
}

func familyUser(u *forgejo.User) *giteafamily.User {
	if u == nil {
		return nil
	}
	return &giteafamily.User{ID: u.ID, UserName: u.UserName, FullName: u.FullName, AvatarURL: u.AvatarURL}
}

// --- giteafamily.RepoStatsPort ---

func (s sdkPorts) ListForks(ctx context.Context, owner, repo string, opt giteafamily.ListForksOptions) ([]*giteafamily.Repo, error) {
	_ = ctx
	list, _, err := s.c.ListForks(owner, repo, forgejo.ListForksOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyRepos(list), err
}

func (s sdkPorts) ListRepoStargazers(ctx context.Context, owner, repo string, opt giteafamily.ListStargazersOptions) ([]*giteafamily.User, error) {
	_ = ctx
	list, _, err := s.c.ListRepoStargazers(owner, repo, forgejo.ListStargazersOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyUsers(list), err
}

func familyRepo(r *forgejo.Repository) *giteafamily.Repo {
	if r == nil {
		return nil
	}
	owner := ""
	if r.Owner != nil {
		owner = r.Owner.UserName
	}
	return &giteafamily.Repo{
		ID:            r.ID,
		FullName:      r.FullName,
		Name:          r.Name,
		OwnerUserName: owner,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Archived:      r.Archived,
		Fork:          r.Fork,
		Stars:         r.Stars,
		HTMLURL:       r.HTMLURL,
		Forks:         r.Forks,
	}
}

func familyRepos(list []*forgejo.Repository) []*giteafamily.Repo {
	out := make([]*giteafamily.Repo, 0, len(list))
	for _, r := range list {
		out = append(out, familyRepo(r))
	}
	return out
}

// --- giteafamily.NotificationPort ---

func (s sdkPorts) ListNotifications(ctx context.Context, opt giteafamily.ListNotificationOptions) ([]*giteafamily.NotificationThread, error) {
	_ = ctx
	list, _, err := s.c.ListNotifications(forgejoNotificationListOpt(opt))
	return familyThreads(list), err
}

func (s sdkPorts) ListRepoNotifications(ctx context.Context, owner, repo string, opt giteafamily.ListNotificationOptions) ([]*giteafamily.NotificationThread, error) {
	_ = ctx
	list, _, err := s.c.ListRepoNotifications(owner, repo, forgejoNotificationListOpt(opt))
	return familyThreads(list), err
}

func (s sdkPorts) MarkNotificationReadByID(ctx context.Context, id int64) error {
	_ = ctx
	_, _, err := s.c.ReadNotification(id, forgejo.NotifyStatusRead)
	return err
}

func (s sdkPorts) MarkNotificationsRead(ctx context.Context, opt giteafamily.MarkNotificationOptions) error {
	_ = ctx
	_, _, err := s.c.ReadNotifications(forgejo.MarkNotificationOptions{
		LastReadAt: opt.LastReadAt,
		ToStatus:   forgejo.NotifyStatusRead,
	})
	return err
}

func (s sdkPorts) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opt giteafamily.MarkNotificationOptions) error {
	_ = ctx
	_, _, err := s.c.ReadRepoNotifications(owner, repo, forgejo.MarkNotificationOptions{
		LastReadAt: opt.LastReadAt,
		ToStatus:   forgejo.NotifyStatusRead,
	})
	return err
}

func forgejoNotificationListOpt(opt giteafamily.ListNotificationOptions) forgejo.ListNotificationOptions {
	lo := forgejo.ListNotificationOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		Since:       opt.Since,
	}
	if len(opt.Status) > 0 {
		lo.Status = make([]forgejo.NotifyStatus, 0, len(opt.Status))
		for _, st := range opt.Status {
			lo.Status = append(lo.Status, forgejo.NotifyStatus(st))
		}
	}
	return lo
}

func familyThread(t *forgejo.NotificationThread) *giteafamily.NotificationThread {
	if t == nil {
		return nil
	}
	out := &giteafamily.NotificationThread{ID: t.ID, Unread: t.Unread, UpdatedAt: t.UpdatedAt}
	if t.Repository != nil {
		out.RepoID = t.Repository.ID
		out.RepoFullName = t.Repository.FullName
	}
	if t.Subject != nil {
		out.SubjectTitle = t.Subject.Title
		out.SubjectType = string(t.Subject.Type)
		out.SubjectURL = t.Subject.URL
	}
	return out
}

func familyThreads(list []*forgejo.NotificationThread) []*giteafamily.NotificationThread {
	out := make([]*giteafamily.NotificationThread, 0, len(list))
	for _, t := range list {
		out = append(out, familyThread(t))
	}
	return out
}

// --- giteafamily.CommitPort ---

func (s sdkPorts) GetSingleCommit(ctx context.Context, owner, repo, sha string) (*giteafamily.Commit, error) {
	_ = ctx
	c, _, err := s.c.GetSingleCommit(owner, repo, sha)
	return familyCommit(c), err
}

func (s sdkPorts) ListRepoCommits(ctx context.Context, owner, repo string, opt giteafamily.ListCommitOptions) ([]*giteafamily.Commit, error) {
	_ = ctx
	list, _, err := s.c.ListRepoCommits(owner, repo, forgejo.ListCommitOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.Commit, 0, len(list))
	for _, c := range list {
		out = append(out, familyCommit(c))
	}
	return out, err
}

func (s sdkPorts) CompareCommits(ctx context.Context, owner, repo, base, head string) (*giteafamily.CompareResult, error) {
	_ = ctx
	cmp, _, err := s.c.CompareCommits(owner, repo, base, head)
	if cmp == nil {
		return nil, err
	}
	out := &giteafamily.CompareResult{TotalCommits: cmp.TotalCommits}
	for _, c := range cmp.Commits {
		out.Commits = append(out.Commits, familyCommit(c))
	}
	return out, err
}

func (s sdkPorts) CreateStatus(ctx context.Context, owner, repo, sha string, opt giteafamily.CreateStatusOption) error {
	_ = ctx
	_, _, err := s.c.CreateStatus(owner, repo, sha, forgejo.CreateStatusOption{
		State:       forgejo.StatusState(opt.State),
		Context:     opt.Context,
		Description: opt.Description,
		TargetURL:   opt.TargetURL,
	})
	return err
}

func familyCommit(c *forgejo.Commit) *giteafamily.Commit {
	if c == nil {
		return nil
	}
	out := &giteafamily.Commit{}
	if c.CommitMeta != nil {
		out.SHA = c.CommitMeta.SHA
		out.Created = c.CommitMeta.Created
	}
	if c.RepoCommit != nil {
		out.Message = c.RepoCommit.Message
		if c.RepoCommit.Author != nil {
			out.AuthorName = c.RepoCommit.Author.Name
		}
	}
	return out
}

// --- giteafamily.SearchPort ---

func (s sdkPorts) ListIssues(ctx context.Context, opt giteafamily.FamilyIssueListOption) ([]*giteafamily.Issue, error) {
	_ = ctx
	list, _, err := s.c.ListIssues(forgejo.ListIssueOption{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.KeyWord,
		State:       forgejo.StateType(opt.State),
		AssignedBy:  opt.Assignee,
		Labels:      stringx.SplitCSVNonEmpty(opt.Labels),
		Type:        forgejo.IssueType(opt.Type),
	})
	return familyIssues(list), err
}

func (s sdkPorts) SearchUsers(ctx context.Context, opt giteafamily.SearchUserOptions) ([]*giteafamily.User, error) {
	_ = ctx
	list, _, err := s.c.SearchUsers(forgejo.SearchUsersOption{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.Keyword,
	})
	return familyUsers(list), err
}

func familyIssue(i *forgejo.Issue) *giteafamily.Issue {
	if i == nil {
		return nil
	}
	out := &giteafamily.Issue{
		ID:       i.ID,
		Index:    i.Index,
		Title:    i.Title,
		Body:     i.Body,
		State:    string(i.State),
		HTMLURL:  i.HTMLURL,
		Created:  i.Created,
		Updated:  i.Updated,
		Closed:   i.Closed,
		Comments: i.Comments,
	}
	if i.Poster != nil {
		out.Poster = familyUser(i.Poster)
	}
	for _, l := range i.Labels {
		out.Labels = append(out.Labels, familyLabel(l))
	}
	for _, a := range i.Assignees {
		out.Assignees = append(out.Assignees, familyUser(a))
	}
	if i.Milestone != nil {
		out.Milestone = &giteafamily.MilestoneRef{ID: i.Milestone.ID, Title: i.Milestone.Title}
	}
	return out
}

func familyIssues(list []*forgejo.Issue) []*giteafamily.Issue {
	out := make([]*giteafamily.Issue, 0, len(list))
	for _, i := range list {
		out = append(out, familyIssue(i))
	}
	return out
}

func (s sdkPorts) ListStatuses(ctx context.Context, owner, repo, sha string, opt giteafamily.ListStatusesOption) ([]*giteafamily.Status, error) {
	_ = ctx
	list, _, err := s.c.ListStatuses(owner, repo, sha, forgejo.ListStatusesOption{ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize}})
	out := make([]*giteafamily.Status, 0, len(list))
	for _, st := range list {
		if st != nil {
			out = append(out, &giteafamily.Status{State: string(st.State), Context: st.Context, Description: st.Description, TargetURL: st.TargetURL})
		}
	}
	return out, err
}

// --- giteafamily.ReviewPort ---

func (s sdkPorts) ListPullReviews(ctx context.Context, owner, repo string, index int64, opt giteafamily.ListPullReviewsOptions) ([]*giteafamily.PullReview, error) {
	_ = ctx
	list, _, err := s.c.ListPullReviews(owner, repo, index, forgejo.ListPullReviewsOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.PullReview, 0, len(list))
	for _, r := range list {
		out = append(out, familyReview(r))
	}
	return out, err
}

func (s sdkPorts) GetPullReview(ctx context.Context, owner, repo string, index, reviewID int64) (*giteafamily.PullReview, error) {
	_ = ctx
	r, _, err := s.c.GetPullReview(owner, repo, index, reviewID)
	return familyReview(r), err
}

func (s sdkPorts) CreatePullReview(ctx context.Context, owner, repo string, index int64, opt giteafamily.CreatePullReviewOption) (*giteafamily.PullReview, error) {
	_ = ctx
	rc := forgejo.CreatePullReviewOptions{
		State:    forgejo.ReviewStateType(opt.State),
		Body:     opt.Body,
		CommitID: opt.CommitID,
	}
	for _, c := range opt.Comments {
		rc.Comments = append(rc.Comments, forgejo.CreatePullReviewComment{
			Path:       c.Path,
			Body:       c.Body,
			OldLineNum: c.OldLineNum,
			NewLineNum: c.NewLineNum,
		})
	}
	r, _, err := s.c.CreatePullReview(owner, repo, index, rc)
	return familyReview(r), err
}

func (s sdkPorts) CreateReviewRequests(ctx context.Context, owner, repo string, index int64, opt giteafamily.PullReviewRequestOptions) error {
	_ = ctx
	_, err := s.c.CreateReviewRequests(owner, repo, index, forgejo.PullReviewRequestOptions{Reviewers: opt.Reviewers})
	return err
}

func (s sdkPorts) DismissPullReview(ctx context.Context, owner, repo string, index, reviewID int64, opt giteafamily.DismissPullReviewOptions) error {
	_ = ctx
	_, err := s.c.DismissPullReview(owner, repo, index, reviewID, forgejo.DismissPullReviewOptions{Message: opt.Message})
	return err
}

func familyReview(r *forgejo.PullReview) *giteafamily.PullReview {
	if r == nil {
		return nil
	}
	out := &giteafamily.PullReview{
		ID:          r.ID,
		State:       string(r.State),
		Body:        r.Body,
		HTMLURL:     r.HTMLURL,
		SubmittedAt: r.Submitted,
	}
	if r.Reviewer != nil {
		out.Reviewer = familyUser(r.Reviewer)
	}
	return out
}

// --- giteafamily.DiffPort ---

func (s sdkPorts) GetPullRequestDiff(ctx context.Context, owner, repo string, index int64) ([]byte, error) {
	_ = ctx
	data, _, err := s.c.GetPullRequestDiff(owner, repo, index, forgejo.PullRequestDiffOptions{})
	return data, err
}

func (s sdkPorts) ListPullRequestFiles(ctx context.Context, owner, repo string, index int64, opt giteafamily.ListPullRequestFilesOptions) ([]*giteafamily.FamilyChangedFile, error) {
	_ = ctx
	list, _, err := s.c.ListPullRequestFiles(owner, repo, index, forgejo.ListPullRequestFilesOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.FamilyChangedFile, 0, len(list))
	for _, cf := range list {
		out = append(out, familyChangedFile(cf))
	}
	return out, err
}

func (s sdkPorts) CreateIssueComment(ctx context.Context, owner, repo string, index int64, opt giteafamily.CreateIssueCommentOption) (*giteafamily.FamilyComment, int, error) {
	_ = ctx
	c, resp, err := s.c.CreateIssueComment(owner, repo, index, forgejo.CreateIssueCommentOption{Body: opt.Body})
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return familyComment(c), status, err
}

func (s sdkPorts) DeleteIssueComment(ctx context.Context, owner, repo string, id int64) (int, error) {
	_ = ctx
	resp, err := s.c.DeleteIssueComment(owner, repo, id)
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return status, err
}

func familyChangedFile(cf *forgejo.ChangedFile) *giteafamily.FamilyChangedFile {
	if cf == nil {
		return nil
	}
	return &giteafamily.FamilyChangedFile{
		Filename:         cf.Filename,
		PreviousFilename: cf.PreviousFilename,
		Status:           cf.Status,
		Additions:        cf.Additions,
		Deletions:        cf.Deletions,
	}
}

func familyComment(c *forgejo.Comment) *giteafamily.FamilyComment {
	if c == nil {
		return nil
	}
	out := &giteafamily.FamilyComment{ID: c.ID, Body: c.Body, CreatedAt: c.Created, UpdatedAt: c.Updated}
	if c.Poster != nil {
		out.Poster = familyUser(c.Poster)
	}
	return out
}

// --- giteafamily.FilePort ---

func (s sdkPorts) GetFile(ctx context.Context, owner, repo, ref, path string) ([]byte, int, error) {
	_ = ctx
	data, resp, err := s.c.GetFile(owner, repo, ref, path)
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return data, status, err
}

func (s sdkPorts) CreateFile(ctx context.Context, owner, repo, path string, opt giteafamily.FamilyFileOptions) (*giteafamily.FamilyFileResult, error) {
	_ = ctx
	resp, _, err := s.c.CreateFile(owner, repo, path, forgejo.CreateFileOptions{
		FileOptions: forgejo.FileOptions{Message: opt.Message, BranchName: opt.Branch},
		Content:     opt.Content,
	})
	return familyFileResult(resp, err)
}

func (s sdkPorts) UpdateFile(ctx context.Context, owner, repo, path string, opt giteafamily.FamilyFileOptions) (*giteafamily.FamilyFileResult, error) {
	_ = ctx
	resp, _, err := s.c.UpdateFile(owner, repo, path, forgejo.UpdateFileOptions{
		FileOptions: forgejo.FileOptions{Message: opt.Message, BranchName: opt.Branch},
		SHA:         opt.SHA,
		Content:     opt.Content,
	})
	return familyFileResult(resp, err)
}

func (s sdkPorts) DeleteFile(ctx context.Context, owner, repo, path string, opt giteafamily.FamilyFileOptions) (*giteafamily.FamilyFileResult, int, error) {
	_ = ctx
	resp, err := s.c.DeleteFile(owner, repo, path, forgejo.DeleteFileOptions{
		FileOptions: forgejo.FileOptions{Message: opt.Message, BranchName: opt.Branch},
		SHA:         opt.SHA,
	})
	status := 0
	sha := ""
	if resp != nil {
		if resp.Response != nil {
			status = resp.Response.StatusCode
		}
		sha = resp.Header.Get("X-Commit-Sha")
	}
	return &giteafamily.FamilyFileResult{CommitSHA: sha}, status, err
}

func familyFileResult(resp *forgejo.FileResponse, err error) (*giteafamily.FamilyFileResult, error) {
	sha := ""
	if resp != nil && resp.Commit != nil {
		sha = resp.Commit.SHA
	}
	return &giteafamily.FamilyFileResult{CommitSHA: sha}, err
}

// --- giteafamily.RepoPort ---

func (s sdkPorts) SearchRepos(ctx context.Context, opt giteafamily.SearchRepoOptions) ([]*giteafamily.Repo, error) {
	_ = ctx
	list, _, err := s.c.SearchRepos(forgejo.SearchRepoOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		Keyword:     opt.Keyword,
		Sort:        opt.Sort,
		Order:       opt.Order,
	})
	return familyRepos(list), err
}

func (s sdkPorts) ListMyRepos(ctx context.Context, opt giteafamily.FamilyListReposOptions) ([]*giteafamily.Repo, error) {
	_ = ctx
	list, _, err := s.c.ListMyRepos(forgejo.ListReposOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyRepos(list), err
}

func (s sdkPorts) GetRepo(ctx context.Context, owner, repo string) (*giteafamily.Repo, int, error) {
	_ = ctx
	r, resp, err := s.c.GetRepo(owner, repo)
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return familyRepo(r), status, err
}

func (s sdkPorts) CreateRepo(ctx context.Context, opt giteafamily.FamilyCreateRepoOption) (*giteafamily.Repo, error) {
	_ = ctx
	r, _, err := s.c.CreateRepo(forgejo.CreateRepoOption{
		Name:          opt.Name,
		Description:   opt.Description,
		Private:       opt.Private,
		AutoInit:      opt.AutoInit,
		DefaultBranch: opt.DefaultBranch,
	})
	return familyRepo(r), err
}

func (s sdkPorts) CreateOrgRepo(ctx context.Context, org string, opt giteafamily.FamilyCreateRepoOption) (*giteafamily.Repo, error) {
	_ = ctx
	r, _, err := s.c.CreateOrgRepo(org, forgejo.CreateRepoOption{
		Name:          opt.Name,
		Description:   opt.Description,
		Private:       opt.Private,
		AutoInit:      opt.AutoInit,
		DefaultBranch: opt.DefaultBranch,
	})
	return familyRepo(r), err
}

func (s sdkPorts) CreateFork(ctx context.Context, owner, repo string, opt giteafamily.FamilyForkRepoOption) (*giteafamily.Repo, error) {
	_ = ctx
	r, _, err := s.c.CreateFork(owner, repo, forgejo.CreateForkOption{
		Organization: opt.Organization,
		Name:         opt.Name,
	})
	return familyRepo(r), err
}

func (s sdkPorts) DeleteRepo(ctx context.Context, owner, repo string) error {
	_ = ctx
	_, err := s.c.DeleteRepo(owner, repo)
	return err
}

func (s sdkPorts) EditRepo(ctx context.Context, owner, repo string, opt giteafamily.FamilyEditRepoOption) (*giteafamily.Repo, error) {
	_ = ctx
	r, _, err := s.c.EditRepo(owner, repo, forgejo.EditRepoOption{
		Name:          opt.Name,
		Description:   opt.Description,
		DefaultBranch: opt.DefaultBranch,
		Private:       opt.Private,
	})
	return familyRepo(r), err
}

// --- giteafamily.IssuePort ---

func (s sdkPorts) ListRepoIssues(ctx context.Context, owner, repo string, opt giteafamily.FamilyIssueListOption) ([]*giteafamily.Issue, error) {
	_ = ctx // forgejo SDK takes no context
	list, _, err := s.c.ListRepoIssues(owner, repo, forgejo.ListIssueOption{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.KeyWord,
		State:       forgejo.StateType(opt.State),
		AssignedBy:  opt.Assignee,
		Labels:      stringx.SplitCSVNonEmpty(opt.Labels),
		Type:        forgejo.IssueType(opt.Type),
	})
	return familyIssues(list), err
}

func (s sdkPorts) GetIssue(ctx context.Context, owner, repo string, index int64) (*giteafamily.Issue, error) {
	_ = ctx // forgejo SDK takes no context
	i, _, err := s.c.GetIssue(owner, repo, index)
	return familyIssue(i), err
}

func (s sdkPorts) CreateIssue(ctx context.Context, owner, repo string, opt giteafamily.FamilyCreateIssueOption) (*giteafamily.Issue, error) {
	_ = ctx // forgejo SDK takes no context
	i, _, err := s.c.CreateIssue(owner, repo, forgejo.CreateIssueOption{
		Title:     opt.Title,
		Body:      opt.Body,
		Assignees: opt.Assignees,
		Milestone: opt.Milestone,
		Labels:    opt.Labels,
	})
	return familyIssue(i), err
}

func (s sdkPorts) EditIssue(ctx context.Context, owner, repo string, index int64, opt giteafamily.FamilyEditIssueOption) (*giteafamily.Issue, error) {
	_ = ctx // forgejo SDK takes no context
	edit := forgejo.EditIssueOption{Title: opt.Title}
	if opt.Body != nil {
		edit.Body = opt.Body
	}
	if opt.State != nil {
		st := forgejo.StateType(*opt.State)
		edit.State = &st
	}
	edit.Assignees = opt.Assignees
	if opt.Milestone != nil {
		edit.Milestone = opt.Milestone
	}
	i, _, err := s.c.EditIssue(owner, repo, index, edit)
	return familyIssue(i), err
}

func (s sdkPorts) ReplaceIssueLabels(ctx context.Context, owner, repo string, index int64, opt giteafamily.IssueLabelsOption) error {
	_ = ctx // forgejo SDK takes no context
	_, _, err := s.c.ReplaceIssueLabels(owner, repo, index, forgejo.IssueLabelsOption{Labels: opt.Labels})
	return err
}

func (s sdkPorts) AddIssueLabels(ctx context.Context, owner, repo string, index int64, opt giteafamily.IssueLabelsOption) error {
	_ = ctx // forgejo SDK takes no context
	_, _, err := s.c.AddIssueLabels(owner, repo, index, forgejo.IssueLabelsOption{Labels: opt.Labels})
	return err
}

func (s sdkPorts) DeleteIssueLabel(ctx context.Context, owner, repo string, index, labelID int64) error {
	_ = ctx // forgejo SDK takes no context
	_, err := s.c.DeleteIssueLabel(owner, repo, index, labelID)
	return err
}

func (s sdkPorts) ListIssueComments(ctx context.Context, owner, repo string, index int64, opt giteafamily.ListIssueCommentsOptions) ([]*giteafamily.FamilyComment, error) {
	_ = ctx // forgejo SDK takes no context
	list, _, err := s.c.ListIssueComments(owner, repo, index, forgejo.ListIssueCommentOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.FamilyComment, 0, len(list))
	for _, c := range list {
		out = append(out, familyComment(c))
	}
	return out, err
}

func (s sdkPorts) EditIssueComment(ctx context.Context, owner, repo string, commentID int64, opt giteafamily.EditIssueCommentOption) (*giteafamily.FamilyComment, error) {
	_ = ctx // forgejo SDK takes no context
	c, _, err := s.c.EditIssueComment(owner, repo, commentID, forgejo.EditIssueCommentOption{Body: opt.Body})
	return familyComment(c), err
}

// --- giteafamily.PRPort ---

func (s sdkPorts) CreatePullRequest(ctx context.Context, owner, repo string, opt giteafamily.FamilyCreatePullRequestOption) (*giteafamily.FamilyPullRequest, error) {
	_ = ctx
	pr, _, err := s.c.CreatePullRequest(owner, repo, forgejo.CreatePullRequestOption{
		Head:  opt.Head,
		Base:  opt.Base,
		Title: opt.Title,
		Body:  opt.Body,
	})
	return familyPR(pr), err
}

func (s sdkPorts) GetPullRequest(ctx context.Context, owner, repo string, index int64) (*giteafamily.FamilyPullRequest, error) {
	_ = ctx
	pr, _, err := s.c.GetPullRequest(owner, repo, index)
	return familyPR(pr), err
}

func (s sdkPorts) ListPullRequests(ctx context.Context, owner, repo string, opt giteafamily.FamilyListPullRequestsOptions) ([]*giteafamily.FamilyPullRequest, int, error) {
	_ = ctx
	prs, resp, err := s.c.ListRepoPullRequests(owner, repo, forgejo.ListPullRequestsOptions{
		State:       forgejo.StateType(opt.State),
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.FamilyPullRequest, 0, len(prs))
	for _, pr := range prs {
		out = append(out, familyPR(pr))
	}
	total := 0
	if resp != nil {
		total = provider.ParseTotalCountHeader(resp.Header, 0)
	}
	return out, total, err
}

func (s sdkPorts) MergePullRequest(ctx context.Context, owner, repo string, index int64, opt giteafamily.FamilyMergePullRequestOption) (int, error) {
	_ = ctx
	_, resp, err := s.c.MergePullRequest(owner, repo, index, forgejo.MergePullRequestOption{
		Style:                  forgejo.MergeStyle(opt.Style),
		Title:                  opt.Title,
		DeleteBranchAfterMerge: opt.DeleteBranchAfterMerge != nil && *opt.DeleteBranchAfterMerge,
	})
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return status, err
}

func (s sdkPorts) EditPullRequest(ctx context.Context, owner, repo string, index int64, opt giteafamily.FamilyEditPullRequestOption) (*giteafamily.FamilyPullRequest, error) {
	_ = ctx
	edit := forgejo.EditPullRequestOption{
		Title: opt.Title,
		Body:  opt.Body,
		Base:  opt.Base,
	}
	if opt.State != nil {
		st := forgejo.StateType(*opt.State)
		edit.State = &st
	}
	pr, _, err := s.c.EditPullRequest(owner, repo, index, edit)
	return familyPR(pr), err
}

// familyPR is the field-by-field projection of the SDK's PullRequest onto
// the engine's wire shape. Forgejo (older fork) has no RequestedReviewers
// or Draft on the wire; the dialect parameters pick the live subset.
func familyPR(pr *forgejo.PullRequest) *giteafamily.FamilyPullRequest {
	if pr == nil {
		return nil
	}
	out := &giteafamily.FamilyPullRequest{
		ID:        pr.ID,
		Index:     pr.Index,
		Title:     pr.Title,
		Body:      pr.Body,
		State:     string(pr.State),
		HTMLURL:   pr.HTMLURL,
		HasMerged: pr.HasMerged,
		Merged:    pr.Merged,
		MergeBase: pr.MergeBase,
		Created:   pr.Created,
		Updated:   pr.Updated,
		Closed:    pr.Closed,
	}
	if pr.Poster != nil {
		out.Poster = familyUser(pr.Poster)
	}
	for _, l := range pr.Labels {
		out.Labels = append(out.Labels, familyLabel(l))
	}
	for _, a := range pr.Assignees {
		out.Assignees = append(out.Assignees, familyUser(a))
	}
	if pr.Head != nil {
		out.Head = &giteafamily.FamilyPRBranchInfo{Ref: pr.Head.Ref, Sha: pr.Head.Sha}
	}
	if pr.Base != nil {
		out.Base = &giteafamily.FamilyPRBranchInfo{Ref: pr.Base.Ref, Sha: pr.Base.Sha}
	}
	return out
}

func (s sdkPorts) ListPullRequestCommits(ctx context.Context, owner, repo string, index int64, opt giteafamily.ListOptions) ([]*giteafamily.Commit, error) {
	_ = ctx // forgejo SDK takes no context
	list, _, err := s.c.ListPullRequestCommits(owner, repo, index, forgejo.ListPullRequestCommitsOptions{ListOptions: forgejo.ListOptions{Page: opt.Page, PageSize: opt.PageSize}})
	out := make([]*giteafamily.Commit, 0, len(list))
	for _, c := range list {
		out = append(out, familyCommit(c))
	}
	return out, err
}

// --- giteafamily.ReleasePort ---

func (s sdkPorts) ListRepoTags(ctx context.Context, owner, repo string, opt giteafamily.ListOptions) ([]*giteafamily.FamilyTag, error) {
	_ = ctx
	list, _, err := s.c.ListRepoTags(owner, repo, forgejo.ListRepoTagsOptions{
		ListOptions: forgejo.ListOptions{Page: opt.Page, PageSize: opt.PageSize},
	})
	out := make([]*giteafamily.FamilyTag, 0, len(list))
	for _, t := range list {
		ft := &giteafamily.FamilyTag{Name: t.Name}
		if t.Commit != nil {
			ft.CommitSHA = t.Commit.SHA
		}
		out = append(out, ft)
	}
	return out, err
}

func (s sdkPorts) ListReleases(ctx context.Context, owner, repo string, opt giteafamily.ListOptions) ([]*giteafamily.FamilyRelease, error) {
	_ = ctx
	list, _, err := s.c.ListReleases(owner, repo, forgejo.ListReleasesOptions{
		ListOptions: forgejo.ListOptions{Page: opt.Page, PageSize: opt.PageSize},
	})
	return familyReleases(list), err
}

func (s sdkPorts) CreateRelease(ctx context.Context, owner, repo string, opt giteafamily.FamilyCreateReleaseOption) (*giteafamily.FamilyRelease, error) {
	_ = ctx
	r, _, err := s.c.CreateRelease(owner, repo, forgejo.CreateReleaseOption{
		TagName:      opt.TagName,
		Target:       opt.Target,
		Title:        opt.Title,
		Note:         opt.Note,
		IsDraft:      opt.IsDraft,
		IsPrerelease: opt.IsPrerelease,
	})
	return familyRelease(r), err
}

func (s sdkPorts) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*giteafamily.FamilyRelease, error) {
	_ = ctx
	r, _, err := s.c.GetReleaseByTag(owner, repo, tag)
	return familyRelease(r), err
}

func (s sdkPorts) EditRelease(ctx context.Context, owner, repo string, id int64, opt giteafamily.FamilyEditReleaseOption) (*giteafamily.FamilyRelease, error) {
	_ = ctx
	r, _, err := s.c.EditRelease(owner, repo, id, forgejo.EditReleaseOption{
		TagName:      opt.TagName,
		Title:        opt.Title,
		Note:         opt.Note,
		IsDraft:      opt.IsDraft,
		IsPrerelease: opt.IsPrerelease,
	})
	return familyRelease(r), err
}

func (s sdkPorts) DeleteReleaseByTag(ctx context.Context, owner, repo, tag string) error {
	_ = ctx
	_, err := s.c.DeleteReleaseByTag(owner, repo, tag)
	return err
}

func (s sdkPorts) GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error) {
	_ = ctx
	kind := forgejo.TarGZArchive
	if format == "zip" {
		kind = forgejo.ZipArchive
	}
	data, _, err := s.c.GetArchive(owner, repo, ref, kind)
	return data, err
}

func familyRelease(r *forgejo.Release) *giteafamily.FamilyRelease {
	if r == nil {
		return nil
	}
	return &giteafamily.FamilyRelease{
		ID:           r.ID,
		TagName:      r.TagName,
		Title:        r.Title,
		Note:         r.Note,
		URL:          r.URL,
		IsDraft:      r.IsDraft,
		IsPrerelease: r.IsPrerelease,
		CreatedAt:    r.CreatedAt,
		PublishedAt:  r.PublishedAt,
	}
}

func familyReleases(list []*forgejo.Release) []*giteafamily.FamilyRelease {
	out := make([]*giteafamily.FamilyRelease, 0, len(list))
	for _, r := range list {
		out = append(out, familyRelease(r))
	}
	return out
}

// --- giteafamily.ReactionPort ---

// ListIssueReactions returns the full reaction list in one call: the
// forgejo endpoint is served unpaginated (the port's contract is "all
// reactions"; gitea walks its pages internally).
func (s sdkPorts) ListIssueReactions(ctx context.Context, owner, repo string, index int64) ([]*giteafamily.FamilyReaction, error) {
	_ = ctx
	list, _, err := s.c.GetIssueReactions(owner, repo, index)
	return familyReactions(list), err
}

func (s sdkPorts) PostIssueReaction(ctx context.Context, owner, repo string, index int64, emoji string) (*giteafamily.FamilyReaction, error) {
	_ = ctx
	r, _, err := s.c.PostIssueReaction(owner, repo, index, emoji)
	return familyReaction(r), err
}

func (s sdkPorts) DeleteIssueReaction(ctx context.Context, owner, repo string, index int64, content string) error {
	_ = ctx
	_, err := s.c.DeleteIssueReaction(owner, repo, index, content)
	return err
}

func (s sdkPorts) GetIssueCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*giteafamily.FamilyReaction, error) {
	_ = ctx
	list, _, err := s.c.GetIssueCommentReactions(owner, repo, commentID)
	return familyReactions(list), err
}

func (s sdkPorts) PostIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*giteafamily.FamilyReaction, error) {
	_ = ctx
	r, _, err := s.c.PostIssueCommentReaction(owner, repo, commentID, emoji)
	return familyReaction(r), err
}

func (s sdkPorts) DeleteIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, content string) error {
	_ = ctx
	_, err := s.c.DeleteIssueCommentReaction(owner, repo, commentID, content)
	return err
}

func familyReaction(r *forgejo.Reaction) *giteafamily.FamilyReaction {
	if r == nil {
		return nil
	}
	out := &giteafamily.FamilyReaction{Reaction: r.Reaction}
	if r.User != nil {
		out.UserID = r.User.ID
		out.Username = r.User.UserName
	}
	return out
}

func familyReactions(list []*forgejo.Reaction) []*giteafamily.FamilyReaction {
	out := make([]*giteafamily.FamilyReaction, 0, len(list))
	for _, r := range list {
		out = append(out, familyReaction(r))
	}
	return out
}

// --- giteafamily.BranchProtectionPort ---

func (s sdkPorts) ListBranchProtections(ctx context.Context, owner, repo string, opt giteafamily.ListOptions) ([]*giteafamily.FamilyBranchProtection, error) {
	_ = ctx
	list, _, err := s.c.ListBranchProtections(owner, repo, forgejo.ListBranchProtectionsOptions{
		ListOptions: forgejo.ListOptions{Page: opt.Page, PageSize: opt.PageSize},
	})
	out := make([]*giteafamily.FamilyBranchProtection, 0, len(list))
	for _, bp := range list {
		out = append(out, familyBranchProtection(bp))
	}
	return out, err
}

func (s sdkPorts) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*giteafamily.FamilyBranchProtection, error) {
	_ = ctx
	bp, _, err := s.c.GetBranchProtection(owner, repo, branch)
	return familyBranchProtection(bp), err
}

func (s sdkPorts) CreateBranchProtection(ctx context.Context, owner, repo string, opt giteafamily.FamilyCreateBranchProtectionOption) (*giteafamily.FamilyBranchProtection, error) {
	_ = ctx
	bp, _, err := s.c.CreateBranchProtection(owner, repo, forgejo.CreateBranchProtectionOption{
		BranchName:        opt.BranchName,
		RequiredApprovals: opt.RequiredApprovals,
		EnableStatusCheck: opt.EnableStatusCheck,
		EnablePush:        opt.EnablePush,
	})
	return familyBranchProtection(bp), err
}

func (s sdkPorts) EditBranchProtection(ctx context.Context, owner, repo, branch string, opt giteafamily.FamilyEditBranchProtectionOption) (*giteafamily.FamilyBranchProtection, error) {
	_ = ctx
	bp, _, err := s.c.EditBranchProtection(owner, repo, branch, forgejo.EditBranchProtectionOption{
		RequiredApprovals:      opt.RequiredApprovals,
		EnableStatusCheck:      opt.EnableStatusCheck,
		EnablePush:             opt.EnablePush,
		BlockOnRejectedReviews: opt.BlockOnRejectedReviews,
	})
	return familyBranchProtection(bp), err
}

func (s sdkPorts) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	_ = ctx
	_, err := s.c.DeleteBranchProtection(owner, repo, branch)
	return err
}

func familyBranchProtection(bp *forgejo.BranchProtection) *giteafamily.FamilyBranchProtection {
	if bp == nil {
		return nil
	}
	return &giteafamily.FamilyBranchProtection{
		BranchName:             bp.BranchName,
		RequiredApprovals:      bp.RequiredApprovals,
		EnableStatusCheck:      bp.EnableStatusCheck,
		EnablePush:             bp.EnablePush,
		BlockOnRejectedReviews: bp.BlockOnRejectedReviews,
	}
}

// compile-time anchors: the adapter must keep satisfying every family port.
var (
	_ giteafamily.HookPort             = sdkPorts{}
	_ giteafamily.DeployKeyPort        = sdkPorts{}
	_ giteafamily.MilestonePort        = sdkPorts{}
	_ giteafamily.LabelPort            = sdkPorts{}
	_ giteafamily.BranchPort           = sdkPorts{}
	_ giteafamily.CollaboratorPort     = sdkPorts{}
	_ giteafamily.RepoStatsPort        = sdkPorts{}
	_ giteafamily.NotificationPort     = sdkPorts{}
	_ giteafamily.CommitPort           = sdkPorts{}
	_ giteafamily.SearchPort           = sdkPorts{}
	_ giteafamily.ReviewPort           = sdkPorts{}
	_ giteafamily.DiffPort             = sdkPorts{}
	_ giteafamily.FilePort             = sdkPorts{}
	_ giteafamily.RepoPort             = sdkPorts{}
	_ giteafamily.IssuePort            = sdkPorts{}
	_ giteafamily.PRPort               = sdkPorts{}
	_ giteafamily.ReleasePort          = sdkPorts{}
	_ giteafamily.ReactionPort         = sdkPorts{}
	_ giteafamily.BranchProtectionPort = sdkPorts{}
)
