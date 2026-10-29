package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yi-nology/go-git-platform/provider"
)

// fakeProvider embeds the nil Provider interface; only what the tools
// touch is real.
type fakeProvider struct {
	provider.Provider
	caps provider.CapabilitySet
}

func (f fakeProvider) Platform() provider.Platform { return provider.PlatformGitHub }
func (f fakeProvider) Capabilities() provider.CapabilitySet {
	return f.caps
}
func (f fakeProvider) GetRepo(ctx context.Context, owner, repo string) (*provider.PlatformRepo, error) {
	return &provider.PlatformRepo{Owner: owner, Name: repo}, nil
}
func (f fakeProvider) ListRepos(ctx context.Context, opts provider.ListRepoOptions) ([]*provider.PlatformRepo, error) {
	return []*provider.PlatformRepo{{Owner: opts.Owner, Name: "r1"}}, nil
}
func (f fakeProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	return "contents of " + path, nil
}
func (f fakeProvider) ListCommits(ctx context.Context, owner, repo string, opts provider.ListCommitsOptions) ([]*provider.CommitInfo, error) {
	return []*provider.CommitInfo{{SHA: "abc123"}}, nil
}
func (f fakeProvider) CreateNote(ctx context.Context, owner, repo, number, body string) (string, error) {
	return "note-1", nil
}
func (f fakeProvider) ListIssues(ctx context.Context, opts provider.ListIssuesOptions) ([]*provider.Issue, int, error) {
	return []*provider.Issue{{Number: "1", Title: "broken thing"}}, 1, nil
}
func (f fakeProvider) GetIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	return &provider.Issue{Number: number, Title: "broken thing"}, nil
}
func (f fakeProvider) CreateIssue(ctx context.Context, opts provider.CreateIssueOptions) (*provider.Issue, error) {
	return &provider.Issue{Number: "2", Title: opts.Title}, nil
}
func (f fakeProvider) CreateIssueComment(ctx context.Context, owner, repo, number, body string) (*provider.IssueComment, error) {
	return &provider.IssueComment{Body: body}, nil
}
func (f fakeProvider) SearchRepos(ctx context.Context, opts provider.SearchReposOptions) ([]*provider.SearchRepoResult, *int, error) {
	n := 1
	return []*provider.SearchRepoResult{{FullName: "o/r1"}}, &n, nil
}
func (f fakeProvider) SearchIssues(ctx context.Context, opts provider.SearchIssuesOptions) ([]*provider.SearchIssueResult, *int, error) {
	return nil, nil, nil
}
func (f fakeProvider) SearchUsers(ctx context.Context, opts provider.SearchUsersOptions) ([]*provider.SearchUserResult, *int, error) {
	return nil, nil, nil
}
func (f fakeProvider) UpdateIssue(ctx context.Context, owner, repo, number string, opts provider.UpdateIssueOptions) (*provider.Issue, error) {
	return nil, nil
}
func (f fakeProvider) CloseIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	return nil, nil
}
func (f fakeProvider) ReopenIssue(ctx context.Context, owner, repo, number string) (*provider.Issue, error) {
	return nil, nil
}
func (f fakeProvider) ListIssueComments(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	return nil, nil
}
func (f fakeProvider) UpdateIssueComment(ctx context.Context, owner, repo, number string, commentID int64, body string) (*provider.IssueComment, error) {
	return nil, nil
}
func (f fakeProvider) ListIssueLabels(ctx context.Context, owner, repo string) ([]*provider.IssueLabel, error) {
	return nil, nil
}
func (f fakeProvider) AddIssueLabels(ctx context.Context, owner, repo, number string, labels []string) error {
	return nil
}
func (f fakeProvider) RemoveIssueLabel(ctx context.Context, owner, repo, number, name string) error {
	return nil
}

// connect starts an in-memory MCP session against a server built over
// the given provider and returns a client for tool calls.
func connect(t *testing.T, p provider.Provider, opts Options) *mcp.ClientSession {
	t.Helper()
	server, err := NewServer(p, opts)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		_ = server.Run(ctx, serverTransport)
	}()
	sess, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() {
		_ = sess.Close()
		<-srvDone
	})
	return sess
}

func listTools(t *testing.T, sess *mcp.ClientSession) map[string]bool {
	t.Helper()
	resp, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range resp.Tools {
		names[tl.Name] = true
	}
	return names
}

func TestToolsetMounting(t *testing.T) {
	p := fakeProvider{caps: provider.CapabilitySet{Issues: true, Search: true}}
	names := listTools(t, connect(t, p, Options{}))

	for _, want := range []string{"get_repo", "get_file", "list_crs", "create_cr", "merge_cr",
		"list_issues", "create_issue", "search_repositories"} {
		if !names[want] {
			t.Errorf("tool %q not mounted (have %v)", want, names)
		}
	}
	// CommitStatuses not declared: status tools must be absent
	for _, absent := range []string{"get_commit_statuses", "set_commit_status", "wait_for_status"} {
		if names[absent] {
			t.Errorf("tool %q mounted although capability is not declared", absent)
		}
	}
}

func TestReadOnlyDropsMutations(t *testing.T) {
	p := fakeProvider{caps: provider.CapabilitySet{Issues: true}}
	names := listTools(t, connect(t, p, Options{ReadOnly: true}))

	for _, absent := range []string{"create_cr", "merge_cr", "add_cr_comment", "create_issue", "add_issue_comment"} {
		if names[absent] {
			t.Errorf("mutating tool %q mounted in read-only mode", absent)
		}
	}
	for _, want := range []string{"get_repo", "list_crs", "list_issues", "get_file"} {
		if !names[want] {
			t.Errorf("read tool %q missing in read-only mode", want)
		}
	}
}

