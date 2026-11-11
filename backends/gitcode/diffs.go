package gitcode

import (
	"context"
	"fmt"
	"strconv"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/provider"
)

// GetCRDiff implements provider.DiffManager.
func (p *Provider) GetCRDiff(ctx context.Context, owner, repo, number string) (*provider.MergeDiff, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "GetCRDiff", number)
	if err != nil {
		return nil, err
	}
	files, err := p.client.ListPullRequestFiles(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetCRDiff", err)
	}
	diff := &provider.MergeDiff{}
	for _, f := range files {
		cf := convertChangedFile(f)
		diff.Files = append(diff.Files, cf)
		diff.TotalAdd += cf.Additions
		diff.TotalDel += cf.Deletions
	}
	diff.RawDiff = provider.BuildRawDiff(diff.Files)
	return diff, nil
}

// GetCRFiles implements provider.DiffManager. The SDK's
// ListPullRequestFiles endpoint exposes no pagination parameters, so the
// call stays single-shot.
func (p *Provider) GetCRFiles(ctx context.Context, owner, repo, number string) ([]*provider.ChangedFile, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "GetCRFiles", number)
	if err != nil {
		return nil, err
	}
	files, err := p.client.ListPullRequestFiles(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetCRFiles", err)
	}
	result := make([]*provider.ChangedFile, 0, len(files))
	for _, f := range files {
		result = append(result, convertChangedFile(f))
	}
	return result, nil
}

// CreateNote implements provider.DiffManager.
func (p *Provider) CreateNote(ctx context.Context, owner, repo, number, body string) (string, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "CreateNote", number)
	if err != nil {
		return "", err
	}
	comment, err := p.client.CreatePullRequestComment(ctx, owner, repo, n, body, "", "", "")
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitCode, "CreateNote", err)
	}
	return string(comment.ID), nil
}

// DeleteNote implements provider.DiffManager. The note itself is addressed
// by its numeric platform ID; the change-request number is only validated,
// as GitCode's delete-comment endpoint does not take it.
func (p *Provider) DeleteNote(ctx context.Context, owner, repo, number, noteID string) error {
	if _, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "DeleteNote", number); err != nil {
		return err
	}
	id, err := strconv.ParseInt(noteID, 10, 64)
	if err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteNote", err)
	}
	err = p.client.DeleteIssueComment(ctx, owner, repo, id)
	if err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteNote", err)
	}
	return nil
}

// CreateDiscussion implements provider.DiffManager. Anchored discussions
// (FilePath set) ride the SDK's inline-comment endpoint — the same wire
// createInlineComment uses for review comments (issue #86: the anchors used
// to be silently dropped and every discussion landed as a top-level note).
// NewLine anchors the RIGHT side, OldLine the LEFT side; CommitID carries
// HeadSHA. The base/start SHA pair and multi-line ranges have no GitCode
// wire (registered divergence).
func (p *Provider) CreateDiscussion(ctx context.Context, owner, repo, number string, opts provider.DiscussionOptions) (string, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "CreateDiscussion", number)
	if err != nil {
		return "", err
	}
	var comment *gitcode.PullRequestComment
	if opts.FilePath == "" {
		comment, err = p.client.CreatePullRequestComment(ctx, owner, repo, n, opts.Body, "", "", "")
	} else {
		inline := gitcode.CreatePullRequestInlineCommentOptions{
			Body: opts.Body, Path: opts.FilePath, Side: "RIGHT", CommitID: opts.HeadSHA,
		}
		if opts.NewLine > 0 {
			inline.Line = opts.NewLine
		} else if opts.OldLine > 0 {
			inline.Line = opts.OldLine
			inline.Side = "LEFT"
		}
		comment, err = p.client.CreatePullRequestInlineComment(ctx, owner, repo, n, inline)
	}
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitCode, "CreateDiscussion", err)
	}
	return string(comment.ID), nil
}

func convertChangedFile(f *gitcode.PullRequestFile) *provider.ChangedFile {
	patch := ""
	if f.Patch != nil {
		patch = fmt.Sprint(f.Patch)
	}
	cf := &provider.ChangedFile{
		OldPath:   f.PreviousFilename,
		NewPath:   f.Filename,
		Diff:      patch,
		Additions: f.Additions,
		Deletions: f.Deletions,
		IsNew:     f.Status == "added",
		IsDeleted: f.Status == "removed",
		IsRenamed: f.Status == "renamed",
	}
	if cf.OldPath == "" {
		cf.OldPath = cf.NewPath
	}
	return cf
}

var _ provider.DiffManager = (*Provider)(nil)
