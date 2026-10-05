package gitea

import (
	"context"

	gitea "gitea.dev/sdk"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
)

// giteafamilyDivergences is the Gitea view of the family divergence ledger.
var giteafamilyDivergences = giteafamily.Divergences("Gitea")

// sdkPorts adapts the gitea SDK to the family engine's ports. The gitea SDK
// is the family's upstream vocabulary, so the adapters are direct
// delegation apart from trimming the unused *Response returns and projecting
// the few wire fields the engine consumes.
type sdkPorts struct {
	c *gitea.Client
}

// --- giteafamily.HookPort ---

func (s sdkPorts) CreateRepoHook(ctx context.Context, owner, repo string, opt giteafamily.CreateHookOption) (*giteafamily.Hook, error) {
	h, _, err := s.c.Hooks.CreateRepoHook(ctx, owner, repo, gitea.CreateHookOption{
		Type:   gitea.HookType(opt.Type),
		Config: opt.Config,
		Events: opt.Events,
		Active: opt.Active,
	})
	return familyHook(h), err
}

func (s sdkPorts) DeleteRepoHook(ctx context.Context, owner, repo string, id int64) error {
	_, err := s.c.Hooks.DeleteRepoHook(ctx, owner, repo, id)
	return err
}

func (s sdkPorts) ListRepoHooks(ctx context.Context, owner, repo string, opt giteafamily.ListHooksOptions) ([]*giteafamily.Hook, error) {
	list, _, err := s.c.Hooks.ListRepoHooks(ctx, owner, repo, gitea.ListHooksOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyHooks(list), err
}

func familyHook(h *gitea.Hook) *giteafamily.Hook {
	if h == nil {
		return nil
	}
	return &giteafamily.Hook{ID: h.ID, Config: h.Config, Events: h.Events}
}

func familyHooks(list []*gitea.Hook) []*giteafamily.Hook {
	out := make([]*giteafamily.Hook, 0, len(list))
	for _, h := range list {
		out = append(out, familyHook(h))
	}
	return out
}

// --- giteafamily.DeployKeyPort ---

func (s sdkPorts) ListDeployKeys(ctx context.Context, owner, repo string, opt giteafamily.ListDeployKeysOptions) ([]*giteafamily.DeployKey, error) {
	list, _, err := s.c.Repositories.ListDeployKeys(ctx, owner, repo, gitea.ListDeployKeysOptions{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyDeployKeys(list), err
}

func (s sdkPorts) CreateDeployKey(ctx context.Context, owner, repo string, opt giteafamily.CreateKeyOption) (*giteafamily.DeployKey, error) {
	k, _, err := s.c.Repositories.CreateDeployKey(ctx, owner, repo, gitea.CreateKeyOption{
		Title:    opt.Title,
		Key:      opt.Key,
		ReadOnly: opt.ReadOnly,
	})
	return familyDeployKey(k), err
}

func (s sdkPorts) DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error {
	_, err := s.c.Repositories.DeleteDeployKey(ctx, owner, repo, keyID)
	return err
}

func familyDeployKey(k *gitea.DeployKey) *giteafamily.DeployKey {
	if k == nil {
		return nil
	}
	return &giteafamily.DeployKey{ID: k.ID, Title: k.Title, Key: k.Key, ReadOnly: k.ReadOnly}
}

func familyDeployKeys(list []*gitea.DeployKey) []*giteafamily.DeployKey {
	out := make([]*giteafamily.DeployKey, 0, len(list))
	for _, k := range list {
		out = append(out, familyDeployKey(k))
	}
	return out
}

// --- giteafamily.MilestonePort ---

func (s sdkPorts) ListMilestones(ctx context.Context, owner, repo string, opt giteafamily.ListMilestoneOption) ([]*giteafamily.Milestone, error) {
	listOpts := gitea.ListMilestoneOption{
		ListOptions: gitea.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		State:       gitea.StateType(opt.State),
	}
	list, _, err := s.c.Repositories.ListMilestones(ctx, owner, repo, listOpts)
	return familyMilestones(list), err
}

func (s sdkPorts) GetMilestone(ctx context.Context, owner, repo string, id int64) (*giteafamily.Milestone, error) {
	m, _, err := s.c.Repositories.GetMilestone(ctx, owner, repo, id)
	return familyMilestone(m), err
}

func (s sdkPorts) CreateMilestone(ctx context.Context, owner, repo string, opt giteafamily.CreateMilestoneOption) (*giteafamily.Milestone, error) {
	m, _, err := s.c.Repositories.CreateMilestone(ctx, owner, repo, gitea.CreateMilestoneOption{
		Title:       opt.Title,
		Description: opt.Description,
		Deadline:    opt.Deadline,
	})
	return familyMilestone(m), err
}

func (s sdkPorts) EditMilestone(ctx context.Context, owner, repo string, id int64, opt giteafamily.EditMilestoneOption) (*giteafamily.Milestone, error) {
	editOpts := gitea.EditMilestoneOption{
		Title:       opt.Title,
		Description: opt.Description,
		Deadline:    opt.Deadline,
	}
	if opt.State != nil {
		state := gitea.StateType(*opt.State)
		editOpts.State = &state
	}
	m, _, err := s.c.Repositories.EditMilestone(ctx, owner, repo, id, editOpts)
	return familyMilestone(m), err
}

func (s sdkPorts) DeleteMilestone(ctx context.Context, owner, repo string, id int64) error {
	_, err := s.c.Repositories.DeleteMilestone(ctx, owner, repo, id)
	return err
}

func familyMilestone(m *gitea.Milestone) *giteafamily.Milestone {
	if m == nil {
		return nil
	}
	return &giteafamily.Milestone{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		State:       string(m.State),
		Deadline:    m.Deadline,
	}
}

func familyMilestones(list []*gitea.Milestone) []*giteafamily.Milestone {
	out := make([]*giteafamily.Milestone, 0, len(list))
	for _, m := range list {
		out = append(out, familyMilestone(m))
	}
	return out
}

// compile-time anchors: the adapter must keep satisfying every family port.
var (
	_ giteafamily.HookPort      = sdkPorts{}
	_ giteafamily.DeployKeyPort = sdkPorts{}
	_ giteafamily.MilestonePort = sdkPorts{}
)
