# Phase 2A：Update 三态语义 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `UpdateIssueOptions`/`UpdateCROptions` 支持三态（未指定/置空/置值）——新增 `XxxPatch` 指针字段族 + `ErrClearNotSupported` 哨兵，7 后端接线 + contracttest 三态行为契约，0.x 零 breaking。

**Architecture:** provider 层泛型 `ResolvePatch` + 每字段解析方法（旧字段语义逐字不变、双字段同设 patch 优先）→ contracttest 新增 `ClearExpect` 期望驱动的三态断言（A 级钉 wire 在场空值、B 级钉哨兵+零请求）→ 6 个后端任务逐个接线（GitHub 兼修 State 静默丢弃 bug）→ 台账再生成 + CHANGELOG + 全量验证。

**Tech Stack:** Go 1.27.4；平台 SDK wire 形状以 module cache 源码为准（能力矩阵已由 spec 勘察定稿）。

**Spec:** [docs/superpowers/specs/2026-11-12-phase2a-tristate-design.md](../specs/2026-11-12-phase2a-tristate-design.md)——**A/B 能力矩阵是本计划的逐后端接线依据**；冲突以 spec 为准。

## Global Constraints

- **旧字段行为逐字不变**：legacy 空 = 不动、非空 = 置值；旧字段路径永不返回 `ErrClearNotSupported`（即使平台 B 级——B 级哨兵只由 patch 字段的清空请求触发）。
- **patch 语义**：`nil` = 不动（回落旧字段解析）；非 nil = 全量替换（空值 = 清空）；双字段同设 patch 优先（spec 裁决 3）。
- **B 级处置**：发请求**前**返回 `oekiterrors.Wrap(ErrClearNotSupported, msg, "platform", …, "field", …)`（整单失败，不部分执行）；`errors.Is` 可判；同步 divergence 登记。
- 错误处理/日志/并发遵守 AGENTS.md 规则一（`fmt.Errorf` `%w`、`oekiterrors.Wrap`、禁裸 `go func`）；提交中文 conventional；`mcp/` 独立模块（本计划不碰 mcp）。
- **contracttest 是 spec**：新增断言即新契约；改既有断言必须在提交信息论证。
- 收尾验证（T8 全跑）：`make check` → `make check-mcp` → 双模块 `golangci-lint run ./...` → 覆盖率 ≥45% → 双模块 `GOTOOLCHAIN=go1.27.4 govulncheck ./...`。
- **旧字段注释用散文引导（「三态请用 XxxPatch；本字段维持空=不变旧语义」），不写机器可读 `Deprecated:` 标记**——mcp 工具等合法消费方继续用旧字段（「参数未提供=不变」正是其语义），`Deprecated:` 会触发 SA1019 噪音；迁移说明进 CHANGELOG。
- 每个后端 Task 必须跑该后端包测试（含 contracttest 套件）；全部 `-race -count=1`。

---

### Task 1: provider 三态核心（字段 + 解析 + 哨兵）

**Files:**
- Create: `provider/patch.go`
- Modify: `provider/options.go`（6 个新字段 + 包 doc 三段改写 + 旧字段注释追加）
- Modify: `provider/errors.go`（哨兵，加在既有哨兵块 `ErrPlatformNotSupported` 之后）
- Test: `provider/patch_test.go`（新建）

**Interfaces:**
- Produces（后续全部 Task 消费）:
  - `func ResolvePatch[T any](patch *T, legacy T, isEmpty func(T) bool) (val T, specified bool)`
  - `func (o UpdateIssueOptions) ResolveBody() (string, bool)` / `ResolveAssignees() ([]string, bool)` / `ResolveLabels() ([]string, bool)` / `ResolveMilestone() (string, bool)`
  - `func (o UpdateCROptions) ResolveDescription() (string, bool)` / `ResolveAssignees() ([]string, bool)`
  - `var ErrClearNotSupported = errors.New("clear not supported on this platform")`
  - 新字段：`UpdateIssueOptions{BodyPatch *string; AssigneesPatch *[]string; LabelsPatch *[]string; MilestonePatch *string}`、`UpdateCROptions{DescriptionPatch *string; AssigneesPatch *[]string}`

- [ ] **Step 1: 写失败测试（TDD）**

新建 `provider/patch_test.go`：

```go
package provider

import (
	"errors"
	"testing"
)

func TestResolvePatch(t *testing.T) {
	isEmptyStr := func(s string) bool { return s == "" }
	isEmptySlice := func(s []string) bool { return len(s) == 0 }

	cases := []struct {
		name          string
		patch         *string
		legacy        string
		wantVal       string
		wantSpecified bool
	}{
		{"nil patch + empty legacy = 不动", nil, "", "", false},
		{"nil patch + non-empty legacy = 旧字段置值", nil, "old", "old", true},
		{"patch 置值覆盖 legacy", ptr("new"), "old", "new", true},
		{"patch 空串 = 显式清空（覆盖非空 legacy）", ptr(""), "old", "", true},
		{"patch 指向空串 + 空 legacy = 清空", ptr(""), "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			val, ok := ResolvePatch(c.patch, c.legacy, isEmptyStr)
			if val != c.wantVal || ok != c.wantSpecified {
				t.Fatalf("got (%q, %v), want (%q, %v)", val, ok, c.wantVal, c.wantSpecified)
			}
		})
	}

	// 切片形：空切片 patch = 清空
	sp := []string{}
	val, ok := ResolvePatch(&sp, []string{"a"}, isEmptySlice)
	if !ok || len(val) != 0 {
		t.Fatalf("empty slice patch: got (%v, %v), want ( [], true )", val, ok)
	}
}

func ptr[T any](v T) *T { return &v }

func TestErrClearNotSupported(t *testing.T) {
	err := oekiterrors.Wrap(ErrClearNotSupported, "clear milestone",
		"platform", "github", "field", "milestone")
	if !errors.Is(err, ErrClearNotSupported) {
		t.Fatalf("wrapped sentinel must be Is-matchable: %v", err)
	}
}
```

