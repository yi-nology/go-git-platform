package gitlab

import (
	"context"

	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/model"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/provider"
)

// emojiFromSDK maps GitLab award-emoji names to the SDK's canonical names.
var emojiFromSDK = map[string]string{
	"thumbsup":   model.ReactionPlusOne,
	"thumbsdown": model.ReactionMinusOne,
	"tada":       model.ReactionHooray,
	"laugh":      model.ReactionLaugh,
	"confused":   model.ReactionConfused,
	"heart":      model.ReactionHeart,
	"rocket":     model.ReactionRocket,
	"eyes":       model.ReactionEyes,
}

// emojiToSDK maps the SDK's canonical emoji names to GitLab award-emoji names.
var emojiToSDK = map[string]string{
	model.ReactionPlusOne:  "thumbsup",
	model.ReactionMinusOne: "thumbsdown",
	model.ReactionHooray:   "tada",
	model.ReactionLaugh:    "laugh",
	model.ReactionConfused: "confused",
	model.ReactionHeart:    "heart",
	model.ReactionRocket:   "rocket",
	model.ReactionEyes:     "eyes",
}

// ListIssueReactions implements provider.ReactionManager.
func (p *Provider) ListIssueReactions(ctx context.Context, owner, repo, number string) ([]*model.Reaction, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "ListIssueReactions", number)
	if err != nil {
		return nil, err
	}
	emojis, err := backendutil.AllPages(func(page int) ([]*gitlab.AwardEmoji, error) {
		list, _, err := p.client.AwardEmoji.ListIssueAwardEmoji(pidOf(owner, repo), n, &gitlab.ListAwardEmojiOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListIssueReactions", err)
	}
	return convertAwardEmojis(emojis), nil
}

// AddIssueReaction implements provider.ReactionManager.
func (p *Provider) AddIssueReaction(ctx context.Context, owner, repo, number, emoji string) (*model.Reaction, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "AddIssueReaction", number)
	if err != nil {
		return nil, err
	}
	glName := emojiToGitLab(emoji)
	e, _, err := p.client.AwardEmoji.CreateIssueAwardEmoji(pidOf(owner, repo), n, &gitlab.CreateAwardEmojiOptions{Name: glName}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "AddIssueReaction", err)
	}
	return convertAwardEmoji(e), nil
}

// RemoveIssueReaction implements provider.ReactionManager.
func (p *Provider) RemoveIssueReaction(ctx context.Context, owner, repo, number string, reactionID int64) error {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "RemoveIssueReaction", number)
	if err != nil {
		return err
	}
	_, err = p.client.AwardEmoji.DeleteIssueAwardEmoji(pidOf(owner, repo), n, reactionID, gitlab.WithContext(ctx))
	return provider.Wrap(model.PlatformGitLab, "RemoveIssueReaction", err)
}

// ListCRReactions implements provider.ReactionManager.
func (p *Provider) ListCRReactions(ctx context.Context, owner, repo, number string) ([]*model.Reaction, error) {
	n, err := backendutil.ParseIssueNumber64(model.PlatformGitLab, "ListCRReactions", number)
	if err != nil {
		return nil, err
	}
	emojis, err := backendutil.AllPages(func(page int) ([]*gitlab.AwardEmoji, error) {
		list, _, err := p.client.AwardEmoji.ListMergeRequestAwardEmoji(pidOf(owner, repo), n, &gitlab.ListAwardEmojiOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "ListCRReactions", err)
	}
	return convertAwardEmojis(emojis), nil
}

// --- note-level reactions: one shape, one verdict (issue #57) ---
//
// GitLab's award-emoji-on-note API (GET/POST/DELETE
// /projects/:id/issues/:iid/notes/:note_id/award_emoji and the MR/PR
// counterpart) addresses the reaction by the PARENT awardable (issue IID /
// MR IID) plus the note ID. The provider interface addresses comments by
// their platform ID alone, so the endpoint cannot be driven without an
// interface change — a genuine platform-side capability gap, registered as
// DivergenceStub entries in the backend ledger. All six note-level methods
// fail with ErrNotImplemented and touch no wire: the previous mixed forms
// ((nil, nil) silent-empty reads that were indistinguishable from "no
// reactions", plus sentinel-less text errors on writes) left both escape
// hatches — IsNotImplemented and IsNotFound — false at the same time.

// ListIssueCommentReactions implements provider.ReactionManager.
// On GitLab, issue comments are "notes" on the issue; see the note-level
// header comment for why this cannot be implemented on the current surface.
func (p *Provider) ListIssueCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*model.Reaction, error) {
	return nil, provider.Wrap(model.PlatformGitLab, "ListIssueCommentReactions",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent issue IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// AddIssueCommentReaction implements provider.ReactionManager.
func (p *Provider) AddIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*model.Reaction, error) {
	return nil, provider.Wrap(model.PlatformGitLab, "AddIssueCommentReaction",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent issue IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// RemoveIssueCommentReaction implements provider.ReactionManager.
func (p *Provider) RemoveIssueCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) error {
	return provider.Wrap(model.PlatformGitLab, "RemoveIssueCommentReaction",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent issue IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// ListCRCommentReactions implements provider.ReactionManager.
func (p *Provider) ListCRCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*model.Reaction, error) {
	return nil, provider.Wrap(model.PlatformGitLab, "ListCRCommentReactions",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent MR IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// AddCRCommentReaction implements provider.ReactionManager.
func (p *Provider) AddCRCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*model.Reaction, error) {
	return nil, provider.Wrap(model.PlatformGitLab, "AddCRCommentReaction",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent MR IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// RemoveCRCommentReaction implements provider.ReactionManager.
func (p *Provider) RemoveCRCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) error {
	return provider.Wrap(model.PlatformGitLab, "RemoveCRCommentReaction",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent MR IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

func convertAwardEmojis(in []*gitlab.AwardEmoji) []*model.Reaction {
	out := make([]*model.Reaction, 0, len(in))
	for _, e := range in {
		out = append(out, convertAwardEmoji(e))
	}
	return out
}

func convertAwardEmoji(e *gitlab.AwardEmoji) *model.Reaction {
	if e == nil {
		return nil
	}
	name := e.Name
	if mapped, ok := emojiFromSDK[name]; ok {
		name = mapped
	}
	out := &model.Reaction{
		ID:    e.ID,
		Emoji: name,
	}
	out.User = &model.CRUser{
		ID:       e.User.ID,
		Username: e.User.Username,
	}
	return out
}

// emojiToGitLab maps a SDK canonical emoji name to GitLab's award-emoji
// name. Unknown names are passed through unchanged so that callers can
// use GitLab-native names directly if needed.
func emojiToGitLab(emoji string) string {
	if gl, ok := emojiToSDK[emoji]; ok {
		return gl
	}
	return emoji
}

var _ provider.ReactionManager = (*Provider)(nil)
