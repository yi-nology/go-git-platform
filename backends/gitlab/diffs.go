package gitlab

import (
	"context"
	"strconv"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/provider"
)

// GetCRDiff implements provider.DiffManager.
func (p *Provider) GetCRDiff(ctx context.Context, owner, repo, number string) (*provider.MergeDiff, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "GetCRDiff", number)
	if err != nil {
		return nil, err
	}
	// 全页行走（issue #44）：该端点是 offset 分页，per_page 服务端缺省
	// 20——单页不翻页把 >20 个变更文件的 MR 静默截断，argus 的评审覆盖
	// 面无声缩水。与同文件 ListNotes 走同一 AllPages 预算模式，perPage
	// 显式拉满 API 上限（GitLab REST per_page max 100）。
	diffs, err := backendutil.AllPages(func(page int) ([]*gitlab.MergeRequestDiff, error) {
		batch, _, err := p.client.MergeRequests.ListMergeRequestDiffs(pidOf(owner, repo), n,
			&gitlab.ListMergeRequestDiffsOptions{
				ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: backendutil.GitlabDiffPageSize},
			}, gitlab.WithContext(ctx))
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "GetCRDiff", err)
	}
	diff := &provider.MergeDiff{}
	for _, c := range diffs {
		additions, deletions := provider.CountDiffLines(c.Diff)
		cf := &provider.ChangedFile{
			OldPath: c.OldPath, NewPath: c.NewPath, Diff: c.Diff,
			Additions: additions, Deletions: deletions,
			IsNew: c.NewFile, IsDeleted: c.DeletedFile, IsRenamed: c.RenamedFile,
		}
		diff.Files = append(diff.Files, cf)
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
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "CreateNote", number)
	if err != nil {
		return "", err
	}
	note, _, err := p.client.Notes.CreateMergeRequestNote(pidOf(owner, repo), n,
		&gitlab.CreateMergeRequestNoteOptions{Body: new(body)}, gitlab.WithContext(ctx))
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitLab, "CreateNote", err)
	}
	return strconv.FormatInt(note.ID, 10), nil
}

// DeleteNote implements provider.DiffManager.
func (p *Provider) DeleteNote(ctx context.Context, owner, repo, number, noteID string) error {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "DeleteNote", number)
	if err != nil {
		return err
	}
	nid, err := strconv.ParseInt(noteID, 10, 64)
	if err != nil {
		return provider.Wrapf(provider.PlatformGitLab, "DeleteNote", "invalid note ID %q: %w", noteID, err)
	}
	_, err = p.client.Notes.DeleteMergeRequestNote(pidOf(owner, repo), n, nid, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(provider.PlatformGitLab, "DeleteNote", err)
	}
	return nil
}

// CreateDiscussion implements provider.DiffManager.
func (p *Provider) CreateDiscussion(ctx context.Context, owner, repo, number string, opts provider.DiscussionOptions) (string, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "CreateDiscussion", number)
	if err != nil {
		return "", err
	}
	pid := pidOf(owner, repo)
	discOpts := &gitlab.CreateMergeRequestDiscussionOptions{Body: new(opts.Body)}
	if opts.FilePath != "" {
		position := &gitlab.PositionOptions{
			PositionType: new("text"),
			NewPath:      new(opts.FilePath),
		}
		if opts.BaseSHA != "" {
			position.BaseSHA = new(opts.BaseSHA)
		}
		if opts.StartSHA != "" {
			position.StartSHA = new(opts.StartSHA)
		}
		if opts.HeadSHA != "" {
			position.HeadSHA = new(opts.HeadSHA)
		}
		if opts.OldLine > 0 {
			position.OldPath = new(opts.FilePath)
			position.OldLine = new(int64(opts.OldLine))
		}
		if opts.NewLine > 0 {
			position.NewLine = new(int64(opts.NewLine))
		}
		if opts.StartNewLine > 0 && opts.NewLine > opts.StartNewLine {
			position.LineRange = &gitlab.LineRangeOptions{
				Start: &gitlab.LinePositionOptions{Type: new("new"), NewLine: new(int64(opts.StartNewLine))},
				End:   &gitlab.LinePositionOptions{Type: new("new"), NewLine: new(int64(opts.NewLine))},
			}
		}
		discOpts.Position = position
	}
	disc, _, err := p.client.Discussions.CreateMergeRequestDiscussion(pid, n, discOpts, gitlab.WithContext(ctx))
	if err != nil {
		return "", provider.Wrap(provider.PlatformGitLab, "CreateDiscussion", err)
	}
	return disc.ID, nil
}

var _ provider.DiffManager = (*Provider)(nil)

// UpdateNote updates one merge-request note (MergeRequest Notes API; v0.67.2).
// IssueManager.UpdateIssueComment targets the Issues Notes API and 404s for
// merge requests — consumers doing CR-comment maintenance should prefer this.
func (p *Provider) UpdateNote(ctx context.Context, owner, repo, number, noteID, body string) (*provider.IssueComment, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "UpdateNote", number)
	if err != nil {
		return nil, err
	}
	nid, err := strconv.ParseInt(noteID, 10, 64)
	if err != nil {
		return nil, provider.Wrapf(provider.PlatformGitLab, "UpdateNote", "invalid note ID %q: %w", noteID, err)
	}
	note, _, err := p.client.Notes.UpdateMergeRequestNote(pidOf(owner, repo), n, nid,
		&gitlab.UpdateMergeRequestNoteOptions{Body: new(body)}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "UpdateNote", err)
	}
	return convertIssueComment(note), nil
}

// ListNotes lists every merge-request note, oldest first across all pages
// (MergeRequest Notes API; v0.67.2). IssueManager.ListIssueComments targets
// the Issues Notes API and 404s for merge requests.
func (p *Provider) ListNotes(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "ListNotes", number)
	if err != nil {
		return nil, err
	}
	notes, err := backendutil.AllPages(func(page int) ([]*gitlab.Note, error) {
		batch, _, err := p.client.Notes.ListMergeRequestNotes(pidOf(owner, repo), n, &gitlab.ListMergeRequestNotesOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: backendutil.IssueCommentPageSize},
		}, gitlab.WithContext(ctx))
		return batch, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListNotes", err)
	}
	result := make([]*provider.IssueComment, 0, len(notes))
	for _, note := range notes {
		result = append(result, convertIssueComment(note))
	}
	return result, nil
}
