package tencentcode

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	gongfeng "github.com/yi-nology/go-git-platform/backends/tencentcode/internal/gongfeng"
	"github.com/yi-nology/go-git-platform/provider"
)

// TencentCodeExtras exposes Tencent 工蜂-specific capabilities that are not
// part of the cross-platform Provider interface (native code reviews, MR review
// workflow, commit-level comments/diff, repository tree/blob and branch
// protection). Obtain it via type assertion:
//
//	p, _ := provider.NewProvider(provider.Config{Platform: provider.PlatformTencentCode, Token: t})
//	if tc, ok := p.(provider.TencentCodeExtras); ok {
//	    tc.SubmitCodeReview(ctx, owner, repo, reviewID, opts)
//	}
type TencentCodeExtras interface {
	// --- Native code review (/review) ---
	CreateCodeReview(ctx context.Context, owner, repo string, opts CreateCodeReviewOptions) (*CodeReview, error)
	ListCodeReviews(ctx context.Context, owner, repo string, opts ListCodeReviewsOptions) ([]*CodeReview, error)
	GetCodeReview(ctx context.Context, owner, repo string, reviewID int) (*CodeReview, error)
	UpdateCodeReview(ctx context.Context, owner, repo string, reviewID int, opts UpdateCodeReviewOptions) (*CodeReview, error)
	InviteCodeReviewer(ctx context.Context, owner, repo string, reviewID int, opts InviteReviewerOptions) error
	RemoveCodeReviewer(ctx context.Context, owner, repo string, reviewID int, reviewerID string) error
	SubmitCodeReview(ctx context.Context, owner, repo string, reviewID int, opts SubmitReviewOptions) error
	ReopenCodeReview(ctx context.Context, owner, repo string, reviewID int) error
	GetCodeReviewChangedFiles(ctx context.Context, owner, repo string, reviewID int) ([]*provider.ChangedFile, error)

	// --- MR review workflow (/merge_request/{iid}/review*) ---
	GetMRReview(ctx context.Context, owner, repo string, mrNumber int) (*MRReview, error)
	InviteMRReviewer(ctx context.Context, owner, repo string, mrNumber int, opts InviteReviewerOptions) error
	RemoveMRReviewer(ctx context.Context, owner, repo string, mrNumber int, reviewerID int) error
	CancelMRReview(ctx context.Context, owner, repo string, mrNumber int) error
	SubmitMRReview(ctx context.Context, owner, repo string, mrNumber int, opts SubmitReviewOptions) error
	ReopenMRReview(ctx context.Context, owner, repo string, mrNumber int) error

	// --- Commit-level ops (/repository/commits/{sha}) ---
	GetCommitDiff(ctx context.Context, owner, repo, sha string, opts CommitDiffOptions) ([]*provider.ChangedFile, error)
	ListCommitComments(ctx context.Context, owner, repo, sha string, page, perPage int) ([]*CommitComment, error)
	CreateCommitComment(ctx context.Context, owner, repo, sha string, opts CreateCommitCommentOptions) (*CommitComment, error)
	GetCommitRefs(ctx context.Context, owner, repo, sha string, refType CommitRefType) (*CommitRefs, error)

	// --- Repository tree & blob ---
	GetRepoTree(ctx context.Context, owner, repo, path, ref string, recursive bool) ([]*TreeEntryNode, error)
	GetBlob(ctx context.Context, owner, repo, sha string) ([]byte, error)

	// --- Branch protection ---
	ProtectBranch(ctx context.Context, owner, repo, branch string, opts ProtectBranchOptions) error
	UnprotectBranch(ctx context.Context, owner, repo, branch string) error
	ListProtectedBranchMembers(ctx context.Context, owner, repo, branch string) ([]*ProtectedBranchMember, error)
	AddProtectedBranchMember(ctx context.Context, owner, repo, branch string, opts ProtectedBranchMemberOptions) error
	UpdateProtectedBranchMember(ctx context.Context, owner, repo, branch string, userID int, opts ProtectedBranchMemberOptions) error
	RemoveProtectedBranchMember(ctx context.Context, owner, repo, branch string, userID int) error
}

