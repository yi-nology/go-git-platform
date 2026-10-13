package giteafamily

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- reviews ---

// PullReview is the slice of the family's pull-review object the engine
// consumes. State carries the family's review vocabulary
// (approved/request_changes/comment/pending).
type PullReview struct {
	ID          int64
	State       string
	Body        string
	HTMLURL     string
	Reviewer    *User
	SubmittedAt time.Time
}

// PullReviewComment is the family's inline review comment (line numbers are
// 1-based; OldLineNum vs NewLineNum select the side).
type PullReviewComment struct {
	Path       string
	OldLineNum int64
	NewLineNum int64
	Body       string
}

// ListPullReviewsOptions mirrors the family's review list query.
type ListPullReviewsOptions struct {
	ListOptions ListOptions
}

// PullReviewRequestOptions mirrors the family's review-request body.
type PullReviewRequestOptions struct {
	Reviewers []string
}

// DismissPullReviewOptions mirrors the family's review-dismissal body.
type DismissPullReviewOptions struct {
	Message string
}

// CreatePullReviewOption mirrors the family's review create body.
type CreatePullReviewOption struct {
	CommitID string
	State    string
	Body     string
	Comments []*PullReviewComment
}

// ReviewPort is the pull-review endpoint of the family SDK surface.
type ReviewPort interface {
	ListPullReviews(ctx context.Context, owner, repo string, index int64, opt ListPullReviewsOptions) ([]*PullReview, error)
	GetPullReview(ctx context.Context, owner, repo string, index, reviewID int64) (*PullReview, error)
	CreatePullReview(ctx context.Context, owner, repo string, index int64, opt CreatePullReviewOption) (*PullReview, error)
	CreateReviewRequests(ctx context.Context, owner, repo string, index int64, opt PullReviewRequestOptions) error
	DismissPullReview(ctx context.Context, owner, repo string, index, reviewID int64, opt DismissPullReviewOptions) error
}

// ListReviews lists a change request's reviews (the provider surface carries
// no pagination parameters; the engine walks the family list).
func ListReviews(ctx context.Context, f Family, c ReviewPort, owner, repo, number string) ([]provider.Review, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "ListReviews", number)
	if err != nil {
		return nil, err
	}
	reviews, err := backendutil.AllPages(func(page int) ([]*PullReview, error) {
		return c.ListPullReviews(ctx, owner, repo, n, ListPullReviewsOptions{
			ListOptions: ListOptions{Page: page, PageSize: f.PageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListReviews", err)
	}
	result := make([]provider.Review, 0, len(reviews))
	for _, r := range reviews {
		result = append(result, convertReview(r))
	}
	return result, nil
}

// GetReview fetches one review.
func GetReview(ctx context.Context, f Family, c ReviewPort, owner, repo, number string, reviewID int64) (*provider.Review, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "GetReview", number)
	if err != nil {
		return nil, err
	}
	r, err := c.GetPullReview(ctx, owner, repo, n, reviewID)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetReview", err)
	}
	rev := convertReview(r)
	return &rev, nil
}

// CreateReview creates a review. The provider's GitHub-style event values
// (APPROVE/REQUEST_CHANGES/COMMENT) map onto the family's review-state
// vocabulary; anything unrecognized (COMMENT included) becomes a comment.
func CreateReview(ctx context.Context, f Family, c ReviewPort, owner, repo, number string, opts provider.CreateReviewOptions) (*provider.ReviewResult, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "CreateReview", number)
	if err != nil {
		return nil, err
	}
	opt := CreatePullReviewOption{
		CommitID: opts.CommitID,
		Body:     opts.Body,
	}
	switch opts.Event {
	case "APPROVE":
		opt.State = "APPROVED"
	case "REQUEST_CHANGES":
		opt.State = "REQUEST_CHANGES"
	default:
		opt.State = "COMMENT"
	}
	for _, cm := range opts.Comments {
		rc := &PullReviewComment{Path: cm.Path, Body: cm.Body}
		if cm.Side == "LEFT" {
			rc.OldLineNum = int64(cm.Line)
		} else {
			rc.NewLineNum = int64(cm.Line)
		}
		if cm.StartLine > 0 && cm.EndLine > cm.StartLine {
			if cm.Side == "LEFT" {
				rc.OldLineNum = int64(cm.StartLine)
			} else {
				rc.NewLineNum = int64(cm.StartLine)
			}
		}
		opt.Comments = append(opt.Comments, rc)
	}
	r, err := c.CreatePullReview(ctx, owner, repo, n, opt)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateReview", err)
	}
	return convertReviewResult(r), nil
}

