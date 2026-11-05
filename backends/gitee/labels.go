package gitee

import (
	"context"

	gitee "github.com/next-bin/go-gitee/gitee"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListLabels implements provider.LabelManager.
//
// Dual pagination mode: opts.Page > 0 means the caller manages paging
// explicitly and receives exactly that one page (opts.Page/opts.PerPage
// honored via NormalizePageOpts); opts.Page == 0 (the default) exhausts
// pagination via backendutil.AllPages — fetching 100 per page until an
// empty page — so every label is returned.
func (p *Provider) ListLabels(ctx context.Context, owner, repo string, opts provider.ListLabelsOptions) ([]*provider.Label, error) {
	var labels []*gitee.Label
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		list, _, err := p.client.Labels.List(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &gitee.ListOptions{
			Page: page, PerPage: perPage,
		})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitee, "ListLabels", err)
		}
		labels = list
	} else {
		var err error
		labels, err = backendutil.AllPages(func(page int) ([]*gitee.Label, error) {
			list, _, err := p.client.Labels.List(ctx, backendutil.Esc(owner), backendutil.Esc(repo), &gitee.ListOptions{
				Page: page, PerPage: 100,
			})
			return list, err
		})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitee, "ListLabels", err)
		}
	}
	result := make([]*provider.Label, 0, len(labels))
	for _, l := range labels {
		result = append(result, convertLabel(l))
	}
	return result, nil
}

// CreateLabel implements provider.LabelManager.
func (p *Provider) CreateLabel(ctx context.Context, owner, repo string, opts provider.CreateLabelOptions) (*provider.Label, error) {
	createOpts := &gitee.CreateLabelOptions{
		Name:  gitee.String(opts.Name),
		Color: gitee.String(opts.Color),
	}
	label, _, err := p.client.Labels.Create(ctx, backendutil.Esc(owner), backendutil.Esc(repo), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "CreateLabel", err)
	}
	return convertLabel(label), nil
}

// UpdateLabel implements provider.LabelManager. Gitee addresses labels by name.
func (p *Provider) UpdateLabel(ctx context.Context, owner, repo, name string, opts provider.UpdateLabelOptions) (*provider.Label, error) {
	updateOpts := &gitee.UpdateLabelOptions{}
	if opts.NewName != nil {
		updateOpts.Name = gitee.String(*opts.NewName)
	}
	if opts.Color != nil {
		updateOpts.Color = gitee.String(*opts.Color)
	}
	label, _, err := p.client.Labels.Edit(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(name), updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitee, "UpdateLabel", err)
	}
	return convertLabel(label), nil
}

// DeleteLabel implements provider.LabelManager.
func (p *Provider) DeleteLabel(ctx context.Context, owner, repo, name string) error {
	_, err := p.client.Labels.Delete(ctx, backendutil.Esc(owner), backendutil.Esc(repo), backendutil.Esc(name))
	if err != nil {
		return provider.Wrap(provider.PlatformGitee, "DeleteLabel", err)
	}
	return nil
}

var _ provider.LabelManager = (*Provider)(nil)
