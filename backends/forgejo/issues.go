package forgejo

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListIssues implements provider.IssueManager.
func (p *Provider) ListIssues(ctx context.Context, opts provider.ListIssuesOptions) ([]*provider.Issue, int, error) {
	return giteafamily.ListIssues(ctx, p.family, p.ports, opts)
}

// GetIssue implements provider.IssueManager.
func (p *Provider) GetIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	return giteafamily.GetIssue(ctx, p.family, p.ports, owner, repo, number)
}

// CreateIssue implements provider.IssueManager.
func (p *Provider) CreateIssue(ctx context.Context, opts provider.CreateIssueOptions) (*provider.Issue, error) {
	return giteafamily.CreateIssue(ctx, p.family, p.ports, p.ports, p.labelIDs, opts)
}

// UpdateIssue implements provider.IssueManager.
func (p *Provider) UpdateIssue(ctx context.Context, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	return giteafamily.UpdateIssue(ctx, p.family, p.ports, p.ports, p.labelIDs, owner, repo, number, opts)
}

// CloseIssue implements provider.IssueManager.
func (p *Provider) CloseIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	return giteafamily.CloseIssue(ctx, p.family, p.ports, owner, repo, number)
}

// ReopenIssue implements provider.IssueManager.
func (p *Provider) ReopenIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	return giteafamily.ReopenIssue(ctx, p.family, p.ports, owner, repo, number)
}

// ListIssueComments implements provider.IssueManager.
func (p *Provider) ListIssueComments(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	return giteafamily.ListIssueComments(ctx, p.family, p.ports, owner, repo, number)
}

// CreateIssueComment implements provider.IssueManager.
func (p *Provider) CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*provider.IssueComment, error) {
	return giteafamily.CreateIssueComment(ctx, p.family, p.ports, owner, repo, number, body)
}

// UpdateIssueComment implements provider.IssueManager.
func (p *Provider) UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*provider.IssueComment, error) {
	return giteafamily.UpdateIssueComment(ctx, p.family, p.ports, owner, repo, number, commentID, body)
}

// ListIssueLabels implements provider.IssueManager.
func (p *Provider) ListIssueLabels(ctx context.Context, owner, repo string) ([]*provider.IssueLabel, error) {
	return giteafamily.ListIssueLabels(ctx, p.family, p.ports, owner, repo)
}

// AddIssueLabels implements provider.IssueManager.
func (p *Provider) AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	return giteafamily.AddIssueLabels(ctx, p.family, p.ports, p.ports, p.labelIDs, owner, repo, number, labels)
}

// RemoveIssueLabel implements provider.IssueManager.
func (p *Provider) RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error {
	return giteafamily.RemoveIssueLabel(ctx, p.family, p.ports, p.ports, p.labelIDs, owner, repo, number, name)
}

var _ provider.IssueManager = (*Provider)(nil)
