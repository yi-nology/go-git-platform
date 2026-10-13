package forgejo

import (
	"context"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
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

func (s sdkPorts) ListIssues(ctx context.Context, opt giteafamily.ListIssueOption) ([]*giteafamily.Issue, error) {
	_ = ctx
	list, _, err := s.c.ListIssues(forgejoIssueListOpt(opt))
	return familyIssues(list), err
}

func (s sdkPorts) ListRepoIssues(ctx context.Context, owner, repo string, opt giteafamily.ListIssueOption) ([]*giteafamily.Issue, error) {
	_ = ctx
	list, _, err := s.c.ListRepoIssues(owner, repo, forgejoIssueListOpt(opt))
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

func forgejoIssueListOpt(opt giteafamily.ListIssueOption) forgejo.ListIssueOption {
	return forgejo.ListIssueOption{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		KeyWord:     opt.KeyWord,
		State:       forgejo.StateType(opt.State),
		Type:        forgejo.IssueType(opt.Type),
	}
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