// Compile-time assertion that *Provider satisfies TencentCodeExtras.
var _ TencentCodeExtras = (*Provider)(nil)

// ---------------------------------------------------------------------------
// Shared option / result types
// ---------------------------------------------------------------------------

// ReviewerEvent is the verdict a reviewer submits for a code review or MR review.
type ReviewerEvent string

const (
	ReviewEventComment       ReviewerEvent = "comment"
	ReviewEventApprove       ReviewerEvent = "approve"
	ReviewEventRequireChange ReviewerEvent = "require_change"
	ReviewEventDeny          ReviewerEvent = "deny"
)

// SubmitReviewOptions submits a reviewer verdict with a summary.
type SubmitReviewOptions struct {
	Event   ReviewerEvent
	Summary string
}

// InviteReviewerOptions invites one or more reviewers.
type InviteReviewerOptions struct {
	ReviewerID          string // comma-separated user IDs (code review)
	NecessaryReviewerID string // comma-separated necessary user IDs (code review)
}

// CodeReview represents a Tencent 工蜂 native code review.
type CodeReview struct {
	ID           int              `json:"id"`
	IID          int              `json:"iid"`
	ProjectID    int              `json:"project_id"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	State        string           `json:"state"`
	SourceBranch string           `json:"source_branch"`
	TargetBranch string           `json:"target_branch"`
	SourceCommit string           `json:"source_commit"`
	TargetCommit string           `json:"target_commit"`
	Author       *provider.CRUser `json:"author"`
	Reviewers    []string         `json:"reviewers"`
	WebURL       string           `json:"web_url"`
	CreatedAt    string           `json:"created_at"`
	UpdatedAt    string           `json:"updated_at"`
}

// CreateCodeReviewOptions creates a native code review.
type CreateCodeReviewOptions struct {
	Title                 string
	SourceBranch          string
	TargetBranch          string
	SourceCommit          string
	TargetCommit          string
	Description           string
	ReviewerIDs           string // comma-separated reviewer user IDs
	NecessaryReviewerIDs  string // comma-separated necessary reviewer user IDs
	ApproverRule          int    // -1 = all approve, 1 = single, 2+ = N approve
	NecessaryApproverRule int
}

// ListCodeReviewsOptions filters the code review list.
type ListCodeReviewsOptions struct {
	State    string // approving, change_required, closed
	AuthorID int
	OrderBy  string // created_at, updated_at
	Sort     string // asc, desc
	Page     int
	PerPage  int
}

// UpdateCodeReviewOptions updates a code review's title/description.
type UpdateCodeReviewOptions struct {
	Title       string
	Description string
}

// MRReview represents the review state of a merge request.
type MRReview struct {
	ID           int              `json:"id"`
	IID          int              `json:"iid"`
	ProjectID    int              `json:"project_id"`
	State        string           `json:"state"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	Author       *provider.CRUser `json:"author"`
	SourceBranch string           `json:"source_branch"`
	TargetBranch string           `json:"target_branch"`
	WebURL       string           `json:"web_url"`
	CreatedAt    string           `json:"created_at"`
	UpdatedAt    string           `json:"updated_at"`
}

// CommitDiffOptions controls a single-commit diff.
type CommitDiffOptions struct {
	Path             string
	IgnoreWhiteSpace bool
}

// CommitComment represents a comment on a commit.
type CommitComment struct {
	ID        int              `json:"id"`
	Body      string           `json:"body"`
	Author    *provider.CRUser `json:"author"`
	Path      string           `json:"path"`
	Line      int              `json:"line"`
	LineType  string           `json:"line_type"`
	CreatedAt string           `json:"created_at"`
}

