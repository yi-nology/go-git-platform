package giteafamily

import (
	"context"
	"strconv"
	"time"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- change requests ---

// FamilyPRBranchInfo is the family's PR head/base pointer.
type FamilyPRBranchInfo struct {
	Ref string
	Sha string
}

// FamilyPullRequest is the slice of the family's pull-request object the
// engine consumes. Gitea's fork adds RequestedReviewers and Draft; the
// engine's CRPointersHaveReviewers/PRWireHasDraft dialect parameters decide
// which adapter fields are live.
type FamilyPullRequest struct {
	ID                 int64
	Index              int64
	Title              string
	Body               string
	State              string // the family's issue state vocabulary
	HTMLURL            string
	Poster             *User
	Labels             []*Label
	Assignees          []*User
	RequestedReviewers []*User // gitea only; empty on Forgejo
	Draft              bool    // gitea only; false on Forgejo
	HasMerged          bool
	Merged             *time.Time
	MergeBase          string
	Base               *FamilyPRBranchInfo
	Head               *FamilyPRBranchInfo
	Created            *time.Time
	Updated            *time.Time
	Closed             *time.Time
}

// FamilyCreatePullRequestOption mirrors the family's PR create body.
type FamilyCreatePullRequestOption struct {
	Head  string
	Base  string
	Title string
	Body  string
}

// FamilyListPullRequestsOptions mirrors the family's PR list query (State is
// the family's open/closed list vocabulary).
type FamilyListPullRequestsOptions struct {
	ListOptions ListOptions
	State       string
}

// FamilyEditPullRequestOption mirrors the family's PR edit body.
type FamilyEditPullRequestOption struct {
	Title string
	Body  *string
	Base  string
	State *string
}

// FamilyMergePullRequestOption mirrors the family's merge body
// (Style is merge/squash; DeleteBranchAfterMerge toggles source-branch
// cleanup).
type FamilyMergePullRequestOption struct {
	Style                  string
	Title                  string
	DeleteBranchAfterMerge *bool
}

// PRPort is the pull-request endpoint of the family SDK surface. The merge
// and list calls carry the underlying response's HTTP status alongside the
// error so the engine can implement the family's 405 recovery (merge of an
// already-merged PR) and the list total.
type PRPort interface {
	CreatePullRequest(ctx context.Context, owner, repo string, opt FamilyCreatePullRequestOption) (*FamilyPullRequest, error)
	GetPullRequest(ctx context.Context, owner, repo string, index int64) (*FamilyPullRequest, error)
	ListPullRequests(ctx context.Context, owner, repo string, opt FamilyListPullRequestsOptions) (prs []*FamilyPullRequest, total int, err error)
	MergePullRequest(ctx context.Context, owner, repo string, index int64, opt FamilyMergePullRequestOption) (status int, err error)
	EditPullRequest(ctx context.Context, owner, repo string, index int64, opt FamilyEditPullRequestOption) (*FamilyPullRequest, error)
	ListPullRequestCommits(ctx context.Context, owner, repo string, index int64, opt ListOptions) ([]*Commit, error)
}

// CreateCR opens a change request.
func CreateCR(ctx context.Context, f Family, c PRPort, opts provider.CreateCROptions) (*provider.ChangeRequest, error) {
	pr, err := c.CreatePullRequest(ctx, opts.Owner, opts.Repo, FamilyCreatePullRequestOption{
		Head:  opts.SourceBranch,
		Base:  opts.TargetBranch,
		Title: opts.Title,
		Body:  opts.Description,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateCR", err)
	}
	return ConvertPR(pr, f), nil
}

// GetCR fetches one change request.
func GetCR(ctx context.Context, f Family, c PRPort, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "GetCR", number)
	if err != nil {
		return nil, err
	}
	pr, err := c.GetPullRequest(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetCR", err)
	}
	return ConvertPR(pr, f), nil
}

// ListCRs lists change requests. The family's list endpoint filters by its
// open/closed vocabulary; when the backend's dialect parameter reports the
// narrow vocabulary (Forgejo) the engine maps the provider-uniform state
// onto it, otherwise it passes the provider state through.
func ListCRs(ctx context.Context, f Family, c PRPort, opts provider.ListCROptions) ([]*provider.ChangeRequest, *int, error) {
	opts.Page, opts.PerPage = provider.NormalizePageOpts(opts.Page, opts.PerPage)
	// The family's list endpoint filters by its open/closed vocabulary; both
	// forks reject unknown values ("opened") by returning ALL pull requests,
	// so the provider-uniform state is always normalized.
	state := mapListState(opts.State)
	prs, total, err := c.ListPullRequests(ctx, opts.Owner, opts.Repo, FamilyListPullRequestsOptions{
		State:       state,
		ListOptions: ListOptions{Page: opts.Page, PageSize: opts.PerPage},
	})
	if err != nil {
		return nil, nil, provider.Wrap(f.Platform, "ListCRs", err)
	}
	crs := make([]*provider.ChangeRequest, 0, len(prs))
	for _, pr := range prs {
		crs = append(crs, ConvertPR(pr, f))
	}
	// The family's list endpoints carry the total only in X-Total-Count;
	// without the header the count is unknown and reported as nil rather
	// than guessed from the page length.
	var out *int
	if total > 0 {
		out = &total
	}
	return crs, out, nil
}

// mapListState normalizes the provider-uniform CR state ("opened"/"closed"/
// "merged") to the family's open/closed list filter. "merged" has no list
// filter and is the closest superset ("closed").
//
// **未指定状态(零值)⇒ 不加过滤**:平台上"不加 state"= 全部。
// 以前落到 "open" ⇒ 调用方只想"列出 CR"时,closed/merged 的 **静默丢失**
// (GitHub / GitLab / 工蜂 都是"零值=不过滤",这里与它们不一致)。
func mapListState(s provider.CRState) string {
	switch s {
	case provider.CRStateClosed, provider.CRStateMerged:
		return "closed"
	case provider.CRStateOpened:
		return "open"
	default:
		return ""
	}
}

// MergeCR merges a change request. The family returns 405 when the PR is
// already merged; the engine recovers by re-fetching the CR when the current
// state is merged.
func MergeCR(ctx context.Context, f Family, c PRPort, owner, repo, number string, opts provider.MergeCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "MergeCR", number)
	if err != nil {
		return nil, err
	}
	style := "merge"
	if opts.Squash {
		style = "squash"
	}
	deleteBranch := opts.RemoveSourceBranch
	status, err := c.MergePullRequest(ctx, owner, repo, n, FamilyMergePullRequestOption{
		Style:                  style,
		Title:                  opts.MergeCommitMessage,
		DeleteBranchAfterMerge: &deleteBranch,
	})
	if err != nil {
		if status == 405 {
			cr, getErr := GetCR(ctx, f, c, owner, repo, number)
			if getErr == nil && cr.State == provider.CRStateMerged {
				return cr, nil
			}
		}
		return nil, provider.Wrap(f.Platform, "MergeCR", err)
	}
	return GetCR(ctx, f, c, owner, repo, number)
}

