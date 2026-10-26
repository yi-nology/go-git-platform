package gitcode

import (
	"context"
	"strconv"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCR implements provider.ChangeRequestManager.
func (p *Provider) CreateCR(ctx context.Context, opts provider.CreateCROptions) (*provider.ChangeRequest, error) {
	pr, err := p.client.CreatePullRequest(ctx, opts.Owner, opts.Repo, gitcode.CreatePullRequestOptions{
		Title: opts.Title, Body: opts.Description,
		Head: opts.SourceBranch, Base: opts.TargetBranch,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateCR", err)
	}
	return convertPullRequest(pr), nil
}

// GetCR implements provider.ChangeRequestManager.
func (p *Provider) GetCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "GetCR", number)
	if err != nil {
		return nil, err
	}
	pr, err := p.client.GetPullRequest(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetCR", err)
	}
	return convertPullRequest(pr), nil
}

// ListCRs implements provider.ChangeRequestManager.
func (p *Provider) ListCRs(ctx context.Context, opts provider.ListCROptions) ([]*provider.ChangeRequest, *int, error) {
	opts.Page, opts.PerPage = provider.NormalizePageOpts(opts.Page, opts.PerPage)
	// **未指定状态(零值)⇒ 不过滤**(平台上不加 state = 全部)。
	// 以前默认 "open" ⇒ closed/merged 的 PR **静默丢失**。
	var state gitcode.PullRequestState
	switch opts.State {
	case provider.CRStateClosed:
		state = gitcode.PullRequestStateClosed
	case provider.CRStateMerged:
		state = gitcode.PullRequestStateClosed
	case provider.CRStateOpened:
		state = gitcode.PullRequestStateOpen
	}
	prs, err := p.client.ListPullRequests(ctx, opts.Owner, opts.Repo, gitcode.ListPullRequestsOptions{
		ListOptions: gitcode.ListOptions{Page: opts.Page, PerPage: opts.PerPage},
		State:       state,
	})
	if err != nil {
		return nil, nil, provider.Wrap(provider.PlatformGitCode, "ListCRs", err)
	}
	result := make([]*provider.ChangeRequest, 0, len(prs))
	for _, pr := range prs {
		result = append(result, convertPullRequest(pr))
	}
	// The GitCode SDK exposes no total count from the response, so the total
	// is reported as unknown (nil) instead of being faked from the page
	// length.
	return result, nil, nil
}

// MergeCR implements provider.ChangeRequestManager.
func (p *Provider) MergeCR(ctx context.Context, owner, repo, number string, opts provider.MergeCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "MergeCR", number)
	if err != nil {
		return nil, err
	}
	err = p.client.MergePullRequest(ctx, owner, repo, n, &gitcode.MergePullRequestOptions{
		CommitMessage: opts.MergeCommitMessage,
		Squash:        opts.Squash,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "MergeCR", err)
	}
	return p.GetCR(ctx, owner, repo, number)
}

// CloseCR implements provider.ChangeRequestManager.
func (p *Provider) CloseCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "CloseCR", number)
	if err != nil {
		return nil, err
	}
	pr, err := p.client.ClosePullRequest(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CloseCR", err)
	}
	return convertPullRequest(pr), nil
}

// ReopenCR implements provider.ChangeRequestManager.
func (p *Provider) ReopenCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "ReopenCR", number)
	if err != nil {
		return nil, err
	}
	pr, err := p.client.ReopenPullRequest(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ReopenCR", err)
	}
	return convertPullRequest(pr), nil
}

// UpdateCR implements provider.ChangeRequestManager.
func (p *Provider) UpdateCR(ctx context.Context, owner, repo, number string, opts provider.UpdateCROptions) (*provider.ChangeRequest, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "UpdateCR", number)
	if err != nil {
		return nil, err
	}
	pr, err := p.client.UpdatePullRequest(ctx, owner, repo, n, gitcode.UpdatePullRequestOptions{
		Title: opts.Title, Body: opts.Description, Base: opts.TargetBranch,
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateCR", err)
	}
	return convertPullRequest(pr), nil
}

// UpdateCRLabels implements provider.ChangeRequestManager.
func (p *Provider) UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "UpdateCRLabels", number)
	if err != nil {
		return err
	}
	err = p.client.AddIssueLabels(ctx, owner, repo, n, labels)
	if err != nil {
		return provider.Wrap(provider.PlatformGitCode, "UpdateCRLabels", err)
	}
	return nil
}

// ListCRComments implements provider.ChangeRequestManager. The SDK's
// ListIssueComments endpoint exposes no pagination parameters, so the call
// stays single-shot.
func (p *Provider) ListCRComments(ctx context.Context, owner, repo, number string) ([]*provider.CRComment, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "ListCRComments", number)
	if err != nil {
		return nil, err
	}
	comments, err := p.client.ListIssueComments(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListCRComments", err)
	}
	result := make([]*provider.CRComment, 0, len(comments))
	for _, c := range comments {
		cc := &provider.CRComment{ID: c.ID, Body: c.Body, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
		if c.Author != nil {
			authorID, _ := strconv.ParseInt(string(c.Author.ID), 10, 64)
			cc.Author = &provider.CRUser{
				ID: authorID, Username: c.Author.Login, AvatarURL: c.Author.AvatarURL,
			}
		}
		result = append(result, cc)
	}
	return result, nil
}

// ListCRCommits implements provider.ChangeRequestManager. The SDK's
// ListPullRequestCommits endpoint exposes no pagination parameters, so the
// call stays single-shot.
func (p *Provider) ListCRCommits(ctx context.Context, owner, repo, number string) ([]*provider.CRCommit, error) {
	n, err := backendutil.ParsePRNumber(provider.PlatformGitCode, "ListCRCommits", number)
	if err != nil {
		return nil, err
	}
	commits, err := p.client.ListPullRequestCommits(ctx, owner, repo, n)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListCRCommits", err)
	}
	result := make([]*provider.CRCommit, 0, len(commits))
	for _, c := range commits {
		cc := &provider.CRCommit{SHA: c.SHA, Message: c.Message, CreatedAt: c.CreatedAt}
		if c.Author != nil {
			authorID, _ := strconv.ParseInt(string(c.Author.ID), 10, 64)
			cc.Author = &provider.CRUser{
				ID: authorID, Username: c.Author.Login, AvatarURL: c.Author.AvatarURL,
			}
		}
		result = append(result, cc)
	}
	return result, nil
}

var _ provider.ChangeRequestManager = (*Provider)(nil)
