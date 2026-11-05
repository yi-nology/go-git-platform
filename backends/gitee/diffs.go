package gitee

import (
	"context"
	"strconv"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// GetCRDiff implements provider.DiffManager. The go-gitee SDK's
// PullRequests.ListFiles takes no options at all, so there is no
// pagination parameter to exhaust — the platform returns what it returns
// in a single response.
func (p *Provider) GetCRDiff(ctx context.Context, owner, repo, number string) (*provider.MergeDiff, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, "GetCRDiff", number)
	if err != nil {
		return nil, err
	}
	files, _, err := p.client.PullRequests.ListFiles(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "GetCRDiff", err)
	}
	diff := &provider.MergeDiff{}
	for _, f := range files {
		diff.Files = append(diff.Files, convertPRFile(f))
	}
	diff.TotalAdd, diff.TotalDel = provider.SumDiffStats(diff.Files)
	diff.RawDiff = provider.BuildRawDiff(diff.Files)
	return diff, nil
}

// GetCRFiles implements provider.DiffManager.
func (p *Provider) GetCRFiles(ctx context.Context, owner, repo, number string) ([]*provider.ChangedFile, error) {
	diff, err := p.GetCRDiff(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	return diff.Files, nil
}

// CreateNote implements provider.DiffManager.
func (p *Provider) CreateNote(ctx context.Context, owner, repo, number, body string) (string, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitee, "CreateNote", number)
	if err != nil {
		return "", err
	}
	opts := &gitee.CreatePullRequestCommentOptions{
		Body: gitee.String(body),
	}
	comment, _, err := p.client.PullRequests.CreateComment(ctx, backendutil.Esc(owner), backendutil.Esc(repo), n, opts)
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitee, "CreateNote", err)
	}
	return strconv.FormatInt(int64(deref(comment.ID)), 10), nil
}

// DeleteNote implements provider.DiffManager.
func (p *Provider) DeleteNote(ctx context.Context, owner, repo, number, noteID string) error {
	if _, err := backendutil.ParsePRNumber(provider.PlatformGitee, "DeleteNote", number); err != nil {
		return err
	}
	id, err := strconv.Atoi(noteID)
	if err != nil {
		return provider.Wrapf(provider.PlatformGitee, "DeleteNote", "invalid note id %q", noteID)
	}
	_, err = p.client.PullRequests.DeleteComment(ctx, backendutil.Esc(owner), backendutil.Esc(repo), id)
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "DeleteNote", err)
	}
	return nil
}

// CreateDiscussion implements provider.DiffManager.
//
// Gitee has no separate discussion endpoint; PR comments are the only option.
func (p *Provider) CreateDiscussion(ctx context.Context, owner, repo, number string, opts provider.DiscussionOptions) (string, error) {
	return p.CreateNote(ctx, owner, repo, number, opts.Body)
}

var _ provider.DiffManager = (*Provider)(nil)
