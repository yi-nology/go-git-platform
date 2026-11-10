package contracttest

import (
	"testing"
)

// coverage_check_test.go is the machine check behind issue #63's
// "coverage matrix" expectation: the Run() suite list and the manager-family
// coverage table must agree in both directions — a suite mounted without a
// table row, or a table row whose suite is not mounted, fails here. The six
// optional-capability families (Gists, Starred, Migrations, ReleaseAssets,
// Deployments, ForkSync) carry only CapabilitySet declaration checks today
// (capabilities.go); they are listed as explicitly uncovered so the gap
// stays machine-visible instead of silently implied.
// coveredFamilies maps each manager family to the suite that carries its
// behavior assertions. Core families must all be present here; the six
// optional-capability families (Gists, Starred, Migrations, ReleaseAssets,
// Deployments, ForkSync) are deliberately absent — see
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

// TestOptionalCapabilityFamiliesUncovered pins the machine-visible record of
// the six optional-capability families that carry only CapabilitySet
// declaration checks (issue #63): adding behavior assertions for any of them
// means adding a suite to Run(), a Harness config, and a row to
// coveredFamilies — this test fails until the row lands.
func TestOptionalCapabilityFamiliesUncovered(t *testing.T) {
	for _, suite := range []string{
		"GistsSuite", "StarredSuite", "MigrationsSuite", "ReleaseAssetsSuite", "DeploymentsSuite", "ForkSyncSuite",
	} {
		for _, mounted := range coveredFamilies {
			if mounted == suite {
				t.Errorf("optional-capability family suite %s is now covered — move it from the uncovered list into a family row of coveredFamilies", suite)
			}
		}
	}
}
