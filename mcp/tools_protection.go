package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- branch protection (toolset "protection") ---

type branchNameIn struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

type ensureProtectionIn struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	// RequiredApprovingReviews: platform note — on TencentCode this maps
	// from/to the platform's approver_rule and cannot be written via the
	// protect PUT (desired states must mirror the platform-reported value
	// to converge; see the divergence ledger).
	RequiredApprovingReviews int  `json:"required_approving_reviews,omitempty"`
	RequiredStatusChecks     bool `json:"required_status_checks,omitempty"`
	AllowForcePushes         bool `json:"allow_force_pushes,omitempty"`
	AllowDeletions           bool `json:"allow_deletions,omitempty"`
}

func registerProtection(s *mcp.Server, st *state) {
	bpm, ok := st.p.(provider.BranchProtectionManager)
	if !ok {
		return
	}

	add(s, st, "list_branch_protections", "List branch protections", "List the branch protection rules of a repository.", false, func(ctx context.Context, in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}) ([]*provider.BranchProtection, error) {
		return bpm.ListBranchProtections(ctx, in.Owner, in.Repo)
	})

	add(s, st, "get_branch_protection", "Get branch protection", "Fetch the protection rule of one branch (missing rule = empty result, not an error).", false, func(ctx context.Context, in branchNameIn) (*provider.BranchProtection, error) {
		bp, err := bpm.GetBranchProtection(ctx, in.Owner, in.Repo, in.Branch)
		if err != nil && provider.IsNotFound(err) {
			return nil, nil
		}
		return bp, err
	})

	add(s, st, "ensure_branch_protection", "Ensure branch protection", "Converge a branch's protection to the desired rule: create it when missing, update when diverging, leave untouched when equal. Idempotent — safe to re-run.", true, func(ctx context.Context, in ensureProtectionIn) (map[string]any, error) {
		action, bp, err := provider.EnsureBranchProtection(ctx, bpm, in.Owner, in.Repo,
			provider.CreateBranchProtectionOptions{
				BranchName:               in.Branch,
				RequiredApprovingReviews: in.RequiredApprovingReviews,
				RequiredStatusChecks:     in.RequiredStatusChecks,
				AllowForcePushes:         in.AllowForcePushes,
				AllowDeletions:           in.AllowDeletions,
			})
		if err != nil {
			return nil, err
		}
		return map[string]any{"action": string(action), "rule": bp}, nil
	})

	add(s, st, "delete_branch_protection", "Delete branch protection", "Remove the protection rule from a branch.", true, func(ctx context.Context, in branchNameIn) (map[string]any, error) {
		if err := bpm.DeleteBranchProtection(ctx, in.Owner, in.Repo, in.Branch); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})
}