（import：`errors`、`testing`、`oekiterrors "github.com/oekit/pkg/errors"`；`ptr` 辅助在本文件定义。）

- [ ] **Step 2: 验证失败**

Run: `cd /Users/zhangyi/my_project/go-git-platform && go test ./provider/ -run 'TestResolvePatch|TestErrClearNotSupported'`
Expected: FAIL（未定义符号）。

- [ ] **Step 3: 实现 provider/patch.go**

```go
package provider

// ResolvePatch 是 Update*Options 三态解析核心（2A，spec
// 2026-11-12-phase2a-tristate-design）：
//
//   - patch 非 nil：全量替换——返回 *patch，specified=true；空值（"" /
//     空切片）即显式清空；
//   - patch nil：回落旧字段语义——isEmpty(legacy) 时 specified=false（不
//     动），否则返回 legacy、specified=true（置值）；
//   - 双字段同设：patch 立即返回（优先，spec 裁决 3）。
//
// specified=false → 后端不动该 wire 字段；specified && isEmpty(val) →
// 清空（平台 B 级则返回 ErrClearNotSupported）；specified && !isEmpty →
// 置值。
func ResolvePatch[T any](patch *T, legacy T, isEmpty func(T) bool) (val T, specified bool) {
	if patch != nil {
		return *patch, true
	}
	if isEmpty(legacy) {
		var zero T
		return zero, false
	}
	return legacy, true
}

// --- UpdateIssueOptions 三态解析（value receiver，只读）---

func (o UpdateIssueOptions) ResolveBody() (string, bool) {
	return ResolvePatch(o.BodyPatch, o.Body, func(s string) bool { return s == "" })
}

func (o UpdateIssueOptions) ResolveAssignees() ([]string, bool) {
	return ResolvePatch(o.AssigneesPatch, o.Assignees, func(s []string) bool { return len(s) == 0 })
}

func (o UpdateIssueOptions) ResolveLabels() ([]string, bool) {
	return ResolvePatch(o.LabelsPatch, o.Labels, func(s []string) bool { return len(s) == 0 })
}

func (o UpdateIssueOptions) ResolveMilestone() (string, bool) {
	return ResolvePatch(o.MilestonePatch, o.Milestone, func(s string) bool { return s == "" })
}

// --- UpdateCROptions 三态解析 ---

func (o UpdateCROptions) ResolveDescription() (string, bool) {
	return ResolvePatch(o.DescriptionPatch, o.Description, func(s string) bool { return s == "" })
}

func (o UpdateCROptions) ResolveAssignees() ([]string, bool) {
	return ResolvePatch(o.AssigneesPatch, o.Assignees, func(s []string) bool { return len(s) == 0 })
}
```

- [ ] **Step 4: options.go 加字段 + 改 doc**

`UpdateIssueOptions`（现 :399-406）追加（字段注释逐字）：

```go
	// BodyPatch 三态正文（2A）：nil = 不动；非 nil = 全量替换，"" = 清空。
	// 与 Body 同设时本字段优先。平台无法表达清空（A/B 矩阵 B 级）时
	// UpdateIssue 返回 ErrClearNotSupported。
	BodyPatch *string `json:"body_patch,omitempty"`
	// AssigneesPatch 三态指派人：nil = 不动；非 nil = 全量替换，
	// 空切片 = 清空。语义同 BodyPatch。
	AssigneesPatch *[]string `json:"assignees_patch,omitempty"`
	// LabelsPatch 三态标签：nil = 不动；非 nil = 全量替换，空 = 清空。
	LabelsPatch *[]string `json:"labels_patch,omitempty"`
	// MilestonePatch 三态里程碑：nil = 不动；非 nil = 全量替换，
	// "" = 清空（各平台映射 null/0，见能力矩阵）。
	MilestonePatch *string `json:"milestone_patch,omitempty"`
```

旧字段 `Assignees`/`Labels`/`Milestone`/`Body` 注释**尾部追加**（保留原注释）：

```go
	//
	// 三态（2A 起）：清空/置空用 XxxPatch（见上）；本字段语义不变
	// （空 = 不动），保留为兼容路径——MCP 工具「参数未提供 = 不变」
	// 即此语义。
```

`UpdateCROptions`（:100-108）同款追加 `DescriptionPatch *string`、`AssigneesPatch *[]string`（注释同上形）。

**包 doc 改写**（options.go:10-16 现文「zero-value fields … no way to explicitly set a field to its zero value」）：

```go
//   - Update*Options（e.g. UpdateIssueOptions, UpdateCROptions）：旧字段
//     的零值（空串、false）仍表示 "leave unchanged"。2A 起配套 XxxPatch
//     指针字段提供三态：nil = 不动、非 nil = 全量替换（空值 = 显式清空），
//     双字段同设时 patch 优先——见 provider/patch.go ResolvePatch。
```

- [ ] **Step 5: errors.go 加哨兵**

在 `ErrPlatformNotSupported` 之后：

```go
	// ErrClearNotSupported 是三态 Update 的 B 级哨兵（2A 能力矩阵）：
	// 调用方经 XxxPatch 显式要求清空，但该平台+字段的 wire 无法表达
	// 清空（结构不可表达且无侧端点，或既有登记 no-op）。只由 patch
	// 字段触发；旧字段路径永不返回。kv 携带 platform/op/field。
	ErrClearNotSupported = errors.New("clear not supported on this platform")
```

- [ ] **Step 6: 跑测试 + Commit**

```bash
go test -race -count=1 ./provider/
git add provider/patch.go provider/patch_test.go provider/options.go provider/errors.go
git commit -m "feat(provider): Update 三态核心——ResolvePatch 泛型 + 六解析方法 + XxxPatch 字段族 + ErrClearNotSupported 哨兵（2A）"
```

---

### Task 2: contracttest 三态 harness + GitHub 接线（含 State 修复）

