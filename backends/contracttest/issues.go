package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// ClearExpect 声明某字段「清空请求」的期望形态（2A A/B 能力矩阵）。
// Unsupported=true：B 级——必须返回 ErrClearNotSupported 且零新请求；
// 否则：请求必须发出，BodyField 在场为空值（""/[]/{}/null/0）、
// PathContains 命中（侧端点定位）、Method 为 "" 时 PATCH/PUT 皆可。
type ClearExpect struct {
	Unsupported  bool
	Method       string // "" = PATCH 或 PUT
	BodyField    string // "" = 不断言 body（无 body 的侧端点，如 DELETE）
	PathContains string // "" = 不断言路径
}

// IssuesHarnessConfig carries the fixtures a backend's main Harness needs to
// auto-mount the issue-management suite via Harness.Issues.
type IssuesHarnessConfig struct {
	// ListResponse is the JSON array for issue-list GETs. First item: number
	// 1, title "bug", state "open", milestone {number:1, title:"v1"}.
	ListResponse string
	// GetResponse is the JSON object for single-issue GETs (number 1, title
	// "bug", state "open", milestone {number:1, title:"v1"}).
	GetResponse string
	// MutateResponse is the JSON object for POST/PATCH/PUT (same shape as
	// GetResponse).
	MutateResponse string
	// CommentsResponse is the JSON array for issue-comment GETs. First item:
	// id 1, body "a comment".
	CommentsResponse string
	// LabelsResponse is the JSON array for repository-label GETs. First
	// item: id 1, name "bug", color "#4cc917".
	LabelsResponse string
	// CreateIssueAssigneesByID declares that the backend resolves issue
	// assignee usernames through a users lookup (GitLab's /users?username=
	// exact-match filter, Tencent 工蜂's /users/{username} endpoint) and
	// writes numeric assignee IDs. When set, the suite asserts the
	// lookup-then-write wire shape on CreateIssue; when unset, the create
	// subtest exercises the plain title-only wire (the default).
	CreateIssueAssigneesByID bool
	// UpdateCommentViaIssue declares that the backend addresses an issue
	// comment edit through the issue (GitLab, Tencent 工蜂 note APIs): the
	// suite additionally asserts the recorded edit request hits
	// issues/1/notes/1, so a swap of the number and commentID plumbing
	// fails instead of passing on the body assertion alone.
	UpdateCommentViaIssue bool
	// IssueClear 声明各字段清空请求的期望（键：body/assignees/labels/
	// milestone）。nil = 四字段全部必须可清空（全 A 级平台可不设）。
	IssueClear map[string]ClearExpect
}

// IssuesHarness is the full harness RunIssuesSuite consumes; auto-mounting
// builds it from the enclosing Harness plus IssuesHarnessConfig.
type IssuesHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider func(t *testing.T, cfg provider.Config) provider.Provider
	IssuesHarnessConfig
}

// testIssuesSuite auto-mounts RunIssuesSuite from a main Harness with the
// same bidirectional drift checks as the labels suite.
func testIssuesSuite(t *testing.T, h Harness) {
	srv := httptest.NewServer(stubHandler(h))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	declared := p.Capabilities().Issues
	switch {
	case h.Issues == nil && !declared:
		t.Skipf("%s declares no Issues capability", h.Name)
	case h.Issues == nil:
		t.Errorf("%s declares Capabilities().Issues but its Harness provides no Issues config — the issues suite is not wired", h.Name)
	case !declared:
		t.Errorf("%s Harness provides an Issues config but the platform does not declare Capabilities().Issues", h.Name)
	default:
		RunIssuesSuite(t, IssuesHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			IssuesHarnessConfig: *h.Issues,
		})
	}
}

