package forgejo

import (
	"context"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
	"github.com/yi-nology/go-git-platform/provider"
)

// CreateCR implements provider.ChangeRequestManager.
func (p *Provider) CreateCR(ctx context.Context, opts provider.CreateCROptions) (*provider.ChangeRequest, error) {
	return giteafamily.CreateCR(ctx, p.family, p.ports, opts)
}

// GetCR implements provider.ChangeRequestManager.
func (p *Provider) GetCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	return giteafamily.GetCR(ctx, p.family, p.ports, owner, repo, number)
}

// ListCRs implements provider.ChangeRequestManager.
func (p *Provider) ListCRs(ctx context.Context, opts provider.ListCROptions) ([]*provider.ChangeRequest, int, error) {
	return giteafamily.ListCRs(ctx, p.family, p.ports, opts)
}

// MergeCR implements provider.ChangeRequestManager.
func (p *Provider) MergeCR(ctx context.Context, owner, repo, number string, opts provider.MergeCROptions) (*provider.ChangeRequest, error) {
	return giteafamily.MergeCR(ctx, p.family, p.ports, owner, repo, number, opts)
}

// CloseCR implements provider.ChangeRequestManager.
func (p *Provider) CloseCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	return giteafamily.CloseCR(ctx, p.family, p.ports, owner, repo, number)
}

// ReopenCR implements provider.ChangeRequestManager.
func (p *Provider) ReopenCR(ctx context.Context, owner, repo, number string) (*provider.ChangeRequest, error) {
	return giteafamily.ReopenCR(ctx, p.family, p.ports, owner, repo, number)
}

// UpdateCR implements provider.ChangeRequestManager.
func (p *Provider) UpdateCR(ctx context.Context, owner, repo, number string, opts provider.UpdateCROptions) (*provider.ChangeRequest, error) {
	return giteafamily.UpdateCR(ctx, p.family, p.ports, owner, repo, number, opts)
}

// ListCRComments implements provider.ChangeRequestManager.
func (p *Provider) ListCRComments(ctx context.Context, owner, repo, number string) ([]*provider.CRComment, error) {
	return giteafamily.ListCRComments(ctx, p.family, p.ports, owner, repo, number)
}

// ListCRCommits implements provider.ChangeRequestManager.
func (p *Provider) ListCRCommits(ctx context.Context, owner, repo, number string) ([]*provider.CRCommit, error) {
	return giteafamily.ListCRCommits(ctx, p.family, p.ports, owner, repo, number)
}

// UpdateCRLabels implements provider.ChangeRequestManager.
func (p *Provider) UpdateCRLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	return giteafamily.UpdateCRLabels(ctx, p.family, p.ports, p.ports, p.labelIDs, owner, repo, number, labels)
}

var _ provider.ChangeRequestManager = (*Provider)(nil)
