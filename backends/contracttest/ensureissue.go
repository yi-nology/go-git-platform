package contracttest

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// EnsureIssueSuite 钉两个新收敛函数的 wire 级行为契约（2B spec §1.2 body
// 「contracttest 按后端桩做 wire 级行为契约（Unchanged 零写请求 / Updated
// 恰一次写 / 缺能力哨兵）」与 §四验收「两个新收敛函数有 wire 级行为契约」）：
//
//   - Unchanged 零写：want == 桩已报现状 → OutcomeUnchanged 且写计数 0；
//   - Updated 恰写：want == 相反态（State）/差集目标（Labels）→
//     OutcomeUpdated 且写计数 = 观察到的确切值——本仓 7 后端两条路径都恰
//     1 个写请求（State：Close/Reopen 单次写；Labels 加路径：批量
//     AddIssueLabels 一次调用 = 一个写请求，删路径：每个 RemoveIssueLabel
//     一个写请求、夹具下至多删 1 个标签；tencentcode 删路径先 GET 后 PUT、
//     gitea/forgejo 加路径先 GET 标签表再 POST——GET 不计入写数）；
//   - 缺能力哨兵（ErrNotImplemented）是 unit-only：7 个后端全部实现
//     IssueManager，wire 面触发不了——已由 provider/ensure_issue_test.go
//     case 4「缺 IssueManager 能力 NotImplemented 零 HTTP」覆盖。本套件
//     保留 IssueManager 缺失时的 t.Skipf 守卫（同 errorclass.go）。
//
// 桩复用 issues.go 的 issueStubServer：它按 7 后端各自的 wire 形状路由
// （GetResponse/MutateResponse/LabelsResponse 夹具 +
// writeUserLookup/labelsPage 助手），且先记录后路由——每个请求都进
// recordedRequest，写计数 = 记录中非 GET/HEAD 的请求数（等价 handler 内
// 计数，另留 path/body 可断言）。不用 stubHandler：它不能按方法计数
// （同 ErrorClassSuite/OperationSuite 自带 httptest 的缘由）。
// 现状快照（读 want 基准）在一次性桩上取——夹具静态、四个子测共用，被测
// 计数桩只承载 Ensure 调用本体，计数记录纯净可断言。
func testEnsureIssueSuite(t *testing.T, h Harness) {
	t.Helper()
	if h.Issues == nil {
		t.Skipf("%s provides no Issues config — no issue fixtures to serve", h.Name)
	}
	ih := IssuesHarness{
		Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
		IssuesHarnessConfig: *h.Issues,
	}
	cur := ensureIssueSnapshot(t, h, ih)
	repoNames := repoLabelNames(t, ih.LabelsResponse)

	t.Run("State_Unchanged_ZeroWrites", func(t *testing.T) { assertEnsureStateUnchanged(t, h, ih, cur) })
	t.Run("State_Updated_OneWrite", func(t *testing.T) { assertEnsureStateUpdated(t, h, ih, cur) })
	t.Run("Labels_Unchanged_ZeroWrites", func(t *testing.T) { assertEnsureLabelsUnchanged(t, h, ih, cur) })
	t.Run("Labels_Updated_OneWrite", func(t *testing.T) { assertEnsureLabelsUpdated(t, h, ih, cur, repoNames) })
}

// ensureIssueSnapshot 在一次性桩上读现状（夹具静态，四子测共用）并兼作
// IssueManager 守卫：缺能力整套 Skip（同 errorclass.go）。
func ensureIssueSnapshot(t *testing.T, h Harness, ih IssuesHarness) *provider.Issue {
	t.Helper()
	srv, _ := issueStubServer(ih)
	t.Cleanup(srv.Close)
	p := h.NewProvider(t, baseCfg(h, srv.URL))
	im, ok := p.(provider.IssueManager)
	if !ok {
		t.Skipf("%s lacks IssueManager — no wire method to drive", h.Name)
	}
	cur, err := im.GetIssue(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatalf("GetIssue (fixture snapshot): %v", err)
	}
	return cur
}