func TestToolsetSubset(t *testing.T) {
	p := fakeProvider{caps: provider.CapabilitySet{Issues: true}}
	names := listTools(t, connect(t, p, Options{Toolsets: []string{"issues"}}))
	if len(names) == 0 {
		t.Fatal("no tools mounted for issues toolset")
	}
	for _, absent := range []string{"get_repo", "create_cr", "search_repositories"} {
		if names[absent] {
			t.Errorf("tool %q mounted although toolset was not selected", absent)
		}
	}
	if !names["list_issues"] {
		t.Error("list_issues missing from issues toolset")
	}
}

func TestToolCallEndToEnd(t *testing.T) {
	p := fakeProvider{caps: provider.CapabilitySet{}}
	sess := connect(t, p, Options{})

	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_file",
		Arguments: map[string]any{"owner": "o", "repo": "r", "path": "README.md"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("get_file returned tool error: %v", res.Content)
	}
}

func TestReadOnlyToolCallRejected(t *testing.T) {
	p := fakeProvider{caps: provider.CapabilitySet{}}
	// on the read-only server, mutating tools don't exist, so calling
	// one must fail at the protocol level (unknown tool) — the
	// strongest guarantee we can give.
	sessRO := connect(t, p, Options{ReadOnly: true})

	_, err := sessRO.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_cr",
		Arguments: map[string]any{"owner": "o", "repo": "r", "title": "t", "source_branch": "a", "target_branch": "b"},
	})
	if err == nil {
		t.Fatal("create_cr should not exist on a read-only server")
	}
}

// --- branch protection / collaborators / CR notes (fake extensions) ---

func (f fakeProvider) ListBranchProtections(ctx context.Context, owner, repo string) ([]*provider.BranchProtection, error) {
	return []*provider.BranchProtection{{BranchName: "main"}}, nil
}
func (f fakeProvider) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*provider.BranchProtection, error) {
	return &provider.BranchProtection{BranchName: branch}, nil
}
func (f fakeProvider) CreateBranchProtection(ctx context.Context, owner, repo string, opts provider.CreateBranchProtectionOptions) (*provider.BranchProtection, error) {
	return &provider.BranchProtection{BranchName: opts.BranchName}, nil
}
func (f fakeProvider) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, opts provider.UpdateBranchProtectionOptions) (*provider.BranchProtection, error) {
	return &provider.BranchProtection{BranchName: branch}, nil
}
func (f fakeProvider) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	return nil
}
func (f fakeProvider) ListCollaborators(ctx context.Context, owner, repo string) ([]*provider.Collaborator, error) {
	return []*provider.Collaborator{{Username: "dev", Permission: "write"}}, nil
}
func (f fakeProvider) AddCollaborator(ctx context.Context, owner, repo, username string, opts provider.AddCollaboratorOptions) error {
	return nil
}
func (f fakeProvider) RemoveCollaborator(ctx context.Context, owner, repo, username string) error {
	return nil
}
func (f fakeProvider) ListNotes(ctx context.Context, owner, repo, number string) ([]*provider.IssueComment, error) {
	return []*provider.IssueComment{{Body: "first"}}, nil
}
func (f fakeProvider) UpdateNote(ctx context.Context, owner, repo, number, noteID, body string) (*provider.IssueComment, error) {
	return &provider.IssueComment{Body: body}, nil
}

// TestProtectionCollaboratorNoteToolsets pins the capability-gated mounting
// of the new toolsets: protection/collaborators mount when the provider
// implements BranchProtectionManager/CollaboratorManager, and the CR-note
// read/edit tools ride NoteManager inside the crs toolset.
func TestProtectionCollaboratorNoteToolsets(t *testing.T) {
	p := fakeProvider{caps: provider.CapabilitySet{}}
	names := listTools(t, connect(t, p, Options{}))

	for _, want := range []string{
		"list_branch_protections", "get_branch_protection", "ensure_branch_protection", "delete_branch_protection",
		"list_collaborators", "add_collaborator", "remove_collaborator",
		"list_cr_comments", "update_cr_comment",
	} {
		if !names[want] {
			t.Errorf("tool %q not mounted (have %v)", want, names)
		}
	}

	// Read-only: the write tools vanish, the reads stay.
	ro := listTools(t, connect(t, p, Options{ReadOnly: true}))
	for _, absent := range []string{
		"ensure_branch_protection", "delete_branch_protection",
		"add_collaborator", "remove_collaborator", "update_cr_comment",
	} {
		if ro[absent] {
			t.Errorf("mutating tool %q mounted in read-only mode", absent)
		}
	}
	for _, want := range []string{"list_branch_protections", "get_branch_protection", "list_collaborators", "list_cr_comments"} {
		if !ro[want] {
			t.Errorf("read tool %q missing in read-only mode", want)
		}
	}

	// Subset selection: "protection" alone mounts only protection tools.
	sub := listTools(t, connect(t, p, Options{Toolsets: []string{"protection"}}))
	for _, absent := range []string{"list_collaborators", "get_repo", "list_cr_comments"} {
		if sub[absent] {
			t.Errorf("tool %q mounted although toolset was not selected", absent)
		}
	}
	if !sub["ensure_branch_protection"] {
		t.Error("ensure_branch_protection missing from protection toolset")
	}
}

// TestCollaboratorPermissionValidated pins the tool-boundary vocabulary
// check: an unknown permission is a tool error, not a deep platform 4xx.
func TestCollaboratorPermissionValidated(t *testing.T) {
	p := fakeProvider{caps: provider.CapabilitySet{}}
	sess := connect(t, p, Options{})

	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "add_collaborator",
		Arguments: map[string]any{
			"owner": "o", "repo": "r", "user": "dev", "permission": "owner",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("invalid permission must be a tool error, got: %v", res.Content)
	}
}
