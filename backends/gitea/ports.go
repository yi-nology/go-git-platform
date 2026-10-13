package gitea

import (
	"context"

	gitea "gitea.dev/sdk"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
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

func (s sdkPorts) SearchRepos(ctx context.Context, opt giteafamily.SearchRepoOptions) ([]*giteafamily.Repo, error) {
	list, _, err := s.c.Repositories.SearchRepos(ctx, gitea.SearchRepoOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		Keyword:     opt.Keyword,
		Sort:        opt.Sort,
		Order:       opt.Order,
	})
	return familyRepos(list), err
}

func (s sdkPorts) ListIssues(ctx context.Context, opt giteafamily.ListIssueOption) ([]*giteafamily.Issue, error) {
	list, _, err := s.c.Issues.ListIssues(ctx, giteaIssueListOpt(opt))
	return familyIssues(list), err
}

func (s sdkPorts) ListRepoIssues(ctx context.Context, owner, repo string, opt giteafamily.ListIssueOption) ([]*giteafamily.Issue, error) {
	list, _, err := s.c.Issues.ListRepoIssues(ctx, owner, repo, giteaIssueListOpt(opt))
	return familyIssues(list), err
}

func (s sdkPorts) SearchUsers(ctx context.Context, opt giteafamily.SearchUserOptions) ([]*giteafamily.User, error) {
	list, _, err := s.c.Users.SearchUsers(ctx, gitea.SearchUsersOption{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.Keyword,
	})
	return familyUsers(list), err
}

func giteaIssueListOpt(opt giteafamily.ListIssueOption) gitea.ListIssueOption {
	return gitea.ListIssueOption{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.KeyWord,
		State:       gitea.StateType(opt.State),
		Type:        gitea.IssueType(opt.Type),
	}
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

// compile-time anchors: the adapter must keep satisfying every family port.
var (
	_ giteafamily.HookPort         = sdkPorts{}
	_ giteafamily.DeployKeyPort    = sdkPorts{}
	_ giteafamily.MilestonePort    = sdkPorts{}
	_ giteafamily.LabelPort        = sdkPorts{}
	_ giteafamily.BranchPort       = sdkPorts{}
	_ giteafamily.CollaboratorPort = sdkPorts{}
	_ giteafamily.RepoStatsPort    = sdkPorts{}
	_ giteafamily.NotificationPort = sdkPorts{}
	_ giteafamily.CommitPort       = sdkPorts{}
	_ giteafamily.SearchPort       = sdkPorts{}
)
