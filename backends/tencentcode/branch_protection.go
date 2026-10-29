package tencentcode

import (
	"context"
	"fmt"
	"net/http"

	gongfeng "github.com/yi-nology/go-git-platform/backends/tencentcode/internal/gongfeng"

	"github.com/yi-nology/go-git-platform/backends/internal/backendutil"
	"github.com/yi-nology/go-git-platform/provider"
)

// ListBranchProtections implements provider.BranchProtectionManager via the
// registered detour: 工蜂 exposes no list-protected-branches endpoint, so
// the branch list is walked and its protected rows returned as summaries.
// The row payload carries no approval-rule fields — approver rules surface
// through GetBranchProtection.
func (p *Provider) ListBranchProtections(ctx context.Context, owner, repo string) ([]*provider.BranchProtection, error) {
	pid := owner + "/" + repo
	branches, err := backendutil.AllPages(func(page int) ([]*gongfeng.Branch, error) {
		list, _, err := p.client.Branches.ListBranches(ctx, pid,
			&gongfeng.ListBranchesOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
		return list, err
	})
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "ListBranchProtections", err)
	}
	result := make([]*provider.BranchProtection, 0)
	for _, b := range branches {
		if b != nil && b.Protected {
			result = append(result, &provider.BranchProtection{BranchName: b.Name})
		}
	}
	return result, nil
}

// GetBranchProtection implements provider.BranchProtectionManager.
func (p *Provider) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*provider.BranchProtection, error) {
	pid := owner + "/" + repo
	pb, _, err := p.client.Branches.GetProtectedBranch(ctx, pid, branch)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformTencentCode, "GetBranchProtection", err)
	}
	if pb == nil || !pb.Protected {
		return nil, provider.Wrapf(provider.PlatformTencentCode, "GetBranchProtection",
			"branch protection %q not found", branch)
	}
	return convertProtectedBranch(pb), nil
}

// CreateBranchProtection implements provider.BranchProtectionManager. The
// platform's protect PUT is an upsert carrying only access levels and the
// force-push flag (registered ignores for the rest of the contract
// options); the PUT response is the protection detail and is converted
// directly — no read-back round trip.
func (p *Provider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*provider.BranchProtection, error) {
	putOpts := &ProtectBranchOptions{AllowForcePush: opts.AllowForcePushes}
	pb, err := p.putBranchProtection(ctx, "CreateBranchProtection", owner, repo, opts.BranchName, putOpts)
	if err != nil {
		return nil, err
	}
	return convertProtectedBranch(pb), nil
}

// UpdateBranchProtection implements provider.BranchProtectionManager via the
// same protect PUT (upsert). Nil fields stay off the wire so they are left
// unchanged; AllowForcePushes is write-only on this platform — a successful
// update cannot be read back through the detail payload.
func (p *Provider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*provider.BranchProtection, error) {
	putOpts := &ProtectBranchOptions{}
	if opts.AllowForcePushes != nil {
		putOpts.AllowForcePush = *opts.AllowForcePushes
	}
	pb, err := p.putBranchProtection(ctx, "UpdateBranchProtection", owner, repo, branch, putOpts)
	if err != nil {
		return nil, err
	}
	return convertProtectedBranch(pb), nil
}

// DeleteBranchProtection implements provider.BranchProtectionManager. The
// platform's unprotect endpoint is a PUT, not a DELETE.
func (p *Provider) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	return p.doRequest(ctx, "DeleteBranchProtection", http.MethodPut,
		fmt.Sprintf("projects/%s/repository/branches/%s/unprotect", esc(pid(owner, repo)), esc(branch)),
		nil, nil)
}

// putBranchProtection PUTs the protect endpoint and decodes the protection
// detail from the response.
func (p *Provider) putBranchProtection(ctx context.Context, op, owner, repo, branch string, putOpts *ProtectBranchOptions) (*gongfeng.ProtectedBranch, error) {
	var pb gongfeng.ProtectedBranch
	err := p.doRequest(ctx, op, http.MethodPut,
		fmt.Sprintf("projects/%s/repository/branches/%s/protect", esc(pid(owner, repo)), esc(branch)),
		putOpts, &pb)
	if err != nil {
		return nil, err
	}
	return &pb, nil
}

// convertProtectedBranch maps a gongfeng.ProtectedBranch to the provider
// rule. ApproverRule is the platform's review gate (-1 = all members must
// approve, 1 = single, N = N approvals — registered mapping); the payload
// exposes no status-check or deletion flags, and allow_force_push is
// write-only, so those fields read back as zero values.
func convertProtectedBranch(pb *gongfeng.ProtectedBranch) *provider.BranchProtection {
	return &provider.BranchProtection{
		BranchName:               pb.Name,
		RequiredApprovingReviews: pb.ApproverRule,
	}
}

var _ provider.BranchProtectionManager = (*Provider)(nil)
