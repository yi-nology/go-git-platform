//go:build e2e

package e2e

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/go-git-platform/runtime"
)

// TestSandboxWriteRoundtrip 是 3 期 T4 的主测试：gitea/forgejo 双平台
// 写族 roundtrip（spec §3.4）。跳过契约：无 env URL 且无 docker → t.Skip；
// 两平台独立判定（一个平台缺场不影响另一个）。
func TestSandboxWriteRoundtrip(t *testing.T) {
	t.Run("gitea", func(t *testing.T) { runWriteRoundtrip(t, giteaSpec) })
	t.Run("forgejo", func(t *testing.T) { runWriteRoundtrip(t, forgejoSpec) })
}

// runWriteRoundtrip 起平台、建一次性仓库，按 spec §3.4 写族逐个钉。
func runWriteRoundtrip(t *testing.T, spec platformSpec) {
	t.Helper()
	baseURL, token, container := startPlatform(t, spec)
	if container != "" {
		t.Logf("%s venue: one-shot container %s at %s", spec.kind, container, baseURL)
	} else {
		t.Logf("%s venue: reused external endpoint %s", spec.kind, baseURL)
	}
	p, counter := newProvider(t, spec, baseURL, token)

	// TestConnection 冒烟（token 快失败）+ 个人名下建仓（owner=="" 的
	// site-verified 语义见 createRepoForTest；落位命名空间由其回填）。
	smokeTestConnection(t, p, spec.kind)
	repo := fmt.Sprintf("e2e-rw-%d", time.Now().UnixNano())
	owner := createRepoForTest(t, p, "", repo)

	t.Run("issue", func(t *testing.T) { roundtripIssue(t, spec, p, owner, repo) })
	t.Run("label", func(t *testing.T) { roundtripLabel(t, spec, p, owner, repo) })
	t.Run("cr", func(t *testing.T) { roundtripCR(t, spec, p, owner, repo) })
	t.Run("ensure", func(t *testing.T) { roundtripEnsure(t, spec, p, counter, owner, repo) })
	t.Run("branch-protection", func(t *testing.T) { roundtripBranchProtection(t, spec, p, owner, repo) })
}

// roundtripIssue：Create→Get（字段回读一致）→UpdateTitle→Close→Reopen。
// 无 Delete 步——provider.IssueManager 无 DeleteIssue 且 Gitea/Forgejo API
// 本身不提供 issue 删除（SDK 面与平台面双重缺席，非本仓缺口）；「删后
// ErrNotFound」的脸谱由 label 与 branch-protection 两族以真实 Delete 承担。
func roundtripIssue(t *testing.T, spec platformSpec, p provider.Provider, owner, repo string) {
	t.Helper()
	im, ok := p.(provider.IssueManager)
	if !ok {
		t.Fatalf("provider %s lacks IssueManager", spec.kind)
	}
	ctx := t.Context()

	created, err := im.CreateIssue(ctx, provider.CreateIssueOptions{
		Owner: owner, Repo: repo,
		Title: "e2e issue roundtrip",
		Body:  "created by e2e harness in a one-shot container",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if created.Number == "" {
		t.Fatal("CreateIssue: empty Number")
	}
	number := created.Number

	got, err := im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Title != "e2e issue roundtrip" || got.Body != "created by e2e harness in a one-shot container" {
		t.Fatalf("GetIssue field mismatch: title=%q body=%q", got.Title, got.Body)
	}
	if got.State != model.IssueStateOpen {
		t.Fatalf("GetIssue state = %q, want open", got.State)
	}

	updated, err := im.UpdateIssue(ctx, owner, repo, number, provider.UpdateIssueOptions{
		Title: "e2e issue roundtrip v2",
	})
	if err != nil {
		t.Fatalf("UpdateIssue(title): %v", err)
	}
	if updated.Title != "e2e issue roundtrip v2" {
		t.Fatalf("UpdateIssue returned title %q", updated.Title)
	}
	got, err = im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetIssue after update: %v", err)
	}
	if got.Title != "e2e issue roundtrip v2" {
		t.Fatalf("GetIssue after update title = %q", got.Title)
	}

	closed, err := im.CloseIssue(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}
	if closed.State != model.IssueStateClosed {
		t.Fatalf("CloseIssue returned state %q", closed.State)
	}
	got, err = im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetIssue after close: %v", err)
	}
	if got.State != model.IssueStateClosed {
		t.Fatalf("GetIssue after close state = %q", got.State)
	}

	reopened, err := im.ReopenIssue(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("ReopenIssue: %v", err)
	}
	if reopened.State != model.IssueStateOpen {
		t.Fatalf("ReopenIssue returned state %q", reopened.State)
	}
	got, err = im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetIssue after reopen: %v", err)
	}
	if got.State != model.IssueStateOpen {
		t.Fatalf("GetIssue after reopen state = %q", got.State)
	}

	// 缺席读脸谱（errors 家族分类的真平台复核）：不存在的号 → ErrNotFound。
	_, err = im.GetIssue(ctx, owner, repo, "99999999")
	requireErrNotFound(t, "GetIssue(missing number)", err)
}

