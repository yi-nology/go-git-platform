package tencentcode_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/tencentcode"
	"github.com/yi-nology/go-git-platform/provider"
)

// recordCalls wraps an httptest handler: every request is appended (as
// "METHOD /escaped/path") under the mutex before fn serves it. Handlers
// route on the decoded r.URL.Path — the escaped form is only for the
// recorded pins.
func recordCalls(mu *sync.Mutex, calls *[]string, fn func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*calls = append(*calls, r.Method+" "+r.URL.EscapedPath())
		mu.Unlock()
		fn(w, r)
	}
}

// assertCalls fails when the recorded call sequence differs from want.
func assertCalls(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("recorded %d calls, want %d:\n got: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call %d = %q, want %q\n full got: %v", i, got[i], want[i], got)
		}
	}
}

// readRequestBody decodes a JSON request body for wire assertions.
func readRequestBody(r *http.Request, v any) error {
	defer r.Body.Close() //nolint:errcheck // assertion helper; body read errors surface in the decode below
	return json.NewDecoder(r.Body).Decode(v)
}

// TestTencentCode_CollaboratorsWire pins the project-members surface behind
// provider.CollaboratorManager: the list paginates the members collection,
// add posts a resolved numeric user_id, and remove DELETEs by numeric ID —
// 工蜂's members API is ID-addressed, so both mutations resolve the
// username through the users lookup first (and only once, via the TTL
// cache).
func TestTencentCode_CollaboratorsWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var addBody map[string]any
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/users/dev":
			writeJSON(w, map[string]any{"id": 7, "username": "dev", "name": "Dev"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/projects/owner/repo/members":
			_ = readRequestBody(r, &addBody)
			writeJSON(w, map[string]any{"id": 7, "username": "dev", "name": "Dev", "access_level": 30})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default: // members list
			writeListPage(w, r, []map[string]any{
				{"id": 7, "username": "dev", "name": "Dev", "access_level": 30, "state": "active"},
			})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	if !p.Capabilities().Collaborators {
		t.Fatal("CollaboratorManager implementation must be declared in Capabilities().Collaborators")
	}
	var cm provider.CollaboratorManager = p

	collabs, err := cm.ListCollaborators(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("ListCollaborators: %v", err)
	}
	if len(collabs) != 1 || collabs[0].Username != "dev" || collabs[0].Permission != "developer" || collabs[0].User.Name != "Dev" {
		t.Fatalf("unexpected collaborators: %+v", collabs)
	}

	if err := cm.AddCollaborator(context.Background(), "owner", "repo", "dev", provider.AddCollaboratorOptions{Permission: "write"}); err != nil {
		t.Fatalf("AddCollaborator: %v", err)
	}
	if err := cm.RemoveCollaborator(context.Background(), "owner", "repo", "dev"); err != nil {
		t.Fatalf("RemoveCollaborator: %v", err)
	}

	assertCalls(t, calls, []string{
		http.MethodGet + " /api/v3/projects/owner%2Frepo/members", // page 1
		http.MethodGet + " /api/v3/projects/owner%2Frepo/members", // page 2 (empty)
		http.MethodGet + " /api/v3/users/dev",
		http.MethodPost + " /api/v3/projects/owner%2Frepo/members",
		http.MethodDelete + " /api/v3/projects/owner%2Frepo/members/7",
	})
	// "write" maps onto the Developer access level (registered mapping).
	if addBody["user_id"].(float64) != 7 || addBody["access_level"].(float64) != 30 {
		t.Errorf("add body must carry user_id=7 + access_level=30, got %v", addBody)
	}
}

// TestTencentCode_OrgExtrasWire pins the group/namespace surface: group
// lists paginate the groups collection, member mutations address the group
// numerically, and the edit endpoint PUTs the collection with the group ID
// in the body (platform shape).
func TestTencentCode_OrgExtrasWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var editBody map[string]any
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/groups":
			switch r.Method {
			case http.MethodGet:
				writeListPage(w, r, []map[string]any{{"id": 3, "name": "team", "path": "team", "web_url": "https://t/g/3"}})
			case http.MethodPut:
				_ = readRequestBody(r, &editBody)
				writeJSON(w, map[string]any{"id": 3, "name": "renamed", "path": "team"})
			default:
				writeJSON(w, map[string]any{"id": 3, "name": "team", "path": "team"})
			}
		case "/api/v3/groups/3/members":
			writeJSON(w, map[string]any{"id": 7, "username": "dev", "access_level": 40})
		case "/api/v3/namespaces":
			writeListPage(w, r, []map[string]any{{"id": 3, "name": "team", "path": "team", "kind": "group"}})
		default:
			writeJSON(w, map[string]any{"id": 3})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	var ox tencentcode.TencentCodeOrgExtras = p

	groups, err := ox.ListGroups(context.Background(), tencentcode.ListGroupsOptions{})
	if err != nil || len(groups) != 1 || groups[0].ID != 3 {
		t.Fatalf("ListGroups: %+v err=%v", groups, err)
	}
	if _, err := ox.CreateGroup(context.Background(), tencentcode.CreateGroupOptions{Name: "team", Path: "team"}); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if _, err := ox.EditGroup(context.Background(), 3, tencentcode.EditGroupOptions{Name: "renamed"}); err != nil {
		t.Fatalf("EditGroup: %v", err)
	}
	m, err := ox.AddGroupMember(context.Background(), 3, tencentcode.AddGroupMemberOptions{UserID: 7, AccessLevel: 40})
	if err != nil || m.AccessLevel != 40 {
		t.Fatalf("AddGroupMember: %+v err=%v", m, err)
	}
	ns, err := ox.ListNamespaces(context.Background(), tencentcode.ListNamespacesOptions{})
	if err != nil || len(ns) != 1 || ns[0].Kind != "group" {
		t.Fatalf("ListNamespaces: %+v err=%v", ns, err)
	}

	// The group edit must PUT the collection carrying the ID in the body.
	if editBody == nil || editBody["id"].(float64) != 3 || editBody["name"] != "renamed" {
		t.Errorf("EditGroup must PUT the group id + fields in the body, got %v", editBody)
	}
}