// CreateCommitCommentOptions creates a comment on a commit.
type CreateCommitCommentOptions struct {
	Note     string
	Path     string
	Line     int
	LineType string // "old" or "new"
}

// CommitRefType selects which refs to return for a commit.
type CommitRefType string

const (
	CommitRefAll    CommitRefType = "all"
	CommitRefBranch CommitRefType = "branch"
	CommitRefTag    CommitRefType = "tag"
)

// CommitRefs lists branches and tags containing a commit.
type CommitRefs struct {
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
}

// TreeEntryNode represents a single entry in a repository tree listing.
type TreeEntryNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "tree" or "blob"
	Mode string `json:"mode"`
	Path string `json:"path"`
}

// ProtectBranchOptions configures branch protection rules on Tencent Code (工蜂).
type ProtectBranchOptions struct {
	MergeAccessLevel          int  `json:"merge_access_level,omitempty"`
	PushAccessLevel           int  `json:"push_access_level,omitempty"`
	AllowForcePush            bool `json:"allow_force_push,omitempty"`
	CodeOwnerApprovalRequired bool `json:"code_owner_approval_required,omitempty"`
}

// ProtectedBranchMember represents a member granted access to a protected branch.
type ProtectedBranchMember struct {
	UserID      int    `json:"user_id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	AccessLevel int    `json:"access_level"`
}

// ProtectedBranchMemberOptions adds/updates a protected branch member.
type ProtectedBranchMemberOptions struct {
	UserID      int
	AccessLevel int
}

// ===========================================================================
// Native code review (/review)
// ===========================================================================

func (t *Provider) CreateCodeReview(ctx context.Context, owner, repo string, opts CreateCodeReviewOptions) (*CodeReview, error) {
	sdkOpts := &gongfeng.CreateCommitReviewOptions{
		Title:        gongfeng.Ptr(opts.Title),
		SourceBranch: gongfeng.Ptr(opts.SourceBranch),
		TargetBranch: gongfeng.Ptr(opts.TargetBranch),
		Description:  gongfeng.Ptr(opts.Description),
	}
	if opts.SourceCommit != "" {
		sdkOpts.SourceCommit = gongfeng.Ptr(opts.SourceCommit)
	}
	if opts.TargetCommit != "" {
		sdkOpts.TargetCommit = gongfeng.Ptr(opts.TargetCommit)
	}
	if opts.ReviewerIDs != "" {
		sdkOpts.ReviewerIDs = gongfeng.Ptr(opts.ReviewerIDs)
	}
	if opts.NecessaryReviewerIDs != "" {
		sdkOpts.NecessaryReviewerIDs = gongfeng.Ptr(opts.NecessaryReviewerIDs)
	}
	if opts.ApproverRule != 0 {
		sdkOpts.ApproverRule = gongfeng.Ptr(opts.ApproverRule)
	}
	if opts.NecessaryApproverRule != 0 {
		sdkOpts.NecessaryApproverRule = gongfeng.Ptr(opts.NecessaryApproverRule)
	}
	review, _, err := t.client.Reviews.CreateCommitReview(ctx, pid(owner, repo), sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateCodeReview", err)
	}
	return reviewToCodeReview(review), nil
}

func (t *Provider) ListCodeReviews(ctx context.Context, owner, repo string, opts ListCodeReviewsOptions) ([]*CodeReview, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	sdkOpts := &gongfeng.ListCommitReviewsOptions{
		ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage},
	}
	if opts.State != "" {
		sdkOpts.State = gongfeng.Ptr(opts.State)
	}
	if opts.AuthorID != 0 {
		sdkOpts.AuthorID = gongfeng.Ptr(opts.AuthorID)
	}
	if opts.OrderBy != "" {
		sdkOpts.OrderBy = gongfeng.Ptr(opts.OrderBy)
	}
	if opts.Sort != "" {
		sdkOpts.Sort = gongfeng.Ptr(opts.Sort)
	}
	reviews, _, err := t.client.Reviews.ListCommitReviews(ctx, pid(owner, repo), sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListCodeReviews", err)
	}
	result := make([]*CodeReview, 0, len(reviews))
	for _, r := range reviews {
		result = append(result, reviewToCodeReview(r))
	}
	return result, nil
}

func (t *Provider) GetCodeReview(ctx context.Context, owner, repo string, reviewID int) (*CodeReview, error) {
	review, _, err := t.client.Reviews.GetCommitReview(ctx, pid(owner, repo), reviewID)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetCodeReview", err)
	}
	return reviewToCodeReview(review), nil
}

func (t *Provider) UpdateCodeReview(ctx context.Context, owner, repo string, reviewID int, opts UpdateCodeReviewOptions) (*CodeReview, error) {
	sdkOpts := &gongfeng.UpdateCommitReviewOptions{
		Title:       gongfeng.Ptr(opts.Title),
		Description: gongfeng.Ptr(opts.Description),
	}
	review, _, err := t.client.Reviews.UpdateCommitReview(ctx, pid(owner, repo), reviewID, sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "UpdateCodeReview", err)
	}
	return reviewToCodeReview(review), nil
}

func (t *Provider) InviteCodeReviewer(ctx context.Context, owner, repo string, reviewID int, opts InviteReviewerOptions) error {
	sdkOpts := &gongfeng.InviteCommitReviewerOptions{}
	if opts.ReviewerID != "" {
		if id, err := strconv.Atoi(opts.ReviewerID); err == nil {
			sdkOpts.ReviewerID = gongfeng.Ptr(id)
		}
	}
	if opts.NecessaryReviewerID != "" {
		if id, err := strconv.Atoi(opts.NecessaryReviewerID); err == nil {
			sdkOpts.NecessaryReviewerID = gongfeng.Ptr(id)
		}
	}
	_, err := t.client.Reviews.InviteCommitReviewer(ctx, pid(owner, repo), reviewID, sdkOpts)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "InviteCodeReviewer", err)
	}
	return nil
}

func (t *Provider) RemoveCodeReviewer(ctx context.Context, owner, repo string, reviewID int, reviewerID string) error {
	sdkOpts := &gongfeng.RemoveCommitReviewerOptions{}
	if id, err := strconv.Atoi(reviewerID); err == nil {
		sdkOpts.ReviewerID = gongfeng.Ptr(id)
	}
	_, err := t.client.Reviews.RemoveCommitReviewer(ctx, pid(owner, repo), reviewID, sdkOpts)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "RemoveCodeReviewer", err)
	}
	return nil
}

func (t *Provider) SubmitCodeReview(ctx context.Context, owner, repo string, reviewID int, opts SubmitReviewOptions) error {
	sdkOpts := &gongfeng.SubmitCommitReviewSummaryOptions{
		ReviewerEvent: gongfeng.Ptr(string(opts.Event)),
		Summary:       gongfeng.Ptr(opts.Summary),
	}
	_, _, err := t.client.Reviews.SubmitCommitReviewSummary(ctx, pid(owner, repo), reviewID, sdkOpts)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "SubmitCodeReview", err)
	}
	return nil
}

func (t *Provider) ReopenCodeReview(ctx context.Context, owner, repo string, reviewID int) error {
	_, _, err := t.client.Reviews.ReopenCommitReview(ctx, pid(owner, repo), reviewID)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "ReopenCodeReview", err)
	}
	return nil
}

func (t *Provider) GetCodeReviewChangedFiles(ctx context.Context, owner, repo string, reviewID int) ([]*provider.ChangedFile, error) {
	// The SDK provides DownloadCommitReviewChangedFiles which writes to an io.Writer.
	// For JSON parsing, use a raw API call via the SDK client.
	var files []*provider.ChangedFile
	err := t.doRequest(ctx, "GetCodeReviewChangedFiles", "GET",
		fmt.Sprintf("projects/%s/review/%d/changed_files", esc(pid(owner, repo)), reviewID), nil, &files)
	if err != nil {
		return nil, err
	}
	return files, nil
}

// ===========================================================================
// MR review workflow (/merge_request/{iid}/review*)
// ===========================================================================

func (t *Provider) GetMRReview(ctx context.Context, owner, repo string, mrNumber int) (*MRReview, error) {
	review, _, err := t.client.Reviews.GetMRReview(ctx, pid(owner, repo), mrNumber)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetMRReview", err)
	}
	return reviewToMRReview(review), nil
}

func (t *Provider) InviteMRReviewer(ctx context.Context, owner, repo string, mrNumber int, opts InviteReviewerOptions) error {
	sdkOpts := &gongfeng.InviteMRReviewerOptions{}
	if opts.ReviewerID != "" {
		if id, err := strconv.Atoi(opts.ReviewerID); err == nil {
			sdkOpts.ReviewerID = gongfeng.Ptr(id)
		}
	}
	if opts.NecessaryReviewerID != "" {
		if id, err := strconv.Atoi(opts.NecessaryReviewerID); err == nil {
			sdkOpts.NecessaryReviewerID = gongfeng.Ptr(id)
		}
	}
	_, err := t.client.Reviews.InviteMRReviewer(ctx, pid(owner, repo), mrNumber, sdkOpts)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "InviteMRReviewer", err)
	}
	return nil
}

func (t *Provider) RemoveMRReviewer(ctx context.Context, owner, repo string, mrNumber int, reviewerID int) error {
	sdkOpts := &gongfeng.RemoveMRReviewerOptions{
		ReviewerID: gongfeng.Ptr(reviewerID),
	}
	_, err := t.client.Reviews.RemoveMRReviewer(ctx, pid(owner, repo), mrNumber, sdkOpts)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "RemoveMRReviewer", err)
	}
	return nil
}

func (t *Provider) CancelMRReview(ctx context.Context, owner, repo string, mrNumber int) error {
	_, err := t.client.Reviews.CancelMRReview(ctx, pid(owner, repo), mrNumber)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "CancelMRReview", err)
	}
	return nil
}

func (t *Provider) SubmitMRReview(ctx context.Context, owner, repo string, mrNumber int, opts SubmitReviewOptions) error {
	sdkOpts := &gongfeng.SubmitMRReviewSummaryOptions{
		ReviewerEvent: gongfeng.Ptr(string(opts.Event)),
		Summary:       gongfeng.Ptr(opts.Summary),
	}
	_, _, err := t.client.Reviews.SubmitMRReviewSummary(ctx, pid(owner, repo), mrNumber, sdkOpts)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "SubmitMRReview", err)
	}
	return nil
}

func (t *Provider) ReopenMRReview(ctx context.Context, owner, repo string, mrNumber int) error {
	_, _, err := t.client.Reviews.ReopenMRReview(ctx, pid(owner, repo), mrNumber)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "ReopenMRReview", err)
	}
	return nil
}

// ===========================================================================
// Commit-level operations (/repository/commits/{sha})
// ===========================================================================

func (t *Provider) GetCommitDiff(ctx context.Context, owner, repo, sha string, opts CommitDiffOptions) ([]*provider.ChangedFile, error) {
	sdkOpts := &gongfeng.GetCommitDiffOptions{}
	if opts.Path != "" {
		sdkOpts.Path = gongfeng.Ptr(opts.Path)
	}
	if opts.IgnoreWhiteSpace {
		sdkOpts.IgnoreWhiteSpace = gongfeng.Ptr(true)
	}
	diffs, _, err := t.client.Commits.GetCommitDiff(ctx, pid(owner, repo), sha, sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetCommitDiff", err)
	}
	result := make([]*provider.ChangedFile, 0, len(diffs))
	for _, d := range diffs {
		result = append(result, convertDiff(d))
	}
	return result, nil
}

func (t *Provider) ListCommitComments(ctx context.Context, owner, repo, sha string, page, perPage int) ([]*CommitComment, error) {
	page, perPage = provider.NormalizePageOpts(page, perPage)
	sdkOpts := &gongfeng.ListCommitCommentsOptions{
		ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage},
	}
	comments, _, err := t.client.Commits.ListCommitComments(ctx, pid(owner, repo), sha, sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListCommitComments", err)
	}
	result := make([]*CommitComment, 0, len(comments))
	for _, c := range comments {
		cc := &CommitComment{
			Body:     c.Note,
			Path:     c.Path,
			Line:     c.Line,
			LineType: c.LineType,
		}
		if c.Author != nil {
			cc.Author = convertUser(c.Author)
		}
		result = append(result, cc)
	}
	return result, nil
}

func (t *Provider) CreateCommitComment(ctx context.Context, owner, repo, sha string, opts CreateCommitCommentOptions) (*CommitComment, error) {
	sdkOpts := &gongfeng.CreateCommitCommentOptions{
		Note: gongfeng.Ptr(opts.Note),
	}
	if opts.Path != "" {
		sdkOpts.Path = gongfeng.Ptr(opts.Path)
	}
	if opts.Line > 0 {
		sdkOpts.Line = gongfeng.Ptr(opts.Line)
	}
	if opts.LineType != "" {
		sdkOpts.LineType = gongfeng.Ptr(opts.LineType)
	}
	comment, _, err := t.client.Commits.CreateCommitComment(ctx, pid(owner, repo), sha, sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateCommitComment", err)
	}
	cc := &CommitComment{
		Body:     comment.Note,
		Path:     comment.Path,
		Line:     comment.Line,
		LineType: comment.LineType,
	}
	if comment.Author != nil {
		cc.Author = convertUser(comment.Author)
	}
	return cc, nil
}

func (t *Provider) GetCommitRefs(ctx context.Context, owner, repo, sha string, refType CommitRefType) (*CommitRefs, error) {
	sdkOpts := &gongfeng.ListCommitRefsOptions{}
	if refType != "" {
		sdkOpts.Type = gongfeng.Ptr(string(refType))
	}
	refs, _, err := t.client.Commits.ListCommitRefs(ctx, pid(owner, repo), sha, sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetCommitRefs", err)
	}
	result := &CommitRefs{}
	for _, ref := range refs {
		switch ref.Type {
		case "branch":
			result.Branches = append(result.Branches, ref.Name)
		case "tag":
			result.Tags = append(result.Tags, ref.Name)
		default:
			result.Branches = append(result.Branches, ref.Name)
		}
	}
	return result, nil
}

// ===========================================================================
// Repository tree & blob
// ===========================================================================

func (t *Provider) GetRepoTree(ctx context.Context, owner, repo, path, ref string, recursive bool) ([]*TreeEntryNode, error) {
	sdkOpts := &gongfeng.ListTreeOptions{}
	if path != "" {
		sdkOpts.Path = gongfeng.Ptr(path)
	}
	if ref != "" {
		sdkOpts.RefName = gongfeng.Ptr(ref)
	}
	nodes, _, err := t.client.Repositories.ListTree(ctx, pid(owner, repo), sdkOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetRepoTree", err)
	}
	result := make([]*TreeEntryNode, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, &TreeEntryNode{
			ID:   n.ID,
			Name: n.Name,
			Type: n.Type,
			Mode: n.Mode,
		})
	}
	return result, nil
}

func (t *Provider) GetBlob(ctx context.Context, owner, repo, sha string) ([]byte, error) {
	// The SDK does not expose a blob endpoint directly. Use a raw API call.
	var blob struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	err := t.doRequest(ctx, "GetBlob", "GET",
		fmt.Sprintf("projects/%s/repository/blobs/%s", esc(pid(owner, repo)), esc(sha)), nil, &blob)
	if err != nil {
		return nil, err
	}
	if blob.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(blob.Content, "\n", ""))
		if err != nil {
			return nil, err
		}
		return decoded, nil
	}
	return []byte(blob.Content), nil
}

// ===========================================================================
// Branch protection
// ===========================================================================

func (t *Provider) ProtectBranch(ctx context.Context, owner, repo, branch string, opts ProtectBranchOptions) error {
	return t.doRequest(ctx, "ProtectBranch", "PUT",
		fmt.Sprintf("projects/%s/repository/branches/%s/protect", esc(pid(owner, repo)), esc(branch)), opts, nil)
}

func (t *Provider) UnprotectBranch(ctx context.Context, owner, repo, branch string) error {
	return t.doRequest(ctx, "UnprotectBranch", "PUT",
		fmt.Sprintf("projects/%s/repository/branches/%s/unprotect", esc(pid(owner, repo)), esc(branch)), nil, nil)
}

func (t *Provider) ListProtectedBranchMembers(ctx context.Context, owner, repo, branch string) ([]*ProtectedBranchMember, error) {
	var members []*ProtectedBranchMember
	err := t.doRequest(ctx, "ListProtectedBranchMembers", "GET",
		fmt.Sprintf("projects/%s/branches/protected/%s/members", esc(pid(owner, repo)), esc(branch)), nil, &members)
	if err != nil {
		return nil, err
	}
	return members, nil
}

func (t *Provider) AddProtectedBranchMember(ctx context.Context, owner, repo, branch string, opts ProtectedBranchMemberOptions) error {
	body := map[string]any{"user_id": opts.UserID}
	if opts.AccessLevel != 0 {
		body["access_level"] = opts.AccessLevel
	}
	return t.doRequest(ctx, "AddProtectedBranchMember", "POST",
		fmt.Sprintf("projects/%s/branches/protected/%s/members", esc(pid(owner, repo)), esc(branch)), body, nil)
}

func (t *Provider) UpdateProtectedBranchMember(ctx context.Context, owner, repo, branch string, userID int, opts ProtectedBranchMemberOptions) error {
	body := map[string]any{}
	if opts.AccessLevel != 0 {
		body["access_level"] = opts.AccessLevel
	}
	return t.doRequest(ctx, "UpdateProtectedBranchMember", "PUT",
		fmt.Sprintf("projects/%s/branches/protected/%s/members/%d", esc(pid(owner, repo)), esc(branch), userID), body, nil)
}

func (t *Provider) RemoveProtectedBranchMember(ctx context.Context, owner, repo, branch string, userID int) error {
	return t.doRequest(ctx, "RemoveProtectedBranchMember", "DELETE",
		fmt.Sprintf("projects/%s/branches/protected/%s/members/%d", esc(pid(owner, repo)), esc(branch), userID), nil, nil)
}

// ===========================================================================
// Helpers to convert SDK types to extras types
// ===========================================================================

func reviewToCodeReview(r *gongfeng.Review) *CodeReview {
	cr := &CodeReview{
		ID:          r.ID,
		ProjectID:   r.ProjectID,
		Title:       r.Title,
		Description: r.Description,
		State:       r.State,
		CreatedAt:   r.CreatedAt.String(),
		UpdatedAt:   r.UpdatedAt.String(),
	}
	if r.Author != nil {
		cr.Author = convertUser(r.Author)
	}
	return cr
}

func reviewToMRReview(r *gongfeng.Review) *MRReview {
	mr := &MRReview{
		ID:          r.ID,
		ProjectID:   r.ProjectID,
		State:       r.State,
		Title:       r.Title,
		Description: r.Description,
		CreatedAt:   r.CreatedAt.String(),
		UpdatedAt:   r.UpdatedAt.String(),
	}
	if r.Author != nil {
		mr.Author = convertUser(r.Author)
	}
	return mr
}
