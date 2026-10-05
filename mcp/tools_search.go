package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// --- search ---

type searchReposIn struct {
	Query string `json:"query" jsonschema:"platform search query"`
	Page  int    `json:"page,omitempty"`
}

type searchReposOut struct {
	Total int                          `json:"total"`
	Repos []*provider.SearchRepoResult `json:"repos"`
}

type searchIssuesIn struct {
	Query string `json:"query" jsonschema:"platform search query"`
	Page  int    `json:"page,omitempty"`
}

type searchIssuesOut struct {
	Total  int                           `json:"total"`
	Issues []*provider.SearchIssueResult `json:"issues"`
}

type searchUsersIn struct {
	Query string `json:"query" jsonschema:"platform search query"`
	Page  int    `json:"page,omitempty"`
}

type searchUsersOut struct {
	Total int                          `json:"total"`
	Users []*provider.SearchUserResult `json:"users"`
}

func registerSearch(s *mcp.Server, st *state) {
	sm, ok := st.p.(provider.SearchManager)
	if !ok {
		return
	}

	add(s, st, "search_repositories", "Search repositories", "Search repositories by query.", false, func(ctx context.Context, in searchReposIn) (searchReposOut, error) {
		repos, total, err := sm.SearchRepos(ctx, provider.SearchReposOptions{Query: in.Query, Page: in.Page})
		if err != nil {
			return searchReposOut{}, err
		}
		t := 0
		if total != nil {
			t = *total
		}
		return searchReposOut{Total: t, Repos: repos}, nil
	})

	add(s, st, "search_issues", "Search issues", "Search issues and pull/merge requests across repositories by query.", false, func(ctx context.Context, in searchIssuesIn) (searchIssuesOut, error) {
		issues, total, err := sm.SearchIssues(ctx, provider.SearchIssuesOptions{Query: in.Query, Page: in.Page})
		if err != nil {
			return searchIssuesOut{}, err
		}
		t := 0
		if total != nil {
			t = *total
		}
		return searchIssuesOut{Total: t, Issues: issues}, nil
	})

	add(s, st, "search_users", "Search users", "Search users and organizations by query.", false, func(ctx context.Context, in searchUsersIn) (searchUsersOut, error) {
		users, total, err := sm.SearchUsers(ctx, provider.SearchUsersOptions{Query: in.Query, Page: in.Page})
		if err != nil {
			return searchUsersOut{}, err
		}
		t := 0
		if total != nil {
			t = *total
		}
		return searchUsersOut{Total: t, Users: users}, nil
	})
}