// CloseCR closes a change request.
func CloseCR(ctx context.Context, f Family, c PRPort, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "CloseCR", number)
	if err != nil {
		return nil, err
	}
	state := "closed"
	if _, err := c.EditPullRequest(ctx, owner, repo, n, FamilyEditPullRequestOption{State: &state}); err != nil {
		return nil, provider.Wrap(f.Platform, "CloseCR", err)
	}
	return GetCR(ctx, f, c, owner, repo, number)
}

// ReopenCR reopens a change request.
func ReopenCR(ctx context.Context, f Family, c PRPort, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "ReopenCR", number)
	if err != nil {
		return nil, err
	}
	state := "open"
	pr, err := c.EditPullRequest(ctx, owner, repo, n, FamilyEditPullRequestOption{State: &state})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ReopenCR", err)
	}
	return ConvertPR(pr, f), nil
}

// UpdateCR edits a change request; empty fields leave the value unchanged.
func UpdateCR(ctx context.Context, f Family, c PRPort, owner, repo, number string, opts provider.UpdateCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "UpdateCR", number)
	if err != nil {
		return nil, err
	}
	editOpts := FamilyEditPullRequestOption{}
	if opts.Title != "" {
		editOpts.Title = opts.Title
	}
	if opts.Description != "" {
		editOpts.Body = &opts.Description
	}
	if opts.TargetBranch != "" {
		editOpts.Base = opts.TargetBranch
	}
	pr, err := c.EditPullRequest(ctx, owner, repo, n, editOpts)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateCR", err)
	}
	return ConvertPR(pr, f), nil
}

