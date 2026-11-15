package provider

import (
	"context"

	"github.com/yi-nology/go-git-platform/model"
)

// CapabilitySet statically declares which optional capability interfaces a
// Provider implements. Values are compile-time constants per backend; no
// runtime probing is performed. Consumers should route on these flags
// instead of probing with type assertions:
//
//	if p.Capabilities().Labels {
//		lm := p.(provider.LabelManager)
//		// ...
//	}
//
// When a new optional capability interface is added to the SDK, add a field
// here and update every backend's Capabilities method; the contract suite
// enforces that declarations match implementations.
type CapabilitySet struct {
	Issues            bool // provider.IssueManager
	Search            bool // provider.SearchManager
	Labels            bool // provider.LabelManager
	Milestones        bool // provider.MilestoneManager
	Reviews           bool // provider.ReviewManager
	CommitStatuses    bool // provider.CommitStatusManager
	Notifications     bool // provider.NotificationManager
	Reactions         bool // provider.ReactionManager
	BranchProtections bool // provider.BranchProtectionManager
	Collaborators     bool // provider.CollaboratorManager
	WebhookConfig     bool // provider.WebhookConfigManager
	DeployKeys        bool // provider.DeploymentKeyManager
	RepoStats         bool // provider.RepoStatsManager
	Users             bool // provider.UserManager
	Gists             bool // provider.GistManager
	Starred           bool // provider.StarredManager
	Migrations        bool // provider.MigrationManager
	ReleaseAssets     bool // provider.ReleaseAssetManager
	CILogs            bool // provider.CILogManager
	Deployments       bool // provider.DeploymentManager
	RepoTemplates     bool // provider.TemplateRepoManager
	ForkSync          bool // provider.ForkSyncManager
	TeamReviews       bool // provider.TeamReviewManager
}

// Provider is the unified interface for all Git hosting platforms.
// It composes 8 focused sub-interfaces for high cohesion and low coupling.
//
// Consumers can depend on smaller interfaces (e.g., WebhookManager)
// when they don't need full Provider capabilities.
//
// IssueManager, SearchManager, and CommitStatusManager are NOT part of
// Provider: only some platforms support them. Consumers that need issues,
// search, or commit statuses should type-assert against the optional
// capability interfaces:
//
//	if ism, ok := p.(provider.IssueManager); ok { ... }
//	if sm, ok := p.(provider.SearchManager); ok { ... }
//	if csm, ok := p.(provider.CommitStatusManager); ok { ... }
//
//	Caps declares the same information programmatically: consumers can route
//	on p.Capabilities() instead of probing with type assertions.
type Provider interface {
	// Platform returns the platform type.
	Platform() model.Platform
	// TestConnection verifies the connection and checks capabilities.
	TestConnection(ctx context.Context) (*model.TestConnectionResult, error)
	// Capabilities statically declares the optional capability interfaces
	// this provider implements. See CapabilitySet.
	Capabilities() CapabilitySet
	// Divergences statically declares this backend's divergence ledger: the
	// registered places where its behavior departs from the unified
	// semantics (stub / ignore / mapping / detour). Consumers can route on
	// these entries — e.g. provider.Ignores — and docs/divergence-ledger.md
	// is generated from them. Like CapabilitySet this is a compile-time
	// declaration; the contract suite locks ledger entries to behavior.
	Divergences() []Divergence

	RepoManager
	ChangeRequestManager
	WebhookManager
	BranchManager
	DiffManager
	CommitManager
	FileManager
	ReleaseManager
}
