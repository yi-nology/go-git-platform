package tencentcode

import (
	"context"

	gongfeng "github.com/studyzy/gongfeng-sdk-go"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCR implements provider.ChangeRequestManager.
func (p *Provider) CreateCR(ctx context.Context, opts provider.CreateCROptions) (*provider.ChangeRequest, error) {
	pid := opts.Owner + "/" + opts.Repo
	createOpts := &gongfeng.CreateMergeRequestOptions{
		SourceBranch: gongfeng.Ptr(opts.SourceBranch),
		TargetBranch: gongfeng.Ptr(opts.TargetBranch),
		Title:        gongfeng.Ptr(opts.Title),
	}
	if opts.Description != "" {
		createOpts.Description = gongfeng.Ptr(opts.Description)
	}
	mr, _, err := p.client.MergeRequests.CreateMergeRequest(ctx, pid, createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateCR", err)
	}
	return convertMR(mr), nil
}

// GetCR implements provider.ChangeRequestManager.
func (p *Provider) GetCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "GetCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	mr, _, err := p.client.MergeRequests.GetMergeRequest(ctx, pid, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetCR", err)
	}
	return convertMR(mr), nil
}

// ListCRs implements provider.ChangeRequestManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListCRs(ctx context.Context, opts provider.ListCROptions) ([]*provider.ChangeRequest, int, error) {
	pid := opts.Owner + "/" + opts.Repo
	buildOpts := func(page, perPage int) *gongfeng.ListMergeRequestsOptions {
		listOpts := &gongfeng.ListMergeRequestsOptions{
			ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage},
		}
		if opts.State != "" {
			listOpts.State = gongfeng.Ptr(string(opts.State))
		}
		return listOpts
	}
	var (
		mrs      []*gongfeng.MergeRequest
		resp     *gongfeng.Response
		allPages bool
	)
	if opts.Page > 0 {
		// Caller-driven pagination: serve the requested page only. The
		// page response still carries the platform total, so it is kept.
		perPage := opts.PerPage
		if perPage <= 0 || perPage > provider.MaxPerPage {
			perPage = provider.MaxPerPage
		}
		list, r, err := p.client.MergeRequests.ListMergeRequests(ctx, pid, buildOpts(opts.Page, perPage))
		if err != nil {
			return nil, 0, provider.Wrap(provider.PlatformTencentCode, "ListCRs", err)
		}
		mrs, resp = list, r
	} else {
		allPages = true
		var err error
		if mrs, err = backendutil.AllPages(func(page int) ([]*gongfeng.MergeRequest, error) {
			list, _, err := p.client.MergeRequests.ListMergeRequests(ctx, pid, buildOpts(page, provider.MaxPerPage))
			return list, err
		}); err != nil {
			return nil, 0, provider.Wrap(provider.PlatformTencentCode, "ListCRs", err)
		}
	}
	crs := make([]*provider.ChangeRequest, 0, len(mrs))
	for _, mr := range mrs {
		crs = append(crs, convertMR(mr))
	}
	if allPages {
		// Merged pages carry no single authoritative total; the merged
		// size is the complete count by construction.
		return crs, len(crs), nil
	}
	return crs, extractTotalCount(resp, len(crs)), nil
}

// MergeCR implements provider.ChangeRequestManager.
func (p *Provider) MergeCR(ctx context.Context, owner, repo, number string, opts provider.MergeCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "MergeCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	// Pre-flight: check MR state.
	existingMR, _, err := p.client.MergeRequests.GetMergeRequest(ctx, pid, n)
	if err == nil {
		if mapState(existingMR.State) != provider.CRStateOpened {
			return nil, provider.Wrapf(provider.PlatformTencentCode, "MergeCR",
				"MR is not in 'opened' state (current: %s)", existingMR.State)
		}
	}
	acceptOpts := &gongfeng.AcceptMergeRequestOptions{}
	if opts.MergeCommitMessage != "" {
		acceptOpts.MergeCommitMessage = gongfeng.Ptr(opts.MergeCommitMessage)
	}
	mr, _, err := p.client.MergeRequests.AcceptMergeRequest(ctx, pid, n, acceptOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "MergeCR", err)
	}
	return convertMR(mr), nil
}

