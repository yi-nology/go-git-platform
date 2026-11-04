package github

import (
	"context"
	"fmt"
	"net/http"

	sdkgithub "github.com/google/go-github/v93/github"

	"github.com/yi-nology/go-git-platform/provider"
)

// CreateDeployment implements provider.DeploymentManager (GitHub's
// deployments API is the only wire with a manual create + status state
// machine; GitLab's deployment records are CI-produced without a manual
// status wire, so absence is expressed by not declaring).
func (p *Provider) CreateDeployment(ctx context.Context, owner, repo, ref, environment string, opts provider.CreateDeploymentOptions) (*provider.Deployment, error) {
	var payload any
	if len(opts.Payload) > 0 {
		payload = opts.Payload
	}
	body := sdkgithub.DeploymentRequest{
		Ref:         ref,
		Environment: new(environment),
		Task:        new(opts.Task),
		Description: new(opts.Description),
		AutoMerge:   opts.AutoMerge,
		Payload:     payload,
	}
	if opts.RequiredContexts != nil {
		body.RequiredContexts = opts.RequiredContexts
	}
	dep, _, err := p.client.Repositories.CreateDeployment(ctx, owner, repo, body)
	if err != nil {
		return nil, provider.Wrap(provider.PlatformGitHub, "CreateDeployment", err)
	}
	return convertDeployment(dep), nil
}

// ListDeployments implements provider.DeploymentManager.
func (p *Provider) ListDeployments(ctx context.Context, owner, repo, environment string, page, perPage int) ([]*provider.Deployment, int, error) {
	opts := &sdkgithub.DeploymentsListOptions{
		Environment: environment,
		ListOptions: sdkgithub.ListOptions{Page: page, PerPage: perPage},
	}
	deps, resp, err := p.client.Repositories.ListDeployments(ctx, owner, repo, opts)
	if err != nil {
		return nil, 0, provider.Wrap(provider.PlatformGitHub, "ListDeployments", err)
	}
	out := make([]*provider.Deployment, 0, len(deps))
	for _, d := range deps {
		if d != nil {
			out = append(out, convertDeployment(d))
		}
	}
	total := 0
	if resp != nil {
		total = resp.LastPage
	}
	return out, total, nil
}

// CreateDeploymentStatus implements provider.DeploymentManager.
func (p *Provider) CreateDeploymentStatus(ctx context.Context, owner, repo string, deploymentID int64, opts provider.DeploymentStatusOptions) error {
	if !isValidDeploymentState(opts.State) {
		return provider.New(provider.PlatformGitHub, "CreateDeploymentStatus", http.StatusUnprocessableEntity,
			fmt.Sprintf("invalid state %q (pending|in_progress|queued|success|failure|error|canceled|inactive)", opts.State))
	}
	body := sdkgithub.DeploymentStatusRequest{
		State:          opts.State,
		Description:    new(opts.Description),
		LogURL:         new(opts.LogURL),
		EnvironmentURL: new(opts.EnvironmentURL),
		AutoInactive:   opts.AutoInactive,
	}
	if _, _, err := p.client.Repositories.CreateDeploymentStatus(ctx, owner, repo, deploymentID, body); err != nil {
		return provider.Wrap(provider.PlatformGitHub, "CreateDeploymentStatus", err)
	}
	return nil
}

// isValidDeploymentState gates the write-side vocabulary at the backend
// boundary so an invalid state fails with the unified 422 shape instead of
// an obscure platform 4xx.
func isValidDeploymentState(s string) bool {
	switch s {
	case "pending", "in_progress", "queued", "success", "failure", "error", "canceled", "inactive":
		return true
	}
	return false
}

// convertDeployment maps the SDK deployment onto the unified shape.
func convertDeployment(d *sdkgithub.Deployment) *provider.Deployment {
	if d == nil {
		return nil
	}
	return &provider.Deployment{
		ID:          d.GetID(),
		SHA:         d.GetSHA(),
		Ref:         d.GetRef(),
		Environment: d.GetEnvironment(),
		Description: d.GetDescription(),
		Creator:     d.GetCreator().GetLogin(),
		CreatedAt:   d.GetCreatedAt().Time,
		URL:         d.GetURL(),
	}
}
