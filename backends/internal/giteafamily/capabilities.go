package giteafamily

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- deploy keys ---

// DeployKey is the slice of the family's deploy-key object the engine
// consumes.
type DeployKey struct {
	ID       int64
	Title    string
	Key      string
	ReadOnly bool
}

// ListDeployKeysOptions mirrors the family's deploy-key list query.
type ListDeployKeysOptions struct {
	ListOptions ListOptions
}

// CreateKeyOption mirrors the family's deploy-key create body.
type CreateKeyOption struct {
	Title    string
	Key      string
	ReadOnly bool
}

// DeployKeyPort is the deploy-key endpoint of the family SDK surface.
type DeployKeyPort interface {
	ListDeployKeys(ctx context.Context, owner, repo string, opt ListDeployKeysOptions) ([]*DeployKey, error)
	CreateDeployKey(ctx context.Context, owner, repo string, opt CreateKeyOption) (*DeployKey, error)
	DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error
}

// ListDeployKeys returns every deploy key. The provider surface carries no
// pagination parameters, so the full key list is fetched by exhausting the
// endpoint's pagination (backendutil.AllPages).
func ListDeployKeys(ctx context.Context, f Family, c DeployKeyPort, owner, repo string) ([]*provider.DeployKey, error) {
	keys, err := backendutil.AllPages(func(page int) ([]*DeployKey, error) {
		return c.ListDeployKeys(ctx, owner, repo, ListDeployKeysOptions{
			ListOptions: ListOptions{Page: page, PageSize: f.PageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListDeployKeys", err)
	}
	result := make([]*provider.DeployKey, 0, len(keys))
	for _, k := range keys {
		result = append(result, convertDeployKey(k))
	}
	return result, nil
}

// AddDeployKey creates a deploy key.
func AddDeployKey(ctx context.Context, f Family, c DeployKeyPort, owner, repo string, opts provider.AddDeployKeyOptions) (*provider.DeployKey, error) {
	key, err := c.CreateDeployKey(ctx, owner, repo, CreateKeyOption{
		Title:    opts.Title,
		Key:      opts.Key,
		ReadOnly: opts.ReadOnly,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "AddDeployKey", err)
	}
	return convertDeployKey(key), nil
}

// DeleteDeployKey removes a deploy key.
func DeleteDeployKey(ctx context.Context, f Family, c DeployKeyPort, owner, repo string, keyID int64) error {
	if err := c.DeleteDeployKey(ctx, owner, repo, keyID); err != nil {
		return provider.Wrap(f.Platform, "DeleteDeployKey", err)
	}
	return nil
}

// convertDeployKey projects a family DeployKey onto the provider shape.
func convertDeployKey(k *DeployKey) *provider.DeployKey {
	if k == nil {
		return nil
	}
	return &provider.DeployKey{
		ID:       k.ID,
		Title:    k.Title,
		Key:      k.Key,
		ReadOnly: k.ReadOnly,
	}
}

// --- milestones ---

// Milestone is the slice of the family's milestone object the engine
// consumes. Number carries the family milestone ID (the identifier the
// write endpoints take); Deadline keys the wire's due_on.
type Milestone struct {
	ID          int64
	Title       string
	Description string
	State       string
	Deadline    *time.Time
}

// ListMilestoneOption mirrors the family's milestone list query
// (state: "open"/"closed"/"all").
type ListMilestoneOption struct {
	ListOptions ListOptions
	State       string
}

// CreateMilestoneOption mirrors the family's milestone create body.
type CreateMilestoneOption struct {
	Title       string
	Description string
	Deadline    *time.Time
}

// EditMilestoneOption mirrors the family's milestone edit body. Nil fields
// marshal as JSON null and the server leaves them unchanged; Title is the
// family's only non-pointer edit field, so an update that does not rename
// sends an empty title — the API keeps the existing title for blank values
// (a title is required to be non-empty, so blank cannot be a legitimate
// rename).
type EditMilestoneOption struct {
	Title       string
	Description *string
	State       *string
	Deadline    *time.Time
}

// MilestonePort is the milestone endpoint of the family SDK surface.
type MilestonePort interface {
	ListMilestones(ctx context.Context, owner, repo string, opt ListMilestoneOption) ([]*Milestone, error)
	GetMilestone(ctx context.Context, owner, repo string, id int64) (*Milestone, error)
	CreateMilestone(ctx context.Context, owner, repo string, opt CreateMilestoneOption) (*Milestone, error)
	EditMilestone(ctx context.Context, owner, repo string, id int64, opt EditMilestoneOption) (*Milestone, error)
	DeleteMilestone(ctx context.Context, owner, repo string, id int64) error
}

// ListMilestones lists milestones, state-filtered by "open"/"closed" (the
// family also accepts "all"; the API defaults to open).
//
// Dual-mode pagination is handled by backendutil.PageList:
// opts.Page == 0 walks every page (budget-capped, fails loud);
// opts.Page > 0 returns exactly that single caller-driven page.
func ListMilestones(ctx context.Context, f Family, c MilestonePort, owner, repo string, opts provider.ListMilestonesOptions) ([]provider.Milestone, error) {
	listOpts := ListMilestoneOption{}
	if opts.State != "" {
		listOpts.State = opts.State
	}
	milestones, err := backendutil.PageList(opts.Page, opts.PerPage, f.PageSize,
		func(page, perPage int) ([]*Milestone, error) {
			listOpts.ListOptions = ListOptions{Page: page, PageSize: perPage}
			return c.ListMilestones(ctx, owner, repo, listOpts)
		})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListMilestones", err)
	}
	result := make([]provider.Milestone, 0, len(milestones))
	for _, m := range milestones {
		result = append(result, convertMilestone(m))
	}
	return result, nil
}

// GetMilestone fetches one milestone by ID.
func GetMilestone(ctx context.Context, f Family, c MilestonePort, owner, repo, number string) (*provider.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(f.Platform, "GetMilestone", number)
	if err != nil {
		return nil, err
	}
	m, err := c.GetMilestone(ctx, owner, repo, id)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "GetMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// CreateMilestone creates a milestone.
func CreateMilestone(ctx context.Context, f Family, c MilestonePort, owner, repo string, opts provider.CreateMilestoneOptions) (*provider.Milestone, error) {
	m, err := c.CreateMilestone(ctx, owner, repo, CreateMilestoneOption{
		Title:       opts.Title,
		Description: opts.Description,
		Deadline:    opts.DueOn,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// UpdateMilestone edits a milestone; see EditMilestoneOption for the
// nil-means-unchanged edit semantics.
func UpdateMilestone(ctx context.Context, f Family, c MilestonePort, owner, repo, number string, opts provider.UpdateMilestoneOptions) (*provider.Milestone, error) {
	id, err := backendutil.ParseMilestoneNumber(f.Platform, "UpdateMilestone", number)
	if err != nil {
		return nil, err
	}
	editOpts := EditMilestoneOption{}
	if opts.Title != nil {
		editOpts.Title = *opts.Title
	}
	if opts.Description != nil {
		editOpts.Description = opts.Description
	}
	if opts.State != "" {
		state := string(opts.State)
		editOpts.State = &state
	}
	if opts.DueOn != nil {
		editOpts.Deadline = opts.DueOn
	}
	m, err := c.EditMilestone(ctx, owner, repo, id, editOpts)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateMilestone", err)
	}
	ms := convertMilestone(m)
	return &ms, nil
}

// DeleteMilestone removes a milestone.
func DeleteMilestone(ctx context.Context, f Family, c MilestonePort, owner, repo, number string) error {
	id, err := backendutil.ParseMilestoneNumber(f.Platform, "DeleteMilestone", number)
	if err != nil {
		return err
	}
	if err := c.DeleteMilestone(ctx, owner, repo, id); err != nil {
		return provider.Wrap(f.Platform, "DeleteMilestone", err)
	}
	return nil
}

// convertMilestone projects a family Milestone onto the provider shape.
func convertMilestone(m *Milestone) provider.Milestone {
	var ms provider.Milestone
	if m == nil {
		return ms
	}
	ms = provider.Milestone{
		Number:      strconv.FormatInt(m.ID, 10),
		Title:       m.Title,
		Description: m.Description,
		State:       provider.MilestoneState(m.State),
		DueOn:       m.Deadline,
	}
	return ms
}

// --- divergence ledger ---

// Divergences returns the family's registered divergence ledger: the places
// where family backend behavior departs from the unified provider
// semantics. displayName is the platform's display name ("Gitea"/"Forgejo")
// used in the reason texts.
func Divergences(displayName string) []provider.Divergence {
	return []provider.Divergence{
		{Capability: "ChangeRequestManager", Method: "GetCR", Field: "BaseSHA", Kind: provider.DivergenceMapping,
			Reason: fmt.Sprintf("%s payloads expose no merge base; BaseSHA carries the target-branch tip instead (StartSHA equals BaseSHA), as does every other method returning a change request.", displayName)},
		{Capability: "ChangeRequestManager", Method: "ListCRs", Field: "BaseSHA", Kind: provider.DivergenceMapping,
			Reason: "See GetCR."},
	}
}
