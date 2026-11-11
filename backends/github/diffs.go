package github

import (
	"context"
	"strconv"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// GetCRDiff implements provider.DiffManager.
func (p *Provider) GetCRDiff(ctx context.Context, owner, repo, number string) (*provider.MergeDiff, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "GetCRDiff", number)
	if err != nil {
		return nil, err
	}
	diff := &provider.MergeDiff{}
	page := 1
	for {
		files, _, err := p.client.PullRequests.ListFiles(ctx, owner, repo, n, &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "GetCRDiff", err)
		}
		for _, f := range files {
			cf := &provider.ChangedFile{
				OldPath:   f.GetPreviousFilename(),
				NewPath:   f.GetFilename(),
				Diff:      f.GetPatch(),
				Additions: f.GetAdditions(),
				Deletions: f.GetDeletions(),
				IsNew:     f.GetStatus() == "added",
				IsDeleted: f.GetStatus() == "removed",
				IsRenamed: f.GetStatus() == "renamed",
			}
			if cf.OldPath == "" {
				cf.OldPath = cf.NewPath
			}
			diff.Files = append(diff.Files, cf)
		}
		if len(files) < 100 {
			break
		}
		page++
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
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "CreateNote", number)
	if err != nil {
		return "", err
	}
	comment, _, err := p.client.Issues.CreateComment(ctx, owner, repo, n, github.IssueCommentRequest{
		Body: body,
	})
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitHub, "CreateNote", err)
	}
	return strconv.FormatInt(comment.GetID(), 10), nil
}

// DeleteNote implements provider.DiffManager. The note itself is addressed
// by its numeric platform ID; the change-request number is only validated,
// as GitHub's delete-comment endpoint does not take it.
func (p *Provider) DeleteNote(ctx context.Context, owner, repo, number, noteID string) error {
	if _, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "DeleteNote", number); err != nil {
		return err
	}
	id, err := strconv.ParseInt(noteID, 10, 64)
	if err != nil {
		return provider.Wrapf(provider.PlatformGitHub, "DeleteNote", "invalid note ID %q: %w", noteID, err)
	}
	_, err = p.client.Issues.DeleteComment(ctx, owner, repo, id)
	if err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DeleteNote", err)
	}
	return nil
}

// CreateDiscussion implements provider.DiffManager.
func (p *Provider) CreateDiscussion(ctx context.Context, owner, repo, number string, opts provider.DiscussionOptions) (string, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitHub, "CreateDiscussion", number)
	if err != nil {
		return "", err
	}
	comment := &github.CreatePullRequestCommentRequest{
		Body: opts.Body,
		Path: opts.FilePath,
	}
	if opts.NewLine > 0 {
		comment.Line = new(opts.NewLine)
		comment.Side = new("RIGHT")
	} else if opts.OldLine > 0 {
		comment.Line = new(opts.OldLine)
		comment.Side = new("LEFT")
	}
	c, _, err := p.client.PullRequests.CreateComment(ctx, owner, repo, n, *comment)
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitHub, "CreateDiscussion", err)
	}
	return strconv.FormatInt(c.GetID(), 10), nil
}

// compile-time guard
var _ provider.DiffManager = (*Provider)(nil)