// ensureIssueStub 建被测计数桩：issueStubServer 先记录后路由，记录只含
// Ensure 调用本体的流量。
func ensureIssueStub(t *testing.T, h Harness, ih IssuesHarness) (provider.Provider, *[]recordedRequest) {
	t.Helper()
	srv, requests := issueStubServer(ih)
	t.Cleanup(srv.Close)
	return h.NewProvider(t, baseCfg(h, srv.URL)), requests
}

// assertEnsureStateUnchanged 钉 State 收敛的 Unchanged 面：want == 现状
// → OutcomeUnchanged、零写请求、且确实先读了现状（GET ≥ 1）。
func assertEnsureStateUnchanged(t *testing.T, h Harness, ih IssuesHarness, cur *provider.Issue) {
	t.Helper()
	ctx := context.Background()
	p, requests := ensureIssueStub(t, h, ih)
	outcome, got, err := provider.EnsureIssueState(ctx, p, "owner", "repo", "1", cur.State)
	if err != nil {
		t.Fatalf("EnsureIssueState(want=%q): %v", cur.State, err)
	}
	if outcome != provider.OutcomeUnchanged {
		t.Errorf("outcome = %q, want %q", outcome, provider.OutcomeUnchanged)
	}
	if got == nil {
		t.Error("Unchanged must return the read snapshot, got nil")
	}
	if n := countWrites(*requests); n != 0 {
		t.Fatalf("Unchanged must issue zero write requests, got %d (%s)", n, methodsOf(*requests))
	}
	if n := countGETs(requests); n < 1 {
		t.Errorf("EnsureIssueState must read the current state first, got %d GETs", n)
	}
}

// assertEnsureStateUpdated 钉 State 收敛的 Updated 面：want == 相反态 →
// OutcomeUpdated、恰 1 个写请求（Close/Reopen 单次写）、先读后写。
func assertEnsureStateUpdated(t *testing.T, h Harness, ih IssuesHarness, cur *provider.Issue) {
	t.Helper()
	if cur.State != provider.IssueStateOpen && cur.State != provider.IssueStateClosed {
		t.Fatalf("fixture state %q is neither open nor closed", cur.State)
	}
	want := provider.IssueStateClosed
	if cur.State == provider.IssueStateClosed {
		want = provider.IssueStateOpen
	}
	ctx := context.Background()
	p, requests := ensureIssueStub(t, h, ih)
	outcome, got, err := provider.EnsureIssueState(ctx, p, "owner", "repo", "1", want)
	if err != nil {
		t.Fatalf("EnsureIssueState(want=%q): %v", want, err)
	}
	if outcome != provider.OutcomeUpdated {
		t.Errorf("outcome = %q, want %q", outcome, provider.OutcomeUpdated)
	}
	if got == nil {
		t.Error("Updated must return the write result, got nil")
	}
	if n := countWrites(*requests); n != 1 {
		t.Fatalf("Updated must issue exactly 1 write request, got %d (%s)", n, methodsOf(*requests))
	}
	if n := countGETs(requests); n < 1 {
		t.Errorf("EnsureIssueState must read the current state first, got %d GETs", n)
	}
}

// assertEnsureLabelsUnchanged 钉 Labels 收敛的 Unchanged 面：want == 现状
// 集合 → OutcomeUnchanged、零写请求、返回集合与 want 相等、先读现状。
func assertEnsureLabelsUnchanged(t *testing.T, h Harness, ih IssuesHarness, cur *provider.Issue) {
	t.Helper()
	ctx := context.Background()
	want := append([]string(nil), cur.Labels...)
	p, requests := ensureIssueStub(t, h, ih)
	outcome, got, err := provider.EnsureIssueLabels(ctx, p, "owner", "repo", "1", want)
	if err != nil {
		t.Fatalf("EnsureIssueLabels(want=%v): %v", want, err)
	}
	if outcome != provider.OutcomeUnchanged {
		t.Errorf("outcome = %q, want %q", outcome, provider.OutcomeUnchanged)
	}
	if !sameLabelSet(got, want) {
		t.Errorf("returned labels = %v, want set-equal to %v", got, want)
	}
	if n := countWrites(*requests); n != 0 {
		t.Fatalf("Unchanged must issue zero write requests, got %d (%s)", n, methodsOf(*requests))
	}
	if n := countGETs(requests); n < 1 {
		t.Errorf("EnsureIssueLabels must read the current labels first, got %d GETs", n)
	}
}

