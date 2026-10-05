package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- crs: CR lifecycle writes ---

type createCRIn struct {
	Owner        string   `json:"owner"`
	Repo         string   `json:"repo"`
	Title        string   `json:"title"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	Description  string   `json:"description,omitempty"`
	Labels       []string `json:"labels,omitempty"`
}

type mergeCRIn struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number string `json:"number"`
	Squash bool   `json:"squash,omitempty" jsonschema:"squash commits into one on merge"`
}

type addCRCommentIn struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number string `json:"number"`
	Body   string `json:"body"`
}

func registerCRs(s *mcp.Server, st *state) {
	add(s, st, "create_cr", "Create change request", "Open a pull/merge request from source_branch into target_branch.", true, func(ctx context.Context, in createCRIn) (*provider.ChangeRequest, error) {
		return st.p.CreateCR(ctx, provider.CreateCROptions{
			Owner: in.Owner, Repo: in.Repo, Title: in.Title,
			Description: in.Description, SourceBranch: in.SourceBranch,
			TargetBranch: in.TargetBranch, Labels: in.Labels,
		})
	})

	add(s, st, "merge_cr", "Merge change request", "Merge a pull/merge request.", true, func(ctx context.Context, in mergeCRIn) (*provider.ChangeRequest, error) {
		return st.p.MergeCR(ctx, in.Owner, in.Repo, in.Number, provider.MergeCROptions{Squash: in.Squash})
	})

	add(s, st, "add_cr_comment", "Comment on change request", "Add a note/comment to a pull/merge request.", true, func(ctx context.Context, in addCRCommentIn) (map[string]any, error) {
		id, err := st.p.CreateNote(ctx, in.Owner, in.Repo, in.Number, in.Body)
		return map[string]any{"note_id": id}, err
	})
}
