package giteafamily

import (
	"context"
	"strconv"
	"time"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- notifications ---

// NotificationThread is the slice of the family's notification object the
// engine consumes.
type NotificationThread struct {
	ID           int64
	Unread       bool
	RepoID       int64
	RepoFullName string
	SubjectTitle string
	SubjectType  string
	SubjectURL   string
	UpdatedAt    time.Time
}

// ListNotificationOptions mirrors the family's notification list filters
// (since + status vocabulary), pagination applied by the caller.
type ListNotificationOptions struct {
	ListOptions ListOptions
	// Status carries the family's read-state vocabulary: the engine passes
	// all three states when the caller wants All.
	Status []string
	Since  time.Time
}

// MarkNotificationOptions mirrors the family's mark-read body.
type MarkNotificationOptions struct {
	LastReadAt time.Time
}

// NotificationPort is the notification endpoint of the family SDK surface.
type NotificationPort interface {
	ListNotifications(ctx context.Context, opt ListNotificationOptions) ([]*NotificationThread, error)
	ListRepoNotifications(ctx context.Context, owner, repo string, opt ListNotificationOptions) ([]*NotificationThread, error)
	MarkNotificationReadByID(ctx context.Context, id int64) error
	MarkNotificationsRead(ctx context.Context, opt MarkNotificationOptions) error
	MarkRepoNotificationsRead(ctx context.Context, owner, repo string, opt MarkNotificationOptions) error
}

// notificationReadStates is the family's full read-state vocabulary
// (read/unread/pinned), passed when the caller wants every notification.
var notificationReadStates = []string{"read", "unread", "pinned"}

// buildListNotificationOptions maps provider options onto the family's
// list filters; pagination is applied by the callers.
func buildListNotificationOptions(opts provider.ListNotificationsOptions) ListNotificationOptions {
	listOpts := ListNotificationOptions{}
	if opts.Since != "" {
		listOpts.Since, _ = time.Parse(time.RFC3339, opts.Since)
	}
	if opts.All {
		listOpts.Status = notificationReadStates
	}
	return listOpts
}

// ListNotifications lists notifications across the account.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func ListNotifications(ctx context.Context, f Family, c NotificationPort, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	listOpts := buildListNotificationOptions(opts)
	threads, err := backendutil.PageList(opts.Page, opts.PerPage, f.PageSize,
		func(page, perPage int) ([]*NotificationThread, error) {
			listOpts.ListOptions = ListOptions{Page: page, PageSize: perPage}
			return c.ListNotifications(ctx, listOpts)
		})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListNotifications", err)
	}
	result := make([]*provider.Notification, 0, len(threads))
	for _, t := range threads {
		result = append(result, convertNotification(t))
	}
	return result, nil
}

// ListRepoNotifications lists one repository's notifications (dual-mode
// pagination as in ListNotifications).
func ListRepoNotifications(ctx context.Context, f Family, c NotificationPort, owner, repo string, opts provider.ListNotificationsOptions) ([]*provider.Notification, error) {
	listOpts := buildListNotificationOptions(opts)
	threads, err := backendutil.PageList(opts.Page, opts.PerPage, f.PageSize,
		func(page, perPage int) ([]*NotificationThread, error) {
			listOpts.ListOptions = ListOptions{Page: page, PageSize: perPage}
			return c.ListRepoNotifications(ctx, owner, repo, listOpts)
		})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListRepoNotifications", err)
	}
	result := make([]*provider.Notification, 0, len(threads))
	for _, t := range threads {
		result = append(result, convertNotification(t))
	}
	return result, nil
}

// MarkNotificationRead marks one thread read.
func MarkNotificationRead(ctx context.Context, f Family, c NotificationPort, threadID string) error {
	id, err := strconv.ParseInt(threadID, 10, 64)
	if err != nil {
		return provider.Wrapf(f.Platform, "MarkNotificationRead", "invalid thread ID %q", threadID)
	}
	return provider.Wrap(f.Platform, "MarkNotificationRead", c.MarkNotificationReadByID(ctx, id))
}

// MarkNotificationsRead marks every notification read.
func MarkNotificationsRead(ctx context.Context, f Family, c NotificationPort, opts provider.MarkNotificationsOptions) error {
	markOpts := MarkNotificationOptions{}
	if opts.LastReadAt != "" {
		markOpts.LastReadAt, _ = time.Parse(time.RFC3339, opts.LastReadAt)
	}
	return provider.Wrap(f.Platform, "MarkNotificationsRead", c.MarkNotificationsRead(ctx, markOpts))
}

