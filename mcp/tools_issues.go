package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/pkg/projection"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- issues ---

type listIssuesIn struct {
	Owner   string   `json:"owner"`
	Repo    string   `json:"repo"`
	State   string   `json:"state,omitempty" jsonschema:"open|closed|all; empty = platform default"`
	Page    int      `json:"page,omitempty"`
	PerPage int      `json:"per_page,omitempty" jsonschema:"items per page (1-100); default 30"`
	Fields  []string `json:"fields,omitempty" jsonschema:"projection paths (e.g. [\"number\",\"title\"]) to trim each issue"`
}

type listIssuesOut struct {
	Total     int               `json:"total"`
	Issues    []*provider.Issue `json:"issues,omitempty"`
	Projected []map[string]any  `json:"projected,omitempty"`
}

type createIssueIn struct {
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

type addIssueCommentIn struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number string `json:"number"`
	Body   string `json:"body"`
}

type closeIssueIn struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number string `json:"number"`
}

// validIssueStates gates the list filter at the tool boundary so a typo
// surfaces as a readable tool error instead of a platform-side 4xx.
var validIssueStates = map[string]bool{
	"open": true, "closed": true, "all": true,
}

func registerIssues(s *mcp.Server, st *state) {
	im, ok := st.p.(provider.IssueManager)
	if !ok {
		return
	}

	add(s, st, "list_issues", "List issues", "List issues of a repository, optionally trimmed to the given fields.", false, func(ctx context.Context, in listIssuesIn) (listIssuesOut, error) {
		if in.State != "" && !validIssueStates[in.State] {
			return listIssuesOut{}, fmt.Errorf("invalid state %q (open|closed|all)", in.State)
		}
		opts := provider.ListIssuesOptions{Owner: in.Owner, Repo: in.Repo, Page: in.Page, PerPage: perPage(in.PerPage)}
		if in.State != "" {
			opts.State = provider.IssueState(in.State)
		}
		issues, total, err := im.ListIssues(ctx, opts)
		if err != nil {
			return listIssuesOut{}, err
		}
		if len(in.Fields) == 0 {
			return listIssuesOut{Total: total, Issues: issues}, nil
		}
		trimmed, err := projection.ProjectList(issues, in.Fields...)
		if err != nil {
			return listIssuesOut{}, err
		}
		return listIssuesOut{Total: total, Projected: trimmed}, nil
	})

	add(s, st, "get_issue", "Get issue", "Fetch one issue with its full metadata.", false, func(ctx context.Context, in getCRIn) (*provider.Issue, error) {
		return im.GetIssue(ctx, in.Owner, in.Repo, in.Number)
	})

	add(s, st, "create_issue", "Create issue", "Open a new issue.", true, func(ctx context.Context, in createIssueIn) (*provider.Issue, error) {
		return im.CreateIssue(ctx, provider.CreateIssueOptions{
			Owner: in.Owner, Repo: in.Repo, Title: in.Title, Body: in.Body, Labels: in.Labels,
		})
	})

	add(s, st, "add_issue_comment", "Comment on issue", "Add a comment to an issue.", true, func(ctx context.Context, in addIssueCommentIn) (*provider.IssueComment, error) {
		return im.CreateIssueComment(ctx, in.Owner, in.Repo, in.Number, in.Body)
	})

	add(s, st, "close_issue", "Close issue", "Close an issue.", true, func(ctx context.Context, in closeIssueIn) (*provider.Issue, error) {
		return im.CloseIssue(ctx, in.Owner, in.Repo, in.Number)
	})
}