// RequestReviewers requests review from the given users.
func RequestReviewers(ctx context.Context, f Family, c ReviewPort, owner, repo, number string, reviewers []string) error {
	n, err := backendutil.ParsePRNumber64(f.Platform, "RequestReviewers", number)
	if err != nil {
		return err
	}
	if err := c.CreateReviewRequests(ctx, owner, repo, n, PullReviewRequestOptions{Reviewers: reviewers}); err != nil {
		return provider.Wrap(f.Platform, "RequestReviewers", err)
	}
	return nil
}

// DismissReview dismisses a review with a message.
func DismissReview(ctx context.Context, f Family, c ReviewPort, owner, repo, number string, reviewID int64, message string) error {
	n, err := backendutil.ParsePRNumber64(f.Platform, "DismissReview", number)
	if err != nil {
		return err
	}
	if err := c.DismissPullReview(ctx, owner, repo, n, reviewID, DismissPullReviewOptions{Message: message}); err != nil {
		return provider.Wrap(f.Platform, "DismissReview", err)
	}
	return nil
}

// convertReview maps a family PullReview to the provider Review.
func convertReview(r *PullReview) provider.Review {
	var review provider.Review
	if r == nil {
		return review
	}
	review = provider.Review{
		ID:          r.ID,
		Body:        r.Body,
		SubmittedAt: r.SubmittedAt,
	}
	if r.Reviewer != nil {
		review.User = r.Reviewer.UserName
	}
	switch strings.ToLower(r.State) {
	case "approved":
		review.State = provider.ReviewStateApproved
	case "request_changes":
		review.State = provider.ReviewStateChangesRequested
	case "comment":
		review.State = provider.ReviewStateCommented
	case "pending":
		review.State = provider.ReviewStatePending
	default:
		if r.State != "" {
			review.State = provider.ReviewState(r.State)
		}
	}
	return review
}

// convertReviewResult maps a family PullReview to the create-call result.
func convertReviewResult(r *PullReview) *provider.ReviewResult {
	if r == nil {
		return nil
	}
	result := &provider.ReviewResult{
		ID:      strconv.FormatInt(r.ID, 10),
		HTMLURL: r.HTMLURL,
	}
	if r.Reviewer != nil {
		result.User = &provider.CRUser{
			ID:        r.Reviewer.ID,
			Username:  r.Reviewer.UserName,
			AvatarURL: r.Reviewer.AvatarURL,
		}
	}
	return result
}

// --- diffs ---

// FamilyChangedFile is the slice of the family's changed-file object the
// engine consumes (the family's status vocabulary: added/removed/renamed).
type FamilyChangedFile struct {
	Filename         string
	PreviousFilename string
	Status           string
	Additions        int
	Deletions        int
}

// ListPullRequestFilesOptions mirrors the family's changed-file list query.
type ListPullRequestFilesOptions struct {
	ListOptions ListOptions
}

// CreateIssueCommentOption mirrors the family's issue-comment create body
// (shared by CR comments and diff discussions in the family dialect).
type CreateIssueCommentOption struct {
	Body string
}

// FamilyComment is the family's issue/CR comment wire type.
type FamilyComment struct {
	ID        int64
	Body      string
	Poster    *User
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DiffPort is the diff + comment endpoint of the family SDK surface.
type DiffPort interface {
	GetPullRequestDiff(ctx context.Context, owner, repo string, index int64) ([]byte, error)
	ListPullRequestFiles(ctx context.Context, owner, repo string, index int64, opt ListPullRequestFilesOptions) ([]*FamilyChangedFile, error)
	CreateIssueComment(ctx context.Context, owner, repo string, index int64, opt CreateIssueCommentOption) (*FamilyComment, int, error)
	DeleteIssueComment(ctx context.Context, owner, repo string, id int64) (int, error)
}

// GetCRDiff returns the unified diff plus the per-file breakdown of a change
// request (the engine fetches the raw diff and the file list, summing the
// family's per-file add/del counts for the totals).
func GetCRDiff(ctx context.Context, f Family, c DiffPort, owner, repo, number string) (*provider.MergeDiff, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "GetCRDiff", number)
	if err != nil {
		return nil, err
	}
	diffBytes, err := c.GetPullRequestDiff(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetCRDiff", err)
	}
	rawDiff := string(diffBytes)

	files, err := GetCRFiles(ctx, f, c, owner, repo, number)
	if err != nil {
		return nil, err
	}
	totalAdd, totalDel := 0, 0
	for _, cf := range files {
		totalAdd += cf.Additions
		totalDel += cf.Deletions
	}
	return &provider.MergeDiff{Files: files, TotalAdd: totalAdd, TotalDel: totalDel, RawDiff: rawDiff}, nil
}

