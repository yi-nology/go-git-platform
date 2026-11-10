package giteafamily

import (
	"context"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

type fakeBPSection struct {
	createOpts FamilyCreateBranchProtectionOption
	editOpts   FamilyEditBranchProtectionOption
	readBack   *FamilyBranchProtection
}

type fakeBPPort struct {
	fakeBPSection
}

func (f *fakeBPPort) ListBranchProtections(context.Context, string, string, ListOptions) ([]*FamilyBranchProtection, error) {
	return nil, nil
}
func (f *fakeBPPort) GetBranchProtection(context.Context, string, string, string) (*FamilyBranchProtection, error) {
	return f.readBack, nil
}
func (f *fakeBPPort) CreateBranchProtection(_ context.Context, _ string, _ string, opt FamilyCreateBranchProtectionOption) (*FamilyBranchProtection, error) {
	f.createOpts = opt
	return f.readBack, nil
}
func (f *fakeBPPort) EditBranchProtection(_ context.Context, _ string, _ string, _ string, opt FamilyEditBranchProtectionOption) (*FamilyBranchProtection, error) {
	f.editOpts = opt
	return f.readBack, nil
}
func (f *fakeBPPort) DeleteBranchProtection(context.Context, string, string, string) error {
	return nil
}

// TestBranchProtection_AllowDeletionsRoundTrip pins the symmetric inverse
// mapping (issue #93): the family's delete toggle rides
// BlockOnRejectedReviews, and both directions must honor that — the old
// create/update dropped the write while the read inverted the wire value,
// so Create(AllowDeletions=false) read back true and
// EnsureBranchProtection compared requested=false vs current=true forever.
func TestBranchProtection_AllowDeletionsRoundTrip(t *testing.T) {
	cases := []struct {
		name            string
		requested       bool
		wantWireBlocked bool // BlockOnRejectedReviews on the wire
	}{
		{name: "AllowDeletions=false blocks on the wire", requested: false, wantWireBlocked: true},
		{name: "AllowDeletions=true unblocks on the wire", requested: true, wantWireBlocked: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fam := Family{Platform: provider.PlatformGitea}
			port := &fakeBPPort{
				fakeBPSection: fakeBPSection{
					// read-back mirrors what the platform reports after the
					// write: the inverse of the requested AllowDeletions.
					readBack: &FamilyBranchProtection{
						BranchName:             "main",
						BlockOnRejectedReviews: tc.wantWireBlocked,
					},
				},
			}

			created, err := CreateBranchProtection(context.Background(), fam, port, "o", "r",
				provider.CreateBranchProtectionOptions{BranchName: "main", AllowDeletions: tc.requested})
			if err != nil {
				t.Fatalf("CreateBranchProtection: %v", err)
			}
			if port.createOpts.BlockOnRejectedReviews != tc.wantWireBlocked {
				t.Errorf("create wire BlockOnRejectedReviews = %v, want %v",
					port.createOpts.BlockOnRejectedReviews, tc.wantWireBlocked)
			}
			if created.AllowDeletions != tc.requested {
				t.Errorf("create read-back AllowDeletions = %v, want %v (round trip broken)", created.AllowDeletions, tc.requested)
			}

			updated, err := UpdateBranchProtection(context.Background(), fam, port, "o", "r", "main",
				provider.UpdateBranchProtectionOptions{AllowDeletions: &tc.requested})
			if err != nil {
				t.Fatalf("UpdateBranchProtection: %v", err)
			}
			if port.editOpts.BlockOnRejectedReviews == nil || *port.editOpts.BlockOnRejectedReviews != tc.wantWireBlocked {
				t.Errorf("edit wire BlockOnRejectedReviews = %v, want &%v (write must land, not be dropped)",
					port.editOpts.BlockOnRejectedReviews, tc.wantWireBlocked)
			}
			if updated.AllowDeletions != tc.requested {
				t.Errorf("update read-back AllowDeletions = %v, want %v", updated.AllowDeletions, tc.requested)
			}
		})
	}
}

// TestBranchProtection_EnsureComparisonConverges locks the EnsureBranchProtection
// comparison face onto the fixed mapping: a desired AllowDeletions=false
// against a platform state whose wire reports BlockOnRejectedReviews=true
// must read back equal (EnsureUnchanged), not diverge forever.
func TestBranchProtection_EnsureComparisonConverges(t *testing.T) {
	fam := Family{Platform: provider.PlatformGitea}
	port := &fakeBPPort{
		fakeBPSection: fakeBPSection{
			readBack: &FamilyBranchProtection{
				BranchName:             "main",
				BlockOnRejectedReviews: true, // ← written by the fixed inverse mapping
			},
		},
	}
	cur, err := GetBranchProtection(context.Background(), fam, port, "o", "r", "main")
	if err != nil {
		t.Fatalf("GetBranchProtection: %v", err)
	}
	desired := provider.CreateBranchProtectionOptions{BranchName: "main", AllowDeletions: false}
	if cur.AllowDeletions != desired.AllowDeletions ||
		cur.RequiredApprovingReviews != desired.RequiredApprovingReviews ||
		cur.RequiredStatusChecks != desired.RequiredStatusChecks ||
		cur.AllowForcePushes != desired.AllowForcePushes {
		t.Fatalf("ensure comparison must converge (EnsureUnchanged shape), cur=%+v desired=%+v — this is the divergence EnsureBranchProtection used to loop on", cur, desired)
	}
}