// MarkRepoNotificationsRead marks one repository's notifications read.
func MarkRepoNotificationsRead(ctx context.Context, f Family, c NotificationPort, owner, repo string, opts provider.MarkNotificationsOptions) error {
	markOpts := MarkNotificationOptions{}
	if opts.LastReadAt != "" {
		markOpts.LastReadAt, _ = time.Parse(time.RFC3339, opts.LastReadAt)
	}
	return provider.Wrap(f.Platform, "MarkRepoNotificationsRead", c.MarkRepoNotificationsRead(ctx, owner, repo, markOpts))
}

// convertNotification maps a family notification thread to the provider
// shape.
func convertNotification(t *NotificationThread) *provider.Notification {
	n := &provider.Notification{
		ID:     strconv.FormatInt(t.ID, 10),
		Unread: t.Unread,
	}
	n.Subject = provider.NotificationSubject{
		Title: t.SubjectTitle,
		Type:  t.SubjectType,
		URL:   t.SubjectURL,
	}
	n.Repo = &provider.EventRepo{ID: t.RepoID, FullName: t.RepoFullName}
	n.Repo.Owner, n.Repo.Name = provider.SplitFullName(t.RepoFullName)
	n.UpdatedAt = t.UpdatedAt
	return n
}

// --- commits ---

// Commit is the slice of the family's commit object the engine consumes
// (the family's Commit wraps CommitMeta + RepoCommit).
type Commit struct {
	SHA        string
	Message    string
	AuthorName string
	Created    time.Time
}

// CompareResult mirrors the family's compare response.
type CompareResult struct {
	TotalCommits int
	Commits      []*Commit
}

// ListCommitOptions mirrors the family's commit list query.
type ListCommitOptions struct {
	ListOptions ListOptions
}

// CreateStatusOption mirrors the family's commit-status create body. The
// family's state vocabulary is success/failure/pending/error; the empty
// state defaults to pending (matching the backends' historical behavior).
type CreateStatusOption struct {
	State       string
	Context     string
	Description string
	TargetURL   string
}

// Status is the slice of the family's commit-status object the engine
// consumes.
type Status struct {
	State       string
	Context     string
	Description string
	TargetURL   string
}

// ListStatusesOption mirrors the family's status list query.
type ListStatusesOption struct {
	ListOptions ListOptions
}

// CommitPort is the commit + commit-status endpoint of the family SDK
// surface.
type CommitPort interface {
	GetSingleCommit(ctx context.Context, owner, repo, sha string) (*Commit, error)
	ListRepoCommits(ctx context.Context, owner, repo string, opt ListCommitOptions) ([]*Commit, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string) (*CompareResult, error)
	CreateStatus(ctx context.Context, owner, repo, sha string, opt CreateStatusOption) error
	ListStatuses(ctx context.Context, owner, repo, sha string, opt ListStatusesOption) ([]*Status, error)
}

// GetCommit fetches one commit.
func GetCommit(ctx context.Context, f Family, c CommitPort, owner, repo, sha string) (*provider.CommitInfo, error) {
	cc, err := c.GetSingleCommit(ctx, owner, repo, sha)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetCommit", err)
	}
	return convertCommit(cc), nil
}

// ListCommits lists commits (dual-mode pagination: Page == 0 walks every
// page; Page > 0 returns exactly that single caller-driven page).
func ListCommits(ctx context.Context, f Family, c CommitPort, owner, repo string, opts provider.ListCommitsOptions) ([]*provider.CommitInfo, error) {
	commits, err := backendutil.PageList(opts.Page, opts.PerPage, f.PageSize,
		func(page, perPage int) ([]*Commit, error) {
			return c.ListRepoCommits(ctx, owner, repo, ListCommitOptions{
				ListOptions: ListOptions{Page: page, PageSize: perPage},
			})
		})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListCommits", err)
	}
	result := make([]*provider.CommitInfo, 0, len(commits))
	for _, c := range commits {
		result = append(result, convertCommit(c))
	}
	return result, nil
}