// RunIssuesSuite executes the issue-management contract suite. The mock
// routes by method and path shape so platform-specific paths don't matter:
// GET ending /issues/{alphanumeric} → GetResponse (issue numbers are not
// always numeric — Gitee's are alphanumeric); GET containing
// comments|notes →
// CommentsResponse; GET containing labels (not issues) → LabelsResponse;
// other GET → ListResponse; POST → 201 + MutateResponse (a POST to a labels
// path — adding labels to an issue — returns LabelsResponse, since
// GitHub-shaped APIs answer with the label array); PUT to a labels path —
// replacing an issue's labels — likewise returns LabelsResponse (an issue
// object there breaks the []*Label parse); PATCH/PUT →
// MutateResponse; DELETE → 204. Requests are recorded for wire assertions.
func RunIssuesSuite(t *testing.T, h IssuesHarness) {
	newIM := func(t *testing.T) (provider.IssueManager, *[]recordedRequest) {
		srv, requests := issueStubServer(h)
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		im, ok := p.(provider.IssueManager)
		if !ok {
			t.Fatalf("%s does not implement provider.IssueManager", h.Name)
		}
		return im, requests
	}

	t.Run("List_ParsesAndNormalizes", func(t *testing.T) {
		im, _ := newIM(t)
		assertIssueListNormalized(t, im)
	})
	t.Run("List_PageZero_SingleRequest", func(t *testing.T) {
		// Caller-paged family (issue #51 — see provider.NormalizePageOpts):
		// a default ListIssues (Page == 0) must serve the first page in
		// EXACTLY one request. A walk means the backend still exhausts
		// pagination on the zero value — the pre-fix gitea/forgejo/gitcode/
		// tencentcode shape, where a default call's request volume scaled
		// silently with the repository's issue count.
		im, requests := newIM(t)
		issues, total, err := im.ListIssues(context.Background(), provider.ListIssuesOptions{Owner: "owner", Repo: "repo"})
		if err != nil {
			t.Fatalf("ListIssues(Page==0): %v", err)
		}
		if got := countGETs(requests); got != 1 {
			t.Fatalf("Page==0: expected exactly 1 GET, got %d — caller-paged contract violated (issue #51)", got)
		}
		if total != len(issues) {
			t.Fatalf("Page==0: total = %d, want len(result) = %d — the int return is the served rows' count, never a platform total (issue #51)", total, len(issues))
		}
	})
	t.Run("List_TotalEqualsItemCount", func(t *testing.T) {
		// The int return means "how many rows I served" on every backend —
		// the fixture list response carries X-Total: 999 so a backend
		// forwarding the platform total fails here even when the page is
		// served correctly (the pre-fix TencentCode shape).
		im, _ := newIM(t)
		issues, total, err := im.ListIssues(context.Background(), provider.ListIssuesOptions{Owner: "owner", Repo: "repo", Page: 1, PerPage: 10})
		if err != nil {
			t.Fatalf("ListIssues(Page==1): %v", err)
		}
		if total != len(issues) {
			t.Fatalf("Page==1: total = %d, want len(result) = %d (issue #51)", total, len(issues))
		}
	})
	t.Run("Get_ReturnsIssue", func(t *testing.T) {
		im, _ := newIM(t)
		assertIssueGet(t, im)
	})
	t.Run("Create_PostsTitle", func(t *testing.T) {
		im, requests := newIM(t)
		assertIssueCreateWire(t, im, requests)
	})
	t.Run("Create_ResolvesAssigneesByID", func(t *testing.T) {
		if !h.CreateIssueAssigneesByID {
			t.Skipf("%s does not declare CreateIssueAssigneesByID", h.Name)
		}
		im, requests := newIM(t)
		assertIssueCreateAssigneesByIDWire(t, im, requests)
	})
	t.Run("Update_PatchesTitle", func(t *testing.T) {
		im, requests := newIM(t)
		assertIssueUpdateWire(t, im, requests)
		assertIssueUpdateTriState(t, im, requests, h.IssueClear)
	})
	t.Run("CloseAndReopen_Succeed", func(t *testing.T) {
		im, _ := newIM(t)
		assertIssueCloseReopen(t, im)
	})
	t.Run("Comments_ListCreateUpdate", func(t *testing.T) {
		im, requests := newIM(t)
		assertIssueComments(t, h, im, requests)
	})
	t.Run("IssueLabels_ListAddRemove", func(t *testing.T) {
		im, requests := newIM(t)
		assertIssueLabelOps(t, im, requests)
	})
}

