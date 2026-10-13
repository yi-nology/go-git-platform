package giteafamily

import (
	"context"
	"time"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- releases ---

// FamilyTag is the slice of the family's tag object the engine consumes.
type FamilyTag struct {
	Name      string
	CommitSHA string
}

// FamilyRelease is the slice of the family's release object the engine
// consumes.
type FamilyRelease struct {
	ID           int64
	TagName      string
	Title        string
	Note         string
	URL          string
	IsDraft      bool
	IsPrerelease bool
	CreatedAt    time.Time
	PublishedAt  time.Time
}

// FamilyCreateReleaseOption mirrors the family's release create body.
type FamilyCreateReleaseOption struct {
	TagName      string
	Target       string
	Title        string
	Note         string
	IsDraft      bool
	IsPrerelease bool
}

// FamilyEditReleaseOption mirrors the family's release edit body (draft and
// prerelease are pointers so nil passes through untouched).
type FamilyEditReleaseOption struct {
	TagName      string
	Title        string
	Note         string
	IsDraft      *bool
	IsPrerelease *bool
}

// ReleasePort is the release + tag + archive endpoint of the family SDK
// surface. format is the provider's archive vocabulary ("zip" or anything
// else for tar.gz); the adapters map it onto the SDK's ArchiveType.
type ReleasePort interface {
	ListRepoTags(ctx context.Context, owner, repo string, opt ListOptions) ([]*FamilyTag, error)
	ListReleases(ctx context.Context, owner, repo string, opt ListOptions) ([]*FamilyRelease, error)
	CreateRelease(ctx context.Context, owner, repo string, opt FamilyCreateReleaseOption) (*FamilyRelease, error)
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*FamilyRelease, error)
	EditRelease(ctx context.Context, owner, repo string, id int64, opt FamilyEditReleaseOption) (*FamilyRelease, error)
	DeleteReleaseByTag(ctx context.Context, owner, repo, tag string) error
	GetArchive(ctx context.Context, owner, repo, ref, format string) ([]byte, error)
}

