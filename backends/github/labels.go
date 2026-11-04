package github

import (
	"context"
	"strings"

	"github.com/google/go-github/v93/github"

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
	var labels []*github.Label
	if opts.Page > 0 {
		page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
		list, _, err := p.client.Issues.ListLabels(ctx, owner, repo, &github.ListOptions{Page: page, PerPage: perPage})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "ListLabels", err)
		}
		labels = list
	} else {
		var err error
		labels, err = backendutil.AllPages(func(page int) ([]*github.Label, error) {
			list, _, err := p.client.Issues.ListLabels(ctx, owner, repo, &github.ListOptions{
				Page: page, PerPage: 100,
			})
			return list, err
		})
		if err != nil {
			return nil, provider.Wrap(provider.PlatformGitHub, "ListLabels", err)
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
	label := &github.CreateIssueLabelRequest{
		Name:        opts.Name,
		Color:       new(opts.Color),
		Description: new(opts.Description),
	}
	created, _, err := p.client.Issues.CreateLabel(ctx, owner, repo, *label)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateLabel", err)
	}
	return convertLabel(created), nil
}

// UpdateLabel implements provider.LabelManager. GitHub addresses labels by
// name; nil option fields are simply omitted from the PATCH body.
func (p *Provider) UpdateLabel(ctx context.Context, owner, repo, name string, opts provider.UpdateLabelOptions) (*provider.Label, error) {
	label := &github.UpdateIssueLabelRequest{}
	if opts.NewName != nil {
		label.NewName = opts.NewName
	}
	if opts.Color != nil {
		label.Color = opts.Color
	}
	if opts.Description != nil {
		label.Description = opts.Description
	}
	updated, _, err := p.client.Issues.UpdateLabel(ctx, owner, repo, name, *label)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "UpdateLabel", err)
	}
	return convertLabel(updated), nil
}

// DeleteLabel implements provider.LabelManager.
func (p *Provider) DeleteLabel(ctx context.Context, owner, repo, name string) error {
	if _, err := p.client.Issues.DeleteLabel(ctx, owner, repo, name); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DeleteLabel", err)
	}
	return nil
}

// convertLabel maps a github.Label to a provider.Label. GitHub colors are
// already 6-digit hex without '#'; TrimPrefix keeps malformed payloads
// canonical too.
func convertLabel(l *github.Label) *provider.Label {
	if l == nil {
		return nil
	}
	return &provider.Label{
		ID:          l.GetID(),
		Name:        l.GetName(),
		Color:       strings.TrimPrefix(l.GetColor(), "#"),
		Description: l.GetDescription(),
	}
}

var _ provider.LabelManager = (*Provider)(nil)
