package gitlab

import (
	"context"

	oekiterrors "github.com/oekit/pkg/errors"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCR implements provider.ChangeRequestManager.
func (p *Provider) CreateCR(ctx context.Context, opts provider.CreateCROptions) (*provider.ChangeRequest, error) {
	createOpts := &gitlab.CreateMergeRequestOptions{
		SourceBranch:       new(opts.SourceBranch),
		TargetBranch:       new(opts.TargetBranch),
		Title:              new(opts.Title),
		Description:        new(opts.Description),
		RemoveSourceBranch: new(opts.RemoveSourceBranch),
	}
	if len(opts.Labels) > 0 {
		labels := gitlab.LabelOptions(opts.Labels)
		createOpts.Labels = &labels
	}
	mr, _, err := p.client.MergeRequests.CreateMergeRequest(pidOf(opts.Owner, opts.Repo), createOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "CreateCR", err)
	}
	return convertMR(mr), nil
}

// GetCR implements provider.ChangeRequestManager.
func (p *Provider) GetCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "GetCR", number)
	if err != nil {
		return nil, err
	}
	mr, _, err := p.client.MergeRequests.GetMergeRequest(pidOf(owner, repo), n, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "GetCR", err)
	}
	return convertMR(mr), nil
}

// ListCRs implements provider.ChangeRequestManager.
func (p *Provider) ListCRs(ctx context.Context, opts provider.ListCROptions) ([]*provider.ChangeRequest, *int, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	listOpts := &gitlab.ListProjectMergeRequestsOptions{
		ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
	}
	if opts.State != "" {
		listOpts.State = new(string(opts.State))
	}
	if opts.SourceBranch != "" {
		listOpts.SourceBranch = new(opts.SourceBranch)
	}
	if opts.TargetBranch != "" {
		listOpts.TargetBranch = new(opts.TargetBranch)
	}
	mrs, resp, err := p.client.MergeRequests.ListProjectMergeRequests(pidOf(opts.Owner, opts.Repo), listOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformGitLab, "ListCRs", err)
	}
	// X-Total is authoritative when present; otherwise the count is unknown
	// (nil), never the page length.
	var total *int
	if resp != nil && resp.TotalItems > 0 {
		t := int(resp.TotalItems)
		total = &t
	}
	crs := make([]*provider.ChangeRequest, 0, len(mrs))
	for _, mr := range mrs {
		crs = append(crs, convertBasicMR(mr))
	}
	return crs, total, nil
}

// MergeCR implements provider.ChangeRequestManager.
func (p *Provider) MergeCR(ctx context.Context, owner, repo, number string, opts provider.MergeCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "MergeCR", number)
	if err != nil {
		return nil, err
	}
	pid := pidOf(owner, repo)
	// Pre-flight: reject MRs in a non-mergeable state. This produces a
	// friendlier error than waiting for the API to fail.
	existing, _, err := p.client.MergeRequests.GetMergeRequest(pid, n, nil, gitlab.WithContext(ctx))
	if err == nil && existing != nil {
		if existing.DetailedMergeStatus != "" && existing.DetailedMergeStatus != "mergeable" && existing.DetailedMergeStatus != "checking" {
			return nil, provider.Wrapf(provider.PlatformGitLab, "MergeCR",
				"MR cannot be merged (status: %s). It may have conflicts or an active pipeline", existing.DetailedMergeStatus)
		}
		if existing.State != "opened" {
			return nil, provider.Wrapf(provider.PlatformGitLab, "MergeCR",
				"MR is not in 'opened' state (current: %s)", existing.State)
		}
	}
	acceptOpts := &gitlab.AcceptMergeRequestOptions{}
	if opts.MergeCommitMessage != "" {
		acceptOpts.MergeCommitMessage = new(opts.MergeCommitMessage)
	}
	if opts.Squash {
		acceptOpts.Squash = new(true)
	}
	if opts.RemoveSourceBranch {
		acceptOpts.ShouldRemoveSourceBranch = new(true)
	}
	mr, _, err := p.client.MergeRequests.AcceptMergeRequest(pid, n, acceptOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "MergeCR", oekiterrors.Wrap(err, "merge failed"))
	}
	return convertMR(mr), nil
}

