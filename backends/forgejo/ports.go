package forgejo

import (
	"context"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	"github.com/yi-nology/go-git-platform/backends/internal/giteafamily"
)

// forgejoDivergences is the Forgejo view of the family divergence ledger.
var forgejoDivergences = giteafamily.Divergences("Forgejo")

// sdkPorts adapts the forgejo SDK to the family engine's ports. The Forgejo
// SDK accepts no context parameter (registered platform limitation) and
// carries fork-identical wire types, so every adapter drops ctx and
// converts the structs field by field.
type sdkPorts struct {
	c *forgejo.Client
}

// --- giteafamily.HookPort ---

func (s sdkPorts) CreateRepoHook(ctx context.Context, owner, repo string, opt giteafamily.CreateHookOption) (*giteafamily.Hook, error) {
	_ = ctx // forgejo SDK takes no context
	h, _, err := s.c.CreateRepoHook(owner, repo, forgejo.CreateHookOption{
		Type:   forgejo.HookType(opt.Type),
		Config: opt.Config,
		Events: opt.Events,
		Active: opt.Active,
	})
	return familyHook(h), err
}

func (s sdkPorts) DeleteRepoHook(ctx context.Context, owner, repo string, id int64) error {
	_ = ctx // forgejo SDK takes no context
	_, err := s.c.DeleteRepoHook(owner, repo, id)
	return err
}

func (s sdkPorts) ListRepoHooks(ctx context.Context, owner, repo string, opt giteafamily.ListHooksOptions) ([]*giteafamily.Hook, error) {
	_ = ctx // forgejo SDK takes no context
	list, _, err := s.c.ListRepoHooks(owner, repo, forgejo.ListHooksOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyHooks(list), err
}

func familyHook(h *forgejo.Hook) *giteafamily.Hook {
	if h == nil {
		return nil
	}
	return &giteafamily.Hook{ID: h.ID, Config: h.Config, Events: h.Events}
}

func familyHooks(list []*forgejo.Hook) []*giteafamily.Hook {
	out := make([]*giteafamily.Hook, 0, len(list))
	for _, h := range list {
		out = append(out, familyHook(h))
	}
	return out
}

// --- giteafamily.DeployKeyPort ---

func (s sdkPorts) ListDeployKeys(ctx context.Context, owner, repo string, opt giteafamily.ListDeployKeysOptions) ([]*giteafamily.DeployKey, error) {
	_ = ctx // forgejo SDK takes no context
	list, _, err := s.c.ListDeployKeys(owner, repo, forgejo.ListDeployKeysOptions{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
	})
	return familyDeployKeys(list), err
}

func (s sdkPorts) CreateDeployKey(ctx context.Context, owner, repo string, opt giteafamily.CreateKeyOption) (*giteafamily.DeployKey, error) {
	_ = ctx // forgejo SDK takes no context
	k, _, err := s.c.CreateDeployKey(owner, repo, forgejo.CreateKeyOption{
		Title:    opt.Title,
		Key:      opt.Key,
		ReadOnly: opt.ReadOnly,
	})
	return familyDeployKey(k), err
}

func (s sdkPorts) DeleteDeployKey(ctx context.Context, owner, repo string, keyID int64) error {
	_ = ctx // forgejo SDK takes no context
	_, err := s.c.DeleteDeployKey(owner, repo, keyID)
	return err
}

func familyDeployKey(k *forgejo.DeployKey) *giteafamily.DeployKey {
	if k == nil {
		return nil
	}
	return &giteafamily.DeployKey{ID: k.ID, Title: k.Title, Key: k.Key, ReadOnly: k.ReadOnly}
}

func familyDeployKeys(list []*forgejo.DeployKey) []*giteafamily.DeployKey {
	out := make([]*giteafamily.DeployKey, 0, len(list))
	for _, k := range list {
		out = append(out, familyDeployKey(k))
	}
	return out
}

// --- giteafamily.MilestonePort ---

func (s sdkPorts) ListMilestones(ctx context.Context, owner, repo string, opt giteafamily.ListMilestoneOption) ([]*giteafamily.Milestone, error) {
	_ = ctx // forgejo SDK takes no context
	listOpts := forgejo.ListMilestoneOption{
		ListOptions: forgejo.ListOptions{Page: opt.ListOptions.Page, PageSize: opt.ListOptions.PageSize},
		State:       forgejo.StateType(opt.State),
	}
	list, _, err := s.c.ListRepoMilestones(owner, repo, listOpts)
	return familyMilestones(list), err
}

func (s sdkPorts) GetMilestone(ctx context.Context, owner, repo string, id int64) (*giteafamily.Milestone, error) {
	_ = ctx // forgejo SDK takes no context
	m, _, err := s.c.GetMilestone(owner, repo, id)
	return familyMilestone(m), err
}

func (s sdkPorts) CreateMilestone(ctx context.Context, owner, repo string, opt giteafamily.CreateMilestoneOption) (*giteafamily.Milestone, error) {
	_ = ctx // forgejo SDK takes no context
	m, _, err := s.c.CreateMilestone(owner, repo, forgejo.CreateMilestoneOption{
		Title:       opt.Title,
		Description: opt.Description,
		Deadline:    opt.Deadline,
	})
	return familyMilestone(m), err
}

func (s sdkPorts) EditMilestone(ctx context.Context, owner, repo string, id int64, opt giteafamily.EditMilestoneOption) (*giteafamily.Milestone, error) {
	_ = ctx // forgejo SDK takes no context
	editOpts := forgejo.EditMilestoneOption{
		Title:       opt.Title,
		Description: opt.Description,
		Deadline:    opt.Deadline,
	}
	if opt.State != nil {
		state := forgejo.StateType(*opt.State)
		editOpts.State = &state
	}
	m, _, err := s.c.EditMilestone(owner, repo, id, editOpts)
	return familyMilestone(m), err
}

func (s sdkPorts) DeleteMilestone(ctx context.Context, owner, repo string, id int64) error {
	_ = ctx // forgejo SDK takes no context
	_, err := s.c.DeleteMilestone(owner, repo, id)
	return err
}

func familyMilestone(m *forgejo.Milestone) *giteafamily.Milestone {
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

func familyMilestones(list []*forgejo.Milestone) []*giteafamily.Milestone {
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