**Files:**
- Modify: `backends/contracttest/issues.go`（ClearExpect + 三态断言 + IssuesHarnessConfig 字段 + RunIssuesSuite 挂接）
- Modify: `backends/contracttest/crs.go`（CR 侧同款）
- Modify: `backends/contracttest/contracttest.go`（`assertBodyLacks`/`assertBodyHasEmpty` 辅助，若既有辅助可扩展则就地扩展）
- Modify: `backends/github/issues.go:86-116`、`backends/github/crs.go:146-174`（接线 + State 修复）
- Modify: `backends/github/contract_test.go`（harness 配置）
- Test: 既有 `backends/github/contract_test.go`（套件即门）

**Interfaces:**
- Consumes: Task 1 全部产出。
- Produces（T3-T7 消费）:
  - `type ClearExpect struct { Unsupported bool; Method string; BodyField string; PathContains string }`
  - `IssuesHarnessConfig.IssueClear map[string]ClearExpect`（键：`body`/`assignees`/`labels`/`milestone`）
  - CR 配置类型（现场读 `crs.go` 配置类型名）上新增 `CRClear map[string]ClearExpect`（键：`description`/`assignees`）
  - 辅助：`assertBodyLacks`、`assertBodyHasEmpty`

- [ ] **Step 1: 读现场结构**

```bash
grep -n "type recordedRequest" -r backends/contracttest/
grep -n "type.*CR.*Config struct\|func RunCR" backends/contracttest/crs.go | head
sed -n '93,165p' backends/contracttest/issues.go   # RunIssuesSuite 全体
```
记录：recordedRequest 的字段名（Method/Path/Body 形态）、CR 配置类型名、RunIssuesSuite 内 assert 调用点与 harness 配置可达性。

- [ ] **Step 2: 写 harness（ClearExpect + 断言 + 配置字段）**

`issues.go` 新增（字段名以 Step 1 实测适配）：

```go
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
```

`IssuesHarnessConfig` 追加：

```go
	// IssueClear 声明各字段清空请求的期望（键：body/assignees/labels/
	// milestone）。nil = 四字段全部必须可清空（全 A 级平台可不设）。
	IssueClear map[string]ClearExpect
```

`assertIssueUpdateTriState`（挂接在 `assertIssueUpdateWire` 调用之后，issues.go:159 附近）：

```go
// assertIssueUpdateTriState 钉 UpdateIssue 三态（2A spec）：
// unset 不得携带字段、legacy 置值回归、patch 置值与 legacy 同形、
// 每字段清空按 ClearExpect 分派（B 级哨兵 + 零请求 / A 级 wire 在场空值）。
func assertIssueUpdateTriState(t *testing.T, im provider.IssueManager, requests *[]recordedRequest, clear map[string]ClearExpect) {
	t.Helper()
	ctx := context.Background()

	// 1) unset：全零 options——若产生写请求，body 不得含四字段。
	before := len(*requests)
	if _, err := im.UpdateIssue(ctx, "owner", "repo", "1", provider.UpdateIssueOptions{}); err != nil {
		t.Fatalf("unset UpdateIssue: %v", err)
	}
	for _, f := range []string{"body", "assignees", "labels", "milestone"} {
		if len(*requests) > before {
			assertBodyLacks(t, requests, f) // 只查最新请求
		}
	}

	// 2) legacy 置值回归（旧行为不许破）。
	if _, err := im.UpdateIssue(ctx, "owner", "repo", "1", provider.UpdateIssueOptions{Assignees: []string{"alice"}}); err != nil {
		t.Fatalf("legacy set: %v", err)
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, "assignees", "alice")

	// 3) patch 置值与 legacy 同形。
	if _, err := im.UpdateIssue(ctx, "owner", "repo", "1", provider.UpdateIssueOptions{AssigneesPatch: &[]string{"bob"}}); err != nil {
		t.Fatalf("patch set: %v", err)
	}
	assertBodyHas(t, requests, http.MethodPatch, http.MethodPut, "assignees", "bob")

	// 4) 逐字段清空。
	clearOne := func(field string, opts provider.UpdateIssueOptions) {
		t.Helper()
		want, ok := clear[field]
		if !ok {
			want = ClearExpect{} // 未声明 = 全 A 级：方法 PATCH/PUT、body 在场
		}
		before := len(*requests)
		_, err := im.UpdateIssue(ctx, "owner", "repo", "1", opts)
		if want.Unsupported {
			if !errors.Is(err, provider.ErrClearNotSupported) {
				t.Errorf("%s clear: want ErrClearNotSupported, got err=%v", field, err)
			}
			if len(*requests) != before {
				t.Errorf("%s clear: B 级必须零请求，多出 %d 条", field, len(*requests)-before)
			}
			return
		}
		if err != nil {
			t.Fatalf("%s clear: %v", field, err)
		}
		if len(*requests) == before {
			t.Fatalf("%s clear: 没有发出请求（静默 no-op 是三态要修的缺陷）", field)
		}
		last := (*requests)[len(*requests)-1]
		if want.Method != "" && last.Method != want.Method {
			t.Errorf("%s clear: method = %s, want %s", field, last.Method, want.Method)
		}
		if want.PathContains != "" && !strings.Contains(last.Path, want.PathContains) {
			t.Errorf("%s clear: path %q missing %q", field, last.Path, want.PathContains)
		}
		if want.BodyField != "" {
			assertBodyHasEmpty(t, requests, want.BodyField)
		}
	}
	clearOne("body", provider.UpdateIssueOptions{BodyPatch: new(string)}) // 注：new("") 见下
	...
}
```

**注意（实现者按此修正上面骨架）**：清空调用用 `ptr("")` / `&[]string{}` 形式（`new(string)` 是空串指针亦可——`new("")` 更直白）：
`clearOne("body", provider.UpdateIssueOptions{BodyPatch: new("")})`、
`clearOne("assignees", …{AssigneesPatch: &[]string{}})`、
`clearOne("labels", …{LabelsPatch: &[]string{}})`、
`clearOne("milestone", …{MilestonePatch: new("")})`。