// ListTags lists every tag (the provider surface carries no pagination
// parameters; the engine walks the endpoint).
func ListTags(ctx context.Context, f Family, c ReleasePort, owner, repo string) ([]*provider.TagInfo, error) {
	tags, err := backendutil.AllPages(func(page int) ([]*FamilyTag, error) {
		return c.ListRepoTags(ctx, owner, repo, ListOptions{Page: page, PageSize: f.PageSize})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListTags", err)
	}
	result := make([]*provider.TagInfo, 0, len(tags))
	for _, t := range tags {
		result = append(result, &provider.TagInfo{Name: t.Name, Commit: t.CommitSHA})
	}
	return result, nil
}

// ListReleases lists every release (the provider surface carries no
// pagination parameters; the engine walks the endpoint).
func ListReleases(ctx context.Context, f Family, c ReleasePort, owner, repo string) ([]*provider.ReleaseInfo, error) {
	releases, err := backendutil.AllPages(func(page int) ([]*FamilyRelease, error) {
		return c.ListReleases(ctx, owner, repo, ListOptions{Page: page, PageSize: f.PageSize})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListReleases", err)
	}
	result := make([]*provider.ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		result = append(result, convertRelease(r))
	}
	return result, nil
}

// CreateRelease creates a release. The family's CreateReleaseOption.Target
// always serializes (target_commitish carries no omitempty), so a
// Target-less create sends an empty string, which the server treats as
// unchanged (the default branch).
func CreateRelease(ctx context.Context, f Family, c ReleasePort, owner, repo string, opts provider.CreateReleaseOptions) (*provider.ReleaseInfo, error) {
	r, err := c.CreateRelease(ctx, owner, repo, FamilyCreateReleaseOption{
		TagName:      opts.TagName,
		Target:       opts.Target,
		Title:        opts.Title,
		Note:         opts.Body,
		IsDraft:      opts.Draft,
		IsPrerelease: opts.Prerelease,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateRelease", err)
	}
	return convertRelease(r), nil
}

// GetReleaseByTag fetches one release addressed by its tag (the family
// exposes a dedicated tag-addressed endpoint).
func GetReleaseByTag(ctx context.Context, f Family, c ReleasePort, owner, repo, tag string) (*provider.ReleaseInfo, error) {
	r, err := c.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetReleaseByTag", err)
	}
	return convertRelease(r), nil
}

// UpdateRelease edits a release addressed by tag. The edit endpoint is
// id-addressed, so the tag is resolved through the by-tag endpoint first
// (exact lookup, no list window). The family's EditReleaseOption name/note
// fields are plain strings that would clobber the release when sent empty,
// so the current values from the resolution fetch backfill every option the
// caller left nil; draft/prerelease are pointers that pass through.
func UpdateRelease(ctx context.Context, f Family, c ReleasePort, owner, repo, tag string, opts provider.UpdateReleaseOptions) (*provider.ReleaseInfo, error) {
	cur, err := c.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateRelease", err)
	}
	title, note := cur.Title, cur.Note
	if opts.Name != nil {
		title = *opts.Name
	}
	if opts.Body != nil {
		note = *opts.Body
	}
	r, err := c.EditRelease(ctx, owner, repo, cur.ID, FamilyEditReleaseOption{
		TagName:      tag,
		Title:        title,
		Note:         note,
		IsDraft:      opts.Draft,
		IsPrerelease: opts.Prerelease,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateRelease", err)
	}
	return convertRelease(r), nil
}

// DeleteRelease removes a release via the tag-addressed delete (the family's
// server keeps the tag).
func DeleteRelease(ctx context.Context, f Family, c ReleasePort, owner, repo, tag string) error {
	if err := c.DeleteReleaseByTag(ctx, owner, repo, tag); err != nil {
		return provider.Wrap(f.Platform, "DeleteRelease", err)
	}
	return nil
}

// GetArchive downloads a repository archive at ref in the requested format
// ("zip" or tar.gz).
func GetArchive(ctx context.Context, f Family, c ReleasePort, owner, repo, ref, format string) ([]byte, error) {
	data, err := c.GetArchive(ctx, owner, repo, ref, format)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetArchive", err)
	}
	return data, nil
}

// convertRelease maps a family release to the provider shape.
func convertRelease(r *FamilyRelease) *provider.ReleaseInfo {
	if r == nil {
		return nil
	}
	return &provider.ReleaseInfo{
		ID:          r.ID,
		TagName:     r.TagName,
		Title:       r.Title,
		Body:        r.Note,
		URL:         r.URL,
		Draft:       r.IsDraft,
		Prerelease:  r.IsPrerelease,
		CreatedAt:   r.CreatedAt,
		PublishedAt: r.PublishedAt,
	}
}

// --- reactions ---

// FamilyReaction is the slice of the family's reaction object the engine
// consumes. The family has no reaction IDs: DeleteIssueReaction takes the
// reaction content string, so Reaction (the emoji) doubles as the delete
// key while UserID addresses the reaction for removal lookups.
type FamilyReaction struct {
	UserID   int64
	Username string
	Reaction string
}

// ReactionPort is the reaction endpoint of the family SDK surface.
// ListIssueReactions returns the FULL list (gitea walks pages internally;
// forgejo's endpoint is served unpaginated) — the port's contract is
// "all reactions", the pagination detail stays in the adapter.
type ReactionPort interface {
	ListIssueReactions(ctx context.Context, owner, repo string, index int64) ([]*FamilyReaction, error)
	PostIssueReaction(ctx context.Context, owner, repo string, index int64, emoji string) (*FamilyReaction, error)
	DeleteIssueReaction(ctx context.Context, owner, repo string, index int64, content string) error
	GetIssueCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*FamilyReaction, error)
	PostIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*FamilyReaction, error)
	DeleteIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, content string) error
}

// ListIssueReactions lists every reaction on an issue.
func ListIssueReactions(ctx context.Context, f Family, c ReactionPort, owner, repo, number string) ([]*provider.Reaction, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "ListIssueReactions", number)
	if err != nil {
		return nil, err
	}
	reactions, err := c.ListIssueReactions(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListIssueReactions", err)
	}
	return convertReactions(reactions), nil
}

// AddIssueReaction adds an emoji reaction to an issue.
func AddIssueReaction(ctx context.Context, f Family, c ReactionPort, owner, repo, number, emoji string) (*provider.Reaction, error) {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "AddIssueReaction", number)
	if err != nil {
		return nil, err
	}
	r, err := c.PostIssueReaction(ctx, owner, repo, n, emoji)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "AddIssueReaction", err)
	}
	return convertReaction(r), nil
}

// RemoveIssueReaction removes a reaction.
//
// KNOWN N+1 COST: the family's reactions have no IDs — reactionID is the
// reacting user's UserID (set in convertReaction) and
// DeleteIssueReaction only accepts the reaction content string, so the full
// list is scanned for the matching user's reaction first. An O(reactions)
// scan per call; unavoidable given the family's API design.
func RemoveIssueReaction(ctx context.Context, f Family, c ReactionPort, owner, repo, number string, reactionID int64) error {
	n, err := backendutil.ParseIssueNumber64(f.Platform, "RemoveIssueReaction", number)
	if err != nil {
		return err
	}
	reactions, err := c.ListIssueReactions(ctx, owner, repo, n)
	if err != nil {
		return provider.Wrap(f.Platform, "RemoveIssueReaction", err)
	}
	for _, r := range reactions {
		if r.UserID == reactionID {
			return provider.Wrap(f.Platform, "RemoveIssueReaction", c.DeleteIssueReaction(ctx, owner, repo, n, r.Reaction))
		}
	}
	return provider.Wrapf(f.Platform, "RemoveIssueReaction", "reaction %d not found", reactionID)
}

