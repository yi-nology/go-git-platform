package giteafamily

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- shared small converters (labels / users / repos / branches) ---

// Label is the slice of the family's label object the engine consumes.
type Label struct {
	ID          int64
	Name        string
	Color       string
	Description string
}

// User is the slice of the family's user object the engine consumes.
type User struct {
	ID        int64
	UserName  string
	FullName  string
	AvatarURL string
}

// Repo is the slice of the family's repository object the engine consumes.
type Repo struct {
	ID            int64
	FullName      string
	Name          string
	OwnerUserName string
	Description   string
	CloneURL      string
	SSHURL        string
	DefaultBranch string
	Private       bool
	Archived      bool
	Fork          bool
	Stars         int
	Language      string
	HTMLURL       string
	Forks         int
}

// Branch is the slice of the family's branch object the engine consumes.
type Branch struct {
	Name string
}

// ConvertRepo projects a family Repo onto the provider repository shape.
func ConvertRepo(r *Repo, platform provider.Platform) *provider.PlatformRepo {
	if r == nil {
		return nil
	}
	return &provider.PlatformRepo{
		ID:            r.ID,
		FullName:      r.FullName,
		Name:          r.Name,
		Owner:         r.OwnerUserName,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Archived:      r.Archived,
		Fork:          r.Fork,
		Stars:         r.Stars,
		Language:      r.Language,
		Platform:      platform,
	}
}

// convertUser projects a family User onto the provider user shape.
func convertUser(u *User) *provider.CRUser {
	if u == nil {
		return nil
	}
	return &provider.CRUser{ID: u.ID, Username: u.UserName, Name: u.FullName, AvatarURL: u.AvatarURL}
}

// --- labels ---

// ListLabelsOptions mirrors the family's label list query.
type ListLabelsOptions struct {
	ListOptions ListOptions
}

// CreateLabelOption mirrors the family's label create body. The family
// accepts colors with or without a leading '#'; the engine sends the
// '#'-prefixed form.
type CreateLabelOption struct {
	Name        string
	Color       string
	Description string
}

// EditLabelOption mirrors the family's label edit body.
type EditLabelOption struct {
	Name        *string
	Color       *string
	Description *string
}

// LabelPort is the label endpoint of the family SDK surface. Label IDs
// resolve by name through LabelScanLimitMaxPages pages of LabelScanLimitPageSize.
type LabelPort interface {
	ListRepoLabels(ctx context.Context, owner, repo string, opt ListLabelsOptions) ([]*Label, error)
	CreateLabel(ctx context.Context, owner, repo string, opt CreateLabelOption) (*Label, error)
	EditLabel(ctx context.Context, owner, repo string, id int64, opt EditLabelOption) (*Label, error)
	DeleteLabel(ctx context.Context, owner, repo string, id int64) error
}

// convertLabel maps a family Label to a provider.Label. Family colors may
// carry a leading '#'; it is stripped for the SDK's canonical form.
func convertLabel(l *Label) *provider.Label {
	if l == nil {
		return nil
	}
	return &provider.Label{
		ID:          l.ID,
		Name:        l.Name,
		Color:       strings.TrimPrefix(l.Color, "#"),
		Description: l.Description,
	}
}