`assertBodyHasEmpty(t, requests, field)`：取最新写请求 body，`json.Unmarshal` 到 `map[string]any`，断言 **key 在场**且值为空形态（`nil`(null) / `""` / 空 `[]any` / 空 `map` / `float64(0)`）——**缺席即失败**（静默 no-op 是本 spec 要修的缺陷）。`assertBodyLacks` 同构取反。recordedRequest 的 body/path 字段名按 Step 1 实测。

CR 侧（crs.go）同构：`assertCRUpdateTriState`，字段键 `description`/`assignees`，清空 options 用 `DescriptionPatch: new("")`、`AssigneesPatch: &[]string{}`；legacy/patch 置值回归各一例（description 用 `"legacy-desc"`/`"patch-desc"`）。

**挂接**：`RunIssuesSuite` 在 `assertIssueUpdateWire` 后调 `assertIssueUpdateTriState(t, im, requests, h.IssueClear)`（h 字段名按 Step 1 适配）；CR 套件同理。**本 Task 只改 harness + github 配置**——其他后端配置在各自 Task 补。

- [ ] **Step 3: GitHub 接线**

`backends/github/issues.go` UpdateIssue（现 :86-116）改为（逐字段）：

```go
	req := &github.UpdateIssueRequest{}
	if opts.Title != "" {
		req.Title = new(opts.Title)
	}
	// 三态收编（2A）：Resolve* 非 specified 不动；specified 空值即清空。
	if body, ok := opts.ResolveBody(); ok {
		req.Body = new(body)
	}
	if as, ok := opts.ResolveAssignees(); ok {
		req.Assignees = as // 清空时为非空 nil-free 的 []string{} ⇒ omitzero 发 []
	}
	if ls, ok := opts.ResolveLabels(); ok {
		req.Labels = ls
	}
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			// A/B 矩阵 B 级：go-github Milestone *int omitempty 发不出 null。
			return nil, oekiterrors.Wrap(provider.ErrClearNotSupported, "clear milestone", "platform", "github", "field", "milestone")
		}
		m64, err := backendutil.ParseMilestoneNumber(m) // 按现场既有解析调用原样保留
		if err != nil {
			return nil, err
		}
		req.Milestone = new(int(m64))
	}
	// State 静默丢弃修复（2A 附带，勘察违约实锤）：其余 6 后端都映射，
	// 本函数此前漏掉——CloseIssue/ReopenIssue 是独立方法但 State 是
	// UpdateIssueOptions 公共契约。
	if opts.State != "" {
		s := string(opts.State)
		req.State = new(s)
	}
	issue, _, err := p.client.Issues.Update(ctx, owner, repo, n, *req)
```

（`req.State` 字段类型以 go-github v93 源码为准——`CloseIssue` 已用 `State: new("closed")` 即 `*string`；`new(int(m64))` 等既有写法保持原样。）

`backends/github/crs.go` UpdateCR（现 :146-174）：

```go
	pr := &github.PullRequest{}
	if opts.Title != "" {
		pr.Title = new(opts.Title)
	}
	if desc, ok := opts.ResolveDescription(); ok {
		pr.Body = new(desc) // 清空 ⇒ new("") ⇒ "body":""
	}
	if opts.TargetBranch != "" {
		pr.Base = &github.PullRequestBranch{Ref: new(opts.TargetBranch)}
	}
	// assignees 走 issues 线（PR wire 无 assignee 字段）——三态同线：
	// 清空时 []string{} 经 omitzero 发 "assignees":[]。
	if as, ok := opts.ResolveAssignees(); ok {
		issue := github.UpdateIssueRequest{Assignees: as}
		if _, _, err := p.client.Issues.Update(ctx, owner, repo, n, issue); err != nil {
			return nil, err // 按现场既有错误返回形态
		}
	}
	result, _, err := p.client.PullRequests.Edit(ctx, owner, repo, n, pr)
```

（原「nil/空 = 不变（len>0 才落线）」注释改为指向 ResolveAssignees 三态。）

- [ ] **Step 4: github harness 配置**

`backends/github/contract_test.go` 的 Issues/CR 配置追加：

```go
		IssueClear: map[string]contracttest.ClearExpect{
			"body":      {BodyField: "body"},
			"assignees": {BodyField: "assignees"},
			"labels":    {BodyField: "labels"},
			"milestone": {Unsupported: true}, // go-github *int omitempty 发不出 null
		},
		// CR 侧（配置类型同名字段 CRClear）：
		"CRClear": … // 按现场配置结构落位：
		// "description": {BodyField: "body"},   // PR wire 字段名是 body
		// "assignees":   {BodyField: "assignees"},
```

（CR 配置类型与嵌入字段名以 Step 1 实测为准。）

- [ ] **Step 5: 跑测试**

```bash
go test -race -count=1 ./backends/github/ ./provider/
```
Expected: PASS（github 套件含新三态断言；State 修复由 unset/set 回归之外新增一例——**在 contracttest 或 github 单测补**：`UpdateIssueOptions{State: "closed"}` → wire 含 `"state":"closed"`，加入 `assertIssueUpdateWire` 或独立小断言，提交信息注明这是违约修复的钉子）。

- [ ] **Step 6: Commit**

```bash
git add backends/contracttest/issues.go backends/contracttest/crs.go backends/contracttest/contracttest.go backends/github/issues.go backends/github/crs.go backends/github/contract_test.go
git commit -m "feat(contracttest,github): 三态 harness（ClearExpect 期望驱动）+ GitHub 接线——含 UpdateIssue 丢 State 违约修复（2A T2）"
```

---

### Task 3: GitLab 接线

**Files:**
- Modify: `backends/gitlab/issues.go:106-148`、`backends/gitlab/crs.go:147-176`
- Modify: `backends/gitlab/contract_test.go`（IssueClear/CRClear 配置）

**Interfaces:**
- Consumes: Task 1 解析方法 + Task 2 harness。
- Produces: 无新接口（纯接线）。

- [ ] **Step 1: issues.go 接线（矩阵全 A，里程碑一等公民）**