// GetCRFiles lists the files changed by a change request, normalizing the
// family's status vocabulary onto the provider's ChangedFile flags.
func GetCRFiles(ctx context.Context, f Family, c DiffPort, owner, repo, number string) ([]*provider.ChangedFile, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "GetCRFiles", number)
	if err != nil {
		return nil, err
	}
	changedFiles, err := c.ListPullRequestFiles(ctx, owner, repo, n, ListPullRequestFilesOptions{
		ListOptions: ListOptions{Page: 1, PageSize: f.PageSize},
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetCRFiles", err)
	}
	result := make([]*provider.ChangedFile, 0, len(changedFiles))
	for _, cf := range changedFiles {
		out := &provider.ChangedFile{
			OldPath:   cf.PreviousFilename,
			NewPath:   cf.Filename,
			Additions: cf.Additions,
			Deletions: cf.Deletions,
			IsNew:     cf.Status == "added",
			IsDeleted: cf.Status == "removed",
			IsRenamed: cf.Status == "renamed",
		}
		if out.OldPath == "" {
			out.OldPath = out.NewPath
		}
		result = append(result, out)
	}
	return result, nil
}

// CreateNote adds a comment to a change request (the family's issue-comment
// endpoint doubles as the CR comment endpoint — a CR is an issue).
func CreateNote(ctx context.Context, f Family, c DiffPort, owner, repo, number, body string) (string, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "CreateNote", number)
	if err != nil {
		return "", err
	}
	comment, _, err := c.CreateIssueComment(ctx, owner, repo, n, CreateIssueCommentOption{Body: body})
	if err != nil {
		return "", provider.Wrap(f.Platform, "CreateNote", err)
	}
	return strconv.FormatInt(comment.ID, 10), nil
}

// DeleteNote removes a comment. The note is addressed by its numeric
// platform ID; the change-request number is only validated, as the family's
// delete-comment endpoint does not take it. The family's 404 on an
// already-gone comment is tolerated (idempotent delete).
func DeleteNote(ctx context.Context, f Family, c DiffPort, owner, repo, number, noteID string) error {
	if _, err := backendutil.ParsePRNumber64(f.Platform, "DeleteNote", number); err != nil {
		return err
	}
	id, err := strconv.ParseInt(noteID, 10, 64)
	if err != nil {
		return provider.Wrap(f.Platform, "DeleteNote", err)
	}
	status, err := c.DeleteIssueComment(ctx, owner, repo, id)
	if err != nil {
		if status == 404 {
			return nil
		}
		return provider.Wrap(f.Platform, "DeleteNote", err)
	}
	return nil
}

// CreateDiscussion creates a top-level comment on a change request. The
// family has no threaded line comments, so a discussion collapses to one
// comment whose body carries the line reference inline (file/line prefixed).
func CreateDiscussion(ctx context.Context, f Family, c DiffPort, owner, repo, number string, opts provider.DiscussionOptions) (string, error) {
	n, err := backendutil.ParsePRNumber64(f.Platform, "CreateDiscussion", number)
	if err != nil {
		return "", err
	}
	comment, _, err := c.CreateIssueComment(ctx, owner, repo, n, CreateIssueCommentOption{Body: opts.Body})
	if err != nil {
		return "", provider.Wrap(f.Platform, "CreateDiscussion", err)
	}
	return strconv.FormatInt(comment.ID, 10), nil
}

// --- files ---

// FamilyFileResult is the family's file write response (the Commit field's
// SHA rides the family's commit payload; delete responses carry the SHA on
// the X-Commit-Sha header).
type FamilyFileResult struct {
	CommitSHA string
}

// FamilyFileOptions is the family's file create/update/delete body
// (SHA is the blob SHA for update/delete; the family's file options share
// one struct across the three verbs).
type FamilyFileOptions struct {
	Message string
	Branch  string
	SHA     string
	Content string
	Author  string
	Email   string
}

