package gitea

import (
	"context"

	gitea "gitea.dev/sdk"

	"github.com/oekit/pkg/stringx"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// giteafamilyDivergences is the Gitea view of the family divergence ledger.
var giteafamilyDivergences = giteafamily.Divergences("Gitea")

// sdkPorts adapts the gitea SDK to the family engine's ports. The gitea SDK
// is the family's upstream vocabulary, so the adapters are direct
// delegation apart from trimming the unused *Response returns and projecting
// the few wire fields the engine consumes.
type sdkPorts struct {
	c *gitea.Client
}

// --- giteafamily.HookPort ---

func (s sdkPorts) CreateRepoHook(ctx context.Context, owner, repo string, opt giteafamily.CreateHookOption) (*giteafamily.Hook, error) {
	h, _, err := s.c.Hooks.CreateRepoHook(ctx, owner, repo, gitea.CreateHookOption{
		Type:   gitea.HookType(opt.Type),
		Config: opt.Config,
		Events: opt.Events,
		Active: opt.Active,
	})
	return familyHook(h), err
}

func (s sdkPorts) DeleteRepoHook(ctx context.Context, owner, repo string, id int64) error {
	_, err := s.c.Hooks.DeleteRepoHook(ctx, owner, repo, id)
	return err
}

func (s sdkPorts) ListRepoHooks(ctx context.Context, owner, repo string, opt giteafamily.ListHooksOptions) ([]*giteafamily.Hook, error) {
	list, _, err := s.c.Hooks.ListRepoHooks(ctx, owner, repo, gitea.ListHooksOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyHooks(list), err
}

func (s sdkPorts) GetRepoHook(ctx context.Context, owner, repo string, id int64) (*giteafamily.Hook, int, error) {
	h, resp, err := s.c.Hooks.GetRepoHook(ctx, owner, repo, id)
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return familyHook(h), status, err
}

func (s sdkPorts) EditRepoHook(ctx context.Context, owner, repo string, id int64, opt giteafamily.EditHookOption) (*giteafamily.Hook, error) {
	// The gitea SDK's edit returns no body either; the engine hydrates the
	// updated hook with a follow-up GetRepoHook.
	_, err := s.c.Hooks.EditRepoHook(ctx, owner, repo, id, gitea.EditHookOption{
		Config: opt.Config,
		Events: opt.Events,
	})
	return nil, err
}

func familyHook(h *gitea.Hook) *giteafamily.Hook {
	if h == nil {
		return nil
	}
	return &giteafamily.Hook{ID: h.ID, Config: h.Config, Events: h.Events}
}

func familyHooks(list []*gitea.Hook) []*giteafamily.Hook {
	out := make([]*giteafamily.Hook, 0, len(list))
	for _, h := range list {
		out = append(out, familyHook(h))
	}
	return out
}

// --- giteafamily.DeployKeyPort ---

func (s sdkPorts) ListDeployKeys(ctx context.Context, owner, repo string, opt giteafamily.ListDeployKeysOptions) ([]*giteafamily.DeployKey, error) {
	list, _, err := s.c.Repositories.ListDeployKeys(ctx, owner, repo, gitea.ListDeployKeysOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyDeployKeys(list), err
}

func (s sdkPorts) CreateDeployKey(ctx context.Context, owner, repo string, opt giteafamily.CreateKeyOption) (*giteafamily.DeployKey, error) {
	k, _, err := s.c.Repositories.CreateDeployKey(ctx, owner, repo, gitea.CreateKeyOption{
		Title:    opt.Title,
		Key:      opt.Key,
		ReadOnly: opt.ReadOnly,
	})
	return familyDeployKey(k), err
}

func (s sdkPorts) DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error {
	_, err := s.c.Repositories.DeleteDeployKey(ctx, owner, repo, keyID)
	return err
}

func familyDeployKey(k *gitea.DeployKey) *giteafamily.DeployKey {
	if k == nil {
		return nil
	}
	return &giteafamily.DeployKey{ID: k.ID, Title: k.Title, Key: k.Key, ReadOnly: k.ReadOnly}
}

func familyDeployKeys(list []*gitea.DeployKey) []*giteafamily.DeployKey {
	out := make([]*giteafamily.DeployKey, 0, len(list))
	for _, k := range list {
		out = append(out, familyDeployKey(k))
	}
	return out
}

// --- giteafamily.MilestonePort ---

func (s sdkPorts) ListMilestones(ctx context.Context, owner, repo string, opt giteafamily.ListMilestoneOption) ([]*giteafamily.Milestone, error) {
	listOpts := gitea.ListMilestoneOption{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		State:       gitea.StateType(opt.State),
	}
	list, _, err := s.c.Repositories.ListMilestones(ctx, owner, repo, listOpts)
	return familyMilestones(list), err
}

func (s sdkPorts) GetMilestone(ctx context.Context, owner, repo string, id int64) (*giteafamily.Milestone, error) {
	m, _, err := s.c.Repositories.GetMilestone(ctx, owner, repo, id)
	return familyMilestone(m), err
}

func (s sdkPorts) CreateMilestone(ctx context.Context, owner, repo string, opt giteafamily.CreateMilestoneOption) (*giteafamily.Milestone, error) {
	m, _, err := s.c.Repositories.CreateMilestone(ctx, owner, repo, gitea.CreateMilestoneOption{
		Title:       opt.Title,
		Description: opt.Description,
		Deadline:    opt.Deadline,
	})
	return familyMilestone(m), err
}

func (s sdkPorts) EditMilestone(ctx context.Context, owner, repo string, id int64, opt giteafamily.EditMilestoneOption) (*giteafamily.Milestone, error) {
	editOpts := gitea.EditMilestoneOption{
		Title:       opt.Title,
		Description: opt.Description,
		Deadline:    opt.Deadline,
	}
	if opt.State != nil {
		state := gitea.StateType(*opt.State)
		editOpts.State = &state
	}
	m, _, err := s.c.Repositories.EditMilestone(ctx, owner, repo, id, editOpts)
	return familyMilestone(m), err
}

func (s sdkPorts) DeleteMilestone(ctx context.Context, owner, repo string, id int64) error {
	_, err := s.c.Repositories.DeleteMilestone(ctx, owner, repo, id)
	return err
}

func familyMilestone(m *gitea.Milestone) *giteafamily.Milestone {
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

func familyMilestones(list []*gitea.Milestone) []*giteafamily.Milestone {
	out := make([]*giteafamily.Milestone, 0, len(list))
	for _, m := range list {
		out = append(out, familyMilestone(m))
	}
	return out
}

// --- giteafamily.LabelPort ---

func (s sdkPorts) ListRepoLabels(ctx context.Context, owner, repo string, opt giteafamily.ListLabelsOptions) ([]*giteafamily.Label, error) {
	list, _, err := s.c.Repositories.ListRepoLabels(ctx, owner, repo, gitea.ListLabelsOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyLabels(list), err
}

func (s sdkPorts) CreateLabel(ctx context.Context, owner, repo string, opt giteafamily.CreateLabelOption) (*giteafamily.Label, error) {
	l, _, err := s.c.Repositories.CreateLabel(ctx, owner, repo, gitea.CreateLabelOption{
		Name:        opt.Name,
		Color:       opt.Color,
		Description: opt.Description,
	})
	return familyLabel(l), err
}

func (s sdkPorts) EditLabel(ctx context.Context, owner, repo string, id int64, opt giteafamily.EditLabelOption) (*giteafamily.Label, error) {
	l, _, err := s.c.Repositories.EditLabel(ctx, owner, repo, id, gitea.EditLabelOption{
		Name:        opt.Name,
		Color:       opt.Color,
		Description: opt.Description,
	})
	return familyLabel(l), err
}

func (s sdkPorts) DeleteLabel(ctx context.Context, owner, repo string, id int64) error {
	_, err := s.c.Repositories.DeleteLabel(ctx, owner, repo, id)
	return err
}

func familyLabel(l *gitea.Label) *giteafamily.Label {
	if l == nil {
		return nil
	}
	return &giteafamily.Label{ID: l.ID, Name: l.Name, Color: l.Color, Description: l.Description}
}

func familyLabels(list []*gitea.Label) []*giteafamily.Label {
	out := make([]*giteafamily.Label, 0, len(list))
	for _, l := range list {
		out = append(out, familyLabel(l))
	}
	return out
}

// --- giteafamily.BranchPort ---

func (s sdkPorts) ListRepoBranches(ctx context.Context, owner, repo string, opt giteafamily.ListRepoBranchesOptions) ([]*giteafamily.Branch, error) {
	list, _, err := s.c.Repositories.ListRepoBranches(ctx, owner, repo, gitea.ListRepoBranchesOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
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
	b, _, err := s.c.Repositories.CreateBranch(ctx, owner, repo, gitea.CreateBranchOption{
		BranchName:    opt.BranchName,
		OldBranchName: opt.OldBranchName,
	})
	if b == nil {
		return nil, err
	}
	return &giteafamily.Branch{Name: b.Name}, err
}

func (s sdkPorts) DeleteRepoBranch(ctx context.Context, owner, repo, branch string) error {
	_, _, err := s.c.Repositories.DeleteRepoBranch(ctx, owner, repo, branch)
	return err
}

// --- giteafamily.CollaboratorPort ---

func (s sdkPorts) ListCollaborators(ctx context.Context, owner, repo string, opt giteafamily.ListCollaboratorsOptions) ([]*giteafamily.User, error) {
	list, _, err := s.c.Repositories.ListCollaborators(ctx, owner, repo, gitea.ListCollaboratorsOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyUsers(list), err
}

func (s sdkPorts) CollaboratorPermission(ctx context.Context, owner, repo, username string) (string, error) {
	perm, _, err := s.c.Repositories.CollaboratorPermission(ctx, owner, repo, username)
	if perm == nil {
		return "", err
	}
	return string(perm.Permission), err
}

func (s sdkPorts) AddCollaborator(ctx context.Context, owner, repo, username string, opt giteafamily.AddCollaboratorOption) error {
	aco := gitea.AddCollaboratorOption{}
	if opt.Permission != nil {
		perm := gitea.AccessMode(*opt.Permission)
		aco.Permission = &perm
	}
	_, err := s.c.Repositories.AddCollaborator(ctx, owner, repo, username, aco)
	return err
}

func (s sdkPorts) DeleteCollaborator(ctx context.Context, owner, repo, username string) error {
	_, err := s.c.Repositories.DeleteCollaborator(ctx, owner, repo, username)
	return err
}

func familyUsers(list []*gitea.User) []*giteafamily.User {
	out := make([]*giteafamily.User, 0, len(list))
	for _, u := range list {
		if u != nil {
			out = append(out, familyUser(u))
		}
	}
	return out
}

func familyUser(u *gitea.User) *giteafamily.User {
	if u == nil {
		return nil
	}
	return &giteafamily.User{ID: u.ID, UserName: u.UserName, FullName: u.FullName, AvatarURL: u.AvatarURL}
}

// --- giteafamily.RepoStatsPort ---

func (s sdkPorts) ListForks(ctx context.Context, owner, repo string, opt giteafamily.ListForksOptions) ([]*giteafamily.Repo, error) {
	list, _, err := s.c.Repositories.ListForks(ctx, owner, repo, gitea.ListForksOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyRepos(list), err
}

func (s sdkPorts) ListRepoStargazers(ctx context.Context, owner, repo string, opt giteafamily.ListStargazersOptions) ([]*giteafamily.User, error) {
	list, _, err := s.c.Repositories.ListRepoStargazers(ctx, owner, repo, gitea.ListStargazersOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyUsers(list), err
}

func familyRepo(r *gitea.Repository) *giteafamily.Repo {
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
		Language:      r.Language,
		HTMLURL:       r.HTMLURL,
		Forks:         r.Forks,
	}
}

func familyRepos(list []*gitea.Repository) []*giteafamily.Repo {
	out := make([]*giteafamily.Repo, 0, len(list))
	for _, r := range list {
		out = append(out, familyRepo(r))
	}
	return out
}

// --- giteafamily.NotificationPort ---

func (s sdkPorts) ListNotifications(ctx context.Context, opt giteafamily.ListNotificationOptions) ([]*giteafamily.NotificationThread, error) {
	list, _, err := s.c.Notifications.List(ctx, giteaNotificationListOpt(opt))
	return familyThreads(list), err
}

func (s sdkPorts) ListRepoNotifications(ctx context.Context, owner, repo string, opt giteafamily.ListNotificationOptions) ([]*giteafamily.NotificationThread, error) {
	list, _, err := s.c.Notifications.ListByRepo(ctx, owner, repo, giteaNotificationListOpt(opt))
	return familyThreads(list), err
}

func (s sdkPorts) MarkNotificationReadByID(ctx context.Context, id int64) error {
	_, _, err := s.c.Notifications.MarkReadByID(ctx, id)
	return err
}

func (s sdkPorts) MarkNotificationsRead(ctx context.Context, opt giteafamily.MarkNotificationOptions) error {
	_, _, err := s.c.Notifications.MarkRead(ctx, gitea.MarkNotificationOptions{LastReadAt: opt.LastReadAt})
	return err
}

func (s sdkPorts) MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opt giteafamily.MarkNotificationOptions) error {
	_, _, err := s.c.Notifications.MarkReadByRepo(ctx, owner, repo, gitea.MarkNotificationOptions{LastReadAt: opt.LastReadAt})
	return err
}

func giteaNotificationListOpt(opt giteafamily.ListNotificationOptions) gitea.ListNotificationOptions {
	lo := gitea.ListNotificationOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		Since:       opt.Since,
	}
	if len(opt.Status) > 0 {
		lo.Status = make([]gitea.NotificationStatus, 0, len(opt.Status))
		for _, st := range opt.Status {
			lo.Status = append(lo.Status, gitea.NotificationStatus(st))
		}
	}
	return lo
}

func familyThread(t *gitea.NotificationThread) *giteafamily.NotificationThread {
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

func familyThreads(list []*gitea.NotificationThread) []*giteafamily.NotificationThread {
	out := make([]*giteafamily.NotificationThread, 0, len(list))
	for _, t := range list {
		out = append(out, familyThread(t))
	}
	return out
}

// --- giteafamily.CommitPort ---

func (s sdkPorts) GetSingleCommit(ctx context.Context, owner, repo, sha string) (*giteafamily.Commit, error) {
	c, _, err := s.c.Repositories.GetSingleCommit(ctx, owner, repo, sha)
	return familyCommit(c), err
}

func (s sdkPorts) ListRepoCommits(ctx context.Context, owner, repo string, opt giteafamily.ListCommitOptions) ([]*giteafamily.Commit, error) {
	list, _, err := s.c.Repositories.ListRepoCommits(ctx, owner, repo, gitea.ListCommitOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.Commit, 0, len(list))
	for _, c := range list {
		out = append(out, familyCommit(c))
	}
	return out, err
}

func (s sdkPorts) CompareCommits(ctx context.Context, owner, repo, base, head string) (*giteafamily.CompareResult, error) {
	cmp, _, err := s.c.Repositories.CompareCommits(ctx, owner, repo, base, head)
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
	_, _, err := s.c.Repositories.CreateStatus(ctx, owner, repo, sha, gitea.CreateStatusOption{
		State:       gitea.StatusState(opt.State),
		Context:     opt.Context,
		Description: opt.Description,
		TargetURL:   opt.TargetURL,
	})
	return err
}

func familyCommit(c *gitea.Commit) *giteafamily.Commit {
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
	list, _, err := s.c.Issues.ListIssues(ctx, gitea.ListIssueOption{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.KeyWord,
		State:       gitea.StateType(opt.State),
		AssignedBy:  opt.Assignee,
		Labels:      stringx.SplitCSVNonEmpty(opt.Labels),
		Type:        gitea.IssueType(opt.Type),
	})
	return familyIssues(list), err
}

func (s sdkPorts) SearchUsers(ctx context.Context, opt giteafamily.SearchUserOptions) ([]*giteafamily.User, error) {
	list, _, err := s.c.Users.SearchUsers(ctx, gitea.SearchUsersOption{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.Keyword,
	})
	return familyUsers(list), err
}

func familyIssue(i *gitea.Issue) *giteafamily.Issue {
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

func familyIssues(list []*gitea.Issue) []*giteafamily.Issue {
	out := make([]*giteafamily.Issue, 0, len(list))
	for _, i := range list {
		out = append(out, familyIssue(i))
	}
	return out
}

func (s sdkPorts) ListStatuses(ctx context.Context, owner, repo, sha string, opt giteafamily.ListStatusesOption) ([]*giteafamily.Status, error) {
	list, _, err := s.c.Repositories.ListStatuses(ctx, owner, repo, sha, gitea.ListStatusesOption{ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize}})
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
	list, _, err := s.c.PullRequests.ListPullReviews(ctx, owner, repo, index, gitea.ListPullReviewsOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.PullReview, 0, len(list))
	for _, r := range list {
		out = append(out, familyReview(r))
	}
	return out, err
}

func (s sdkPorts) GetPullReview(ctx context.Context, owner, repo string, index, reviewID int64) (*giteafamily.PullReview, error) {
	r, _, err := s.c.PullRequests.GetPullReview(ctx, owner, repo, index, reviewID)
	return familyReview(r), err
}

func (s sdkPorts) CreatePullReview(ctx context.Context, owner, repo string, index int64, opt giteafamily.CreatePullReviewOption) (*giteafamily.PullReview, error) {
	rc := gitea.CreatePullReviewOptions{
		State:    gitea.ReviewStateType(opt.State),
		Body:     opt.Body,
		CommitID: opt.CommitID,
	}
	for _, c := range opt.Comments {
		rc.Comments = append(rc.Comments, gitea.CreatePullReviewComment{
			Path:       c.Path,
			Body:       c.Body,
			OldLineNum: c.OldLineNum,
			NewLineNum: c.NewLineNum,
		})
	}
	r, _, err := s.c.PullRequests.CreatePullReview(ctx, owner, repo, index, rc)
	return familyReview(r), err
}

func (s sdkPorts) CreateReviewRequests(ctx context.Context, owner, repo string, index int64, opt giteafamily.PullReviewRequestOptions) error {
	_, err := s.c.PullRequests.CreateReviewRequests(ctx, owner, repo, index, gitea.PullReviewRequestOptions{Reviewers: opt.Reviewers})
	return err
}

func (s sdkPorts) DismissPullReview(ctx context.Context, owner, repo string, index, reviewID int64, opt giteafamily.DismissPullReviewOptions) error {
	_, err := s.c.PullRequests.DismissPullReview(ctx, owner, repo, index, reviewID, gitea.DismissPullReviewOptions{Message: opt.Message})
	return err
}

func familyReview(r *gitea.PullReview) *giteafamily.PullReview {
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
	data, _, err := s.c.PullRequests.GetPullRequestDiff(ctx, owner, repo, index, gitea.PullRequestDiffOptions{})
	return data, err
}

func (s sdkPorts) ListPullRequestFiles(ctx context.Context, owner, repo string, index int64, opt giteafamily.ListPullRequestFilesOptions) ([]*giteafamily.FamilyChangedFile, error) {
	list, _, err := s.c.PullRequests.ListPullRequestFiles(ctx, owner, repo, index, gitea.ListPullRequestFilesOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.FamilyChangedFile, 0, len(list))
	for _, cf := range list {
		out = append(out, familyChangedFile(cf))
	}
	return out, err
}

func (s sdkPorts) CreateIssueComment(ctx context.Context, owner, repo string, index int64, opt giteafamily.CreateIssueCommentOption) (*giteafamily.FamilyComment, int, error) {
	c, resp, err := s.c.Issues.CreateIssueComment(ctx, owner, repo, index, gitea.CreateIssueCommentOption{Body: opt.Body})
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return familyComment(c), status, err
}

func (s sdkPorts) DeleteIssueComment(ctx context.Context, owner, repo string, id int64) (int, error) {
	resp, err := s.c.Issues.DeleteIssueComment(ctx, owner, repo, id)
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return status, err
}

func familyChangedFile(cf *gitea.ChangedFile) *giteafamily.FamilyChangedFile {
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

func familyComment(c *gitea.Comment) *giteafamily.FamilyComment {
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
	data, resp, err := s.c.Repositories.GetFile(ctx, owner, repo, ref, path)
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return data, status, err
}

func (s sdkPorts) CreateFile(ctx context.Context, owner, repo, path string, opt giteafamily.FamilyFileOptions) (*giteafamily.FamilyFileResult, error) {
	resp, _, err := s.c.Repositories.CreateFile(ctx, owner, repo, path, gitea.CreateFileOptions{
		FileOptions: gitea.FileOptions{Message: opt.Message, BranchName: opt.Branch},
		Content:     opt.Content,
	})
	return familyFileResult(resp, err)
}

func (s sdkPorts) UpdateFile(ctx context.Context, owner, repo, path string, opt giteafamily.FamilyFileOptions) (*giteafamily.FamilyFileResult, error) {
	resp, _, err := s.c.Repositories.UpdateFile(ctx, owner, repo, path, gitea.UpdateFileOptions{
		FileOptions: gitea.FileOptions{Message: opt.Message, BranchName: opt.Branch},
		SHA:         opt.SHA,
		Content:     opt.Content,
	})
	return familyFileResult(resp, err)
}

func (s sdkPorts) DeleteFile(ctx context.Context, owner, repo, path string, opt giteafamily.FamilyFileOptions) (*giteafamily.FamilyFileResult, int, error) {
	resp, err := s.c.Repositories.DeleteFile(ctx, owner, repo, path, gitea.DeleteFileOptions{
		FileOptions: gitea.FileOptions{Message: opt.Message, BranchName: opt.Branch},
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

func familyFileResult(resp *gitea.FileResponse, err error) (*giteafamily.FamilyFileResult, error) {
	sha := ""
	if resp != nil && resp.Commit != nil {
		sha = resp.Commit.SHA
	}
	return &giteafamily.FamilyFileResult{CommitSHA: sha}, err
}

// --- giteafamily.RepoPort ---

func (s sdkPorts) SearchRepos(ctx context.Context, opt giteafamily.SearchRepoOptions) ([]*giteafamily.Repo, error) {
	list, _, err := s.c.Repositories.SearchRepos(ctx, gitea.SearchRepoOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		Keyword:     opt.Keyword,
		Sort:        opt.Sort,
		Order:       opt.Order,
	})
	return familyRepos(list), err
}

func (s sdkPorts) ListMyRepos(ctx context.Context, opt giteafamily.FamilyListReposOptions) ([]*giteafamily.Repo, error) {
	list, _, err := s.c.Repositories.ListMyRepos(ctx, gitea.ListReposOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyRepos(list), err
}

func (s sdkPorts) GetRepo(ctx context.Context, owner, repo string) (*giteafamily.Repo, int, error) {
	r, resp, err := s.c.Repositories.GetRepo(ctx, owner, repo)
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return familyRepo(r), status, err
}

func (s sdkPorts) CreateRepo(ctx context.Context, opt giteafamily.FamilyCreateRepoOption) (*giteafamily.Repo, error) {
	r, _, err := s.c.Repositories.CreateRepo(ctx, gitea.CreateRepoOption{
		Name:          opt.Name,
		Description:   opt.Description,
		Private:       opt.Private,
		AutoInit:      opt.AutoInit,
		DefaultBranch: opt.DefaultBranch,
	})
	return familyRepo(r), err
}

func (s sdkPorts) CreateOrgRepo(ctx context.Context, org string, opt giteafamily.FamilyCreateRepoOption) (*giteafamily.Repo, error) {
	r, _, err := s.c.Repositories.CreateOrgRepo(ctx, org, gitea.CreateRepoOption{
		Name:          opt.Name,
		Description:   opt.Description,
		Private:       opt.Private,
		AutoInit:      opt.AutoInit,
		DefaultBranch: opt.DefaultBranch,
	})
	return familyRepo(r), err
}

func (s sdkPorts) CreateFork(ctx context.Context, owner, repo string, opt giteafamily.FamilyForkRepoOption) (*giteafamily.Repo, error) {
	r, _, err := s.c.Repositories.CreateFork(ctx, owner, repo, gitea.CreateForkOption{
		Organization: opt.Organization,
		Name:         opt.Name,
	})
	return familyRepo(r), err
}

func (s sdkPorts) DeleteRepo(ctx context.Context, owner, repo string) error {
	_, err := s.c.Repositories.DeleteRepo(ctx, owner, repo)
	return err
}

func (s sdkPorts) EditRepo(ctx context.Context, owner, repo string, opt giteafamily.FamilyEditRepoOption) (*giteafamily.Repo, error) {
	r, _, err := s.c.Repositories.EditRepo(ctx, owner, repo, gitea.EditRepoOption{
		Name:          opt.Name,
		Description:   opt.Description,
		DefaultBranch: opt.DefaultBranch,
		Private:       opt.Private,
	})
	return familyRepo(r), err
}

// --- giteafamily.IssuePort ---

func (s sdkPorts) ListRepoIssues(ctx context.Context, owner, repo string, opt giteafamily.FamilyIssueListOption) ([]*giteafamily.Issue, error) {
	list, _, err := s.c.Issues.ListRepoIssues(ctx, owner, repo, gitea.ListIssueOption{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.KeyWord,
		State:       gitea.StateType(opt.State),
		AssignedBy:  opt.Assignee,
		Labels:      stringx.SplitCSVNonEmpty(opt.Labels),
		Type:        gitea.IssueType(opt.Type),
	})
	return familyIssues(list), err
}

func (s sdkPorts) GetIssue(ctx context.Context, owner, repo string, index int64) (*giteafamily.Issue, error) {
	i, _, err := s.c.Issues.GetIssue(ctx, owner, repo, index)
	return familyIssue(i), err
}

func (s sdkPorts) CreateIssue(ctx context.Context, owner, repo string, opt giteafamily.FamilyCreateIssueOption) (*giteafamily.Issue, error) {
	i, _, err := s.c.Issues.CreateIssue(ctx, owner, repo, gitea.CreateIssueOption{
		Title:     opt.Title,
		Body:      opt.Body,
		Assignees: opt.Assignees,
		Milestone: opt.Milestone,
		Labels:    opt.Labels,
	})
	return familyIssue(i), err
}

func (s sdkPorts) EditIssue(ctx context.Context, owner, repo string, index int64, opt giteafamily.FamilyEditIssueOption) (*giteafamily.Issue, error) {
	edit := gitea.EditIssueOption{Title: opt.Title}
	if opt.Body != nil {
		edit.Body = opt.Body
	}
	if opt.State != nil {
		st := gitea.StateType(*opt.State)
		edit.State = &st
	}
	edit.Assignees = opt.Assignees
	if opt.Milestone != nil {
		edit.Milestone = opt.Milestone
	}
	i, _, err := s.c.Issues.EditIssue(ctx, owner, repo, index, edit)
	return familyIssue(i), err
}

func (s sdkPorts) ReplaceIssueLabels(ctx context.Context, owner, repo string, index int64, opt giteafamily.IssueLabelsOption) error {
	_, _, err := s.c.Issues.ReplaceIssueLabels(ctx, owner, repo, index, gitea.IssueLabelsOption{Labels: opt.Labels})
	return err
}

func (s sdkPorts) AddIssueLabels(ctx context.Context, owner, repo string, index int64, opt giteafamily.IssueLabelsOption) error {
	_, _, err := s.c.Issues.AddIssueLabels(ctx, owner, repo, index, gitea.IssueLabelsOption{Labels: opt.Labels})
	return err
}

func (s sdkPorts) DeleteIssueLabel(ctx context.Context, owner, repo string, index, labelID int64) error {
	_, err := s.c.Issues.DeleteIssueLabel(ctx, owner, repo, index, labelID)
	return err
}

func (s sdkPorts) ListIssueComments(ctx context.Context, owner, repo string, index int64, opt giteafamily.ListIssueCommentsOptions) ([]*giteafamily.FamilyComment, error) {
	list, _, err := s.c.Issues.ListIssueComments(ctx, owner, repo, index, gitea.ListIssueCommentOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	out := make([]*giteafamily.FamilyComment, 0, len(list))
	for _, c := range list {
		out = append(out, familyComment(c))
	}
	return out, err
}

func (s sdkPorts) EditIssueComment(ctx context.Context, owner, repo string, commentID int64, opt giteafamily.EditIssueCommentOption) (*giteafamily.FamilyComment, error) {
	c, _, err := s.c.Issues.EditIssueComment(ctx, owner, repo, commentID, gitea.EditIssueCommentOption{Body: opt.Body})
	return familyComment(c), err
}

// --- giteafamily.PRPort ---

func (s sdkPorts) CreatePullRequest(ctx context.Context, owner, repo string, opt giteafamily.FamilyCreatePullRequestOption) (*giteafamily.FamilyPullRequest, error) {
	pr, _, err := s.c.PullRequests.CreatePullRequest(ctx, owner, repo, gitea.CreatePullRequestOption{
		Head:  opt.Head,
		Base:  opt.Base,
		Title: opt.Title,
		Body:  opt.Body,
	})
	return familyPR(pr), err
}

func (s sdkPorts) GetPullRequest(ctx context.Context, owner, repo string, index int64) (*giteafamily.FamilyPullRequest, error) {
	pr, _, err := s.c.PullRequests.GetPullRequest(ctx, owner, repo, index)
	return familyPR(pr), err
}

func (s sdkPorts) ListPullRequests(ctx context.Context, owner, repo string, opt giteafamily.FamilyListPullRequestsOptions) ([]*giteafamily.FamilyPullRequest, int, error) {
	prs, resp, err := s.c.PullRequests.ListRepoPullRequests(ctx, owner, repo, gitea.ListPullRequestsOptions{
		State:       gitea.StateType(opt.State),
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
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
	_, resp, err := s.c.PullRequests.MergePullRequest(ctx, owner, repo, index, gitea.MergePullRequestOption{
		Style:                  gitea.MergeStyle(opt.Style),
		Title:                  opt.Title,
		DeleteBranchAfterMerge: opt.DeleteBranchAfterMerge,
	})
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.Response.StatusCode
	}
	return status, err
}

func (s sdkPorts) EditPullRequest(ctx context.Context, owner, repo string, index int64, opt giteafamily.FamilyEditPullRequestOption) (*giteafamily.FamilyPullRequest, error) {
	edit := gitea.EditPullRequestOption{
		Title: opt.Title,
		Body:  opt.Body,
		Base:  opt.Base,
	}
	if opt.State != nil {
		st := gitea.StateType(*opt.State)
		edit.State = &st
	}
	pr, _, err := s.c.PullRequests.EditPullRequest(ctx, owner, repo, index, edit)
	return familyPR(pr), err
}

// familyPR is the field-by-field projection of the SDK's PullRequest onto
// the engine's wire shape. Gitea carries RequestedReviewers and Draft;
// Forgejo does not (the dialect parameters pick the live subset).
func familyPR(pr *gitea.PullRequest) *giteafamily.FamilyPullRequest {
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
		Draft:     pr.Draft,
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
	for _, r := range pr.RequestedReviewers {
		out.RequestedReviewers = append(out.RequestedReviewers, familyUser(r))
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
	list, _, err := s.c.PullRequests.ListPullRequestCommits(ctx, owner, repo, index, gitea.ListPullRequestCommitsOptions{ListOptions: gitea.ListOptions{Page: opt.Page, PageSize: opt.PageSize}})
	out := make([]*giteafamily.Commit, 0, len(list))
	for _, c := range list {
		out = append(out, familyCommit(c))
	}
	return out, err
}

// --- giteafamily.ReleasePort ---

func (s sdkPorts) ListRepoTags(ctx context.Context, owner, repo string, opt giteafamily.ListOptions) ([]*giteafamily.FamilyTag, error) {
	list, _, err := s.c.Repositories.ListRepoTags(ctx, owner, repo, gitea.ListRepoTagsOptions{
		ListOptions: gitea.ListOptions{Page: opt.Page, PageSize: opt.PageSize},
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
	list, _, err := s.c.Releases.ListReleases(ctx, owner, repo, gitea.ListReleasesOptions{
		ListOptions: gitea.ListOptions{Page: opt.Page, PageSize: opt.PageSize},
	})
	return familyReleases(list), err
}

func (s sdkPorts) CreateRelease(ctx context.Context, owner, repo string, opt giteafamily.FamilyCreateReleaseOption) (*giteafamily.FamilyRelease, error) {
	r, _, err := s.c.Releases.CreateRelease(ctx, owner, repo, gitea.CreateReleaseOption{
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
	r, _, err := s.c.Releases.GetReleaseByTag(ctx, owner, repo, tag)
	return familyRelease(r), err
}

func (s sdkPorts) EditRelease(ctx context.Context, owner, repo string, id int64, opt giteafamily.FamilyEditReleaseOption) (*giteafamily.FamilyRelease, error) {
	r, _, err := s.c.Releases.EditRelease(ctx, owner, repo, id, gitea.EditReleaseOption{
		TagName:      opt.TagName,
		Title:        opt.Title,
		Note:         opt.Note,
		IsDraft:      opt.IsDraft,
		IsPrerelease: opt.IsPrerelease,
	})
	return familyRelease(r), err
}

func (s sdkPorts) DeleteReleaseByTag(ctx context.Context, owner, repo, tag string) error {
	_, err := s.c.Releases.DeleteReleaseByTag(ctx, owner, repo, tag)
	return err
}

func (s sdkPorts) GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error) {
	kind := gitea.TarGZArchive
	if format == "zip" {
		kind = gitea.ZipArchive
	}
	data, _, err := s.c.Repositories.GetArchive(ctx, owner, repo, ref, kind)
	return data, err
}

func familyRelease(r *gitea.Release) *giteafamily.FamilyRelease {
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

func familyReleases(list []*gitea.Release) []*giteafamily.FamilyRelease {
	out := make([]*giteafamily.FamilyRelease, 0, len(list))
	for _, r := range list {
		out = append(out, familyRelease(r))
	}
	return out
}

// --- giteafamily.ReactionPort ---

// ListIssueReactions returns the FULL reaction list by walking the gitea
// SDK's paginated endpoint (the port's contract is "all reactions"; forgejo
// serves the same list unpaginated).
func (s sdkPorts) ListIssueReactions(ctx context.Context, owner, repo string, index int64) ([]*giteafamily.FamilyReaction, error) {
	list, err := backendutil.AllPages(func(page int) ([]*gitea.Reaction, error) {
		page1, _, err := s.c.Issues.ListIssueReactions(ctx, owner, repo, index, gitea.ListIssueReactionsOptions{
			ListOptions: gitea.ListOptions{Page: page, PageSize: backendutil.LabelPageSize},
		})
		return page1, err
	})
	return familyReactions(list), err
}

func (s sdkPorts) PostIssueReaction(ctx context.Context, owner, repo string, index int64, emoji string) (*giteafamily.FamilyReaction, error) {
	r, _, err := s.c.Issues.PostIssueReaction(ctx, owner, repo, index, emoji)
	return familyReaction(r), err
}

func (s sdkPorts) DeleteIssueReaction(ctx context.Context, owner, repo string, index int64, content string) error {
	_, err := s.c.Issues.DeleteIssueReaction(ctx, owner, repo, index, content)
	return err
}

func (s sdkPorts) GetIssueCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*giteafamily.FamilyReaction, error) {
	list, _, err := s.c.Issues.GetIssueCommentReactions(ctx, owner, repo, commentID)
	return familyReactions(list), err
}

func (s sdkPorts) PostIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*giteafamily.FamilyReaction, error) {
	r, _, err := s.c.Issues.PostIssueCommentReaction(ctx, owner, repo, commentID, emoji)
	return familyReaction(r), err
}

func (s sdkPorts) DeleteIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, content string) error {
	_, err := s.c.Issues.DeleteIssueCommentReaction(ctx, owner, repo, commentID, content)
	return err
}

func familyReaction(r *gitea.Reaction) *giteafamily.FamilyReaction {
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

func familyReactions(list []*gitea.Reaction) []*giteafamily.FamilyReaction {
	out := make([]*giteafamily.FamilyReaction, 0, len(list))
	for _, r := range list {
		out = append(out, familyReaction(r))
	}
	return out
}

// --- giteafamily.BranchProtectionPort ---

func (s sdkPorts) ListBranchProtections(ctx context.Context, owner, repo string, opt giteafamily.ListOptions) ([]*giteafamily.FamilyBranchProtection, error) {
	list, _, err := s.c.Repositories.ListBranchProtections(ctx, owner, repo, gitea.ListBranchProtectionsOptions{
		ListOptions: gitea.ListOptions{Page: opt.Page, PageSize: opt.PageSize},
	})
	out := make([]*giteafamily.FamilyBranchProtection, 0, len(list))
	for _, bp := range list {
		out = append(out, familyBranchProtection(bp))
	}
	return out, err
}

func (s sdkPorts) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*giteafamily.FamilyBranchProtection, error) {
	bp, _, err := s.c.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	return familyBranchProtection(bp), err
}

func (s sdkPorts) CreateBranchProtection(ctx context.Context, owner, repo string, opt giteafamily.FamilyCreateBranchProtectionOption) (*giteafamily.FamilyBranchProtection, error) {
	bp, _, err := s.c.Repositories.CreateBranchProtection(ctx, owner, repo, gitea.CreateBranchProtectionOption{
		BranchName:        opt.BranchName,
		RequiredApprovals: opt.RequiredApprovals,
		EnableStatusCheck: opt.EnableStatusCheck,
		EnablePush:        opt.EnablePush,
	})
	return familyBranchProtection(bp), err
}

func (s sdkPorts) EditBranchProtection(ctx context.Context, owner, repo, branch string, opt giteafamily.FamilyEditBranchProtectionOption) (*giteafamily.FamilyBranchProtection, error) {
	bp, _, err := s.c.Repositories.EditBranchProtection(ctx, owner, repo, branch, gitea.EditBranchProtectionOption{
		RequiredApprovals:      opt.RequiredApprovals,
		EnableStatusCheck:      opt.EnableStatusCheck,
		EnablePush:             opt.EnablePush,
		BlockOnRejectedReviews: opt.BlockOnRejectedReviews,
	})
	return familyBranchProtection(bp), err
}

func (s sdkPorts) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	_, err := s.c.Repositories.DeleteBranchProtection(ctx, owner, repo, branch)
	return err
}

func familyBranchProtection(bp *gitea.BranchProtection) *giteafamily.FamilyBranchProtection {
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