```go
	updateOpts := &gitlab.UpdateIssueOptions{}
	if opts.Title != "" {
		updateOpts.Title = new(opts.Title)
	}
	if body, ok := opts.ResolveBody(); ok {
		updateOpts.Description = new(body) // 清空 ⇒ "description":""
	}
	if opts.State != "" { /* 既有 state_event 动词段不动 */ }
	if ls, ok := opts.ResolveLabels(); ok {
		lo := gitlab.LabelOptions(ls) // 清空 ⇒ 非 nil 空 ⇒ marshal ""
		updateOpts.Labels = &lo
	}
	if as, ok := opts.ResolveAssignees(); ok {
		ids := []int64{} // 清空：跳过 resolve 直接空集 ⇒ "assignee_ids":[]
		if len(as) > 0 {
			var err error
			ids, err = p.resolveUserIDs(ctx, "UpdateIssue", as)
			if err != nil {
				return nil, err
			}
		}
		updateOpts.AssigneeIDs = &ids
	}
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			updateOpts.ResetMilestoneID = true // 一等公民：字面 milestone_id:null
		} else {
			m64, err := /* 现场既有解析 */ …
			updateOpts.MilestoneID = new(m64)
		}
	}
```

（既有 `resolveUserIDs` 调用形态/解析函数按现场原样保留；`ResetMilestoneID` 与 `MilestoneID` 互斥——clear 分支不得同时设 MilestoneID。）

- [ ] **Step 2: crs.go 接线**

`UpdateMergeRequestOptions` 同构：`Description` → `ResolveDescription`（`new(body)`）；`AssigneeIDs` → `ResolveAssignees`（空集跳过 resolve，`&ids`）。Title/TargetBranch 守卫不动。

- [ ] **Step 3: 配置 + 测试**

`contract_test.go`：

```go
		IssueClear: map[string]contracttest.ClearExpect{
			"body":      {BodyField: "description"}, // GitLab wire 字段名
			"assignees": {BodyField: "assignee_ids"},
			"labels":    {BodyField: "labels"},
			"milestone": {BodyField: "milestone_id"}, // null 在场
		},
		// CRClear: "description"→{BodyField:"description"}, "assignees"→{BodyField:"assignee_ids"}
```

```bash
go test -race -count=1 ./backends/gitlab/
```
Expected: PASS（含套件三态断言）。

- [ ] **Step 4: Commit**

```bash
git add backends/gitlab/issues.go backends/gitlab/crs.go backends/gitlab/contract_test.go
git commit -m "feat(gitlab): Update 三态接线——全 A 级，里程碑走 ResetMilestoneID 字面 null（2A T3）"
```

---

### Task 4: giteafamily（Gitea+Forgejo）接线

**Files:**
- Modify: `backends/internal/giteafamily/issues.go:157-219`（短路 + buildEditIssueOption + labels 侧端点）
- Modify: `backends/internal/giteafamily/crs.go:235-258`
- Modify: `backends/gitea/contract_test.go`、`backends/forgejo/contract_test.go`（配置）

**Interfaces:**
- Consumes: Task 1/2。
- Produces: 无新接口。

- [ ] **Step 1: 短路修复（关键，漏了 patch-only 更新会退化成 GET）**

`issues.go:157` 的全空短路条件追加四个 patch 判空：

```go
	if opts.Title == "" && opts.Body == "" && opts.State == "" &&
		len(opts.Assignees) == 0 && len(opts.Labels) == 0 && opts.Milestone == "" &&
		opts.BodyPatch == nil && opts.AssigneesPatch == nil &&
		opts.LabelsPatch == nil && opts.MilestonePatch == nil {
		return GetIssue(ctx, f, c, owner, repo, number) // 全零才短路
	}
```

- [ ] **Step 2: buildEditIssueOption + labels 三态**

```go
	// Body：FamilyEditIssueOption.Body *string 无 omitempty——
	// nil ⇒ "body":null（今日不变）；新("") ⇒ ""（清空）。
	if body, ok := opts.ResolveBody(); ok {
		e.Body = new(body)
	}
	if as, ok := opts.ResolveAssignees(); ok {
		e.Assignees = as // nil（不动）/ 非空（置值）/ []（清空 ⇒ wire "[]"）
	}
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			e.Milestone = new(int64(0)) // wire "milestone":0（0-vs-null 语义 Phase 3 验）
		} else {
			m64, err := /* 现场既有解析 */ …
			e.Milestone = new(m64)
		}
	}
```

labels（现 `len(opts.Labels)>0` 才 ReplaceIssueLabels）改：

```go
	if ls, ok := opts.ResolveLabels(); ok {
		ids, err := ResolveLabelIDs(ctx, f, c, owner, repo, ls) // 空集 ⇒ 不查直接 []
		if err != nil {
			return nil, err
		}
		if err := c.ReplaceIssueLabels(ctx, owner, repo, number, IssueLabelsOption{Labels: ids}); err != nil {
			return nil, err
		}
	}
```

（`ResolveLabelIDs` 对空输入必须零 API 调用返回空——读现场确认，不满足则在函数入口加空集早退。）

- [ ] **Step 3: crs.go 接线**

`FamilyEditPullRequestOption`：`Body` → `ResolveDescription`（`new(d)`）；`Assignees` → `ResolveAssignees`（`e.Assignees = as`）。Title/Base/State 守卫不动（PR `title:""` 不对称是 spec 明列的 Phase 3 实测项，不碰）。

- [ ] **Step 4: 配置 + 测试**

gitea 与 forgejo 两份 `contract_test.go` 同款：

```go
		IssueClear: map[string]contracttest.ClearExpect{
			"body":      {BodyField: "body"},
			"assignees": {BodyField: "assignees"},
			"labels":    {Method: http.MethodPut, BodyField: "labels", PathContains: "labels"}, // 侧端点
			"milestone": {BodyField: "milestone"}, // 0 在场
		},
		// CRClear: "description"→{BodyField:"body"}, "assignees"→{BodyField:"assignees"}
```