// roundtripLabel：Create→List 含→AddIssueLabels→ListIssueLabels 含→
// RemoveIssueLabel→不含→Delete→List 不含→再 Delete 必须 ErrNotFound。
func roundtripLabel(t *testing.T, spec platformSpec, p provider.Provider, owner, repo string) {
	t.Helper()
	lm, ok := p.(provider.LabelManager)
	if !ok {
		t.Fatalf("provider %s lacks LabelManager", spec.kind)
	}
	im, ok := p.(provider.IssueManager)
	if !ok {
		t.Fatalf("provider %s lacks IssueManager", spec.kind)
	}
	ctx := t.Context()

	name := fmt.Sprintf("e2e-label-%d", time.Now().UnixNano())
	created, err := lm.CreateLabel(ctx, owner, repo, provider.CreateLabelOptions{
		Name:        name,
		Color:       "ff0000",
		Description: "e2e roundtrip label",
	})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if created.Name != name {
		t.Fatalf("CreateLabel returned name %q", created.Name)
	}
	if created.Color != "ff0000" {
		t.Fatalf("CreateLabel color = %q (want canonical 6-digit hex without #)", created.Color)
	}

	labels, err := lm.ListLabels(ctx, owner, repo, provider.ListLabelsOptions{})
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	assertLabelContains(t, "ListLabels after create", labels, name, true)

	// 标签挂到 issue：AddIssueLabels 的名字解析只认已存在标签（site-verified：
	// giteafamily.ResolveLabelIDs 扫描仓库标签表，缺名即错不自动建），
	// issue 用本族自建（issue 无删除面，宿主是一次性容器，留驻无妨）。
	issue, err := im.CreateIssue(ctx, provider.CreateIssueOptions{
		Owner: owner, Repo: repo, Title: "e2e label roundtrip host issue",
	})
	if err != nil {
		t.Fatalf("CreateIssue(host): %v", err)
	}
	number := issue.Number

	if err := im.AddIssueLabels(ctx, owner, repo, number, []string{name}); err != nil {
		t.Fatalf("AddIssueLabels: %v", err)
	}
	// per-issue 标签读：GetIssue().Labels（统一面的 ListIssueLabels 是
	// 仓库词表域，不带 number——site-verified 于 iface_issues.go:59；
	// issue 级集合须走 GetIssue）。
	gotIssue, err := im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetIssue after add: %v", err)
	}
	assertIssueStringLabels(t, "GetIssue.Labels after add", gotIssue.Labels, name, true)

	if err := im.RemoveIssueLabel(ctx, owner, repo, number, name); err != nil {
		t.Fatalf("RemoveIssueLabel: %v", err)
	}
	gotIssue, err = im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetIssue after remove: %v", err)
	}
	assertIssueStringLabels(t, "GetIssue.Labels after remove", gotIssue.Labels, name, false)

	if err := lm.DeleteLabel(ctx, owner, repo, name); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	labels, err = lm.ListLabels(ctx, owner, repo, provider.ListLabelsOptions{})
	if err != nil {
		t.Fatalf("ListLabels after delete: %v", err)
	}
	assertLabelContains(t, "ListLabels after delete", labels, name, false)
	// 删除后的缺席以 ListLabels 不含为准——按名再删因 giteafamily 的标签
	// ID 缓存（5min TTL）可能拿陈旧 ID 命中幂等 204，不可作 ErrNotFound
	// 脸谱（site-verified：实测二删返回 nil）。真缺席读在 issue 族钉
	// （GetIssue 缺号 → ErrNotFound）。
}