// FilePort is the file endpoint of the family SDK surface. The family's
// error type does not carry the HTTP status, so GetFile's int return carries
// the underlying response's status for IsNotFound classification.
type FilePort interface {
	GetFile(ctx context.Context, owner, repo, ref, path string) (data []byte, status int, err error)
	CreateFile(ctx context.Context, owner, repo, path string, opt FamilyFileOptions) (*FamilyFileResult, error)
	UpdateFile(ctx context.Context, owner, repo, path string, opt FamilyFileOptions) (*FamilyFileResult, error)
	DeleteFile(ctx context.Context, owner, repo, path string, opt FamilyFileOptions) (*FamilyFileResult, int, error)
}

// GetFileContent reads one file's contents at a ref.
func GetFileContent(ctx context.Context, f Family, c FilePort, owner, repo, path, ref string) (string, error) {
	data, status, err := c.GetFile(ctx, owner, repo, ref, path)
	if err != nil {
		if status == 404 {
			return "", provider.New(f.Platform, "GetFileContent", 404, err.Error())
		}
		return "", provider.Wrap(f.Platform, "GetFileContent", err)
	}
	return string(data), nil
}

// CreateFile creates a file on a branch.
func CreateFile(ctx context.Context, f Family, c FilePort, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	res, err := c.CreateFile(ctx, owner, repo, opts.Path, FamilyFileOptions{
		Message: opts.Message,
		Branch:  opts.Branch,
		Content: opts.Content,
		Author:  opts.Author,
		Email:   opts.Email,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateFile", err)
	}
	return &provider.FileResult{CommitSHA: res.CommitSHA}, nil
}

// UpdateFile updates a file on a branch.
func UpdateFile(ctx context.Context, f Family, c FilePort, owner, repo string, opts provider.FileOptions) (*provider.FileResult, error) {
	res, err := c.UpdateFile(ctx, owner, repo, opts.Path, FamilyFileOptions{
		Message: opts.Message,
		Branch:  opts.Branch,
		SHA:     opts.SHA,
		Content: opts.Content,
		Author:  opts.Author,
		Email:   opts.Email,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateFile", err)
	}
	return &provider.FileResult{CommitSHA: res.CommitSHA}, nil
}

// DeleteFile deletes a file from a branch.
func DeleteFile(ctx context.Context, f Family, c FilePort, owner, repo string, opts provider.FileDeleteOptions) (*provider.FileResult, error) {
	res, _, err := c.DeleteFile(ctx, owner, repo, opts.Path, FamilyFileOptions{
		Message: opts.Message,
		Branch:  opts.Branch,
		SHA:     opts.SHA,
		Author:  opts.Author,
		Email:   opts.Email,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "DeleteFile", err)
	}
	return &provider.FileResult{CommitSHA: res.CommitSHA}, nil
}

// --- repos ---

// FamilyCreateRepoOption mirrors the family's repo create body.
type FamilyCreateRepoOption struct {
	Name          string
	Description   string
	Private       bool
	AutoInit      bool
	DefaultBranch string
}

// FamilyForkRepoOption mirrors the family's fork body (Org is the target
// org; empty = the authenticated user's namespace).
type FamilyForkRepoOption struct {
	Organization *string
	Name         *string
}

// FamilyListReposOptions mirrors the family's authenticated-user repo list
// query.
type FamilyListReposOptions struct {
	ListOptions ListOptions
}

// FamilyEditRepoOption mirrors the family's repo edit body (nil fields are
// left unchanged).
type FamilyEditRepoOption struct {
	Name          *string
	Description   *string
	DefaultBranch *string
	Private       *bool
}

// RepoPort is the repository endpoint of the family SDK surface. GetRepo's
// int return carries the underlying response's HTTP status for IsNotFound
// classification (the family's error type flattens it away).
type RepoPort interface {
	SearchRepos(ctx context.Context, opt SearchRepoOptions) ([]*Repo, error)
	ListMyRepos(ctx context.Context, opt FamilyListReposOptions) ([]*Repo, error)
	GetRepo(ctx context.Context, owner, repo string) (*Repo, int, error)
	CreateRepo(ctx context.Context, opt FamilyCreateRepoOption) (*Repo, error)
	CreateOrgRepo(ctx context.Context, org string, opt FamilyCreateRepoOption) (*Repo, error)
	CreateFork(ctx context.Context, owner, repo string, opt FamilyForkRepoOption) (*Repo, error)
	DeleteRepo(ctx context.Context, owner, repo string) error
	EditRepo(ctx context.Context, owner, repo string, opt FamilyEditRepoOption) (*Repo, error)
}

// ListRepos lists the authenticated user's repos. When Owner is set the
// family has no per-owner listing, so the engine searches and filters
// client-side (registered divergence).
func ListRepos(ctx context.Context, f Family, c RepoPort, opts provider.ListRepoOptions) ([]*provider.PlatformRepo, error) {
	opts.Page, opts.PerPage = provider.NormalizePageOpts(opts.Page, opts.PerPage)
	if opts.Owner != "" {
		// The family has no per-owner repo listing; the engine searches one
		// normalized page and filters client-side (registered divergence).
		results, err := c.SearchRepos(ctx, SearchRepoOptions{
			ListOptions: ListOptions{Page: opts.Page, PageSize: opts.PerPage},
		})
		if err != nil {
			return nil, provider.Wrap(f.Platform, "ListRepos", err)
		}
		filtered := make([]*provider.PlatformRepo, 0)
		for _, r := range results {
			pr := ConvertRepo(r, f.Platform)
			if pr != nil && strings.EqualFold(pr.Owner, opts.Owner) {
				filtered = append(filtered, pr)
			}
		}
		return filtered, nil
	}
	repos, err := c.ListMyRepos(ctx, FamilyListReposOptions{
		ListOptions: ListOptions{Page: opts.Page, PageSize: opts.PerPage},
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListRepos", err)
	}
	result := make([]*provider.PlatformRepo, 0, len(repos))
	for _, r := range repos {
		result = append(result, ConvertRepo(r, f.Platform))
	}
	return result, nil
}

// GetRepo fetches one repository.
func GetRepo(ctx context.Context, f Family, c RepoPort, owner, repo string) (*provider.PlatformRepo, error) {
	r, status, err := c.GetRepo(ctx, owner, repo)
	if err != nil {
		if status == 404 {
			return nil, provider.Wrap(f.Platform, "GetRepo", provider.New(f.Platform, "GetRepo", 404, err.Error()))
		}
		return nil, provider.Wrap(f.Platform, "GetRepo", err)
	}
	return ConvertRepo(r, f.Platform), nil
}

// CreateRepo creates a user- or owner(org)-scoped repository.
func CreateRepo(ctx context.Context, f Family, c RepoPort, owner string, opts provider.CreateRepoOptions) (*provider.PlatformRepo, error) {
	opt := FamilyCreateRepoOption{
		Name:        opts.Name,
		Description: opts.Description,
		Private:     opts.Private,
		AutoInit:    opts.AutoInit,
	}
	if opts.DefaultBranch != "" {
		opt.DefaultBranch = opts.DefaultBranch
	}
	var r *Repo
	var err error
	if owner != "" {
		r, err = c.CreateOrgRepo(ctx, owner, opt)
	} else {
		r, err = c.CreateRepo(ctx, opt)
	}
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateRepo", err)
	}
	return ConvertRepo(r, f.Platform), nil
}

// ForkRepo forks a repository.
func ForkRepo(ctx context.Context, f Family, c RepoPort, owner, repo string, opts provider.ForkRepoOptions) (*provider.PlatformRepo, error) {
	forkOpts := FamilyForkRepoOption{}
	if opts.Organization != "" {
		org := opts.Organization
		forkOpts.Organization = &org
	}
	if opts.Name != "" {
		name := opts.Name
		forkOpts.Name = &name
	}
	r, err := c.CreateFork(ctx, owner, repo, forkOpts)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ForkRepo", err)
	}
	return ConvertRepo(r, f.Platform), nil
}

// DeleteRepo removes a repository.
func DeleteRepo(ctx context.Context, f Family, c RepoPort, owner, repo string) error {
	if err := c.DeleteRepo(ctx, owner, repo); err != nil {
		return provider.Wrap(f.Platform, "DeleteRepo", err)
	}
	return nil
}

// UpdateRepo edits a repository; empty option fields leave the value
// unchanged.
func UpdateRepo(ctx context.Context, f Family, c RepoPort, owner, repo string, opts provider.UpdateRepoOptions) (*provider.PlatformRepo, error) {
	editOpts := FamilyEditRepoOption{}
	if opts.Name != "" {
		editOpts.Name = &opts.Name
	}
	if opts.Description != "" {
		editOpts.Description = &opts.Description
	}
	if opts.DefaultBranch != "" {
		editOpts.DefaultBranch = &opts.DefaultBranch
	}
	if opts.Private != nil {
		editOpts.Private = opts.Private
	}
	r, err := c.EditRepo(ctx, owner, repo, editOpts)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateRepo", err)
	}
	return ConvertRepo(r, f.Platform), nil
}