```bash
GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null go test -race -count=1 ./backends/gitea/ ./backends/forgejo/ ./backends/internal/giteafamily/
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backends/internal/giteafamily/issues.go backends/internal/giteafamily/crs.go backends/gitea/contract_test.go backends/forgejo/contract_test.go
git commit -m "feat(giteafamily): Update 三态接线——全空短路补 patch 判空、labels 走侧端点清空、里程碑 wire 0（2A T4）"
```

---

### Task 5: Gitee 接线

**Files:**
- Modify: `backends/gitee/issues.go:81-112`、`backends/gitee/crs.go:110-157`
- Modify: `backends/gitee/contract_test.go`

**Interfaces:**
- Consumes: Task 1/2。
- Produces: 无新接口。

- [ ] **Step 1: issues.go 接线（csv 空串 = clear-or-422，422 也是诚实失败）**

```go
	if body, ok := opts.ResolveBody(); ok {
		updateOpts.Body = gitee.String(body) // 清空 ⇒ gitee.String("") ⇒ "body":""
	}
	if as, ok := opts.ResolveAssignees(); ok {
		updateOpts.Assignee = gitee.String(strings.Join(as, ",")) // 清空 ⇒ ""
	}
	if ls, ok := opts.ResolveLabels(); ok {
		updateOpts.Labels = gitee.String(strings.Join(ls, ","))
	}
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			updateOpts.Milestone = gitee.Int(0) // "milestone":0（null 不可达）
		} else {
			m64, err := /* 现场既有解析 */ …
			updateOpts.Milestone = gitee.Int(m64)
		}
	}
```

（Title/State 守卫不动；`gitee.String("")` 非 nil 指针穿过 omitempty——issue #88 注释已证。）

- [ ] **Step 2: crs.go 接线（RMW 删光）**

```go
	if desc, ok := opts.ResolveDescription(); ok {
		patchOpts.Body = gitee.String(desc)
	}
	if as, ok := opts.ResolveAssignees(); ok {
		// RMW 同一机制：目标集为空 = 删光当前（原 len>0 门移除）。
		if err := p.setCRAssignees(ctx, owner, repo, number, as); err != nil {
			return nil, err
		}
	}
```

**前置核查（Step 内完成）**：读 `setCRAssignees`（crs.go:140-157）确认空目标 = 逐个 remove 当前（若其内部有 `len(target)>0` 门导致空目标直接返回——放开该门或在调用侧处理，报告写明改法）。桩需 seed 当前任一指派人，断言 remove 请求发出（见 Step 3）。

- [ ] **Step 3: 配置 + 测试**

```go
		IssueClear: map[string]contracttest.ClearExpect{
			"body":      {BodyField: "body"},
			"assignees": {BodyField: "assignee"},
			"labels":    {BodyField: "labels"},
			"milestone": {BodyField: "milestone"},
		},
		// CRClear: "description"→{BodyField:"body"}；
		// "assignees"→{PathContains:"assignees"}（RMW 删光——断言
		// remove 侧请求命中；桩无当前指派人时允许零请求，配置注释
		// 说明该退化面，或让桩 seed 一名指派人保证 remove 必发）
```

（CR assignees 清空的断言形态按 setCRAssignees 实测输出定：优先 seed+断言 remove 在场；若桩结构不容许，退化为「err nil + 配置注释披露」并在报告记 ruling。）

```bash
go test -race -count=1 ./backends/gitee/
```
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add backends/gitee/issues.go backends/gitee/crs.go backends/gitee/contract_test.go
git commit -m "feat(gitee): Update 三态接线——csv 空串清空、CR 指派人 RMW 删光（2A T5）"
```

---

### Task 6: GitCode 接线（侧端点 + B 级 + divergence）

**Files:**
- Modify: `backends/gitcode/issues.go:79-105`、`backends/gitcode/crs.go:128-140`
- Modify: `backends/gitcode/divergence.go`（B 级登记）
- Modify: `backends/gitcode/contract_test.go`

**Interfaces:**
- Consumes: Task 1/2；SDK 侧端点 `RemoveIssueAssignees`、`ReplaceIssueLabels`（module cache 源码已证存在、未接线）。
- Produces: 无新接口。

- [ ] **Step 1: 侧端点签名核实**

```bash
go doc github.com/yi-nology/go-gitcode RemoveIssueAssignees
go doc github.com/yi-nology/go-gitcode ReplaceIssueLabels
```
记录方法签名（ctx/owner/repo/n/labels 参数序）。

- [ ] **Step 2: issues.go 接线**

```go
	updateOpts := gitcode.UpdateIssueOptions{
		Title: opts.Title, State: opts.State, // 既有无条件形态保留（omitempty 裁零）
	}
	if body, ok := opts.ResolveBody(); ok {
		if body == "" {
			return nil, oekiterrors.Wrap(provider.ErrClearNotSupported, "clear body", "platform", "gitcode", "field", "body") // B：string omitempty 丢 "" 且无 body 侧端点
		}
		updateOpts.Body = body
	}
	if ls, ok := opts.ResolveLabels(); ok {
		if len(ls) == 0 {
			// A：侧端点 PUT {"labels":[]}
			if err := p.client.ReplaceIssueLabels(ctx, owner, repo, n, ls); err != nil {
				return nil, err
			}
		} else {
			updateOpts.Labels = ls
		}
	}
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			return nil, oekiterrors.Wrap(provider.ErrClearNotSupported, "clear milestone", "platform", "gitcode", "field", "milestone") // B：无 detach 端点
		}
		m64, err := … // 既有解析
		updateOpts.Milestone = m64
	}
	// assignees：置值走 update（omitempty）；清空走侧端点 DELETE。
	if as, ok := opts.ResolveAssignees(); ok {
		if len(as) == 0 {
			if err := p.client.RemoveIssueAssignees(ctx, owner, repo, n /* 参数序按 Step 1 */); err != nil {
				return nil, err
			}
		} else {
			updateOpts.Assignees = as
		}
	}
	issue, err := p.client.UpdateIssue(ctx, owner, repo, n, updateOpts)