// assertIssueListNormalized checks that ListIssues returns parsed, normalized
// issues: title, open state, and the milestone ref.
func assertIssueListNormalized(t *testing.T, im provider.IssueManager) {
	t.Helper()
	issues, _, err := im.ListIssues(context.Background(), provider.ListIssuesOptions{Owner: "owner", Repo: "repo", Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) == 0 || issues[0].Title != "bug" {
		t.Fatalf("expected first issue titled bug, got %+v", issues)
	}
	if issues[0].State != provider.IssueStateOpen {
		t.Errorf("expected state open, got %q", issues[0].State)
	}
	if issues[0].Milestone == nil || issues[0].Milestone.Number != "1" || issues[0].Milestone.Title != "v1" {
		t.Errorf("expected milestone ref {1, v1}, got %+v", issues[0].Milestone)
	}
}

// assertIssueGet checks that GetIssue returns the fixture issue.
func assertIssueGet(t *testing.T, im provider.IssueManager) {
	t.Helper()
	issue, err := im.GetIssue(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue == nil || issue.Title != "bug" {
		t.Fatalf("expected issue titled bug, got %+v", issue)
	}
}

// assertIssueCreateWire checks that CreateIssue succeeds and posts the title.
func assertIssueCreateWire(t *testing.T, im provider.IssueManager, requests *[]recordedRequest) {
	t.Helper()
	issue, err := im.CreateIssue(context.Background(), provider.CreateIssueOptions{Owner: "owner", Repo: "repo", Title: "bug", Body: "broke"})
	if err != nil || issue == nil {
		t.Fatalf("CreateIssue: issue=%v err=%v", issue, err)
	}
	assertBodyHas(t, requests, http.MethodPost, "title", "bug")
}

// assertIssueCreateAssigneesByIDWire checks that CreateIssue's Assignees
// resolve through a users lookup before the write and land on the create
// body as numeric assignee IDs (GitLab's assignee_ids array, Tencent 工蜂's
// assignee_ids csv).
func assertIssueCreateAssigneesByIDWire(t *testing.T, im provider.IssueManager, requests *[]recordedRequest) {
	t.Helper()
	if _, err := im.CreateIssue(context.Background(), provider.CreateIssueOptions{
		Owner: "owner", Repo: "repo", Title: "bug", Assignees: []string{"dev"},
	}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	var postBody string
	sawUsersLookup := false
	for _, req := range *requests {
		reqPath, _, _ := strings.Cut(req.Path, "?")
		if req.Method == http.MethodGet && usersLookupPath.MatchString(reqPath) {
			sawUsersLookup = true
			continue
		}
		if req.Method == http.MethodPost {
			b, _ := json.Marshal(req.Body)
			postBody = string(b)
		}
	}
	if !sawUsersLookup {
		t.Error("expected a users lookup before writing assignees")
	}
	if !strings.Contains(postBody, "assignee_ids") || !strings.Contains(postBody, "101") {
		t.Errorf("issue create body = %q, want assignee_ids containing resolved id 101", postBody)
	}
}

// assertIssueUpdateWire checks that UpdateIssue mutates via PATCH or PUT and
// carries the new title in the body.
func assertIssueUpdateWire(t *testing.T, im provider.IssueManager, requests *[]recordedRequest) {
	t.Helper()
	title := "bug-2"
	if _, err := im.UpdateIssue(context.Background(), "owner", "repo", "1", provider.UpdateIssueOptions{Title: title}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if !sawMethod(*requests, http.MethodPatch, http.MethodPut) {
		t.Errorf("expected a PATCH/PUT update, recorded %s", methodsOf(*requests))
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, "title", "bug-2") // PUT also accepted
}

// assertIssueUpdateTriState 钉 UpdateIssue 三态（2A spec）：unset 不得携带
// 字段、legacy 置值回归、patch 置值与 legacy 同形、每字段清空按 ClearExpect
// 分派（B 级哨兵 + 零请求 / A 级 wire 在场空值）。
//
// wire 键取平台 ClearExpect.BodyField（T3-T7 配置），未声明回落 canonical：
// 清空与置值走同一 wire 字段。置值回归用正文（跨平台唯一值保持字段——
// GitLab/工蜂 上线 description、其余 body）钉精确值；指派人因 ID 解析
// （assignee_ids）只能钉「键在场且非空」。
func assertIssueUpdateTriState(t *testing.T, im provider.IssueManager, requests *[]recordedRequest, clears map[string]ClearExpect) {
	t.Helper()
	ctx := context.Background()

	bodyKey := clearKey(clears, "body", "body")
	assigneesKey := clearKey(clears, "assignees", "assignees")

	// 1) unset：全零 options——若产生写请求，body 不得携带四字段
	//    （外加平台声明的 wire 键）。
	before := len(*requests)
	if _, err := im.UpdateIssue(ctx, "owner", "repo", "1", provider.UpdateIssueOptions{}); err != nil {
		t.Fatalf("unset UpdateIssue: %v", err)
	}
	assertBodyLacks(t, requests, before, unsetKeys(clears, "body", "assignees", "labels", "milestone")...)

	// 2) legacy 置值回归（旧行为不许破）。
	before = len(*requests)
	if _, err := im.UpdateIssue(ctx, "owner", "repo", "1", provider.UpdateIssueOptions{
		Body: "legacy-body", Assignees: []string{"alice"},
	}); err != nil {
		t.Fatalf("legacy set: %v", err)
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, bodyKey, "legacy-body")
	assertBodyHasNonEmpty(t, requests, before, assigneesKey)

	// 3) patch 置值与 legacy 同形。
	before = len(*requests)
	if _, err := im.UpdateIssue(ctx, "owner", "repo", "1", provider.UpdateIssueOptions{
		BodyPatch: new("patch-body"), AssigneesPatch: &[]string{"bob"},
	}); err != nil {
		t.Fatalf("patch set: %v", err)
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, bodyKey, "patch-body")
	assertBodyHasNonEmpty(t, requests, before, assigneesKey)

	// 4) 逐字段清空（B/A 分派见 dispatchClear）。
	clearSteps := []struct {
		field string
		opts  provider.UpdateIssueOptions
	}{
		{"body", provider.UpdateIssueOptions{BodyPatch: new("")}},
		{"assignees", provider.UpdateIssueOptions{AssigneesPatch: &[]string{}}},
		{"labels", provider.UpdateIssueOptions{LabelsPatch: &[]string{}}},
		{"milestone", provider.UpdateIssueOptions{MilestonePatch: new("")}},
	}
	for i := range clearSteps {
		step := &clearSteps[i]
		stepBefore := len(*requests)
		dispatchClear(t, clears, step.field, requests, stepBefore, func() error {
			_, err := im.UpdateIssue(ctx, "owner", "repo", "1", step.opts)
			return err
		})
	}
}

// assertIssueCloseReopen checks that both state transitions succeed.
func assertIssueCloseReopen(t *testing.T, im provider.IssueManager) {
	t.Helper()
	if _, err := im.CloseIssue(context.Background(), "owner", "repo", "1"); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}
	if _, err := im.ReopenIssue(context.Background(), "owner", "repo", "1"); err != nil {
		t.Fatalf("ReopenIssue: %v", err)
	}
}

// assertIssueCommentsPaginated checks that ListIssueComments exhausts the
// platform's pagination. The stub serves a full synthetic first page, the
// fixture comment on the second page, and nothing after — a backend that
// stops after page one never sees "a comment", so these assertions fail
// for it.
func assertIssueCommentsPaginated(t *testing.T, im provider.IssueManager, requests *[]recordedRequest) {
	t.Helper()
	comments, err := im.ListIssueComments(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatalf("ListIssueComments: %v", err)
	}
	if len(comments) < 2 {
		t.Fatalf("expected the paginated merge (page-one items + the fixture comment), got %d comments", len(comments))
	}
	seen := make(map[int64]bool, len(comments))
	found := false
	for _, c := range comments {
		if c == nil {
			continue
		}
		if seen[c.ID] {
			t.Errorf("duplicate comment ID %d in the merged list", c.ID)
		}
		seen[c.ID] = true
		if c.Body == "a comment" {
			found = true
		}
	}
	if !found {
		t.Error(`expected the second-page fixture comment ("a comment") in the merged list`)
	}
	pageGets := 0
	for _, r := range *requests {
		if r.Method == http.MethodGet && (strings.Contains(r.Path, "comments") || strings.Contains(r.Path, "notes")) {
			pageGets++
		}
	}
	if pageGets < 2 {
		t.Errorf("expected at least two comment-page GETs, recorded %s", methodsOf(*requests))
	}
}

// assertIssueComments checks comment listing (body), creation (wire body),
// editing (verb + body, plus the issue-routed note path on platforms that
// address notes through the issue), and deletion (verb + issue-routed note
// path on the same platforms).
func assertIssueComments(t *testing.T, h IssuesHarness, im provider.IssueManager, requests *[]recordedRequest) {
	t.Helper()
	assertIssueCommentsPaginated(t, im, requests)
	if _, err := im.CreateIssueComment(context.Background(), "owner", "repo", "1", "a comment"); err != nil {
		t.Fatalf("CreateIssueComment: %v", err)
	}
	assertBodyHas(t, requests, http.MethodPost, "body", "a comment")
	if _, err := im.UpdateIssueComment(context.Background(), "owner", "repo", "1", 1, "an edited comment"); err != nil {
		t.Fatalf("UpdateIssueComment: %v", err)
	}
	// The edit is a body-replacing PATCH/PUT carrying the new body text;
	// platforms differ only in whether the path routes through the issue
	// (GitLab, 工蜂) or addresses the comment directly.
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, "body", "an edited comment")
	if h.UpdateCommentViaIssue {
		assertIssueNoteEditPath(t, *requests)
	}
	if err := im.DeleteIssueComment(context.Background(), "owner", "repo", "1", 1); err != nil {
		t.Fatalf("DeleteIssueComment: %v", err)
	}
	// Deletion hits the same address space as the edit (direct on GitHub,
	// Gitea, Forgejo, GitCode, Gitee; issue-routed notes on GitLab, 工蜂):
	// a backend that swapped the number/commentID plumbing would DELETE a
	// different path and fail here.
	if h.UpdateCommentViaIssue {
		last := (*requests)[len(*requests)-1]
		if last.Method != http.MethodDelete || !issueNoteEditPath.MatchString(last.Path) {
			t.Fatalf("expected DELETE on the issue-routed note path .../issues/1/notes/1, got %s %s", last.Method, last.Path)
		}
	} else {
		last := (*requests)[len(*requests)-1]
		if last.Method != http.MethodDelete {
			t.Fatalf("expected a DELETE after DeleteIssueComment, got %s %s", last.Method, last.Path)
		}
	}
}

// issueNoteEditPath matches an issue-routed note edit,
// ".../issues/1/notes/1": a backend that swapped the number and commentID
// plumbing would write to a different path and fail here even though the
// body assertion alone still passes.
var issueNoteEditPath = regexp.MustCompile(`issues/1/notes/1$`)

// assertIssueNoteEditPath checks that the recorded PATCH/PUT comment edit
// addressed the issue-routed note endpoint with the fixture's issue number
// 1 and comment ID 1.
func assertIssueNoteEditPath(t *testing.T, requests []recordedRequest) {
	t.Helper()
	for _, r := range requests {
		if (r.Method == http.MethodPatch || r.Method == http.MethodPut) && issueNoteEditPath.MatchString(r.Path) {
			return
		}
	}
	t.Errorf("expected the comment edit to route through issues/1/notes/1, recorded %s", methodsOf(requests))
}

// assertIssueLabelOps checks repository-label listing (color normalization),
// adding labels to an issue, and removing one — via a dedicated DELETE
// endpoint (GitHub-style), a PATCH/PUT write carrying GitLab's
// remove_labels, or a non-empty PATCH/PUT labels replacement that omits
// the removed name (Tencent 工蜂; see sawLabelRemoval for the shapes).
func assertIssueLabelOps(t *testing.T, im provider.IssueManager, requests *[]recordedRequest) {
	t.Helper()
	labels, err := im.ListIssueLabels(context.Background(), "owner", "repo")
	if err != nil || len(labels) == 0 {
		t.Fatalf("ListIssueLabels: labels=%d err=%v", len(labels), err)
	}
	if labels[0].Color != "4cc917" {
		t.Errorf("expected normalized color 4cc917, got %q", labels[0].Color)
	}
	if err := im.AddIssueLabels(context.Background(), "owner", "repo", "1", []string{"bug"}); err != nil {
		t.Fatalf("AddIssueLabels: %v", err)
	}
	if err := im.RemoveIssueLabel(context.Background(), "owner", "repo", "1", "bug"); err != nil {
		t.Fatalf("RemoveIssueLabel: %v", err)
	}
	if !sawLabelRemoval(*requests) {
		t.Errorf("expected label removal via DELETE, a PATCH/PUT write carrying remove_labels, or a non-empty PATCH/PUT labels replacement omitting the removed name, recorded %s", methodsOf(*requests))
	}
}

// sawLabelRemoval reports whether label removal used one of the three wire
// shapes platforms have: a dedicated DELETE endpoint (GitHub, Gitee, Gitea,
// Forgejo, GitCode); a PATCH/PUT write whose body carries a non-empty
// remove_labels key (GitLab — go-gitlab sends the update form-encoded, and
// the recorded Body map normalizes form keys); or a non-empty PATCH/PUT
// labels replacement whose value no longer contains the removed name
// (Tencent 工蜂 — its issue update surface accepts the full label set only,
// so the backend reads, filters, and rewrites). Bare method matching is not
// enough: GitLab's AddIssueLabels is also a PUT, and 工蜂's AddIssueLabels
// is a labels-carrying PUT too, so a RemoveIssueLabel that silently stopped
// writing would otherwise pass.
func sawLabelRemoval(requests []recordedRequest) bool {
	for _, r := range requests {
		switch r.Method {
		case http.MethodDelete:
			return true
		case http.MethodPatch, http.MethodPut:
			if v, ok := r.Body["remove_labels"]; ok && nonEmptyBodyValue(v) {
				return true
			}
			if v, ok := r.Body["labels"]; ok && nonEmptyBodyValue(v) && labelsOmitName(v, "bug") {
				return true
			}
		}
	}
	return false
}

// labelsOmitName reports whether a recorded labels body value — a csv
// string (工蜂) or a JSON array of names — carries no element equal to
// name. Empty values don't count as evidence: the removal-as-replacement
// write must carry a surviving label set.
func labelsOmitName(v any, name string) bool {
	switch labels := v.(type) {
	case string:
		for _, l := range strings.Split(labels, ",") {
			if l == name {
				return false
			}
		}
		return true
	case []any:
		for _, l := range labels {
			if s, ok := l.(string); ok && s == name {
				return false
			}
		}
		return true
	}
	return false
}

// nonEmptyBodyValue reports whether a decoded body value carries content:
// strings must be non-empty, non-nil values of any other decoded type (e.g.
// a JSON array) count as present.
func nonEmptyBodyValue(v any) bool {
	if v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return s != ""
	}
	return true
}

// Alphanumeric (not just digits) because Gitee issue numbers are alphanumeric (e.g. I3XU7A).
var issuePathNum = regexp.MustCompile(`/issues/[A-Za-z0-9]+$`)

// usersLookupPath matches a username→ID resolution lookup: GitLab's
// ListUsers exact-match filter (/users?username=) and 工蜂's GetUser
// (/users/{username}).
var usersLookupPath = regexp.MustCompile(`/users(/[^/]+)?$`)

// commentsPage serves the comment-list fixture per the request's paging
// parameters: page one answers with a full synthetic page (so a backend
// that stops after the first page never sees the fixture comment and fails
// the merged-list assertion), page two carries the fixture comment, and
// later pages are empty. The per-page query parameter is `per_page` on
// GitHub-shaped APIs and `limit` on Gitea/Forgejo; an unparseable size
// defaults to 30 (the GitHub-shaped server default).
func (h IssuesHarness) commentsPage(r *http.Request) []byte {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(q.Get("per_page"))
	if size == 0 {
		size, _ = strconv.Atoi(q.Get("limit"))
	}
	if size <= 0 {
		size = 30
	}
	switch page {
	case 1:
		items := make([]string, 0, size)
		for i := 0; i < size; i++ {
			items = append(items, fmt.Sprintf(`{"id":%d,"body":"page one"}`, 1000+i))
		}
		return []byte("[" + strings.Join(items, ",") + "]")
	case 2:
		return []byte(h.CommentsResponse)
	default:
		return []byte(`[]`)
	}
}

// writeUserLookup answers a username→ID resolution lookup: echo the
// requested username back under a fixed numeric ID. GitLab's ListUsers
// filter arrives as ?username= on a bare /users path and expects an
// array; 工蜂's GetUser arrives as /users/{username} and expects a
// single object.
func writeUserLookup(w http.ResponseWriter, r *http.Request) {
	if name := r.URL.Query().Get("username"); name != "" {
		resp, _ := json.Marshal([]map[string]any{{"id": 101, "username": name}})
		_, _ = w.Write(resp)
		return
	}
	resp, _ := json.Marshal(map[string]any{"id": 101, "username": path.Base(r.URL.Path), "name": "Developer"})
	_, _ = w.Write(resp)
}

// labelsPage serves the repository-labels list fixture: page one carries
// LabelsResponse, later pages are empty (the walk terminator).
func (h IssuesHarness) labelsPage(r *http.Request) []byte {
	if page, _ := strconv.Atoi(r.URL.Query().Get("page")); page > 1 {
		return []byte(`[]`)
	}
	return []byte(h.LabelsResponse)
}

// issueStubServer returns the method/shape-routed recording mock.
func issueStubServer(h IssuesHarness) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeRecordedBody(r)
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.RequestURI(), Body: body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && usersLookupPath.MatchString(r.URL.Path):
			writeUserLookup(w, r)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "labels"):
			// Adding labels to an issue: GitHub-shaped APIs answer with the
			// label array (e.g. []*Label), not a single issue object.
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(h.LabelsResponse))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "labels"):
			// Replacing an issue's labels (side-endpoint PUT): same
			// label-array answer as the POST add above — an issue object
			// here breaks the []*Label parse.
			_, _ = w.Write([]byte(h.LabelsResponse))
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(h.MutateResponse))
		case r.Method == http.MethodPatch || r.Method == http.MethodPut:
			_, _ = w.Write([]byte(h.MutateResponse))
		case issuePathNum.MatchString(r.URL.Path):
			_, _ = w.Write([]byte(h.GetResponse))
		case strings.Contains(r.URL.Path, "comments"), strings.Contains(r.URL.Path, "notes"):
			_, _ = w.Write(h.commentsPage(r))
		case strings.Contains(r.URL.Path, "labels") && !strings.Contains(r.URL.Path, "issues"):
			_, _ = w.Write(h.labelsPage(r))
		default:
			// X-Total: 999 pins the total-semantics contract (issue #51):
			// a platform total that differs from the served page's item
			// count so ListIssues forwarding the header value instead of
			// len(result) fails the suite.
			w.Header().Set("X-Total", "999")
			_, _ = w.Write([]byte(h.ListResponse))
		}
	}))
	return srv, &requests
}

// assertBodyHas asserts that some request with one of the verbs carried want
// under key. Its arguments follow the convention verbs..., key, want: the
// last two variadic elements are the body key and expected value, everything
// before them the accepted HTTP methods.
func assertBodyHas(t *testing.T, requests *[]recordedRequest, methods ...string) {
	t.Helper()
	key, want := methods[len(methods)-2], methods[len(methods)-1]
	verbSet := methods[:len(methods)-2]
	for i := range *requests {
		r := &(*requests)[i]
		for _, m := range verbSet {
			if r.Method != m {
				continue
			}
			if v, ok := r.Body[key]; ok {
				if s, ok := v.(string); ok && strings.Contains(s, want) {
					return
				}
			}
		}
	}
	t.Errorf("expected a %v request whose body carries %q=%q; recorded: %v", verbSet, key, want, *requests)
}

// sawMethod reports whether any recorded request used one of the methods.
func sawMethod(requests []recordedRequest, methods ...string) bool {
	for _, r := range requests {
		for _, m := range methods {
			if r.Method == m {
				return true
			}
		}
	}
	return false
}
