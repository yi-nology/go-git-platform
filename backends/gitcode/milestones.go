package gitcode

import (
	"context"
	"fmt"
	"net/url"
	"time"

	gitcode "github.com/yi-nology/go-gitcode"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"

	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/transport"
)

// This file implements provider.MilestoneManager over go-gitcode's
// milestones surface. GitCode addresses milestones by their ID (the wire's
// "id" — the SDK's Get/Update/Delete path parameter, whatever its
// "number" spelling) — the same value MilestoneRef.Number and
// Milestone.Number carry on this platform. Wire states are
// "open"/"closed", already the SDK's vocabulary.
//
// CreateMilestone and UpdateMilestone are routed through the raw transport
// client rather than the SDK (registered detour, in the spirit of the
// gitee raw detours): the SDK's create/update option structs marshal
// `DueOn string` under `json:"due_on"` WITHOUT omitempty, so a call
// without a due date posts `"due_on": ""` — on GitCode's GitHub-shaped API
// an explicit empty due_on conventionally clears (or errors on) the value,
// which would wipe the milestone's due date on every title-only update.
// The SDK marshals its option struct directly into the request body, so
// the empty key cannot be suppressed through its types; the raw bodies
// carry exactly the fields the caller set (title always on create). The
// raw paths interpolate owner/repo through esc() — the transport client
// concatenates BaseURL and Path verbatim, so unescaped segments with
// '#', '?', '%', or spaces would corrupt the URL.

// ListMilestones implements provider.MilestoneManager. State filters by
// "open"/"closed" (GitCode also accepts "all").
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func (p *Provider) ListMilestones(ctx context.Context, owner, repo string, opts provider.ListMilestonesOptions) ([]provider.Milestone, error) {
	buildOpts := func(page, perPage int) gitcode.ListMilestonesOptions {
		return gitcode.ListMilestonesOptions{
			ListOptions: gitcode.ListOptions{Page: page, PerPage: perPage},
			State:       opts.State,
		}
	}
	var milestones []*gitcode.Milestone
	milestones, err := backendutil.PageList(opts.Page, opts.PerPage, provider.MaxPerPage,
		func(page, perPage int) ([]*gitcode.Milestone, error) {
			return p.client.ListMilestonesWithOptions(ctx, owner, repo, buildOpts(page, perPage))
		})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "ListMilestones", err)
	}
	result := make([]provider.Milestone, 0, len(milestones))
	for _, m := range milestones {
		result = append(result, convertMilestone(m))
	}
	return result, nil
}

// GetMilestone implements provider.MilestoneManager.
func (p *Provider) GetMilestone(ctx context.Context, owner, repo, number string) (*provider.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "GetMilestone", number)
	if err != nil {
		return nil, err
	}
	m, err := p.client.GetMilestone(ctx, owner, repo, int(id))
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "GetMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// CreateMilestone implements provider.MilestoneManager through the raw
// transport client (SDK due_on omission bug — see the file comment). The
// body carries title always, description/due_on only when set.
func (p *Provider) CreateMilestone(ctx context.Context, owner, repo string, opts provider.CreateMilestoneOptions) (*provider.Milestone, error) {
	body := map[string]any{"title": opts.Title}
	if opts.Description != "" {
		body["description"] = opts.Description
	}
	if opts.DueOn != nil {
		body["due_on"] = opts.DueOn.Format(time.RFC3339)
	}
	var m gitcode.Milestone
	if err := p.doJSON(ctx, "POST", fmt.Sprintf("/repos/%s/%s/milestones", esc(owner), esc(repo)), body, &m); err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "CreateMilestone", err)
	}
	ms := convertMilestone(&m)
	return &ms, nil
}

// UpdateMilestone implements provider.MilestoneManager through the raw
// transport client (SDK due_on omission bug — see the file comment). The
// body carries exactly the fields the caller set; everything left nil is
// absent from the wire, leaving the milestone unchanged.
func (p *Provider) UpdateMilestone(ctx context.Context, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*provider.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "UpdateMilestone", number)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if opts.Title != nil {
		body["title"] = *opts.Title
	}
	if opts.Description != nil {
		body["description"] = *opts.Description
	}
	if opts.State != "" {
		body["state"] = string(opts.State)
	}
	if opts.DueOn != nil {
		body["due_on"] = opts.DueOn.Format(time.RFC3339)
	}
	var m gitcode.Milestone
	if err := p.doJSON(ctx, "PATCH", fmt.Sprintf("/repos/%s/%s/milestones/%d", esc(owner), esc(repo), id), body, &m); err != nil {
		return nil, provider.Wrap(provider.PlatformGitCode, "UpdateMilestone", err)
	}
	ms := convertMilestone(&m)
	return &ms, nil
}

// DeleteMilestone implements provider.MilestoneManager.
func (p *Provider) DeleteMilestone(ctx context.Context, owner, repo, number string) error {
	id, err := backendutil.ParseMilestoneNumber(provider.PlatformGitCode, "DeleteMilestone", number)
	if err != nil {
		return err
	}
	if err := p.client.DeleteMilestone(ctx, owner, repo, int(id)); err != nil {
		return provider.Wrap(provider.PlatformGitCode, "DeleteMilestone", err)
	}
	return nil
}

// esc escapes a single URL path segment. Variable segments interpolated
// into raw-client paths (owner, repo, ...) must pass through this — the
// transport client joins BaseURL and Path by plain concatenation, so
// characters like '#', '?', '%', or spaces would otherwise corrupt or
// truncate the URL. (SDK-covered calls are handed to go-gitcode's typed
// methods and are out of scope here.)
func esc(s string) string { return url.PathEscape(s) }

// doJSON is the raw-transport convenience wrapper serving the registered
// detours of this package (see Provider.rawClient): JSON-in / JSON-out
// with the method/path/body/result signature used by the gitee backend.
// Path segments interpolated by the caller must already be esc()-escaped.
func (p *Provider) doJSON(ctx context.Context, method, path string, body, result any) error {
	_, err := p.rawClient.DoJSON(ctx, &transport.Request{
		Method: method,
		Path:   path,
		Body:   body,
		Result: result,
	})
	return err
}

var _ provider.MilestoneManager = (*Provider)(nil)