// CompareCommits compares base..head.
func CompareCommits(ctx context.Context, f Family, c CommitPort, owner, repo, base, head string) (*provider.CompareResult, error) {
	cmp, err := c.CompareCommits(ctx, owner, repo, base, head)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CompareCommits", err)
	}
	result := &provider.CompareResult{TotalCommits: cmp.TotalCommits}
	for _, c := range cmp.Commits {
		result.Commits = append(result.Commits, convertCommit(c))
	}
	return result, nil
}

// commitStatusStates maps the provider's write vocabulary onto the
// family's status-state strings (empty/unknown defaults to pending).
var commitStatusStates = map[string]string{
	"success": "success",
	"failed":  "failure",
	"pending": "pending",
	"error":   "error",
}

// CreateCommitStatus creates a commit status.
func CreateCommitStatus(ctx context.Context, f Family, c CommitPort, owner, repo, sha string, opts provider.CommitStatusOptions) error {
	state := commitStatusStates[opts.State]
	if state == "" {
		state = "pending"
	}
	if err := c.CreateStatus(ctx, owner, repo, sha, CreateStatusOption{
		State:       state,
		Context:     opts.Context,
		Description: opts.Description,
		TargetURL:   opts.TargetURL,
	}); err != nil {
		return provider.Wrap(f.Platform, "CreateCommitStatus", err)
	}
	return nil
}