// ConvertPR maps a family PullRequest to the provider shape. The engine reads
// the dialect parameters: CRPointersHaveReviewers (gitea) uses the wire's
// RequestedReviewers; PRWireHasDraft (gitea) uses the wire's Draft; Forgejo
// folds reviewers into Assignees and always reports Draft=false.
func ConvertPR(pr *FamilyPullRequest, f Family) *provider.ChangeRequest {
	if pr == nil {
		return nil
	}
	var author *provider.CRUser
	if pr.Poster != nil {
		author = &provider.CRUser{ID: pr.Poster.ID, Username: pr.Poster.UserName, AvatarURL: pr.Poster.AvatarURL}
	}
	var labels []string
	for _, l := range pr.Labels {
		if l != nil {
			labels = append(labels, l.Name)
		}
	}
	var assignees []*provider.CRUser
	for _, a := range pr.Assignees {
		if a != nil {
			assignees = append(assignees, &provider.CRUser{ID: a.ID, Username: a.UserName, AvatarURL: a.AvatarURL})
		}
	}
	var reviewers []*provider.CRUser
	if f.CRPointersHaveReviewers {
		for _, r := range pr.RequestedReviewers {
			if r != nil {
				reviewers = append(reviewers, &provider.CRUser{ID: r.ID, Username: r.UserName, AvatarURL: r.AvatarURL})
			}
		}
	} else {
		// Forgejo has no separate RequestedReviewers; the wire folds them
		// into Assignees, so the reviewers surface mirrors assignees.
		reviewers = assignees
	}
	cr := &provider.ChangeRequest{
		ID:          pr.ID,
		Number:      strconv.FormatInt(pr.Index, 10),
		Title:       pr.Title,
		Description: pr.Body,
		State:       provider.MapBoolStateToCR(pr.State, pr.HasMerged),
		Author:      author,
		Assignees:   assignees,
		Reviewers:   reviewers,
		Labels:      labels,
		WebURL:      pr.HTMLURL,
	}
	if f.PRWireHasDraft {
		cr.Draft = pr.Draft
	}
	if pr.Head != nil {
		cr.SourceBranch = pr.Head.Ref
		cr.HeadSHA = pr.Head.Sha
	}
	if pr.Base != nil {
		cr.TargetBranch = pr.Base.Ref
		cr.BaseSHA = pr.Base.Sha
		cr.StartSHA = pr.Base.Sha // family exposes no distinct merge-base
	}
	if pr.Created != nil {
		cr.CreatedAt = *pr.Created
	}
	if pr.Updated != nil {
		cr.UpdatedAt = *pr.Updated
	}
	return cr
}

// ListCRComments lists the comments on a change request (the provider
// surface carries no pagination parameters; the engine walks the family
// issue-comment endpoint).
func ListCRComments(ctx context.Context, f Family, c IssuePort, owner, repo, number string) ([]*provider.CRComment, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "ListCRComments", number)
	if err != nil {
		return nil, err
	}
	comments, err := backendutil.AllPages(func(page int) ([]*FamilyComment, error) {
		return c.ListIssueComments(ctx, owner, repo, n, ListIssueCommentsOptions{
			ListOptions: ListOptions{Page: page, PageSize: backendutil.IssueCommentPageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListCRComments", err)
	}
	result := make([]*provider.CRComment, 0, len(comments))
	for _, c := range comments {
		cc := &provider.CRComment{ID: c.ID, Body: c.Body, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
		if c.Poster != nil {
			cc.Author = &provider.CRUser{ID: c.Poster.ID, Username: c.Poster.UserName, AvatarURL: c.Poster.AvatarURL}
		}
		result = append(result, cc)
	}
	return result, nil
}

// ListCRCommits lists the commits on a change request (the provider surface
// carries no pagination parameters; the engine walks the family endpoint).
func ListCRCommits(ctx context.Context, f Family, c PRPort, owner, repo, number string) ([]*provider.CRCommit, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "ListCRCommits", number)
	if err != nil {
		return nil, err
	}
	commits, err := backendutil.AllPages(func(page int) ([]*Commit, error) {
		return c.ListPullRequestCommits(ctx, owner, repo, n, ListOptions{Page: page, PageSize: f.PageSize})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListCRCommits", err)
	}
	result := make([]*provider.CRCommit, 0, len(commits))
	for _, c := range commits {
		if c == nil {
			continue
		}
		cc := &provider.CRCommit{SHA: c.SHA, CreatedAt: c.Created}
		cc.Message = c.Message
		if c.AuthorName != "" {
			cc.Author = &provider.CRUser{Name: c.AuthorName}
		}
		result = append(result, cc)
	}
	return result, nil
}

// UpdateCRLabels adds labels to a change request by name-or-ID: numeric
// strings pass through as IDs (the family's label endpoint takes IDs);
// everything else is resolved to an ID through the shared cached resolver.
func UpdateCRLabels(ctx context.Context, f Family, c IssuePort, labelCache LabelPort, cache *backendutil.IDCache, owner, repo, number string, names []string) error {
	n, err := backendutil.ParsePRNumber64(f.Platform, "UpdateCRLabels", number)
	if err != nil {
		return err
	}
	labelIDs := make([]int64, 0, len(names))
	for _, l := range names {
		if id, err := strconv.ParseInt(l, 10, 64); err == nil {
			labelIDs = append(labelIDs, id)
			continue
		}
		id, err := ResolveLabelID(ctx, f, labelCache, cache, "UpdateCRLabels", owner, repo, l)
		if err != nil {
			return err
		}
		labelIDs = append(labelIDs, id)
	}
	if err := c.AddIssueLabels(ctx, owner, repo, n, IssueLabelsOption{Labels: labelIDs}); err != nil {
		return provider.Wrap(f.Platform, "UpdateCRLabels", err)
	}
	return nil
}