```

**顺序裁定**：先发主 update（携带所有置值字段）再发侧端点清空（labels/assignees）——一次调用混合 set+clear 时两请求都到；报告记录顺序。

- [ ] **Step 3: crs.go + divergence**

CR：

```go
	if desc, ok := opts.ResolveDescription(); ok {
		if desc == "" {
			return nil, oekiterrors.Wrap(provider.ErrClearNotSupported, "clear description", "platform", "gitcode", "field", "description")
		}
	}
	if opts.AssigneesPatch != nil {
		// wire 无 assignee 字段（既有 divergence ignore）：patch 任何
		// 形态（置值/清空）都诚实报错——旧字段路径维持静默丢弃不变。
		return nil, oekiterrors.Wrap(provider.ErrClearNotSupported, "update assignees", "platform", "gitcode", "field", "assignees")
	}
	pr, err := p.client.UpdatePullRequest(… /* 既有三字段不变 */)
```

`divergence.go` 新增（Kind 先读 :10-25 各常量 doc——预期 `DivergenceStub` 最贴「能力缺席」；若 doc 语义不合则用 `DivergenceIgnore` 并在 reason 写明哨兵，报告记选择依据）：

```go
	{Capability: "IssueManager", Method: "UpdateIssue", Field: "opts.BodyPatch", Kind: provider.DivergenceStub, Reason: "GitCode 的 issue-update wire 是 string+omitempty，空串不可表达且无 body 侧端点——patch 清空返回 provider.ErrClearNotSupported（非静默丢弃）。"},
	{Capability: "IssueManager", Method: "UpdateIssue", Field: "opts.MilestonePatch", Kind: provider.DivergenceStub, Reason: "GitCode 无 milestone detach 端点（int64+omitempty 丢 0）——patch 清空返回 provider.ErrClearNotSupported。"},
	{Capability: "ChangeRequestManager", Method: "UpdateCR", Field: "opts.DescriptionPatch", Kind: provider.DivergenceStub, Reason: "同 body 面：string+omitempty 丢空串——patch 清空返回 ErrClearNotSupported。"},
	{Capability: "ChangeRequestManager", Method: "UpdateCR", Field: "opts.AssigneesPatch", Kind: provider.DivergenceStub, Reason: "pull-update wire 无 assignee 字段（既有 ignore 针对旧字段静默丢弃）——patch 任意指定均返回 ErrClearNotSupported。"},
```

- [ ] **Step 4: 配置 + 测试**

```go
		IssueClear: map[string]contracttest.ClearExpect{
			"body":      {Unsupported: true},
			"assignees": {Method: http.MethodDelete, PathContains: "assignees"}, // 侧端点无 body
			"labels":    {Method: http.MethodPut, BodyField: "labels", PathContains: "labels"},
			"milestone": {Unsupported: true},
		},
		// CRClear: "description"→{Unsupported:true}, "assignees"→{Unsupported:true}
```

```bash
go test -race -count=1 ./backends/gitcode/ && go generate ./...
```
Expected: 测试 PASS；`go generate` 后 `git diff --stat docs/divergence-ledger.md` 非空（四条新 ledger 行）。

- [ ] **Step 5: Commit**

```bash
git add backends/gitcode/issues.go backends/gitcode/crs.go backends/gitcode/divergence.go backends/gitcode/contract_test.go docs/divergence-ledger.md
git commit -m "feat(gitcode): Update 三态接线——assignees/labels 走侧端点清空，body/milestone/CR 面 B 级哨兵 + divergence 四条（2A T6）"
```

---

### Task 7: TencentCode 接线（尊重既有 no-op 登记）

**Files:**
- Modify: `backends/tencentcode/issues.go:122-163`、`backends/tencentcode/crs.go:186-218`
- Modify: `backends/tencentcode/divergence.go`（:27 既有 labels no-op 条目 reason 更新）
- Modify: `backends/tencentcode/contract_test.go`

**Interfaces:**
- Consumes: Task 1/2。
- Produces: 无新接口。

- [ ] **Step 1: issues.go 接线**

```go
	if body, ok := opts.ResolveBody(); ok {
		updateOpts.Description = gongfeng.Ptr(body) // 清空 ⇒ "description":""
	}
	if as, ok := opts.ResolveAssignees(); ok {
		if len(as) == 0 {
			updateOpts.AssigneeIDs = gongfeng.Ptr("") // csv 空串在场
		} else {
			ids, err := p.resolveUserIDs(ctx, "UpdateIssue", as) // 既有
			…
			updateOpts.AssigneeIDs = gongfeng.Ptr(assigneeIDsCSV(ids))
		}
	}
	if ls, ok := opts.ResolveLabels(); ok {
		if len(ls) == 0 {
			// 既有登记 no-op（divergence.go:27 空集不可达）：尊重之，
			// 不凭结构分析推翻——返回哨兵。
			return nil, oekiterrors.Wrap(provider.ErrClearNotSupported, "clear labels", "platform", "tencentcode", "field", "labels")
		}
		updateOpts.Labels = gongfeng.Ptr(strings.Join(ls, ",")) // 既有
	}
	if m, ok := opts.ResolveMilestone(); ok {
		if m == "" {
			updateOpts.MilestoneID = gongfeng.Ptr(0) // "milestone_id":0
		} else {
			… // 既有解析
		}
	}
```

（Body/State 等既有守卫改道 Resolve 后不动其他。`labelUpdate` helper 不碰——它是 RemoveIssueLabel 路径的，非本域。）

- [ ] **Step 2: crs.go**

```go
	if desc, ok := opts.ResolveDescription(); ok {
		updateOpts.Description = gongfeng.Ptr(desc)
	}
	if as, ok := opts.ResolveAssignees(); ok {
		if len(as) == 0 {
			updateOpts.AssigneeID = gongfeng.Ptr(0) // 单值面：0 在场（Phase 3 验服务端）
		} else {
			ids, err := p.resolveUserIDs(ctx, "UpdateCR", as[:1]) // 既有 first-login mapping 不变
			…
			updateOpts.AssigneeID = gongfeng.Ptr(ids[0])
		}
	}
