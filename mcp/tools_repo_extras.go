package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yi-nology/go-git-platform/provider"
)

// --- repo extras: deployments / templates / fork sync ---

type createDeploymentIn struct {
	Owner       string            `json:"owner"`
	Repo        string            `json:"repo"`
	Ref         string            `json:"ref" jsonschema:"branch, tag, or SHA to deploy"`
	Environment string            `json:"environment"`
	Task        string            `json:"task,omitempty"`
	Description string            `json:"description,omitempty"`
	Payload     map[string]string `json:"payload,omitempty"`
}

type deploymentStatusIn struct {
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	DeploymentID   int64  `json:"deployment_id"`
	State          string `json:"state" jsonschema:"pending|in_progress|queued|success|failure|error|canceled|inactive"`
	Description    string `json:"description,omitempty"`
	LogURL         string `json:"log_url,omitempty"`
	EnvironmentURL string `json:"environment_url,omitempty"`
}

type createRepoFromTemplateIn struct {
	TemplateOwner      string `json:"template_owner"`
	TemplateRepo       string `json:"template_repo"`
	Owner              string `json:"owner,omitempty" jsonschema:"target owner/org; empty = authenticated user"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	Private            bool   `json:"private,omitempty"`
	IncludeAllBranches bool   `json:"include_all_branches,omitempty"`
}

type syncForkIn struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

// registerRepoExtras mounts the deployment/template/fork-sync toolsets,
// each gated on its capability interface (mounting follows the same
// type-assertion mechanism as every other toolset).
func registerRepoExtras(s *mcp.Server, st *state) {
	if dm, ok := st.p.(provider.DeploymentManager); ok {
		add(s, st, "create_deployment", "Create deployment", "Request a platform deployment of ref to environment (GitHub deployments API).", true, func(ctx context.Context, in createDeploymentIn) (*provider.Deployment, error) {
			return dm.CreateDeployment(ctx, in.Owner, in.Repo, in.Ref, in.Environment, provider.CreateDeploymentOptions{
				Task: in.Task, Description: in.Description, Payload: in.Payload,
			})
		})
		add(s, st, "list_deployments", "List deployments", "List a repository's deployment records, newest first (optionally filtered by environment).", false, func(ctx context.Context, in struct {
			Owner       string `json:"owner"`
			Repo        string `json:"repo"`
			Environment string `json:"environment,omitempty"`
			Page        int    `json:"page,omitempty"`
			PerPage     int    `json:"per_page,omitempty"`
		}) ([]*provider.Deployment, error) {
			deps, _, err := dm.ListDeployments(ctx, in.Owner, in.Repo, in.Environment, perPage(in.Page), perPage(in.PerPage))
			return deps, err
		})
		add(s, st, "create_deployment_status", "Report deployment status", "Report state on a deployment (pending|in_progress|queued|success|failure|error|canceled|inactive).", true, func(ctx context.Context, in deploymentStatusIn) (map[string]any, error) {
			if !validDeploymentState(in.State) {
				return nil, fmt.Errorf("invalid state %q (pending|in_progress|queued|success|failure|error|canceled|inactive)", in.State)
			}
			err := dm.CreateDeploymentStatus(ctx, in.Owner, in.Repo, in.DeploymentID, provider.DeploymentStatusOptions{
				State: in.State, Description: in.Description,
				LogURL: in.LogURL, EnvironmentURL: in.EnvironmentURL,
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		})
	}
	if tm, ok := st.p.(provider.TemplateRepoManager); ok {
		add(s, st, "create_repo_from_template", "Create repo from template", "Instantiate a new repository from a platform template repository (GitHub / Gitea).", true, func(ctx context.Context, in createRepoFromTemplateIn) (*provider.PlatformRepo, error) {
			return tm.CreateRepoFromTemplate(ctx, in.TemplateOwner, in.TemplateRepo, provider.CreateRepoFromTemplateOptions{
				Owner: in.Owner, Name: in.Name, Description: in.Description,
				Private: in.Private, IncludeAllBranches: in.IncludeAllBranches,
			})
		})
	}
	if fm, ok := st.p.(provider.ForkSyncManager); ok {
		add(s, st, "sync_fork", "Sync fork", "Merge the upstream branch into this fork's branch of the same name (GitHub merge-upstream); conflicts error and stay for manual resolution.", true, func(ctx context.Context, in syncForkIn) (map[string]any, error) {
			err := fm.SyncFork(ctx, in.Owner, in.Repo, in.Branch)
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		})
	}
}

// validDeploymentState gates the write-side vocabulary at the tool boundary
// (same set as the backend's own guard).
func validDeploymentState(s string) bool {
	switch s {
	case "pending", "in_progress", "queued", "success", "failure", "error", "canceled", "inactive":
		return true
	}
	return false
}
