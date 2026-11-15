package model

// FileResult is the result of a file operation.
type FileResult struct {
	SHA       string `json:"sha,omitempty"`
	CommitSHA string `json:"commit_sha,omitempty"`
}