```

- [ ] **Step 3: divergence 更新**

`divergence.go:27` 既有 labels no-op 条目 Reason 追加：

```go
	// …（保留原文）2A 起 UpdateIssueOptions.LabelsPatch 的清空请求同样
	// 返回 provider.ErrClearNotSupported（结构性空串不可达的登记面一致）。
```

- [ ] **Step 4: 配置 + 测试**

```go
		IssueClear: map[string]contracttest.ClearExpect{
			"body":      {BodyField: "description"},
			"assignees": {BodyField: "assignee_ids"},
			"labels":    {Unsupported: true}, // 既有 no-op 登记尊重
			"milestone": {BodyField: "milestone_id"},
		},
		// CRClear: "description"→{BodyField:"description"}, "assignees"→{BodyField:"assignee_id"}
```

```bash
go test -race -count=1 ./backends/tencentcode/ && go generate ./...
```
Expected: PASS + ledger diff（若 reason 文本进 ledger）。

- [ ] **Step 5: Commit**

```bash
git add backends/tencentcode/issues.go backends/tencentcode/crs.go backends/tencentcode/divergence.go backends/tencentcode/contract_test.go docs/divergence-ledger.md
git commit -m "feat(tencentcode): Update 三态接线——csv/单值面清空在场，labels 尊重既有 no-op 登记返回哨兵（2A T7）"
```

---

### Task 8: 台账收口 + CHANGELOG + 全量验证 + 发版确认

**Files:**
- Modify: `CHANGELOG.md`（Unreleased 段）
- 可能: `docs/divergence-ledger.md`（若 T6/T7 generate 有遗漏，此步复核）
- 无代码改动（验证步）

- [ ] **Step 1: 台账终核**

```bash
go generate ./... && git diff --exit-code docs/divergence-ledger.md
```
Expected: 无 diff（T6/T7 已生成）——有则补跑并入本 Task 提交。

- [ ] **Step 2: CHANGELOG Unreleased**

```markdown
## [Unreleased]

### Added
- feat(provider): Update 三态语义——`UpdateIssueOptions`/`UpdateCROptions` 新增
  `XxxPatch` 指针字段族（nil=不动、非 nil=全量替换、空值=显式清空，双字段
  同设 patch 优先）；新增 `provider.ErrClearNotSupported` 哨兵（平台 wire
  无法表达清空的 B 级组合，仅 patch 字段触发，旧字段路径行为不变）。
  7 后端接线 + contracttest 三态行为契约（unset/legacy set/patch set/
  per-field clear）。迁移说明：新代码请用 XxxPatch；旧字段语义逐字保留。

### Fixed
- fix(github): `UpdateIssue` 静默丢弃 `opts.State`——补齐 state 映射
  （其余 6 后端均映射，无 divergence 登记属违约；CloseIssue/ReopenIssue
  仍是独立方法）。

### Changed
- 分歧台账新增 GitCode 四条 B 级登记（body/milestone/CR description/
  CR assignees patch 清空返回哨兵）；TencentCode labels no-op 条目
  reason 同步。
```

（措辞按仓内中文括注风格微调；行数以实际落地为准。）

- [ ] **Step 3: 全量验证**

```bash
make check && make check-mcp
golangci-lint run ./...
cd mcp && golangci-lint run ./...
go tool cover -func=coverage.out | tail -1   # ≥45%
GOTOOLCHAIN=go1.27.4 govulncheck ./...
cd mcp && GOTOOLCHAIN=go1.27.4 govulncheck ./...
```
Expected: 全绿。失败按输出修复（修复入本 Task 提交）。

- [ ] **Step 4: 文档提交**

```bash
git add CHANGELOG.md docs/divergence-ledger.md
git commit -m "docs: 2A 三态语义 CHANGELOG 未发布段 + 分歧台账终核"
```

**暂停点**：打 tag（预计 `v0.133.0`——以当时 main 最新版本为准顺延 minor）+ push 是对外动作，执行到此向用户确认后再切段、tag、push（模式同 v0.129.0/v0.130.0：merge → 切段 → tag → push main+tag → 盯 CI → 清理 worktree/分支/SDD 工作区）。

---

## Self-Review 记录

- Spec 覆盖：字段/解析/哨兵→T1；契约 harness→T2；7 后端→T2(github 含 State 修复)/T3/T4/T5/T6/T7；divergence→T6/T7；台账/CHANGELOG/验证→T8。A/B 矩阵逐格有归属（B 级 5 格：github milestone→T2、gitcode body/milestone/CR desc/CR assignees→T6、tencentcode labels→T7；gitea milestone A-wire 0→T4、gitee A-wire→T5、tencentcode A-wire→T7）。无缺口。
- 类型一致：`ResolvePatch`/六方法/`ClearExpect`/`ErrClearNotSupported` 命名在 T1 定义、T2-T7 引用一致；`IssueClear`/`CRClear` 键名（body/assignees/labels/milestone、description/assignees）在 harness 与各配置间一致。
- 关键陷阱已写入：giteafamily 全空短路必须补 patch 判空（T4 Step 1）；gitcode CR AssigneesPatch 任意指定即错而旧字段静默丢弃不变（T6 Step 3）；B 级检查在发请求前整单失败（Global Constraints）；`gitee.String("")` 非 nil 过 omitempty（T5）；GitLab ResetMilestoneID 与 MilestoneID 互斥（T3）。
- 已知风险：recordedRequest 字段名/CR 配置类型名/`setCRAssignees` 空目标行为/`ResolveLabelIDs` 空集行为四处「读现场适配」点——均在对应 Step 内有明确的读什么、怎么改、报告记什么；T5 CR assignees 清空断言允许降级为「err nil + 注释披露」并记 ruling。
- 并行会话：main 已在 `7d9a524`（v0.132.0 后），2A 相关文件零漂移（写计划前实测）；执行时按仓规每 task 前 `git status` 复核。