// TestTencentCode_ProjectExtrasWire pins the watcher/star/tag surface:
// watch status parses the platform's bare-bool payload, watching POSTs the
// watch collection, and tags live under repository/tags.
func TestTencentCode_ProjectExtrasWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/projects/owner/repo/watch":
			// 工蜂 serves the watch status as a bare JSON boolean.
			writeJSON(w, true)
		case r.URL.Path == "/api/v3/projects/owner/repo/repository/tags/v1.0":
			if r.Method == http.MethodGet {
				writeJSON(w, map[string]any{"name": "v1.0", "message": "rel", "commit": map[string]any{"id": "abc"}})
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			writeJSON(w, map[string]any{"project_id": 1, "mute": false, "user": map[string]any{"id": 7, "username": "dev", "name": "Dev"}})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	var px tencentcode.TencentCodeProjectExtras = p

	if watching, err := px.GetWatchStatus(context.Background(), "owner", "repo"); err != nil || !watching {
		t.Fatalf("GetWatchStatus: %v %v", watching, err)
	}
	w, err := px.WatchProject(context.Background(), "owner", "repo")
	if err != nil || w.Username != "dev" {
		t.Fatalf("WatchProject: %+v err=%v", w, err)
	}
	tag, err := px.GetTag(context.Background(), "owner", "repo", "v1.0")
	if err != nil || tag.CommitSHA != "abc" || tag.Message != "rel" {
		t.Fatalf("GetTag: %+v err=%v", tag, err)
	}
	if err := px.DeleteTag(context.Background(), "owner", "repo", "v1.0"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}

	assertCalls(t, calls, []string{
		http.MethodGet + " /api/v3/projects/owner%2Frepo/watch",
		http.MethodPut + " /api/v3/projects/owner%2Frepo/watch",
		http.MethodGet + " /api/v3/projects/owner%2Frepo/repository/tags/v1.0",
		http.MethodDelete + " /api/v3/projects/owner%2Frepo/repository/tags/v1.0",
	})
}