// assertEnsureLabelsUpdated 钉 Labels 收敛的 Updated 面：差集目标 →
// OutcomeUpdated、恰 1 个写请求、返回目标与 want 集合相等、先读后写。
// 目标挑法（updatedLabelsWant）保证加/删两条 wire 路径都被 7 后端覆盖到。
func assertEnsureLabelsUpdated(t *testing.T, h Harness, ih IssuesHarness, cur *provider.Issue, repoNames []string) {
	t.Helper()
	want := updatedLabelsWant(cur.Labels, repoNames)
	if sameLabelSet(want, cur.Labels) {
		t.Skipf("fixture yields no label delta (current=%v repo=%v) — cannot drive Updated", cur.Labels, repoNames)
	}
	ctx := context.Background()
	p, requests := ensureIssueStub(t, h, ih)
	outcome, got, err := provider.EnsureIssueLabels(ctx, p, "owner", "repo", "1", want)
	if err != nil {
		t.Fatalf("EnsureIssueLabels(want=%v): %v", want, err)
	}
	if outcome != provider.OutcomeUpdated {
		t.Errorf("outcome = %q, want %q", outcome, provider.OutcomeUpdated)
	}
	if !sameLabelSet(got, want) {
		t.Errorf("convergence target = %v, want set-equal to %v", got, want)
	}
	if n := countWrites(*requests); n != 1 {
		t.Fatalf("Updated must issue exactly 1 write request, got %d (%s)", n, methodsOf(*requests))
	}
	if n := countGETs(requests); n < 1 {
		t.Errorf("EnsureIssueLabels must read the current labels first, got %d GETs", n)
	}
}

// countWrites counts the recorded non-GET/HEAD requests — the write set the
// converge contracts assert on. issueStubServer records before routing, so
// every write is counted exactly once (GETs are reads: the pre-write
// GetIssue, gitea/forgejo's label-name→ID scan, tencentcode's read-modify).
func countWrites(requests []recordedRequest) int {
	n := 0
	for i := range requests {
		switch requests[i].Method {
		case http.MethodGet, http.MethodHead:
		default:
			n++
		}
	}
	return n
}

// sameLabelSet 判定两侧标签集合相等（顺序与各自重复无关）——与
// provider 侧 labelSetEqual 同语义的测试侧独立书写（wire 契约不复用
// 被测实现的判定）。
func sameLabelSet(a, b []string) bool {
	set := make(map[string]struct{}, len(a))
	for _, l := range a {
		set[l] = struct{}{}
	}
	seen := make(map[string]struct{}, len(b))
	for _, l := range b {
		if _, ok := set[l]; !ok {
			return false
		}
		seen[l] = struct{}{}
	}
	return len(seen) == len(set)
}

// repoLabelNames 从后端的仓库标签夹具（LabelsResponse）抽出标签名——
// Updated 加路径必须用仓内真实存在的名字（gitea/forgejo 按名解析 ID，
// 捏造的名字会解析失败），故候选只能来自夹具。
func repoLabelNames(t *testing.T, fixture string) []string {
	t.Helper()
	var labels []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(fixture), &labels); err != nil {
		t.Fatalf("parse LabelsResponse fixture: %v", err)
	}
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Name != "" {
			names = append(names, l.Name)
		}
	}
	return names
}

// updatedLabelsWant 挑「需要写入」的目标全集：优先加路径——挑一个仓标签
// 夹具里存在、但不在 current 里的名字（current ∪ {候选}）；挑不到
// （current 已含全部仓标签，如 GitLab 夹具 current=[bug]、仓标签只有
// bug）走删路径——current 去掉最后一个。current 与仓标签同时为空的组合
// 不存在（7 后端 LabelsResponse 至少含 "bug"），防御性返回 current 让
// 调用方以 Skip 收口。
func updatedLabelsWant(current, repoNames []string) []string {
	inCur := make(map[string]struct{}, len(current))
	for _, l := range current {
		inCur[l] = struct{}{}
	}
	for _, n := range repoNames {
		if _, ok := inCur[n]; !ok {
			return append(append([]string(nil), current...), n)
		}
	}
	if len(current) == 0 {
		return append([]string(nil), current...)
	}
	return append([]string(nil), current[:len(current)-1]...)
}
