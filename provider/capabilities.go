package provider

// DeriveCapabilities builds a CapabilitySet from the concrete provider's
// optional-manager implementations — the runtime type assertion is the SINGLE
// source of truth for what a backend supports. Backend Capabilities() methods
// delegate here, so the declaration can no longer drift from the
// implementation (contracttest's bidirectional check then guards against a
// backend bypassing the derivation with a hand-written literal).
//
// This collapses the historical dual channel: pre-0.82 each backend wrote a
// literal of 12–17 bools that consumers cross-checked against assertions.
func DeriveCapabilities(p Provider) CapabilitySet {
	_, issues := p.(IssueManager)
	_, search := p.(SearchManager)
	_, labels := p.(LabelManager)
	_, milestones := p.(MilestoneManager)
	_, reviews := p.(ReviewManager)
	_, commitStatuses := p.(CommitStatusManager)
	_, notifications := p.(NotificationManager)
	_, reactions := p.(ReactionManager)
	_, branchProtections := p.(BranchProtectionManager)
	_, collaborators := p.(CollaboratorManager)
	_, webhookConfig := p.(WebhookConfigManager)
	_, deployKeys := p.(DeploymentKeyManager)
	_, repoStats := p.(RepoStatsManager)
	_, users := p.(UserManager)
	_, gists := p.(GistManager)
	_, starred := p.(StarredManager)
	_, migrations := p.(MigrationManager)
	_, releaseAssets := p.(ReleaseAssetManager)
	return CapabilitySet{
		Issues:            issues,
		Search:            search,
		Labels:            labels,
		Milestones:        milestones,
		Reviews:           reviews,
		CommitStatuses:    commitStatuses,
		Notifications:     notifications,
		Reactions:         reactions,
		BranchProtections: branchProtections,
		Collaborators:     collaborators,
		WebhookConfig:     webhookConfig,
		DeployKeys:        deployKeys,
		RepoStats:         repoStats,
		Users:             users,
		Gists:             gists,
		Starred:           starred,
		Migrations:        migrations,
		ReleaseAssets:     releaseAssets,
	}
}
