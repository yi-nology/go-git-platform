package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/pkg/projection"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- core: repos, branches, files, CR reads ---

type ownerRepo struct {
	Owner string `json:"owner" jsonschema:"repository owner (organization or user)"`
	Repo  string `json:"repo" jsonschema:"repository name"`
}

type getRepoOut struct {
	Repo *provider.PlatformRepo `json:"repo"`
}

type listReposIn struct {
	Owner   string `json:"owner" jsonschema:"list repositories owned by this account"`
	Page    int    `json:"page,omitempty" jsonschema:"1-based page number"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"items per page (1-100); default 30"`
}

type getFileIn struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Path  string `json:"path" jsonschema:"file path inside the repository"`
	Ref   string `json:"ref,omitempty" jsonschema:"branch, tag or sha; empty = default branch"`
}

type getFileOut struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type listBranchesIn ownerRepo

type listBranchesOut struct {
	Branches []*provider.PlatformBranch `json:"branches"`
}

type listCRsIn struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	State   string `json:"state,omitempty" jsonschema:"open|closed|merged|all; empty = platform default"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"items per page (1-100); default 30"`
	// Fields trims every CR to the selected projection paths (e.g.
	// ["number","title","head.ref"]); empty returns full CRs. Use this
	// to keep large listings inside your context budget.
	Fields []string `json:"fields,omitempty"`
}

type listCRsOut struct {
	Total int                       `json:"total"`
	CRs   []*provider.ChangeRequest `json:"crs,omitempty"`
	// Projected carries the trimmed documents when fields was given.
	Projected []map[string]any `json:"projected,omitempty"`
}

// getCRIn addresses one change request (and doubles as the issue getter's
// input shape).
type getCRIn struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number string `json:"number" jsonschema:"change request number"`
}

type getCommitIn struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	SHA   string `json:"sha"`
}

type listCommitsIn struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	// Branch restricts the listing to one branch; empty = default branch.
	Branch  string `json:"branch,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"items per page (1-100); default 30"`
}

// validCRStates gates the list filter at the tool boundary so a typo
// surfaces as a readable tool error instead of a platform-side 4xx.
var validCRStates = map[string]bool{
	"open": true, "opened": true, "closed": true, "merged": true, "all": true,
}

func registerCore(s *mcp.Server, st *state) {
	add(s, st, "get_repo", "Get repository", "Fetch one repository's metadata.", false, func(ctx context.Context, in ownerRepo) (getRepoOut, error) {
		repo, err := st.p.GetRepo(ctx, in.Owner, in.Repo)
		return getRepoOut{Repo: repo}, err
	})

	add(s, st, "list_repos", "List repositories", "List the repositories owned by an account.", false, func(ctx context.Context, in listReposIn) ([]*provider.PlatformRepo, error) {
		return st.p.ListRepos(ctx, provider.ListRepoOptions{Owner: in.Owner, Page: in.Page, PerPage: perPage(in.PerPage)})
	})

	add(s, st, "get_file", "Get file content", "Read one file from the repository at a ref (empty = default branch).", false, func(ctx context.Context, in getFileIn) (getFileOut, error) {
		content, err := st.p.GetFileContent(ctx, in.Owner, in.Repo, in.Path, in.Ref)
		return getFileOut{Path: in.Path, Content: content}, err
	})

	add(s, st, "list_branches", "List branches", "List the branches of a repository.", false, func(ctx context.Context, in listBranchesIn) (listBranchesOut, error) {
		branches, err := st.p.ListBranches(ctx, in.Owner, in.Repo)
		return listBranchesOut{Branches: branches}, err
	})

	add(s, st, "list_crs", "List change requests", "List pull/merge requests of a repository, optionally trimmed to the given fields to save context.", false, func(ctx context.Context, in listCRsIn) (listCRsOut, error) {
		if in.State != "" && !validCRStates[in.State] {
			return listCRsOut{}, fmt.Errorf("invalid state %q (open|opened|closed|merged|all)", in.State)
		}
		opts := provider.ListCROptions{Owner: in.Owner, Repo: in.Repo, Page: in.Page, PerPage: perPage(in.PerPage)}
		if in.State != "" {
			opts.State = provider.CRState(in.State)
		}
		crs, total, err := st.p.ListCRs(ctx, opts)
		if err != nil {
			return listCRsOut{}, err
		}
		if len(in.Fields) == 0 {
			return listCRsOut{Total: total, CRs: crs}, nil
		}
		trimmed, err := projection.ProjectList(crs, in.Fields...)
		if err != nil {
			return listCRsOut{}, err
		}
		return listCRsOut{Total: total, Projected: trimmed}, nil
	})

	add(s, st, "get_cr", "Get change request", "Fetch one pull/merge request with its full metadata.", false, func(ctx context.Context, in getCRIn) (*provider.ChangeRequest, error) {
		return st.p.GetCR(ctx, in.Owner, in.Repo, in.Number)
	})

	add(s, st, "get_cr_files", "Get change request files", "List the files changed by a pull/merge request with patch text.", false, func(ctx context.Context, in getCRIn) ([]*provider.ChangedFile, error) {
		return st.p.GetCRFiles(ctx, in.Owner, in.Repo, in.Number)
	})

	add(s, st, "get_commit", "Get commit", "Fetch one commit's metadata, message, and files.", false, func(ctx context.Context, in getCommitIn) (*provider.CommitInfo, error) {
		return st.p.GetCommit(ctx, in.Owner, in.Repo, in.SHA)
	})

	add(s, st, "list_commits", "List commits", "List a repository's commits (newest first), optionally on one branch.", false, func(ctx context.Context, in listCommitsIn) ([]*provider.CommitInfo, error) {
		return st.p.ListCommits(ctx, in.Owner, in.Repo, provider.ListCommitsOptions{Branch: in.Branch, Page: in.Page, PerPage: perPage(in.PerPage)})
	})
}
