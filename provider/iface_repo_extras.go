package provider

import "context"

// DeploymentManager reports and drives a platform-native deployment
// pipeline. Optional capability interface: check
// Capabilities().Deployments (or type-assert) before use. Currently
// GitHub-only — its deployments API is the only wire with a manual
// create-deployment + deployment-status state machine (GitLab's deployment
// records are CI-produced and carry no manual status wire).
type DeploymentManager interface {
	// CreateDeployment requests a deployment of ref to environment.
	CreateDeployment(ctx context.Context, owner, repo, ref, environment string, opts CreateDeploymentOptions) (*Deployment, error)
	// ListDeployments lists a repository's deployment records, newest first,
	// optionally filtered by environment. The int return is the number of
	// items on the returned page (the IssueManager total convention); the
	// platform serves no repository-wide total on this endpoint.
	ListDeployments(ctx context.Context, owner, repo string, environment string, page, perPage int) ([]*Deployment, int, error)
	// CreateDeploymentStatus reports state on a deployment.
	CreateDeploymentStatus(ctx context.Context, owner, repo string, deploymentID int64, opts DeploymentStatusOptions) error
}

// CreateDeploymentOptions carries the optional fields of a deployment
// request. Ref and Environment are positional.
type CreateDeploymentOptions struct {
	// Task is the deployment task name ("deploy" default on the wire).
	Task string `json:"task,omitempty"`
	// Description is the payload description.
	Description string `json:"description,omitempty"`
	// AutoMerge / RequiredContexts / Payload fields are GitHub-specific;
	// leave zero to take the platform defaults. RequiredContexts: nil = not
	// sent, non-nil (even empty) = replace the required-context set.
	AutoMerge        *bool             `json:"auto_merge,omitempty"`
	RequiredContexts []string          `json:"required_contexts,omitempty"`
	Payload          map[string]string `json:"payload,omitempty"`
}

// DeploymentStatusOptions carries one deployment state report.
type DeploymentStatusOptions struct {
	// State is required: pending|in_progress|queued|success|failure|error|
	// canceled|inactive (GitHub deployment-status vocabulary).
	State string `json:"state"`
	// Description is a short status description.
	Description string `json:"description,omitempty"`
	// LogURL is the link to the deployment output.
	LogURL string `json:"log_url,omitempty"`
	// EnvironmentURL is the URL exposed to users when the deployment
	// succeeded.
	EnvironmentURL string `json:"environment_url,omitempty"`
	// AutoInactive marks earlier "in_progress" environments inactive when
	// this one succeeds (GitHub semantics; nil = platform default).
	AutoInactive *bool `json:"auto_inactive,omitempty"`
}

// TemplateRepoManager instantiates a repository from a platform template
// repository. Optional capability interface: check
// Capabilities().RepoTemplates before use (GitHub + Gitea).
type TemplateRepoManager interface {
	// CreateRepoFromTemplate generates a new repository owned by
	// opts.Owner (empty = the authenticated user) from the template
	// templateOwner/templateRepo.
	CreateRepoFromTemplate(ctx context.Context, templateOwner, templateRepo string, opts CreateRepoFromTemplateOptions) (*PlatformRepo, error)
}

// CreateRepoFromTemplateOptions carries the new repository's fields.
type CreateRepoFromTemplateOptions struct {
	// Owner is the target owner/org login; empty = authenticated user.
	Owner string `json:"owner,omitempty"`
	// Name is the new repository's name (required).
	Name string `json:"name"`
	// Description of the new repository.
	Description string `json:"description,omitempty"`
	// Private controls visibility.
	Private bool `json:"private,omitempty"`
	// IncludeAllBranches copies every branch instead of the default branch
	// alone (GitHub wire; Gitea takes the platform default).
	IncludeAllBranches bool `json:"include_all_branches,omitempty"`
}

// ForkSyncManager syncs a fork's branch with its upstream counterpart.
// Optional capability interface: check Capabilities().ForkSync before use
// (GitHub merge-upstream).
type ForkSyncManager interface {
	// SyncFork merges the upstream branch into the fork's branch of the
	// same name; conflicts are NOT resolved server-side — the call errors
	// and leaves the conflict for manual resolution.
	SyncFork(ctx context.Context, owner, repo, branch string) error
}
