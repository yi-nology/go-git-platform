package gitlab

import (
	"context"

	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

// emojiFromSDK maps GitLab award-emoji names to the SDK's canonical names.
var emojiFromSDK = map[string]string{
	"thumbsup":   provider.ReactionPlusOne,
	"thumbsdown": provider.ReactionMinusOne,
	"tada":       provider.ReactionHooray,
	"laugh":      provider.ReactionLaugh,
	"confused":   provider.ReactionConfused,
	"heart":      provider.ReactionHeart,
	"rocket":     provider.ReactionRocket,
	"eyes":       provider.ReactionEyes,
}

// emojiToSDK maps the SDK's canonical emoji names to GitLab award-emoji names.
var emojiToSDK = map[string]string{
	provider.ReactionPlusOne:  "thumbsup",
	provider.ReactionMinusOne: "thumbsdown",
	provider.ReactionHooray:   "tada",
	provider.ReactionLaugh:    "laugh",
	provider.ReactionConfused: "confused",
	provider.ReactionHeart:    "heart",
	provider.ReactionRocket:   "rocket",
	provider.ReactionEyes:     "eyes",
}

// ListIssueReactions implements provider.ReactionManager.
func (p *Provider) ListIssueReactions(ctx context.Context, owner, repo, number string) ([]*provider.Reaction, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformGitLab, "ListIssueReactions", number)
	if err != nil {
		return nil, err
	}
	emojis, err := backendutil.AllPages(func(page int) ([]*gitlab.AwardEmoji, error) {
		list, _, err := p.client.AwardEmoji.ListIssueAwardEmoji(pidOf(owner, repo), n, &gitlab.ListAwardEmojiOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListIssueReactions", err)
	}
	return convertAwardEmojis(emojis), nil
}

// AddIssueReaction implements provider.ReactionManager.
func (p *Provider) AddIssueReaction(ctx context.Context, owner, repo, number, emoji string) (*provider.Reaction, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformGitLab, "AddIssueReaction", number)
	if err != nil {
		return nil, err
	}
	glName := emojiToGitLab(emoji)
	e, _, err := p.client.AwardEmoji.CreateIssueAwardEmoji(pidOf(owner, repo), n, &gitlab.CreateAwardEmojiOptions{Name: glName}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "AddIssueReaction", err)
	}
	return convertAwardEmoji(e), nil
}

// RemoveIssueReaction implements provider.ReactionManager.
func (p *Provider) RemoveIssueReaction(ctx context.Context, owner, repo, number string, reactionID int64) error {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformGitLab, "RemoveIssueReaction", number)
	if err != nil {
		return err
	}
	_, err = p.client.AwardEmoji.DeleteIssueAwardEmoji(pidOf(owner, repo), n, reactionID, gitlab.WithContext(ctx))
	return provider.Wrap(provider.PlatformGitLab, "RemoveIssueReaction", err)
}

// ListCRReactions implements provider.ReactionManager.
func (p *Provider) ListCRReactions(ctx context.Context, owner, repo, number string) ([]*provider.Reaction, error) {
	n, err := backendutil.ParseIssueNumber64(provider.PlatformGitLab, "ListCRReactions", number)
	if err != nil {
		return nil, err
	}
	emojis, err := backendutil.AllPages(func(page int) ([]*gitlab.AwardEmoji, error) {
		list, _, err := p.client.AwardEmoji.ListMergeRequestAwardEmoji(pidOf(owner, repo), n, &gitlab.ListAwardEmojiOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListCRReactions", err)
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
func (p *Provider) ListIssueCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*provider.Reaction, error) {
	return nil, provider.Wrap(provider.PlatformGitLab, "ListIssueCommentReactions",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent issue IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// AddIssueCommentReaction implements provider.ReactionManager.
func (p *Provider) AddIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*provider.Reaction, error) {
	return nil, provider.Wrap(provider.PlatformGitLab, "AddIssueCommentReaction",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent issue IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// RemoveIssueCommentReaction implements provider.ReactionManager.
func (p *Provider) RemoveIssueCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) error {
	return provider.Wrap(provider.PlatformGitLab, "RemoveIssueCommentReaction",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent issue IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// ListCRCommentReactions implements provider.ReactionManager.
func (p *Provider) ListCRCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*provider.Reaction, error) {
	return nil, provider.Wrap(provider.PlatformGitLab, "ListCRCommentReactions",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent MR IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// AddCRCommentReaction implements provider.ReactionManager.
func (p *Provider) AddCRCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*provider.Reaction, error) {
	return nil, provider.Wrap(provider.PlatformGitLab, "AddCRCommentReaction",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent MR IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

// RemoveCRCommentReaction implements provider.ReactionManager.
func (p *Provider) RemoveCRCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) error {
	return provider.Wrap(provider.PlatformGitLab, "RemoveCRCommentReaction",
		oekiterrors.Wrap(provider.ErrNotImplemented, "gitlab note award-emoji API needs the parent MR IID alongside the note ID, which the provider interface cannot address; use the raw client directly"))
}

func convertAwardEmojis(in []*gitlab.AwardEmoji) []*provider.Reaction {
	out := make([]*provider.Reaction, 0, len(in))
	for _, e := range in {
		out = append(out, convertAwardEmoji(e))
	}
	return out
}

func convertAwardEmoji(e *gitlab.AwardEmoji) *provider.Reaction {
	if e == nil {
		return nil
	}
	name := e.Name
	if mapped, ok := emojiFromSDK[name]; ok {
		name = mapped
	}
	out := &provider.Reaction{
		ID:    e.ID,
		Emoji: name,
	}
	out.User = &provider.CRUser{
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
