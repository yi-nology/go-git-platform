package gitcode

import (
	"context"
	"strings"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
	gitcode "github.com/yi-nology/go-gitcode"
)

// ListLabels implements provider.LabelManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page. The
// label-list endpoint takes the documented page/per_page query (server
// default 100 per page) since go-gitcode v0.8.0.
func (p *Provider) ListLabels(ctx context.Context, owner, repo string, opts provider.ListLabelsOptions) ([]*provider.Label, error) {
	var labels []*gitcode.Label
	labels, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitcode.Label, error) {
			return p.client.ListIssueLabelsWithOptions(ctx, backendutil.Esc(owner), backendutil.Esc(repo), gitcode.ListOptions{Page: page, PerPage: perPage})
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListLabels", err)
	}
	result := make([]*provider.Label, 0, len(labels))
	for _, l := range labels {
		result = append(result, convertLabel(l))
	}
	return result, nil
}

// CreateLabel implements provider.LabelManager. GitCode's label API has no
// description field; opts.Description is ignored.
func (p *Provider) CreateLabel(ctx context.Context, owner, repo string, opts provider.CreateLabelOptions) (*provider.Label, error) {
	label, err := p.client.CreateIssueLabel(ctx, backendutil.Esc(owner), backendutil.Esc(repo), opts.Name, opts.Color)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateLabel", err)
	}
	return convertLabel(label), nil
}

// UpdateLabel implements provider.LabelManager. GitCode addresses labels by
// name natively. Its label API has no description field; opts.Description
// is ignored.
func (p *Provider) UpdateLabel(ctx context.Context, owner, repo, name string, opts provider.UpdateLabelOptions) (*provider.Label, error) {
	updateOpts := gitcode.UpdateLabelOptions{}
	if opts.NewName != nil {
		updateOpts.Name = *opts.NewName
	}
	if opts.Color != nil {
		// GitCode's label API uses '#'-prefixed colors on the wire (docs:
		// create "eg: #fff", update responses show "#ED4014") — the same
		// form go-gitcode's create path sends. The public SDK contract
		// stays '#' free; only the wire form carries it. TrimPrefix keeps a
		// caller-supplied '#' from doubling.
		updateOpts.Color = "#" + strings.TrimPrefix(*opts.Color, "#")
	}
	label, err := p.client.UpdateIssueLabel(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(name), updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateLabel", err)
	}
	return convertLabel(label), nil
}

// DeleteLabel implements provider.LabelManager.
func (p *Provider) DeleteLabel(ctx context.Context, owner, repo, name string) error {
	if err := p.client.DeleteIssueLabel(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(name)); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteLabel", err)
	}
	return nil
}

var _ provider.LabelManager = (*Provider)(nil)