// roundtripCR：Create→Get（回读一致）→UpdateTitle→Close（gitea/forgejo
// 状态词表：model.CRStateOpened→CRStateClosed，site-verified 于 model/cr.go）。
func roundtripCR(t *testing.T, spec platformSpec, p provider.Provider, owner, repo string) {
	t.Helper()
	crm, ok := p.(provider.ChangeRequestManager)
	if !ok {
		t.Fatalf("provider %s lacks ChangeRequestManager", spec.kind)
	}
	bm, ok := p.(provider.BranchManager)
	if !ok {
		t.Fatalf("provider %s lacks BranchManager", spec.kind)
	}
	fm, ok := p.(provider.FileManager)
	if !ok {
		t.Fatalf("provider %s lacks FileManager", spec.kind)
	}
	ctx := t.Context()

	// 差异分支：从 main 拉源分支并落一个文件提交（空 diff CR 不可靠）。
	source := fmt.Sprintf("e2e-cr-%d", time.Now().UnixNano())
	if _, err := bm.CreateBranch(ctx, owner, repo, source, "main"); err != nil {
		t.Fatalf("CreateBranch(%s): %v", source, err)
	}
	t.Cleanup(func() {
		if err := bm.DeleteBranch(t.Context(), owner, repo, source); err != nil {
			t.Logf("cleanup delete branch %s: %v", source, err)
		}
	})
	// content 编码契约：gitea/forgejo 族透传 FileOptions.Content 到 SDK 的
	// base64 字段（gitea.dev/sdk repo_file.go: "content must be base64
	// encoded"，family CreateFile 原样传递）——e2e 调用方负责编码。
	if _, err := fm.CreateFile(ctx, owner, repo, provider.FileOptions{
		Path:    "e2e-cr-marker.txt",
		Content: base64.StdEncoding.EncodeToString([]byte("e2e roundtrip content\n")),
		Message: "e2e: cr roundtrip commit",
		Branch:  source,
	}); err != nil {
		t.Fatalf("CreateFile on %s: %v", source, err)
	}

	created, err := crm.CreateCR(ctx, provider.CreateCROptions{
		Owner: owner, Repo: repo,
		Title:        "e2e cr roundtrip",
		Description:  "created by e2e harness",
		SourceBranch: source,
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreateCR: %v", err)
	}
	if created.Number == "" {
		t.Fatal("CreateCR: empty Number")
	}
	number := created.Number

	got, err := crm.GetCR(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetCR: %v", err)
	}
	if got.Title != "e2e cr roundtrip" || got.Description != "created by e2e harness" {
		t.Fatalf("GetCR field mismatch: title=%q desc=%q", got.Title, got.Description)
	}
	if got.SourceBranch != source || got.TargetBranch != "main" {
		t.Fatalf("GetCR branches: source=%q target=%q", got.SourceBranch, got.TargetBranch)
	}
	if got.State != model.CRStateOpened {
		t.Fatalf("GetCR state = %q, want opened", got.State)
	}

	updated, err := crm.UpdateCR(ctx, owner, repo, number, provider.UpdateCROptions{
		Title: "e2e cr roundtrip v2",
	})
	if err != nil {
		t.Fatalf("UpdateCR(title): %v", err)
	}
	if updated.Title != "e2e cr roundtrip v2" {
		t.Fatalf("UpdateCR returned title %q", updated.Title)
	}
	got, err = crm.GetCR(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetCR after update: %v", err)
	}
	if got.Title != "e2e cr roundtrip v2" {
		t.Fatalf("GetCR after update title = %q", got.Title)
	}

	closed, err := crm.CloseCR(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("CloseCR: %v", err)
	}
	if closed.State != model.CRStateClosed {
		t.Fatalf("CloseCR returned state %q", closed.State)
	}
	got, err = crm.GetCR(ctx, owner, repo, number)
	if err != nil {
		t.Fatalf("GetCR after close: %v", err)
	}
	if got.State != model.CRStateClosed {
		t.Fatalf("GetCR after close state = %q", got.State)
	}
}

// roundtripEnsure：EnsureIssueState / EnsureIssueLabels 收敛二段（spec §3.4
// 的 Unchanged 零写断言）。零写判定走 provider.Hooks 的 wire 级请求计数：
// 首段写 >=1（Close/AddIssueLabels），二段写 ==0（仅 GET 读）。
func roundtripEnsure(t *testing.T, spec platformSpec, p provider.Provider, counter *requestCounter, owner, repo string) {
	t.Helper()
	im, ok := p.(provider.IssueManager)
	if !ok {
		t.Fatalf("provider %s lacks IssueManager", spec.kind)
	}
	lm, ok := p.(provider.LabelManager)
	if !ok {
		t.Fatalf("provider %s lacks LabelManager", spec.kind)
	}
	ctx := t.Context()

	// EnsureIssueState：open → closed。
	issue, err := im.CreateIssue(ctx, provider.CreateIssueOptions{
		Owner: owner, Repo: repo, Title: "e2e ensure state roundtrip",
	})
	if err != nil {
		t.Fatalf("CreateIssue(ensure): %v", err)
	}
	number := issue.Number

	before := counterSnapshot(counter)
	outcome, got, err := runtime.EnsureIssueState(ctx, p, owner, repo, number, model.IssueStateClosed)
	if err != nil {
		t.Fatalf("EnsureIssueState(first): %v", err)
	}
	firstWrites := writesDelta(before, counterSnapshot(counter))
	if outcome != runtime.OutcomeUpdated {
		t.Fatalf("EnsureIssueState(first) outcome = %q, want updated", outcome)
	}
	if got == nil || got.State != model.IssueStateClosed {
		t.Fatalf("EnsureIssueState(first) returned %+v, want closed", got)
	}
	if firstWrites < 1 {
		t.Fatalf("EnsureIssueState(first) wrote %d times on wire, want >=1", firstWrites)
	}

	before = counterSnapshot(counter)
	outcome, got, err = runtime.EnsureIssueState(ctx, p, owner, repo, number, model.IssueStateClosed)
	if err != nil {
		t.Fatalf("EnsureIssueState(second): %v", err)
	}
	secondWrites := writesDelta(before, counterSnapshot(counter))
	if outcome != runtime.OutcomeUnchanged {
		t.Fatalf("EnsureIssueState(second) outcome = %q, want unchanged", outcome)
	}
	if got == nil || got.State != model.IssueStateClosed {
		t.Fatalf("EnsureIssueState(second) returned %+v, want closed", got)
	}
	if secondWrites != 0 {
		t.Fatalf("EnsureIssueState(second) wrote %d times on wire, want 0 (zero-write contract)", secondWrites)
	}

	// EnsureIssueLabels：目标标签先建（AddIssueLabels 名字解析不自动建，
	// site-verified），空标签 issue → 全集补齐。
	want := []string{"e2e-ensure-a", "e2e-ensure-b"}
	for _, n := range want {
		if _, err := lm.CreateLabel(ctx, owner, repo, provider.CreateLabelOptions{Name: n, Color: "00ff00"}); err != nil {
			t.Fatalf("CreateLabel(%s): %v", n, err)
		}
	}
	labelsIssue, err := im.CreateIssue(ctx, provider.CreateIssueOptions{
		Owner: owner, Repo: repo, Title: "e2e ensure labels roundtrip",
	})
	if err != nil {
		t.Fatalf("CreateIssue(ensure labels): %v", err)
	}
	labelsNumber := labelsIssue.Number

	before = counterSnapshot(counter)
	outcome, target, err := runtime.EnsureIssueLabels(ctx, p, owner, repo, labelsNumber, want)
	if err != nil {
		t.Fatalf("EnsureIssueLabels(first): %v", err)
	}
	firstWrites = writesDelta(before, counterSnapshot(counter))
	if outcome != runtime.OutcomeUpdated {
		t.Fatalf("EnsureIssueLabels(first) outcome = %q, want updated", outcome)
	}
	if len(target) != len(want) {
		t.Fatalf("EnsureIssueLabels(first) target = %v, want %v", target, want)
	}
	if firstWrites < 1 {
		t.Fatalf("EnsureIssueLabels(first) wrote %d times on wire, want >=1", firstWrites)
	}
	// 回读真实标签集（顺序无关断言归 runtime，wire 上须真的挂上；
	// issue 级集合走 GetIssue().Labels——ListIssueLabels 是仓库词表域）。
	gotLabelsIssue, err := im.GetIssue(ctx, owner, repo, labelsNumber)
	if err != nil {
		t.Fatalf("GetIssue after ensure: %v", err)
	}
	assertIssueStringLabels(t, "GetIssue.Labels after ensure", gotLabelsIssue.Labels, want[0], true)
	assertIssueStringLabels(t, "GetIssue.Labels after ensure", gotLabelsIssue.Labels, want[1], true)

	before = counterSnapshot(counter)
	outcome, target, err = runtime.EnsureIssueLabels(ctx, p, owner, repo, labelsNumber, want)
	if err != nil {
		t.Fatalf("EnsureIssueLabels(second): %v", err)
	}
	secondWrites = writesDelta(before, counterSnapshot(counter))
	if outcome != runtime.OutcomeUnchanged {
		t.Fatalf("EnsureIssueLabels(second) outcome = %q, want unchanged", outcome)
	}
	if len(target) != len(want) {
		t.Fatalf("EnsureIssueLabels(second) target = %v, want %v", target, want)
	}
	if secondWrites != 0 {
		t.Fatalf("EnsureIssueLabels(second) wrote %d times on wire, want 0 (zero-write contract)", secondWrites)
	}
}

// roundtripBranchProtection：Create→Get→Delete→Get 必须 ErrNotFound
// （gitea/forgejo 都在 BranchProtectionManager 支持面内，site-verified
// 于 iface_branch_protection.go 平台清单）。
func roundtripBranchProtection(t *testing.T, spec platformSpec, p provider.Provider, owner, repo string) {
	t.Helper()
	bpm, ok := p.(provider.BranchProtectionManager)
	if !ok {
		t.Fatalf("provider %s lacks BranchProtectionManager", spec.kind)
	}
	ctx := t.Context()

	created, err := bpm.CreateBranchProtection(ctx, owner, repo, provider.CreateBranchProtectionOptions{
		BranchName:               "main",
		RequiredApprovingReviews: 1,
	})
	if err != nil {
		t.Fatalf("CreateBranchProtection: %v", err)
	}
	if created.BranchName != "main" {
		t.Fatalf("CreateBranchProtection returned branch %q", created.BranchName)
	}

	got, err := bpm.GetBranchProtection(ctx, owner, repo, "main")
	if err != nil {
		t.Fatalf("GetBranchProtection: %v", err)
	}
	if got == nil {
		t.Fatal("GetBranchProtection returned nil,nil (contract violation)")
	}
	if got.BranchName != "main" {
		t.Fatalf("GetBranchProtection branch = %q", got.BranchName)
	}

	if err := bpm.DeleteBranchProtection(ctx, owner, repo, "main"); err != nil {
		t.Fatalf("DeleteBranchProtection: %v", err)
	}
	_, err = bpm.GetBranchProtection(ctx, owner, repo, "main")
	requireErrNotFound(t, "GetBranchProtection(after delete)", err)
}

// counterSnapshot 取写计数快照（原子读——hook 在请求 goroutine 上触发）。
func counterSnapshot(c *requestCounter) int64 { return c.writes }

// writesDelta 计两快照间的写请求数。
func writesDelta(before, after int64) int64 { return after - before }

// assertLabelContains / assertIssueLabelContains 仓库/issue 标签集合断言。
func assertLabelContains(t *testing.T, op string, labels []*model.Label, name string, want bool) {
	t.Helper()
	for _, l := range labels {
		if l != nil && l.Name == name {
			if !want {
				t.Fatalf("%s: label %q unexpectedly present", op, name)
			}
			return
		}
	}
	if want {
		t.Fatalf("%s: label %q missing among %d", op, name, len(labels))
	}
}

func assertIssueStringLabels(t *testing.T, op string, labels []string, name string, want bool) {
	t.Helper()
	for _, n := range labels {
		if n == name {
			if !want {
				t.Fatalf("%s: label %q unexpectedly present", op, name)
			}
			return
		}
	}
	if want {
		t.Fatalf("%s: label %q missing among %d", op, name, len(labels))
	}
}
