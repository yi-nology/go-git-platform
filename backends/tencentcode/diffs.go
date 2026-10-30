package tencentcode

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	gongfeng "github.com/yi-nology/go-gongfeng"
)

// GetCRDiff implements provider.DiffManager.
func (p *Provider) GetCRDiff(ctx context.Context, owner, repo, number string) (*provider.MergeDiff, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "GetCRDiff", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	changes, _, err := p.client.MergeRequests.GetMergeRequestChanges(ctx, pid, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetCRDiff", err)
	}
	files := make([]*provider.ChangedFile, 0, len(changes.Files))
	totalAdd, totalDel := 0, 0
	for _, d := range changes.Files {
		cf := convertDiff(d)
		totalAdd += cf.Additions
		totalDel += cf.Deletions
		files = append(files, cf)
	}
	return &provider.MergeDiff{Files: files, TotalAdd: totalAdd, TotalDel: totalDel}, nil
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
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "CreateNote", number)
	if err != nil {
		return "", err
	}
	pid := owner + "/" + repo
	opts := &gongfeng.CreateMergeRequestNoteOptions{
		Body: gongfeng.Ptr(body),
	}
	note, _, err := p.client.Notes.CreateMergeRequestNote(ctx, pid, n, opts)
	if err != nil {
		return "", provider.Wrap(provider.PlatformTencentCode, "CreateNote", err)
	}
	return fmt.Sprintf("%d", note.ID), nil
}

// DeleteNote implements provider.DiffManager.
// The gongfeng SDK does not expose a delete-note endpoint, so we use the
// SDK client's NewRequest/Do for a raw API call.
func (p *Provider) DeleteNote(ctx context.Context, owner, repo, number, noteID string) error {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "DeleteNote", number)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("projects/%s/merge_requests/%d/notes/%s", esc(pid(owner, repo)), n, esc(noteID))
	req, err := p.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteNote", err)
	}
	if _, err := p.client.Do(req, nil); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteNote", err)
	}
	return nil
}

// CreateDiscussion implements provider.DiffManager.
// The gongfeng SDK does not expose a discussions endpoint, so we use the
// SDK client's NewRequest/Do for a raw API call.
func (p *Provider) CreateDiscussion(ctx context.Context, owner, repo, number string, opts provider.DiscussionOptions) (string, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "CreateDiscussion", number)
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("projects/%s/merge_requests/%d/discussions", esc(pid(owner, repo)), n)
	payload := map[string]any{"body": opts.Body}
	if opts.FilePath != "" {
		position := map[string]any{
			"position_type": "text",
			"new_path":      opts.FilePath,
		}
		if opts.BaseSHA != "" {
			position["base_sha"] = opts.BaseSHA
		}
		if opts.StartSHA != "" {
			position["start_sha"] = opts.StartSHA
		}
		if opts.HeadSHA != "" {
			position["head_sha"] = opts.HeadSHA
		}
		if opts.OldLine > 0 {
			position["old_path"] = opts.FilePath
			position["old_line"] = opts.OldLine
		}
		if opts.NewLine > 0 {
			position["new_line"] = opts.NewLine
		}
		payload["position"] = position
	}
	req, err := p.client.NewRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return "", provider.Wrap(provider.PlatformTencentCode, "CreateDiscussion", err)
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	if _, err := p.client.Do(req, &resp); err != nil {
		return "", provider.Wrap(provider.PlatformTencentCode, "CreateDiscussion", err)
	}
	return strconv.FormatInt(resp.ID, 10), nil
}

// UpdateNote implements provider.NoteManager. 工蜂 is GitLab-family: MR
// notes and issue notes live in separate iid namespaces, and
// IssueManager's comment methods target the Issues Notes API, which 404s
// for merge requests — CR comment edits must go through the MergeRequest
// Notes API.
func (p *Provider) UpdateNote(ctx context.Context, owner, repo, number, noteID, body string) (*provider.IssueComment, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "UpdateNote", number)
	if err != nil {
		return nil, err
	}
	nid, err := strconv.ParseInt(noteID, 10, 64)
	if err != nil {
		return nil, provider.Wrapf(provider.PlatformTencentCode, "UpdateNote", "invalid note ID %q: %v", noteID, err)
	}
	note, _, err := p.client.Notes.UpdateMergeRequestNote(ctx, pid(owner, repo), n, int(nid),
		&gongfeng.UpdateMergeRequestNoteOptions{Body: gongfeng.Ptr(body)})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "UpdateNote", err)
	}
	return convertIssueComment(note), nil
}

// ListNotes implements provider.NoteManager: every merge-request note,
// oldest first across all pages. The endpoint has no caller-facing
// pagination knobs, so every page is fetched via AllPages (工蜂's
// page-size ceiling is 100).
func (p *Provider) ListNotes(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "ListNotes", number)
	if err != nil {
		return nil, err
	}
	notes, err := backendutil.AllPages(func(page int) ([]*gongfeng.Note, error) {
		list, _, err := p.client.Notes.ListMergeRequestNotes(ctx, pid(owner, repo), n,
			&gongfeng.ListMergeRequestNotesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListNotes", err)
	}
	result := make([]*provider.IssueComment, 0, len(notes))
	for _, note := range notes {
		result = append(result, convertIssueComment(note))
	}
	return result, nil
}

var (
	_ provider.DiffManager = (*Provider)(nil)
	_ provider.NoteManager = (*Provider)(nil)
)
