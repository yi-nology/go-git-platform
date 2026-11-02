package gitcode_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// Both tests pin the live-verified absence (probe round 2026-11-03, real
// commits/issues against api.gitcode.com — see gitcode_api
// docs/wire-provenance.md) of two routes the backend once served:
//
//   - /repos/{o}/{r}/archive[/...] (all three shapes) — GetArchive stays
//     on ReleaseManager (the interface couples it to the live releases
//     family) but fails with ErrNotImplemented.
//   - /repos/{o}/{r}/statuses/{sha} — the CommitStatusManager methods are
//     removed entirely, so the capability is no longer advertised and a
//     direct type assertion must fail.

func TestGitCode_GetArchive_NotImplemented(t *testing.T) {
	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitCode, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	rm := p.(provider.ReleaseManager)

	got, err := rm.GetArchive(context.Background(), "owner", "repo", "v1.0.0", "tar.gz")
	if err == nil {
		t.Fatalf("GetArchive returned %d bytes; GitCode serves no archive endpoint", len(got))
	}
	if !errors.Is(err, provider.ErrNotImplemented) {
		t.Fatalf("err = %v, want the ErrNotImplemented sentinel", err)
	}
}

func TestGitCode_CommitStatusCapability_NotAdvertised(t *testing.T) {
	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitCode, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Capabilities().CommitStatuses {
		t.Error("Capabilities().CommitStatuses = true; the statuses route 404s live and must not be advertised")
	}
	if _, ok := p.(provider.CommitStatusManager); ok {
		t.Error("gitcode still implements CommitStatusManager; the capability was removed with the live-verified dead route")
	}
}

func TestGitCode_ReactionAndDeployKeyCapabilities_NotAdvertised(t *testing.T) {
	p, err := provider.NewProvider(provider.Config{Platform: provider.PlatformGitCode, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, ok := p.(provider.ReactionManager); ok {
		t.Error("gitcode still implements ReactionManager; the reactions routes 404 live")
	}
	if _, ok := p.(interface {
		ListDeployKeys(ctx context.Context, owner, repo string) ([]*provider.DeployKey, error)
	}); ok {
		t.Error("gitcode still implements deploy keys; the /keys route 404s live")
	}
}
