package provider

import "time"

// Deployment is one record in a platform's deployment history.
type Deployment struct {
	ID          int64               `json:"id"`
	SHA         string              `json:"sha,omitempty"`
	Ref         string              `json:"ref,omitempty"`
	Environment string              `json:"environment,omitempty"`
	Description string              `json:"description,omitempty"`
	Creator     string              `json:"creator,omitempty"`
	CreatedAt   time.Time           `json:"created_at,omitempty"`
	URL         string              `json:"url,omitempty"`
	Statuses    []*DeploymentStatus `json:"statuses,omitempty"`
}

// DeploymentStatus is one state report on a deployment (the state machine
// vocabulary is the GitHub deployments one: pending → in_progress/queued →
// success/failure/error/canceled/inactive).
type DeploymentStatus struct {
	State          string    `json:"state"`
	Description    string    `json:"description,omitempty"`
	LogURL         string    `json:"log_url,omitempty"`
	EnvironmentURL string    `json:"environment_url,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
}