// ListLabels lists the repository's labels (one caller-driven page; the
// provider surface is the normalized single-page contract).
func ListLabels(ctx context.Context, f Family, c LabelPort, owner, repo string, opts provider.ListLabelsOptions) ([]*provider.Label, error) {
	page, perPage := provider.NormalizePageOpts(opts.Page, opts.PerPage)
	labels, err := c.ListRepoLabels(ctx, owner, repo, ListLabelsOptions{
		ListOptions: ListOptions{Page: page, PageSize: perPage},
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListLabels", err)
	}
	result := make([]*provider.Label, 0, len(labels))
	for _, l := range labels {
		result = append(result, convertLabel(l))
	}
	return result, nil
}

// CreateLabel creates a label.
func CreateLabel(ctx context.Context, f Family, c LabelPort, owner, repo string, opts provider.CreateLabelOptions) (*provider.Label, error) {
	label, err := c.CreateLabel(ctx, owner, repo, CreateLabelOption{
		Name:        opts.Name,
		Color:       "#" + opts.Color,
		Description: opts.Description,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateLabel", err)
	}
	return convertLabel(label), nil
}

// UpdateLabel edits a label addressed by name (the family's update endpoint
// addresses labels by ID, so the label is resolved by name first via the
// per-provider TTL cache).
func UpdateLabel(ctx context.Context, f Family, c LabelPort, cache *backendutil.IDCache, owner, repo, name string, opts provider.UpdateLabelOptions) (*provider.Label, error) {
	id, err := resolveLabelID(ctx, f, c, cache, "UpdateLabel", owner, repo, name)
	if err != nil {
		return nil, err
	}
	edit := EditLabelOption{}
	if opts.NewName != nil {
		edit.Name = opts.NewName
	}
	if opts.Color != nil {
		c := "#" + *opts.Color
		edit.Color = &c
	}
	if opts.Description != nil {
		edit.Description = opts.Description
	}
	label, err := c.EditLabel(ctx, owner, repo, id, edit)
	if err != nil {
		return nil, provider.Wrap(f.Platform, "UpdateLabel", err)
	}
	return convertLabel(label), nil
}

// DeleteLabel removes a label addressed by name (resolved to its ID first).
func DeleteLabel(ctx context.Context, f Family, c LabelPort, cache *backendutil.IDCache, owner, repo, name string) error {
	id, err := resolveLabelID(ctx, f, c, cache, "DeleteLabel", owner, repo, name)
	if err != nil {
		return err
	}
	if err := c.DeleteLabel(ctx, owner, repo, id); err != nil {
		return provider.Wrap(f.Platform, "DeleteLabel", err)
	}
	return nil
}

// ResolveLabelID finds the numeric ID of the named label via the shared
// paginated resolver with the per-provider TTL cache; exported for the
// backends' issue-assignee path, which shares the same label-ID mapping.
func ResolveLabelID(ctx context.Context, f Family, c LabelPort, cache *backendutil.IDCache, op, owner, repo, name string) (int64, error) {
	return resolveLabelID(ctx, f, c, cache, op, owner, repo, name)
}

// resolveLabelID finds the numeric ID of the named label via the shared
// paginated resolver with the per-provider TTL cache. The family's update
// and delete endpoints address labels by ID while the provider surface
// addresses them by name. Exhausting the scan budget surfaces a scan-limit
// error (distinct from a definitive 404). op is the public operation the
// resolution serves; failures surface under that op rather than under this
// unexported helper's name.
func resolveLabelID(ctx context.Context, f Family, c LabelPort, cache *backendutil.IDCache, op, owner, repo, name string) (int64, error) {
	id, err := cache.ResolveLabel(owner+"/"+repo, name, func(page, perPage int) ([]backendutil.LabelRef, error) {
		labels, err := c.ListRepoLabels(ctx, owner, repo, ListLabelsOptions{
			ListOptions: ListOptions{Page: page, PageSize: perPage},
		})
		if err != nil {
			return nil, err
		}
		refs := make([]backendutil.LabelRef, len(labels))
		for i, l := range labels {
			refs[i] = backendutil.LabelRef{ID: l.ID, Name: l.Name}
		}
		return refs, nil
	}, backendutil.LabelScanMaxPages, backendutil.LabelPageSize)
	if err != nil {
		if errors.Is(err, backendutil.ErrLabelScanLimit) {
			msg := fmt.Sprintf("label %q not found within %d pages (scan limit)", name, backendutil.LabelScanMaxPages)
			return 0, provider.Wrap(f.Platform, op, backendutil.NewScanLimitError(msg))
		}
		return 0, provider.Wrap(f.Platform, op, err)
	}
	return id, nil
}

// --- branches ---

// ListRepoBranchesOptions mirrors the family's branch list query.
type ListRepoBranchesOptions struct {
	ListOptions ListOptions
}

// CreateBranchOption mirrors the family's branch create body.
type CreateBranchOption struct {
	BranchName    string
	OldBranchName string
}

// BranchPort is the branch endpoint of the family SDK surface.
type BranchPort interface {
	ListRepoBranches(ctx context.Context, owner, repo string, opt ListRepoBranchesOptions) ([]*Branch, error)
	CreateBranch(ctx context.Context, owner, repo string, opt CreateBranchOption) (*Branch, error)
	DeleteRepoBranch(ctx context.Context, owner, repo, branch string) error
}

// ListBranches lists every branch. The provider surface carries no
// pagination parameters, so the full branch list is fetched by exhausting
// the endpoint's pagination (backendutil.AllPages).
func ListBranches(ctx context.Context, f Family, c BranchPort, owner, repo string) ([]*provider.PlatformBranch, error) {
	branches, err := backendutil.AllPages(func(page int) ([]*Branch, error) {
		return c.ListRepoBranches(ctx, owner, repo, ListRepoBranchesOptions{
			ListOptions: ListOptions{Page: page, PageSize: f.PageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListBranches", err)
	}
	result := make([]*provider.PlatformBranch, 0, len(branches))
	for _, b := range branches {
		if b != nil {
			result = append(result, &provider.PlatformBranch{Name: b.Name})
		}
	}
	return result, nil
}

// CreateBranch creates a branch from an existing ref.
func CreateBranch(ctx context.Context, f Family, c BranchPort, owner, repo, branch, ref string) (*provider.PlatformBranch, error) {
	b, err := c.CreateBranch(ctx, owner, repo, CreateBranchOption{
		BranchName:    branch,
		OldBranchName: ref,
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "CreateBranch", err)
	}
	if b == nil {
		return nil, provider.Wrapf(f.Platform, "CreateBranch", "empty response")
	}
	return &provider.PlatformBranch{Name: b.Name}, nil
}

// DeleteBranch removes a branch.
func DeleteBranch(ctx context.Context, f Family, c BranchPort, owner, repo, branch string) error {
	if err := c.DeleteRepoBranch(ctx, owner, repo, branch); err != nil {
		return provider.Wrap(f.Platform, "DeleteBranch", err)
	}
	return nil
}

// --- collaborators ---

// AddCollaboratorOption mirrors the family's collaborator add body.
type AddCollaboratorOption struct {
	Permission *string
}

// CollaboratorPort is the collaborator endpoint of the family SDK surface.
// ListCollaborators returns users without permission info, so the engine
// issues one CollaboratorPermission call per user to fill it in.
type CollaboratorPort interface {
	ListCollaborators(ctx context.Context, owner, repo string, opt ListCollaboratorsOptions) ([]*User, error)
	CollaboratorPermission(ctx context.Context, owner, repo, username string) (string, error)
	AddCollaborator(ctx context.Context, owner, repo, username string, opt AddCollaboratorOption) error
	DeleteCollaborator(ctx context.Context, owner, repo, username string) error
}

// ListCollaboratorsOptions mirrors the family's collaborator list query.
type ListCollaboratorsOptions struct {
	ListOptions ListOptions
}

// ListCollaborators lists collaborators, fetching the permission of each
// (the family's list endpoint returns users without permission info).
func ListCollaborators(ctx context.Context, f Family, c CollaboratorPort, owner, repo string) ([]*provider.Collaborator, error) {
	users, err := backendutil.AllPages(func(page int) ([]*User, error) {
		return c.ListCollaborators(ctx, owner, repo, ListCollaboratorsOptions{
			ListOptions: ListOptions{Page: page, PageSize: f.PageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListCollaborators", err)
	}
	result := make([]*provider.Collaborator, 0, len(users))
	for _, u := range users {
		collab := &provider.Collaborator{
			ID:       u.ID,
			Username: u.UserName,
		}
		perm, permErr := c.CollaboratorPermission(ctx, owner, repo, u.UserName)
		if permErr == nil {
			collab.Permission = perm
		}
		result = append(result, collab)
	}
	return result, nil
}

// AddCollaborator adds a collaborator.
func AddCollaborator(ctx context.Context, f Family, c CollaboratorPort, owner, repo, username string, opts provider.AddCollaboratorOptions) error {
	opt := AddCollaboratorOption{}
	if opts.Permission != "" {
		perm := opts.Permission
		opt.Permission = &perm
	}
	if err := c.AddCollaborator(ctx, owner, repo, username, opt); err != nil {
		return provider.Wrap(f.Platform, "AddCollaborator", err)
	}
	return nil
}

// RemoveCollaborator removes a collaborator.
func RemoveCollaborator(ctx context.Context, f Family, c CollaboratorPort, owner, repo, username string) error {
	if err := c.DeleteCollaborator(ctx, owner, repo, username); err != nil {
		return provider.Wrap(f.Platform, "RemoveCollaborator", err)
	}
	return nil
}

// --- repo stats ---

// ListForksOptions mirrors the family's fork list query.
type ListForksOptions struct {
	ListOptions ListOptions
}

// ListStargazersOptions mirrors the family's stargazer list query.
type ListStargazersOptions struct {
	ListOptions ListOptions
}

// RepoStatsPort is the repo-stats endpoint of the family SDK surface. The
// family exposes no contributors list endpoint, so ListContributors always
// returns ErrNotImplemented.
type RepoStatsPort interface {
	ListForks(ctx context.Context, owner, repo string, opt ListForksOptions) ([]*Repo, error)
	ListRepoStargazers(ctx context.Context, owner, repo string, opt ListStargazersOptions) ([]*User, error)
}

// ListForks lists every fork. The provider surface carries no pagination
// parameters, so the full fork list is fetched by exhausting the endpoint's
// pagination.
func ListForks(ctx context.Context, f Family, c RepoStatsPort, owner, repo string) ([]*provider.PlatformRepo, error) {
	forks, err := backendutil.AllPages(func(page int) ([]*Repo, error) {
		return c.ListForks(ctx, owner, repo, ListForksOptions{
			ListOptions: ListOptions{Page: page, PageSize: f.PageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListForks", err)
	}
	result := make([]*provider.PlatformRepo, 0, len(forks))
	for _, r := range forks {
		result = append(result, ConvertRepo(r, f.Platform))
	}
	return result, nil
}

// ListStargazers lists every stargazer. The provider surface carries no
// pagination parameters, so the full stargazer list is fetched by
// exhausting the endpoint's pagination.
func ListStargazers(ctx context.Context, f Family, c RepoStatsPort, owner, repo string) ([]*provider.CRUser, error) {
	stargazers, err := backendutil.AllPages(func(page int) ([]*User, error) {
		return c.ListRepoStargazers(ctx, owner, repo, ListStargazersOptions{
			ListOptions: ListOptions{Page: page, PageSize: f.PageSize},
		})
	})
	if err != nil {
		return nil, provider.Wrap(f.Platform, "ListStargazers", err)
	}
	result := make([]*provider.CRUser, 0, len(stargazers))
	for _, u := range stargazers {
		if u != nil {
			result = append(result, convertUser(u))
		}
	}
	return result, nil
}

// ListContributors always returns ErrNotImplemented: the family exposes no
// contributors list endpoint.
func ListContributors(_ context.Context, f Family, _ RepoStatsPort, _, _ string) ([]*provider.Contributor, error) {
	return nil, provider.Wrapf(f.Platform, "ListContributors", "not supported on this platform")
}
