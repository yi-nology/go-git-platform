package tencentcode

import (
	"context"
	"strings"

	gongfeng "github.com/studyzy/gongfeng-sdk-go"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// This file implements provider.LabelManager over the gongfeng SDK's
// LabelsService. Two registered vocabulary notes apply:
//
//   - addressing: 工蜂 addresses labels by name natively — UpdateLabel and
//     DeleteLabel carry the current name in their option bodies — so no
//     name→ID resolution scan is needed (unlike the GitLab backend, whose
//     API addresses labels by numeric ID).
//   - color: gongfeng exchanges GitLab-form colors with a leading '#'
//     ("#4cc917"); the SDK's canonical form is bare 6-digit hex, so '#'
//     is added outbound and stripped inbound.
//
// The gongfeng Label model carries no id field (name/color/description
// plus issue/merge-request counts only), so provider.Label.ID stays zero
// on this platform; labels are name-addressed end to end, so the ID plays
// no addressing role.

// ListLabels implements provider.LabelManager.
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListLabels(ctx context.Context, owner, repo string, opts provider.ListLabelsOptions) ([]*provider.Label, error) {
	buildOpts := func(page, perPage int) *gongfeng.ListLabelsOptions {
		return &gongfeng.ListLabelsOptions{
			ListOptions: gongfeng.ListOptions{Page: page, PerPage: perPage},
		}
	}
	var labels []*gongfeng.Label
	labels, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gongfeng.Label, error) {
			list, _, err := p.client.Labels.ListLabels(ctx, pid(owner, repo), buildOpts(page, perPage))
			return list, err
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListLabels", err)
	}
	result := make([]*provider.Label, 0, len(labels))
	for _, l := range labels {
		result = append(result, convertLabel(l))
	}
	return result, nil
}

// CreateLabel implements provider.LabelManager. 工蜂 requires colors with
// a leading '#'.
func (p *Provider) CreateLabel(ctx context.Context, owner, repo string, opts provider.CreateLabelOptions) (*provider.Label, error) {
	createOpts := &gongfeng.CreateLabelOptions{
		Name:  gongfeng.Ptr(opts.Name),
		Color: gongfeng.Ptr("#" + opts.Color),
	}
	if opts.Description != "" {
		createOpts.Description = gongfeng.Ptr(opts.Description)
	}
	label, _, err := p.client.Labels.CreateLabel(ctx, pid(owner, repo), createOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "CreateLabel", err)
	}
	return convertLabel(label), nil
}

// UpdateLabel implements provider.LabelManager. The current name travels
// in the update body as 工蜂's addressing key; nil fields in opts stay
// absent from the PUT body, leaving the label unchanged.
func (p *Provider) UpdateLabel(ctx context.Context, owner, repo, name string, opts provider.UpdateLabelOptions) (*provider.Label, error) {
	updateOpts := &gongfeng.UpdateLabelOptions{
		Name: gongfeng.Ptr(name),
	}
	if opts.NewName != nil {
		updateOpts.NewName = opts.NewName
	}
	if opts.Color != nil {
		updateOpts.Color = gongfeng.Ptr("#" + *opts.Color)
	}
	if opts.Description != nil {
		updateOpts.Description = opts.Description
	}
	label, _, err := p.client.Labels.UpdateLabel(ctx, pid(owner, repo), updateOpts)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "UpdateLabel", err)
	}
	return convertLabel(label), nil
}

// DeleteLabel implements provider.LabelManager. The label name travels in
// the delete body as 工蜂's addressing key.
func (p *Provider) DeleteLabel(ctx context.Context, owner, repo, name string) error {
	if _, err := p.client.Labels.DeleteLabel(ctx, pid(owner, repo), &gongfeng.DeleteLabelOptions{
		Name: gongfeng.Ptr(name),
	}); err != nil {
		return provider.Wrap(provider.PlatformTencentCode, "DeleteLabel", err)
	}
	return nil
}

// convertLabel maps a gongfeng.Label to a provider.Label. 工蜂 colors
// carry a leading '#', which is stripped for the SDK's canonical form; the
// gongfeng model has no id field, so ID stays zero.
func convertLabel(l *gongfeng.Label) *provider.Label {
	if l == nil {
		return nil
	}
	return &provider.Label{
		Name:        l.Name,
		Color:       strings.TrimPrefix(l.Color, "#"),
		Description: l.Description,
	}
}

var _ provider.LabelManager = (*Provider)(nil)