// TestTencentCode_AccountAndMemberExtrasWire pins the remaining account /
// member extras: user directory search, email lookup, single key/email
// fetches, owned projects (projects/owned), and the project-member
// edit/get pair (CollaboratorManager only covers add/remove/list).
func TestTencentCode_AccountAndMemberExtrasWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/users":
			writeListPage(w, r, []map[string]any{{"id": 7, "username": "dev", "name": "Dev", "avatar_url": "https://t/a/7"}})
		case "/api/v3/user/email":
			writeJSON(w, map[string]any{"id": 7, "username": "dev", "name": "Dev"})
		case "/api/v3/user/keys/1":
			writeJSON(w, map[string]any{"id": 1, "title": "laptop", "key": "ssh-ed25519 AA"})
		case "/api/v3/user/emails/2":
			writeJSON(w, map[string]any{"id": 2, "email": "dev@example.com"})
		case "/api/v3/projects/owner/repo/members/7":
			writeJSON(w, map[string]any{"id": 7, "username": "dev", "access_level": 40})
		case "/api/v3/projects/owned":
			writeListPage(w, r, []map[string]any{{
				"id": 1, "path": "repo", "path_with_namespace": "owner/repo",
				"owner": map[string]any{"username": "owner"},
			}})
		default:
			writeJSON(w, map[string]any{"id": 1})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)

	users, err := p.ListUsers(context.Background(), tencentcode.ListUsersOptions{Search: "dev"})
	if err != nil || len(users) != 1 || users[0].ID != 7 || users[0].AvatarURL != "https://t/a/7" {
		t.Fatalf("ListUsers: %+v err=%v", users, err)
	}
	byEmail, err := p.GetUserByEmail(context.Background(), "dev@example.com")
	if err != nil || byEmail.Username != "dev" {
		t.Fatalf("GetUserByEmail: %+v err=%v", byEmail, err)
	}
	key, err := p.GetSSHKey(context.Background(), 1)
	if err != nil || key.Title != "laptop" {
		t.Fatalf("GetSSHKey: %+v err=%v", key, err)
	}
	email, err := p.GetEmail(context.Background(), 2)
	if err != nil || email.Email != "dev@example.com" {
		t.Fatalf("GetEmail: %+v err=%v", email, err)
	}
	member, err := p.GetProjectMember(context.Background(), "owner", "repo", 7)
	if err != nil || member.AccessLevel != 40 {
		t.Fatalf("GetProjectMember: %+v err=%v", member, err)
	}
	edited, err := p.EditProjectMember(context.Background(), "owner", "repo", 7, tencentcode.EditProjectMemberOptions{AccessLevel: 40})
	if err != nil || edited.AccessLevel != 40 {
		t.Fatalf("EditProjectMember: %+v err=%v", edited, err)
	}
	owned, err := p.ListOwnedProjects(context.Background(), 0, 0)
	if err != nil || len(owned) != 1 {
		t.Fatalf("ListOwnedProjects: %+v err=%v", owned, err)
	}

	assertCalls(t, calls, []string{
		http.MethodGet + " /api/v3/users", // page 1
		http.MethodGet + " /api/v3/users", // page 2 (empty)
		http.MethodGet + " /api/v3/user/email",
		http.MethodGet + " /api/v3/user/keys/1",
		http.MethodGet + " /api/v3/user/emails/2",
		http.MethodGet + " /api/v3/projects/owner%2Frepo/members/7",
		http.MethodPut + " /api/v3/projects/owner%2Frepo/members/7",
		http.MethodGet + " /api/v3/projects/owned", // page 1
		http.MethodGet + " /api/v3/projects/owned", // page 2 (empty)
	})
}

// TestTencentCode_AccountExtrasWire pins the account surface: the session
// login POSTs credentials to /session, SSH keys and emails live under
// /user/keys and /user/emails, and watched projects reuse the project
// conversion.

func TestTencentCode_AccountExtrasWire(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var sessionBody map[string]any
	srv := httptest.NewServer(recordCalls(&mu, &calls, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/session":
			_ = readRequestBody(r, &sessionBody)
			writeJSON(w, map[string]any{"id": 7, "username": "dev", "private_token": "tok"})
		case "/api/v3/user/keys":
			writeJSON(w, []map[string]any{{"id": 1, "title": "laptop", "key": "ssh-ed25519 AA"}})
		case "/api/v3/user/emails":
			writeJSON(w, []map[string]any{{"id": 2, "email": "dev@example.com"}})
		case "/api/v3/user/watched":
			writeListPage(w, r, []map[string]any{{
				"id": 1, "path": "repo", "path_with_namespace": "owner/repo",
				"owner": map[string]any{"username": "owner"}, "http_url_to_repo": "https://t/o/r.git",
			}})
		default:
			writeJSON(w, map[string]any{"id": 7, "username": "dev"})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	var ax tencentcode.TencentCodeAccountExtras = p

	sess, err := ax.GetSession(context.Background(), "dev", "pw")
	if err != nil || sess.PrivateToken != "tok" {
		t.Fatalf("GetSession: %+v err=%v", sess, err)
	}
	if sessionBody["login"] != "dev" || sessionBody["password"] != "pw" {
		t.Errorf("session login must carry login+password, got %v", sessionBody)
	}
	keys, err := ax.ListSSHKeys(context.Background())
	if err != nil || len(keys) != 1 || keys[0].Title != "laptop" {
		t.Fatalf("ListSSHKeys: %+v err=%v", keys, err)
	}
	emails, err := ax.ListEmails(context.Background())
	if err != nil || len(emails) != 1 || emails[0].Email != "dev@example.com" {
		t.Fatalf("ListEmails: %+v err=%v", emails, err)
	}
	watched, err := ax.ListWatchedProjects(context.Background(), 0, 0)
	if err != nil || len(watched) != 1 {
		t.Fatalf("ListWatchedProjects: %+v err=%v", watched, err)
	}

	assertCalls(t, calls, []string{
		http.MethodPost + " /api/v3/session",
		http.MethodGet + " /api/v3/user/keys",
		http.MethodGet + " /api/v3/user/emails",
		http.MethodGet + " /api/v3/user/watched", // page 1 (then the empty page 2)
		http.MethodGet + " /api/v3/user/watched",
	})
}