// ListCommitStatuses lists every status reported on a commit (the provider
// surface carries no pagination, so the walk exhausts the endpoint).
func ListCommitStatuses(ctx context.Context, f Family, c CommitPort, owner, repo, sha string) ([]provider.CommitStatus, error) {
	statuses, err := backendutil.AllPages(func(page int) ([]*Status, error) {
		return c.ListStatuses(ctx, owner, repo, sha, ListStatusesOption{
			ListOptions: ListOptions{Page: page, PageSize: backendutil.LabelPageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListCommitStatuses", err)
	}
	result := make([]provider.CommitStatus, 0, len(statuses))
	for _, s := range statuses {
		if s == nil {
			continue
		}
		result = append(result, provider.CommitStatus{
			State:       provider.NormalizeCommitStatusState(s.State),
			Context:     s.Context,
			Description: s.Description,
			TargetURL:   s.TargetURL,
		})
	}
	return result, nil
}

// convertCommit maps a family Commit to the provider shape.
func convertCommit(c *Commit) *provider.CommitInfo {
	if c == nil {
		return nil
	}
	ci := &provider.CommitInfo{SHA: c.SHA, CreatedAt: c.Created}
	if c.Message != "" {
		ci.Message = c.Message
	}
	if c.AuthorName != "" {
		ci.Author = &provider.CRUser{Name: c.AuthorName}
	}
	return ci
}

// --- search ---

// SearchRepoOptions mirrors the family's repo search query.
type SearchRepoOptions struct {
	ListOptions ListOptions
	Keyword     string
	Sort        string
	Order       string
}

// (FamilyIssueListOption in issues.go is the full issue-list filter; the
// search port shares it so IssuePort and SearchPort stay interchangeable.)

// Issue is the slice of the family's issue object the engine consumes.
type Issue struct {
	ID        int64
	Index     int64
	Title     string
	Body      string
	State     string
	HTMLURL   string
	Poster    *User
	Labels    []*Label
	Assignees []*User
	Milestone *MilestoneRef
	Created   time.Time
	Updated   time.Time
	Closed    *time.Time
	Comments  int
}

// MilestoneRef is the family's issue-side milestone pointer.
type MilestoneRef struct {
	ID    int64
	Title string
}

// SearchPort is the search endpoint of the family SDK surface. The family's
// SDK returns no totals (nil), matching the provider's *int-nil contract.
type SearchPort interface {
	SearchRepos(ctx context.Context, opt SearchRepoOptions) ([]*Repo, error)
	ListIssues(ctx context.Context, opt FamilyIssueListOption) ([]*Issue, error)
	ListRepoIssues(ctx context.Context, owner, repo string, opt FamilyIssueListOption) ([]*Issue, error)
	SearchUsers(ctx context.Context, opt SearchUserOptions) ([]*User, error)
}

// SearchUserOptions mirrors the family's user search query.
type SearchUserOptions struct {
	ListOptions ListOptions
	Keyword     string
}

// SearchRepos rides the family's global keyword repo search. Sort passes
// through in the family's own vocabulary (alpha/created/updated/size/id) —
// values from other platforms' vocabularies are ignored server-side
// (registered pass-through).
//
// Dual-mode pagination via backendutil.PageList.
func SearchRepos(ctx context.Context, f Family, c SearchPort, opts provider.SearchReposOptions) ([]*provider.SearchRepoResult, *int, error) {
	baseOpts := SearchRepoOptions{
		Keyword: opts.Query,
		Sort:    opts.Sort,
		Order:   opts.Order,
	}
	repos, err := backendutil.PageList(opts.Page, opts.PerPage, f.PageSize,
		func(page, perPage int) ([]*Repo, error) {
			baseOpts.ListOptions = ListOptions{Page: page, PageSize: perPage}
			return c.SearchRepos(ctx, baseOpts)
		})
	if err != nil {
		return nil, nil, provider.Wrap(f.Platform, "SearchRepos", err)
	}
	out := make([]*provider.SearchRepoResult, 0, len(repos))
	for _, r := range repos {
		out = append(out, &provider.SearchRepoResult{
			FullName:      r.FullName,
			Description:   r.Description,
			WebURL:        r.HTMLURL,
			Stars:         r.Stars,
			Forks:         r.Forks,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
		})
	}
	return out, nil, nil
}

// SearchIssues routes with Repo set ("owner/repo") with Repo set ("owner/repo") it routes to the family's
// (Comments/CreatedAt ride the wire Issue only because the search result
// shape carries them; the full issue converter is a later batch)
// server-side repo-scoped listing (ListRepoIssues — honors the same
// keyword/type/state/pagination parameters); without it the global keyword
// issue search runs restricted to real issues via the "issues" type filter.
// Dual-mode pagination via backendutil.PageList.
func SearchIssues(ctx context.Context, f Family, c SearchPort, opts provider.SearchIssuesOptions) ([]*provider.SearchIssueResult, *int, error) {
	listOpts := FamilyIssueListOption{
		KeyWord: opts.Query,
		State:   opts.State,
		Type:    "issues",
	}
	var repoOwner, repoName string
	if opts.Repo != "" {
		repoOwner, repoName = provider.SplitFullName(opts.Repo)
		if repoOwner == "" || repoName == "" {
			return nil, nil, provider.Wrapf(f.Platform, "SearchIssues", "invalid repo %q, want owner/name", opts.Repo)
		}
	}
	issues, err := backendutil.PageList(opts.Page, opts.PerPage, f.PageSize,
		func(page, perPage int) ([]*Issue, error) {
			listOpts.ListOptions = ListOptions{Page: page, PageSize: perPage}
			if repoName != "" {
				return c.ListRepoIssues(ctx, repoOwner, repoName, listOpts)
			}
			return c.ListIssues(ctx, listOpts)
		})
	if err != nil {
		return nil, nil, provider.Wrap(f.Platform, "SearchIssues", err)
	}
	out := make([]*provider.SearchIssueResult, 0, len(issues))
	for _, i := range issues {
		labels := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			labels = append(labels, l.Name)
		}
		out = append(out, &provider.SearchIssueResult{
			Number:    strconv.FormatInt(i.Index, 10),
			Title:     i.Title,
			Body:      i.Body,
			State:     provider.IssueState(i.State),
			WebURL:    i.HTMLURL,
			Labels:    labels,
			Comments:  i.Comments,
			CreatedAt: i.Created,
		})
	}
	return out, nil, nil
}

// SearchUsers rides the family's /users/search endpoint (dual-mode
// pagination via backendutil.PageList).
func SearchUsers(ctx context.Context, f Family, c SearchPort, opts provider.SearchUsersOptions) ([]*provider.SearchUserResult, *int, error) {
	baseOpts := SearchUserOptions{Keyword: opts.Query}
	users, err := backendutil.PageList(opts.Page, opts.PerPage, f.PageSize,
		func(page, perPage int) ([]*User, error) {
			baseOpts.ListOptions = ListOptions{Page: page, PageSize: perPage}
			return c.SearchUsers(ctx, baseOpts)
		})
	if err != nil {
		return nil, nil, provider.Wrap(f.Platform, "SearchUsers", err)
	}
	out := make([]*provider.SearchUserResult, 0, len(users))
	for _, u := range users {
		out = append(out, &provider.SearchUserResult{
			Login:     u.UserName,
			Name:      u.FullName,
			AvatarURL: u.AvatarURL,
		})
	}
	return out, nil, nil
}
