package github

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/google/go-github/v92/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// ListIssueReactions implements provider.ReactionManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListIssueReactions(ctx context.Context, owner, repo, number string) ([]*provider.Reaction, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "ListIssueReactions", number)
	if err != nil {
		return nil, err
	}
	reactions, err := backendutil.AllPages(func(page int) ([]*github.Reaction, error) {
		list, _, err := p.client.Reactions.ListIssueReactions(ctx, owner, repo, n, &github.ListReactionOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: 100},
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListIssueReactions", err)
	}
	return convertReactions(reactions), nil
}

// AddIssueReaction implements provider.ReactionManager.
func (p *Provider) AddIssueReaction(ctx context.Context, owner, repo, number, emoji string) (*provider.Reaction, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "AddIssueReaction", number)
	if err != nil {
		return nil, err
	}
	r, _, err := p.client.Reactions.CreateIssueReaction(ctx, owner, repo, n, emoji)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "AddIssueReaction", err)
	}
	return convertReaction(r), nil
}

// RemoveIssueReaction implements provider.ReactionManager.
func (p *Provider) RemoveIssueReaction(ctx context.Context, owner, repo, number string, reactionID int64) error {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "RemoveIssueReaction", number)
	if err != nil {
		return err
	}
	_, err = p.client.Reactions.DeleteIssueReaction(ctx, owner, repo, n, reactionID)
	return provider.Wrap(provider.PlatformGitHub, "RemoveIssueReaction", err)
}

// ListCRReactions implements provider.ReactionManager.
// On GitHub, PRs share the issue reactions API. The provider interface
// exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListCRReactions(ctx context.Context, owner, repo, number string) ([]*provider.Reaction, error) {
	n, err := backendutil.ParseIssueNumber(provider.PlatformGitHub, "ListCRReactions", number)
	if err != nil {
		return nil, err
	}
	reactions, err := backendutil.AllPages(func(page int) ([]*github.Reaction, error) {
		list, _, err := p.client.Reactions.ListIssueReactions(ctx, owner, repo, n, &github.ListReactionOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: 100},
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListCRReactions", err)
	}
	return convertReactions(reactions), nil
}

// ListIssueCommentReactions implements provider.ReactionManager. The
// provider interface exposes no paging parameters, so pagination is
// exhausted via backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListIssueCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*provider.Reaction, error) {
	reactions, err := backendutil.AllPages(func(page int) ([]*github.Reaction, error) {
		list, _, err := p.client.Reactions.ListIssueCommentReactions(ctx, owner, repo, commentID, &github.ListReactionOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: 100},
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListIssueCommentReactions", err)
	}
	return convertReactions(reactions), nil
}

// AddIssueCommentReaction implements provider.ReactionManager.
func (p *Provider) AddIssueCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*provider.Reaction, error) {
	r, _, err := p.client.Reactions.CreateIssueCommentReaction(ctx, owner, repo, commentID, emoji)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "AddIssueCommentReaction", err)
	}
	return convertReaction(r), nil
}

// RemoveIssueCommentReaction implements provider.ReactionManager.
func (p *Provider) RemoveIssueCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) error {
	_, err := p.client.Reactions.DeleteIssueCommentReaction(ctx, owner, repo, commentID, reactionID)
	return provider.Wrap(provider.PlatformGitHub, "RemoveIssueCommentReaction", err)
}

// ListCRCommentReactions implements provider.ReactionManager. The provider
// interface exposes no paging parameters, so pagination is exhausted via
// backendutil.AllPages (100 per page until an empty page).
func (p *Provider) ListCRCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]*provider.Reaction, error) {
	reactions, err := backendutil.AllPages(func(page int) ([]*github.Reaction, error) {
		list, _, err := p.client.Reactions.ListPullRequestCommentReactions(ctx, owner, repo, commentID, &github.ListReactionOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: 100},
		})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "ListCRCommentReactions", err)
	}
	return convertReactions(reactions), nil
}

// AddCRCommentReaction implements provider.ReactionManager.
func (p *Provider) AddCRCommentReaction(ctx context.Context, owner, repo string, commentID int64, emoji string) (*provider.Reaction, error) {
	r, _, err := p.client.Reactions.CreatePullRequestCommentReaction(ctx, owner, repo, commentID, emoji)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "AddCRCommentReaction", err)
	}
	return convertReaction(r), nil
}

// RemoveCRCommentReaction implements provider.ReactionManager.
func (p *Provider) RemoveCRCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) error {
	_, err := p.client.Reactions.DeletePullRequestCommentReaction(ctx, owner, repo, commentID, reactionID)
	return provider.Wrap(provider.PlatformGitHub, "RemoveCRCommentReaction", err)
}

func convertReactions(in []*github.Reaction) []*provider.Reaction {
	out := make([]*provider.Reaction, 0, len(in))
	for _, r := range in {
		out = append(out, convertReaction(r))
	}
	return out
}

func convertReaction(r *github.Reaction) *provider.Reaction {
	if r == nil {
		return nil
	}
	out := &provider.Reaction{
		ID:    r.GetID(),
		Emoji: r.GetContent(),
	}
	if r.User != nil {
		out.User = &provider.CRUser{
			ID:       r.User.GetID(),
			Username: r.User.GetLogin(),
		}
	}
	return out
}

var _ provider.ReactionManager = (*Provider)(nil)
