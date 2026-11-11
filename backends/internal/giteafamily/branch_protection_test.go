package giteafamily

import (
	"context"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// fakeBPSection/fakeBPPort capture what the engine writes through the port so
// tests can pin the option mapping. The struct shape itself is half the
// assertion: FamilyCreate/EditBranchProtectionOption carry NO
// BlockOnRejectedReviews/EnablePush fields at all (issue #130 removed them),
// so the old inverse mapping cannot even compile back in.
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

// TestBranchProtection_AllowForcePushesPortsOntoWireField pins the issue #130
// rework of the #93 mapping: AllowForcePushes flows into the family option's
// ForcePushes (each port decides whether its SDK has a real wire field —
// gitea maps enable_force_push, forgejo registers an ignore), and
// AllowDeletions is simply absent: the family wire has no deletions face, so
// nothing is written and reads report false instead of inverting an unrelated
// switch (the old mapping flipped block_on_rejected_reviews).
func TestBranchProtection_AllowForcePushesPortsOntoWireField(t *testing.T) {
	fam := Family{Platform: provider.PlatformGitea}
	port := &fakeBPPort{
		fakeBPSection: fakeBPSection{
			readBack: &FamilyBranchProtection{BranchName: "main"},
		},
	}

	created, err := CreateBranchProtection(context.Background(), fam, port, "o", "r",
		provider.CreateBranchProtectionOptions{BranchName: "main", AllowForcePushes: true, AllowDeletions: true})
	if err != nil {
		t.Fatalf("CreateBranchProtection: %v", err)
	}
	if !port.createOpts.ForcePushes {
		t.Errorf("create must carry AllowForcePushes onto the family ForcePushes field")
	}
	if created.AllowForcePushes {
		t.Errorf("read-back AllowForcePushes must come from the wire value (fixture says false), not echo the request")
	}
	if created.AllowDeletions {
		t.Errorf("read-back AllowDeletions must be false: the family wire has no deletions face (issue #130)")
	}

	on := true
	updated, err := UpdateBranchProtection(context.Background(), fam, port, "o", "r", "main",
		provider.UpdateBranchProtectionOptions{AllowForcePushes: &on})
	if err != nil {
		t.Fatalf("UpdateBranchProtection: %v", err)
	}
	if port.editOpts.ForcePushes == nil || !*port.editOpts.ForcePushes {
		t.Errorf("edit must pass AllowForcePushes through as ForcePushes, got %+v", port.editOpts.ForcePushes)
	}
	if updated.AllowDeletions {
		t.Errorf("update read-back AllowDeletions must be false (issue #130)")
	}
}

// TestBranchProtection_UpdateWithoutForcePushesLeavesToggleNil pins the
// edit-body shape: an update that does not mention AllowForcePushes must
// leave the ForcePushes pointer nil (untouched on the wire), and an update
// that sets ONLY AllowDeletions must not touch anything force-push related —
// the unrelated-switch substitution is gone.
func TestBranchProtection_UpdateWithoutForcePushesLeavesToggleNil(t *testing.T) {
	fam := Family{Platform: provider.PlatformForgejo}
	port := &fakeBPPort{fakeBPSection: fakeBPSection{readBack: &FamilyBranchProtection{BranchName: "main"}}}

	off := false
	if _, err := UpdateBranchProtection(context.Background(), fam, port, "o", "r", "main",
		provider.UpdateBranchProtectionOptions{AllowDeletions: &off}); err != nil {
		t.Fatalf("UpdateBranchProtection: %v", err)
	}
	if port.editOpts.ForcePushes != nil {
		t.Errorf("AllowDeletions-only update must not touch the force-push toggle, got %+v", port.editOpts.ForcePushes)
	}
	if port.editOpts.RequiredApprovals != nil || port.editOpts.EnableStatusCheck != nil {
		t.Errorf("unrelated fields must stay nil, got %+v", port.editOpts)
	}
}

// TestBranchProtection_ReadSideMapsWireValues pins the read conversion: the
// wire's ForcePushes (gitea's enable_force_push) maps to AllowForcePushes,
// and RequiredApprovals/EnableStatusCheck keep their verbatim mapping.
func TestBranchProtection_ReadSideMapsWireValues(t *testing.T) {
	fam := Family{Platform: provider.PlatformGitea}
	port := &fakeBPPort{fakeBPSection: fakeBPSection{readBack: &FamilyBranchProtection{
		BranchName:        "main",
		RequiredApprovals: 2,
		EnableStatusCheck: true,
		ForcePushes:       true,
	}}}
	cur, err := GetBranchProtection(context.Background(), fam, port, "o", "r", "main")
	if err != nil {
		t.Fatalf("GetBranchProtection: %v", err)
	}
	if cur.AllowForcePushes != true || cur.RequiredApprovingReviews != 2 || cur.RequiredStatusChecks != true {
		t.Fatalf("read conversion drifted: %+v", cur)
	}
	if cur.AllowDeletions {
		t.Errorf("AllowDeletions must read false (no wire face), got true")
	}
}
