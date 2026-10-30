package contracttest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// testCapabilities asserts that a provider's declared CapabilitySet matches
// the optional interfaces its concrete type actually implements. A declared
// capability must type-assert successfully, and an undeclared one must not.
// This keeps backend declarations from drifting from their method sets as
// new capability interfaces land. When a new optional interface is added to
// the SDK (e.g. MilestoneManager, ReviewManager), extend the checks here.
func testCapabilities(t *testing.T, h Harness) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.EmptyListResponse))
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	caps := p.Capabilities()

	_, issuesImpl := p.(provider.IssueManager)
	_, searchImpl := p.(provider.SearchManager)
	_, labelsImpl := p.(provider.LabelManager)
	_, reviewsImpl := p.(provider.ReviewManager)
	_, milestonesImpl := p.(provider.MilestoneManager)
	_, commitStatusesImpl := p.(provider.CommitStatusManager)
	_, notificationsImpl := p.(provider.NotificationManager)
	_, reactionsImpl := p.(provider.ReactionManager)
	_, branchProtectionsImpl := p.(provider.BranchProtectionManager)
	_, collaboratorsImpl := p.(provider.CollaboratorManager)
	_, webhookConfigImpl := p.(provider.WebhookConfigManager)
	_, deployKeysImpl := p.(provider.DeploymentKeyManager)
	_, repoStatsImpl := p.(provider.RepoStatsManager)
	_, usersImpl := p.(provider.UserManager)
	_, gistsImpl := p.(provider.GistManager)
	_, starredImpl := p.(provider.StarredManager)
	_, migrationsImpl := p.(provider.MigrationManager)
	_, releaseAssetsImpl := p.(provider.ReleaseAssetManager)

	// Declared capabilities must always type-assert, and implemented ones
	// must be declared — both directions, uniformly across capabilities.
	assertCap(t, "Issues", "IssueManager", caps.Issues, issuesImpl)
	assertCap(t, "Search", "SearchManager", caps.Search, searchImpl)
	assertCap(t, "Labels", "LabelManager", caps.Labels, labelsImpl)
	assertCap(t, "Reviews", "ReviewManager", caps.Reviews, reviewsImpl)
	assertCap(t, "Milestones", "MilestoneManager", caps.Milestones, milestonesImpl)
	assertCap(t, "CommitStatuses", "CommitStatusManager", caps.CommitStatuses, commitStatusesImpl)
	assertCap(t, "Notifications", "NotificationManager", caps.Notifications, notificationsImpl)
	assertCap(t, "Reactions", "ReactionManager", caps.Reactions, reactionsImpl)
	assertCap(t, "BranchProtections", "BranchProtectionManager", caps.BranchProtections, branchProtectionsImpl)
	assertCap(t, "Collaborators", "CollaboratorManager", caps.Collaborators, collaboratorsImpl)
	assertCap(t, "WebhookConfig", "WebhookConfigManager", caps.WebhookConfig, webhookConfigImpl)
	assertCap(t, "DeployKeys", "DeploymentKeyManager", caps.DeployKeys, deployKeysImpl)
	assertCap(t, "RepoStats", "RepoStatsManager", caps.RepoStats, repoStatsImpl)
	assertCap(t, "Users", "UserManager", caps.Users, usersImpl)
	assertCap(t, "Gists", "GistManager", caps.Gists, gistsImpl)
	assertCap(t, "Starred", "StarredManager", caps.Starred, starredImpl)
	assertCap(t, "Migrations", "MigrationManager", caps.Migrations, migrationsImpl)
	assertCap(t, "ReleaseAssets", "ReleaseAssetManager", caps.ReleaseAssets, releaseAssetsImpl)
}

// assertCap fails the test when a declared capability and the concrete
// type's optional-interface implementation disagree in either direction.
func assertCap(t *testing.T, capName, ifaceName string, declared, implemented bool) {
	t.Helper()
	if declared != implemented {
		t.Errorf("Capabilities().%s = %v, but %s type assertion = %v; declaration and implementation have drifted",
			capName, declared, ifaceName, implemented)
	}
}