// ListCRReactions reports change-request-level reactions as unsupported: the
// family has no dedicated PR-level reaction API (PR reactions are issue
// reactions — use ListIssueReactions).
func ListCRReactions(ctx context.Context, f Family, c ReactionPort, owner, repo, number string) ([]*provider.Reaction, error) {
	return nil, provider.Wrapf(f.Platform, "ListCRReactions", "%s does not support change-request-level reactions; use ListIssueReactions instead", f.Platform)
}

// ListIssueCommentReactions lists every reaction on an issue comment.
func ListIssueCommentReactions(ctx context.Context, f Family, c ReactionPort, owner, repo string, commentID int64) ([]*provider.Reaction, error) {
	reactions, err := c.GetIssueCommentReactions(ctx, owner, repo, commentID)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListIssueCommentReactions", err)
	}
	return convertReactions(reactions), nil
}

// AddIssueCommentReaction adds an emoji reaction to an issue comment.
func AddIssueCommentReaction(ctx context.Context, f Family, c ReactionPort, owner, repo string, commentID int64, emoji string) (*provider.Reaction, error) {
	r, err := c.PostIssueCommentReaction(ctx, owner, repo, commentID, emoji)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "AddIssueCommentReaction", err)
	}
	return convertReaction(r), nil
}

// RemoveIssueCommentReaction removes a comment reaction (same N+1 scan as
// RemoveIssueReaction: the ID is the reacting user's UserID).
func RemoveIssueCommentReaction(ctx context.Context, f Family, c ReactionPort, owner, repo string, commentID, reactionID int64) error {
	reactions, err := c.GetIssueCommentReactions(ctx, owner, repo, commentID)
	if err != nil {
		return provider.Wrap(f.Platform, "RemoveIssueCommentReaction", err)
	}
	for _, r := range reactions {
		if r.UserID == reactionID {
			return provider.Wrap(f.Platform, "RemoveIssueCommentReaction", c.DeleteIssueCommentReaction(ctx, owner, repo, commentID, r.Reaction))
		}
	}
	return provider.Wrapf(f.Platform, "RemoveIssueCommentReaction", "reaction %d not found", reactionID)
}

// ListCRCommentReactions returns no reactions: the family has no PR-comment
// reaction API.
func ListCRCommentReactions(_ context.Context, _ Family, _ ReactionPort, _, _ string, _ int64) ([]*provider.Reaction, error) {
	return nil, nil
}

// AddCRCommentReaction reports PR-comment reactions as unsupported.
func AddCRCommentReaction(_ context.Context, f Family, _ ReactionPort, _, _ string, _ int64, _ string) (*provider.Reaction, error) {
	return nil, provider.Wrapf(f.Platform, "AddCRCommentReaction", "%s does not support reactions on pull-request review comments", f.Platform)
}

// RemoveCRCommentReaction reports PR-comment reactions as unsupported.
func RemoveCRCommentReaction(_ context.Context, f Family, _ ReactionPort, _, _ string, _, _ int64) error {
	return provider.Wrapf(f.Platform, "RemoveCRCommentReaction", "%s does not support reactions on pull-request review comments", f.Platform)
}

func convertReactions(in []*FamilyReaction) []*provider.Reaction {
	out := make([]*provider.Reaction, 0, len(in))
	for _, r := range in {
		out = append(out, convertReaction(r))
	}
	return out
}

func convertReaction(r *FamilyReaction) *provider.Reaction {
	if r == nil {
		return nil
	}
	out := &provider.Reaction{Emoji: r.Reaction, ID: r.UserID}
	if r.UserID != 0 || r.Username != "" {
		out.User = &provider.CRUser{ID: r.UserID, Username: r.Username}
	}
	return out
}

// --- branch protection ---

// FamilyBranchProtection is the slice of the family's branch-protection
// object the engine consumes.
type FamilyBranchProtection struct {
	BranchName             string
	RequiredApprovals      int64
	EnableStatusCheck      bool
	EnablePush             bool
	BlockOnRejectedReviews bool
}

// FamilyCreateBranchProtectionOption mirrors the family's protection create
// body.
type FamilyCreateBranchProtectionOption struct {
	BranchName        string
	RequiredApprovals int64
	EnableStatusCheck bool
	EnablePush        bool
}