// CloseCR implements provider.ChangeRequestManager.
func (p *Provider) CloseCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "CloseCR", number)
	if err != nil {
		return nil, err
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(pidOf(owner, repo), n,
		&gitlab.UpdateMergeRequestOptions{StateEvent: new("close")}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "CloseCR", err)
	}
	return convertMR(mr), nil
}

// ReopenCR implements provider.ChangeRequestManager.
func (p *Provider) ReopenCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "ReopenCR", number)
	if err != nil {
		return nil, err
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(pidOf(owner, repo), n,
		&gitlab.UpdateMergeRequestOptions{StateEvent: new("reopen")}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ReopenCR", err)
	}
	return convertMR(mr), nil
}

// UpdateCR implements provider.ChangeRequestManager.
func (p *Provider) UpdateCR(ctx context.Context, owner, repo, number string, opts provider.UpdateCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "UpdateCR", number)
	if err != nil {
		return nil, err
	}
	updateOpts := &gitlab.UpdateMergeRequestOptions{}
	if opts.Title != "" {
		updateOpts.Title = new(opts.Title)
	}
	if opts.Description != "" {
		updateOpts.Description = new(opts.Description)
	}
	if opts.TargetBranch != "" {
		updateOpts.TargetBranch = new(opts.TargetBranch)
	}
	mr, _, err := p.client.MergeRequests.UpdateMergeRequest(pidOf(owner, repo), n, updateOpts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "UpdateCR", err)
	}
	return convertMR(mr), nil
}

// UpdateCRLabels implements provider.ChangeRequestManager.
func (p *Provider) UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "UpdateCRLabels", number)
	if err != nil {
		return err
	}
	l := gitlab.LabelOptions(labels)
	_, _, err = p.client.MergeRequests.UpdateMergeRequest(pidOf(owner, repo), n,
		&gitlab.UpdateMergeRequestOptions{Labels: &l}, gitlab.WithContext(ctx))
	if err != nil {
		return provider.Wrap(provider.PlatformGitLab, "UpdateCRLabels", err)
	}
	return nil
}

// ListCRComments implements provider.ChangeRequestManager.
func (p *Provider) ListCRComments(ctx context.Context, owner, repo, number string) ([]*provider.CRComment, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "ListCRComments", number)
	if err != nil {
		return nil, err
	}
	notes, err := backendutil.AllPages(func(page int) ([]*gitlab.Note, error) {
		list, _, err := p.client.Notes.ListMergeRequestNotes(pidOf(owner, repo), n, &gitlab.ListMergeRequestNotesOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListCRComments", err)
	}
	result := make([]*provider.CRComment, 0, len(notes))
	for _, n := range notes {
		c := &provider.CRComment{
			ID:   n.ID,
			Body: n.Body,
			Author: &provider.CRUser{
				ID: n.Author.ID, Username: n.Author.Username,
				Name: n.Author.Name, AvatarURL: n.Author.AvatarURL,
			},
			CreatedAt: timeOrZero(n.CreatedAt),
			UpdatedAt: timeOrZero(n.UpdatedAt),
		}
		result = append(result, c)
	}
	return result, nil
}

// ListCRCommits implements provider.ChangeRequestManager.
func (p *Provider) ListCRCommits(ctx context.Context, owner, repo, number string) ([]*provider.CRCommit, error) {
	n, err := backendutil.ParsePRNumber64(provider.PlatformGitLab, "ListCRCommits", number)
	if err != nil {
		return nil, err
	}
	commits, err := backendutil.AllPages(func(page int) ([]*gitlab.Commit, error) {
		list, _, err := p.client.MergeRequests.GetMergeRequestCommits(pidOf(owner, repo), n, &gitlab.GetMergeRequestCommitsOptions{ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: 100}}, gitlab.WithContext(ctx))
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitLab, "ListCRCommits", err)
	}
	result := make([]*provider.CRCommit, 0, len(commits))
	for _, c := range commits {
		cc := &provider.CRCommit{SHA: c.ShortID, Message: c.Title, CreatedAt: timeOrZero(c.CreatedAt)}
		if c.AuthorName != "" {
			cc.Author = &provider.CRUser{Name: c.AuthorName}
		}
		result = append(result, cc)
	}
	return result, nil
}

var _ provider.ChangeRequestManager = (*Provider)(nil)