// CloseCR implements provider.ChangeRequestManager.
func (p *Provider) CloseCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "CloseCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	updateOpts := &gongfeng.UpdateMergeRequestOptions{
		StateEvent: gongfeng.Ptr("close"),
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(ctx, pid, n, updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CloseCR", err)
	}
	return convertMR(mr), nil
}

// ReopenCR implements provider.ChangeRequestManager.
func (p *Provider) ReopenCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "ReopenCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	updateOpts := &gongfeng.UpdateMergeRequestOptions{
		StateEvent: gongfeng.Ptr("reopen"),
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(ctx, pid, n, updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ReopenCR", err)
	}
	return convertMR(mr), nil
}

// UpdateCR implements provider.ChangeRequestManager.
func (p *Provider) UpdateCR(ctx context.Context, owner, repo, number string, opts provider.UpdateCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "UpdateCR", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	updateOpts := &gongfeng.UpdateMergeRequestOptions{}
	if opts.Title != "" {
		updateOpts.Title = gongfeng.Ptr(opts.Title)
	}
	if opts.Description != "" {
		updateOpts.Description = gongfeng.Ptr(opts.Description)
	}
	if opts.TargetBranch != "" {
		updateOpts.TargetBranch = gongfeng.Ptr(opts.TargetBranch)
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(ctx, pid, n, updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "UpdateCR", err)
	}
	return convertMR(mr), nil
}

// UpdateCRLabels implements provider.ChangeRequestManager.
//
// Tencent Code's API no longer supports setting labels via UpdateMergeRequest.
func (p *Provider) UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	return provider.Wrap(provider.PlatformTencentCode, "UpdateCRLabels", provider.ErrNotImplemented)
}

// ListCRComments implements provider.ChangeRequestManager. The endpoint has
// no caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListCRComments(ctx context.Context, owner, repo, number string) ([]*provider.CRComment, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "ListCRComments", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	notes, err := backendutil.AllPages(func(page int) ([]*gongfeng.Note, error) {
		list, _, err := p.client.Notes.ListMergeRequestNotes(ctx, pid, n,
			&gongfeng.ListMergeRequestNotesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListCRComments", err)
	}
	result := make([]*provider.CRComment, 0, len(notes))
	for _, n := range notes {
		comment := &provider.CRComment{
			ID:        int64(n.ID),
			Body:      n.Body,
			CreatedAt: n.CreatedAt.Time,
			UpdatedAt: n.UpdatedAt.Time,
		}
		if n.Author != nil {
			comment.Author = convertUser(n.Author)
		}
		result = append(result, comment)
	}
	return result, nil
}

// ListCRCommits implements provider.ChangeRequestManager. The endpoint has
// no caller-facing pagination knobs, so every page is fetched via AllPages
// (工蜂's page-size ceiling is 100).
func (p *Provider) ListCRCommits(ctx context.Context, owner, repo, number string) ([]*provider.CRCommit, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformTencentCode, "ListCRCommits", number)
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + repo
	commits, err := backendutil.AllPages(func(page int) ([]*gongfeng.Commit, error) {
		list, _, err := p.client.MergeRequests.ListMergeRequestCommits(ctx, pid, n,
			&gongfeng.ListMergeRequestCommitsOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListCRCommits", err)
	}
	result := make([]*provider.CRCommit, 0, len(commits))
	for _, c := range commits {
		result = append(result, &provider.CRCommit{
			SHA:     c.ID,
			Message: c.Message,
			Author:  &provider.CRUser{Name: c.AuthorName},
		})
	}
	return result, nil
}

var _ provider.ChangeRequestManager = (*Provider)(nil)