// FamilyEditBranchProtectionOption mirrors the family's protection edit body
// (nil fields pass through untouched).
type FamilyEditBranchProtectionOption struct {
	RequiredApprovals      *int64
	EnableStatusCheck      *bool
	EnablePush             *bool
	BlockOnRejectedReviews *bool
}

// BranchProtectionPort is the branch-protection endpoint of the family SDK
// surface.
type BranchProtectionPort interface {
	ListBranchProtections(ctx context.Context, owner, repo string, opt ListOptions) ([]*FamilyBranchProtection, error)
	GetBranchProtection(ctx context.Context, owner, repo, branch string) (*FamilyBranchProtection, error)
	CreateBranchProtection(ctx context.Context, owner, repo string, opt FamilyCreateBranchProtectionOption) (*FamilyBranchProtection, error)
	EditBranchProtection(ctx context.Context, owner, repo, branch string, opt FamilyEditBranchProtectionOption) (*FamilyBranchProtection, error)
	DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error
}

// ListBranchProtections lists every protection rule (the provider surface
// carries no pagination parameters; the engine walks the endpoint).
func ListBranchProtections(ctx context.Context, f Family, c BranchProtectionPort, owner, repo string) ([]*provider.BranchProtection, error) {
	bps, err := backendutil.AllPages(func(page int) ([]*FamilyBranchProtection, error) {
		return c.ListBranchProtections(ctx, owner, repo, ListOptions{Page: page, PageSize: f.PageSize})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListBranchProtections", err)
	}
	result := make([]*provider.BranchProtection, 0, len(bps))
	for _, bp := range bps {
		result = append(result, convertBranchProtection(bp))
	}
	return result, nil
}

// GetBranchProtection fetches one protection rule by branch name.
func GetBranchProtection(ctx context.Context, f Family, c BranchProtectionPort, owner, repo, branch string) (*provider.BranchProtection, error) {
	bp, err := c.GetBranchProtection(ctx, owner, repo, branch)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetBranchProtection", err)
	}
	return convertBranchProtection(bp), nil
}

// CreateBranchProtection creates a protection rule.
func CreateBranchProtection(ctx context.Context, f Family, c BranchProtectionPort, owner, repo string, opts provider.CreateBranchProtectionOptions) (*provider.BranchProtection, error) {
	bp, err := c.CreateBranchProtection(ctx, owner, repo, FamilyCreateBranchProtectionOption{
		BranchName:        opts.BranchName,
		RequiredApprovals: int64(opts.RequiredApprovingReviews),
		EnableStatusCheck: opts.RequiredStatusChecks,
		EnablePush:        opts.AllowForcePushes,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateBranchProtection", err)
	}
	return convertBranchProtection(bp), nil
}

// UpdateBranchProtection edits a protection rule; nil option fields pass
// through untouched. AllowDeletions has no direct family field (the family's
// delete toggle rides BlockOnRejectedReviews inversely), so it maps to an
// explicit no-op — mirroring the backends' historical behavior.
func UpdateBranchProtection(ctx context.Context, f Family, c BranchProtectionPort, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*provider.BranchProtection, error) {
	edit := FamilyEditBranchProtectionOption{}
	if opts.RequiredApprovingReviews != nil {
		v := int64(*opts.RequiredApprovingReviews)
		edit.RequiredApprovals = &v
	}
	if opts.RequiredStatusChecks != nil {
		edit.EnableStatusCheck = opts.RequiredStatusChecks
	}
	if opts.AllowForcePushes != nil {
		edit.EnablePush = opts.AllowForcePushes
	}
	if opts.AllowDeletions != nil {
		edit.BlockOnRejectedReviews = nil // not directly mapped
	}
	bp, err := c.EditBranchProtection(ctx, owner, repo, branch, edit)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateBranchProtection", err)
	}
	return convertBranchProtection(bp), nil
}

// DeleteBranchProtection removes a protection rule.
func DeleteBranchProtection(ctx context.Context, f Family, c BranchProtectionPort, owner, repo, branch string) error {
	if err := c.DeleteBranchProtection(ctx, owner, repo, branch); err != nil {
		return provider.Wrap(f.Platform, "DeleteBranchProtection", err)
	}
	return nil
}

// convertBranchProtection maps a family protection rule to the provider
// shape. The family's delete toggle rides BlockOnRejectedReviews, so
// AllowDeletions is its inverse.
func convertBranchProtection(bp *FamilyBranchProtection) *provider.BranchProtection {
	return &provider.BranchProtection{
		BranchName:               bp.BranchName,
		RequiredApprovingReviews: int(bp.RequiredApprovals),
		RequiredStatusChecks:     bp.EnableStatusCheck,
		AllowForcePushes:         bp.EnablePush,
		AllowDeletions:           !bp.BlockOnRejectedReviews,
	}
}
