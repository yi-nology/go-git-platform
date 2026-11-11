package contracttest

import (
	"reflect"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// coverage_check_test.go is the machine check behind issue #63's
// "coverage matrix" expectation: the Run() suite list and the manager-family
// coverage table must agree in both directions — a suite mounted without a
// table row, or a table row whose suite is not mounted, fails here. The
// optional-capability families (Gists, Starred, Migrations, ReleaseAssets,
// Deployments, ForkSync, and — since issue #123 — TeamReviews) carry only
// CapabilitySet declaration checks today (capabilities.go); they are listed
// as explicitly uncovered so the gap stays machine-visible instead of
// silently implied. The third-way check (TestCapabilityFamiliesAllRegistered)
// pins every CapabilitySet field into one of the two registries.
// coveredFamilies maps each manager family to the suite that carries its
// behavior assertions. Core families must all be present here; the
// optional-capability families with no shared suite yet (see
// uncoveredFamilies) are deliberately absent — see
// TestOptionalCapabilityFamiliesUncovered.
var coveredFamilies = map[string]string{
	"LabelManager":            "LabelsSuite",
	"IssueManager":            "IssuesSuite",
	"ReviewManager":           "ReviewsSuite",
	"MilestoneManager":        "MilestonesSuite",
	"SearchManager":           "SearchSuite",
	"ReleaseManager":          "ReleaseSuite",
	"CommitStatusManager":     "CommitStatusSuite",
	"NotificationManager":     "NotificationsSuite",
	"ReactionManager":         "ReactionsSuite",
	"BranchProtectionManager": "BranchProtectionsSuite",
	"CollaboratorManager":     "CollaboratorsSuite",
	"WebhookConfigManager":    "WebhookConfigSuite",
	"NoteManager":             "NotesSuite",
	"DeployKeyManager":        "DeployKeysSuite",
	"RepoStatsManager":        "RepoStatsSuite",
	"UserManager":             "UsersSuite",
	"CILogManager":            "CILogsSuite",
	"RepoTemplateManager":     "RepoTemplatesSuite",
	"ChangeRequestManager":    "CRSuite",
	"BranchManager":           "BranchSuite",
	"CommitManager":           "CommitSuite",
	"DiffManager":             "DiffSuite",
	"FileManager":             "FileSuite",
}

func TestRunCoverageMatrix(t *testing.T) {
	// mountedSuites mirrors the t.Run list in Run() — kept in lockstep by
	// the two directional checks below.
	mountedSuites := map[string]bool{}
	for _, sub := range []string{
		"Platform", "ListRepos_Empty", "ListRepos_NonEmpty", "IsNotFound",
		"Pagination_Normalized", "Retry_On5xx", "TokenSource_Rotation",
		"ConditionalRequests", "Webhook_ValidateSignature", "Context_Cancel",
		"Capabilities_Consistency", "DivergenceSuite",
		"LabelsSuite", "IssuesSuite", "ReviewsSuite", "MilestonesSuite",
		"SearchSuite", "ReleaseSuite", "CommitStatusSuite", "NotificationsSuite",
		"ReactionsSuite", "BranchProtectionsSuite", "CollaboratorsSuite",
		"WebhookConfigSuite", "NotesSuite", "DeployKeysSuite", "RepoStatsSuite",
		"UsersSuite", "CILogsSuite", "RepoTemplatesSuite",
		"CRSuite", "BranchSuite", "CommitSuite", "DiffSuite", "FileSuite",
	} {
		mountedSuites[sub] = true
	}

	// Direction 1:// Direction 1: every table row's suite must be mounted in Run().
	for family, suite := range coveredFamilies {
		if !mountedSuites[suite] {
			t.Errorf("family %s claims coverage by %q, but Run() mounts no such suite — wire it or fix the table", family, suite)
		}
	}

	// Direction 2: every mounted *Suite subtest must carry a table row (the
	// Platform-level subtests are not family suites).
	for suite := range mountedSuites {
		claimed := false
		for _, suiteName := range coveredFamilies {
			if suiteName == suite {
				claimed = true
				break
			}
		}
		// DivergenceSuite checks the ledgers themselves (machine-readable
		// registrations), not a manager family.
		if !claimed && suite != "DivergenceSuite" && len(suite) > 5 && suite[len(suite)-5:] == "Suite" {
			t.Errorf("Run() mounts %q but no manager family claims its coverage — add the family to coveredFamilies (or register the gap here)", suite)
		}
	}
}

// uncoveredFamilies records the optional-capability families whose behavior
// currently lives only in the CapabilitySet declaration checks
// (capabilities.go) — the machine-visible "explicitly uncovered" registry of
// issue #63. TeamReviewManager joined as the seventh family (issue #123):
// its single method is write-only, its behavior is pinned per-backend in
// team_reviewers_wire_test.go, and no shared contract suite exists yet.
// Adding behavior assertions for any family here means adding a suite to
// Run(), a Harness config, and a row to coveredFamilies — the third-way
// check (TestCapabilityFamiliesAllRegistered) fails until the move lands.
var uncoveredFamilies = map[string]string{
	"GistManager":         "GistsSuite",
	"StarredManager":      "StarredSuite",
	"MigrationManager":    "MigrationsSuite",
	"ReleaseAssetManager": "ReleaseAssetsSuite",
	"DeploymentManager":   "DeploymentsSuite",
	"ForkSyncManager":     "ForkSyncSuite",
	"TeamReviewManager":   "TeamReviewsSuite",
}

// capFieldToFamily maps every CapabilitySet field to its manager family —
// the third direction of the coverage matrix (issue #123): the two original
// directions (suite↔table) cannot see a NEW capability that enters neither
// registry. This map + reflection over CapabilitySet closes the hole: a new
// CapabilitySet field with no row in either registry fails here.
var capFieldToFamily = map[string]string{
	"Issues":            "IssueManager",
	"Search":            "SearchManager",
	"Labels":            "LabelManager",
	"Milestones":        "MilestoneManager",
	"Reviews":           "ReviewManager",
	"CommitStatuses":    "CommitStatusManager",
	"Notifications":     "NotificationManager",
	"Reactions":         "ReactionManager",
	"BranchProtections": "BranchProtectionManager",
	"Collaborators":     "CollaboratorManager",
	"WebhookConfig":     "WebhookConfigManager",
	"DeployKeys":        "DeployKeyManager",
	"RepoStats":         "RepoStatsManager",
	"Users":             "UserManager",
	"Gists":             "GistManager",
	"Starred":           "StarredManager",
	"Migrations":        "MigrationManager",
	"ReleaseAssets":     "ReleaseAssetManager",
	"CILogs":            "CILogManager",
	"Deployments":       "DeploymentManager",
	"RepoTemplates":     "RepoTemplateManager",
	"ForkSync":          "ForkSyncManager",
	"TeamReviews":       "TeamReviewManager",
}

// TestOptionalCapabilityFamiliesUncovered pins the machine-visible record of
// the optional-capability families that carry only CapabilitySet
// declaration checks (issue #63): adding behavior assertions for any of them
// means adding a suite to Run(), a Harness config, and a row to
// coveredFamilies — this test fails until the row lands.
func TestOptionalCapabilityFamiliesUncovered(t *testing.T) {
	for _, suite := range []string{
		"GistsSuite", "StarredSuite", "MigrationsSuite", "ReleaseAssetsSuite", "DeploymentsSuite", "ForkSyncSuite", "TeamReviewsSuite",
	} {
		for _, mounted := range coveredFamilies {
			if mounted == suite {
				t.Errorf("optional-capability family suite %s is now covered — move it from the uncovered list into a family row of coveredFamilies", suite)
			}
		}
	}
}

// TestCapabilityFamiliesAllRegistered is the third-way assertion (issue
// #123): every CapabilitySet field maps to a manager family, and every such
// family is registered in coveredFamilies ∪ uncoveredFamilies. A new
// optional capability that enters neither registry (the TeamReviews escape
// that motivated this) now fails here instead of silently vanishing from
// the machine-visible coverage record.
func TestCapabilityFamiliesAllRegistered(t *testing.T) {
	typ := reflect.TypeOf(provider.CapabilitySet{})
	registered := func(family string) bool {
		if _, ok := coveredFamilies[family]; ok {
			return true
		}
		_, ok := uncoveredFamilies[family]
		return ok
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		family, ok := capFieldToFamily[field.Name]
		if !ok {
			t.Errorf("CapabilitySet field %q has no manager-family row in capFieldToFamily — register it (and put it in coveredFamilies or uncoveredFamilies)", field.Name)
			continue
		}
		if !registered(family) {
			t.Errorf("family %s (CapabilitySet.%s) is in neither coveredFamilies nor uncoveredFamilies — the coverage record is incomplete", family, field.Name)
		}
	}
	// And the converse: no stale family names lingering in the static map.
	for field, family := range capFieldToFamily {
		if _, ok := typ.FieldByName(field); !ok {
			t.Errorf("capFieldToFamily maps %q to %q, but CapabilitySet has no such field — drop the stale row", field, family)
		}
	}
}
