# Phase 2B：操作级 Capability + 统一 Mutation Result Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地 spec 的四大件——`MutationOutcome` 统一结果（Ensure 家族扩展 + Conflict）、两个 issue 收敛函数、类型化操作级能力（`Operation`/`OperationSupport`/`CheckOperation` + 单一真源派生）、权限哨兵分界 + 错误分类矩阵文档与五状态码行为契约。

**Architecture:** 全部 additive——outcome 类型经 `EnsureAction` 别名零破坏交付；操作支持态从既有单一真源（接口断言 ∪ DivergenceStub 台账，经新增 Capability 感知 `IsStub` 查询）派生，七后端零改动；contracttest 按既有「自有 httptest」模式补五状态码套件与操作一致性套件；普通 Create/Update 与 MCP 面零触碰。

**Tech Stack:** Go 1.27.4；stdlib `errors`/`net/http/httptest`；无新依赖。

**Spec:** [docs/superpowers/specs/2026-11-14-phase2b-capability-mutation-design.md](../specs/2026-11-14-phase2b-capability-mutation-design.md)

## Global Constraints

- **零破坏**：既有导出符号签名不变；`EnsureAction` 以类型别名保留，`EnsureCreated/Updated/Unchanged` 常量继续编译；mcp/examples 不改一行（已核：`string(action)` 对别名透明）。
- **派生唯一真源**：操作支持态只从接口断言 + `p.Divergences()` 台账派生，禁止 per-backend 手写支持表；特例经 `OperationCapable` 覆盖。
- **错误纪律**（AGENTS.md 规则一）：`fmt.Errorf` `%w` / `oekiterrors.Wrap` kv；禁 `%v`、禁裸 `_` 丢错误、禁裸 `go func`。
- **contracttest 是 spec**：新断言即新契约；`stubHandler` 不能改状态码（既有套件一律自带 httptest——新套件沿此模式）。
- 提交中文 conventional、明确文件清单 `git add`；`mcp/` 独立模块本计划零触碰（T12 验证仍跑 check-mcp）。
- 收尾验证（T6 全跑）：`make check` → `make check-mcp` → 双模块 `golangci-lint run ./...` → 覆盖率 ≥45% → 双模块 `GOTOOLCHAIN=go1.27.4 govulncheck ./...`。
- 并行会话纪律：每 task 动工前 `git status`；worktree 隔离执行；push/tag 走暂停点确认。
- 每 task 门：`go test -race -count=1` 覆盖其触及的包。

---

### Task 1: MutationOutcome 类型 + EnsureAction 别名 + Ensure* 的 Conflict 路径

**Files:**
- Modify: `provider/ensure.go`（类型块 :13-24 处 + 三个 Ensure* 的写错误点）
- Test: `provider/ensure_conflict_test.go`（新建；若已有 ensure 测试文件则并入，先 `ls provider/*ensure*test*` 现场定）

**Interfaces:**
- Produces（后续 Task 消费）: `type MutationOutcome string`、`OutcomeCreated/Updated/Unchanged/Conflict`、`type EnsureAction = MutationOutcome`（旧常量保留）；三个 Ensure* 在 409 写错误路径返回 `(OutcomeConflict, 实体或 nil, err)`。

- [ ] **Step 1: 写失败测试**

```go
package provider

import (
	"context"
	"errors"
	"testing"
)

// 冲突桩：所有写操作返回包 409 分类的错误（模拟 provider.Wrap 后的形态）。
type conflictWebhookMgr struct{ WebhookManager } // 嵌入 nil 接口，仅覆写用到的方法

func (conflictWebhookMgr) ListWebhooks(context.Context, string, string) ([]*PlatformWebhook, error) {
	return []*PlatformWebhook{{ID: "1", URL: "https://a", Events: []string{"push"}}}, nil
}
func (conflictWebhookMgr) DeleteWebhook(context.Context, string, string, string) error {
	return fmt.Errorf("delete: %w", ErrConflict)
}
func (conflictWebhookMgr) CreateWebhook(context.Context, CreateWebhookOptions) (*PlatformWebhook, error) {
	return nil, fmt.Errorf("create: %w", ErrConflict)
}

func TestEnsureWebhook_ConflictOutcome(t *testing.T) {
	// URL 不匹配 → 走 delete+recreate → 两写皆 409 → OutcomeConflict 且 err 可 Is。
	_, _, err := EnsureWebhook(context.Background(), conflictWebhookMgr{},
		CreateWebhookOptions{Owner: "o", Repo: "r", URL: "https://b", Events: []string{"push"}})
	if !IsConflict(err) {
		t.Fatalf("want ErrConflict in chain, got %v", err)
	}
	// outcome 断言在返回值上——本例 err 非 nil 时 outcome 必须是 OutcomeConflict：
	//（实现后按真实返回值补 outcome 断言，见 Step 3 的返回约定）
}
```

（Step 1 按现场实际签名适配 `WebhookManager` 方法集——上桩只覆写被调用面；`fmt` import 记得加。测试先 FAIL：`OutcomeConflict` 未定义。）

- [ ] **Step 2: 验证失败**

Run: `cd /Users/zhangyi/my_project/go-git-platform && go test ./provider/ -run TestEnsureWebhook_Conflict`
Expected: FAIL（undefined: OutcomeConflict / MutationOutcome）。

- [ ] **Step 3: 实现类型与别名（ensure.go 类型块处）**

```go
// MutationOutcome 是统一变更结果（2B spec 2026-11-14-phase2b）：自动化按
// outcome 分支，不必解析错误文本或比对返回对象。
type MutationOutcome string

const (
	OutcomeCreated   MutationOutcome = "created"   // 资源原本不存在，已创建
	OutcomeUpdated   MutationOutcome = "updated"   // 存在且发生了写入
	OutcomeUnchanged MutationOutcome = "unchanged" // 已达期望态，零写入
	// OutcomeConflict 是唯一同时携带 outcome 与非 nil error 的分支
	// （并发冲突 409，err 经 IsConflict 可判）；其余 outcome 错误一律
	// ("", nil, err)。
	OutcomeConflict MutationOutcome = "conflict"
)

// EnsureAction 是 MutationOutcome 的既有名——类型别名，源码完全兼容；
// 既有 EnsureCreated/EnsureUpdated/EnsureUnchanged 常量保留（新代码
// 优先用 Outcome* 名）。
type EnsureAction = MutationOutcome
```

既有常量块改为（值不变，类型经别名等同）：

```go
const (
	EnsureCreated   = OutcomeCreated
	EnsureUpdated   = OutcomeUpdated
	EnsureUnchanged = OutcomeUnchanged
)
```

- [ ] **Step 4: 三个 Ensure* 的 409 路径**

在每个**写操作**错误返回点前插 Conflict 判定（勘察定位的精确位点）：
- `EnsureWebhook`：:98-102 delete 循环、:103-109 recreate、:173-176 ensureWebhookSecret 的 Update——形态：

```go
	if err := p.DeleteWebhook(ctx, opts.Owner, opts.Repo, h.ID); err != nil {
		if IsConflict(err) {
			return OutcomeConflict, matches[0], err
		}
		return "", nil, err
	}
```

（recreate 点实体传 `nil` 或既有局部变量按现场；ensureWebhookSecret 的 Update 点传 `out` 所在 nil——以「有实体传实体，无则 nil」为准，逐点在报告列明。）
- `EnsureBranchProtection`：:195-204 update、:209-213 create（:206 非 404 的 get 错误**不动**——读路径非变更冲突）。
- `EnsureDeployKey`：:235-237 delete、:238-242 replace-add、:244-248 create-add。
- **只包 409**：`if IsConflict(err)` 才换 outcome；其余错误逐字保持 `return "", nil, err`。

- [ ] **Step 5: 跑测试 + Commit**

```bash
go test -race -count=1 ./provider/
git add provider/ensure.go provider/ensure_conflict_test.go
git commit -m "feat(provider): MutationOutcome 统一变更结果——EnsureAction 类型别名 + 三个 Ensure* 的 409 路径返 OutcomeConflict（2B T1）"
```

---

### Task 2: EnsureIssueState + EnsureIssueLabels（Provider 形态收敛函数）

**Files:**
- Create or Modify: `provider/ensure_issue.go`（新建，避免 ensure.go 继续膨胀）
- Test: `provider/ensure_issue_test.go`

**Interfaces:**
- Consumes: Task 1 的 `MutationOutcome`。
- Produces（spec §1.2）:
  - `func EnsureIssueState(ctx context.Context, p Provider, owner, repo, number string, want IssueState) (MutationOutcome, *Issue, error)`
  - `func EnsureIssueLabels(ctx context.Context, p Provider, owner, repo, number string, want []string) (MutationOutcome, []string, error)`
  - 形态裁定（spec §1.2「包级函数 + 类型断言既有 manager；断言失败 → ErrNotImplemented 包装」）：**取 `Provider` 内部断言**——因 `IssueManager` 不在 `Provider` 组成里（provider.go:67-74），调用方免二次断言；与既有 Ensure* 的直取接口形态**有意不同**，doc 注释写明原因。

- [ ] **Step 1: 写失败测试（fake IssueManager）**

桩：`fakeIssueMgr` 实现 `IssueManager`（嵌入 nil 接口 + 覆写 `GetIssue/CloseIssue/ReopenIssue/AddIssueLabels/RemoveIssueLabel`，记录调用）。用例表：
1. state 已达 want → `(OutcomeUnchanged, 现状, nil)` 且 Close/Reopen **零调用**；
2. state 不符 → 对应 Close/Reopen 恰一调用 → `(OutcomeUpdated, 返回体, nil)`；
3. want 非 Open/Closed → `IsInvalidInput`，零 HTTP；
4. `p` 不实现 IssueManager → `Is(err, ErrNotImplemented)`，零 HTTP；
5. labels 集合相等（顺序无关）→ `(OutcomeUnchanged, 现状labels, nil)` 零写；
6. want ⊃ current → `AddIssueLabels` 恰一次（差集）→ `OutcomeUpdated`；
7. want ⊂ current → 每个差集 label 一次 `RemoveIssueLabel` → `OutcomeUpdated`；
8. `want=[]` 且 current 非空 → N 次 Remove → `OutcomeUpdated`，返回 `[]`；
9. Get 失败 → 错误透传（outcome 空）。

- [ ] **Step 2: 验证失败**

Run: `go test ./provider/ -run TestEnsureIssue`
Expected: FAIL（函数未定义）。

- [ ] **Step 3: 实现**

```go
package provider

import (
	"context"

	oekiterrors "github.com/oekit/pkg/errors"
)

// EnsureIssueState 收敛 issue 状态（2B spec）：GET 现状，已达 want 零写入
// 返 Unchanged；否则 Close/Reopen 恰一次返 Updated。
// 形态与既有 Ensure* 不同（取 Provider 而非 IssueManager）：IssueManager
// 不在 Provider 组成里，内部断言让调用方免二次断言，缺能力统一
// ErrNotImplemented。
func EnsureIssueState(ctx context.Context, p Provider, owner, repo, number string, want IssueState) (MutationOutcome, *Issue, error) {
	if want != IssueStateOpen && want != IssueStateClosed {
		return "", nil, oekiterrors.Wrap(ErrInvalidInput, "EnsureIssueState: want must be open or closed", "want", string(want))
	}
	im, ok := p.(IssueManager)
	if !ok {
		return "", nil, oekiterrors.Wrap(ErrNotImplemented, "EnsureIssueState: provider lacks IssueManager")
	}
	cur, err := im.GetIssue(ctx, owner, repo, number)
	if err != nil {
		return "", nil, err
	}
	if cur.State == want {
		return OutcomeUnchanged, cur, nil
	}
	var out *Issue
	if want == IssueStateClosed {
		out, err = im.CloseIssue(ctx, owner, repo, number)
	} else {
		out, err = im.ReopenIssue(ctx, owner, repo, number)
	}
	if err != nil {
		if IsConflict(err) {
			return OutcomeConflict, cur, err
		}
		return "", nil, err
	}
	return OutcomeUpdated, out, nil
}
```

`EnsureIssueLabels` 同构（集合比较用 map 归一、顺序无关；差集计算：
`toAdd = want∖current`（保序去重）、`toRemove = current∖want`；
写序：先 Add 批（非空才调）再逐个 Remove；返回值 = 收敛后目标集
（`want` 去重保序；Unchanged 时返回 current 原样）。doc 注释写明
「全集替换语义、返回值为本地计算的收敛目标（非再读回）」。

- [ ] **Step 4: 跑测试 + Commit**

```bash
go test -race -count=1 ./provider/
git add provider/ensure_issue.go provider/ensure_issue_test.go
git commit -m "feat(provider): EnsureIssueState/EnsureIssueLabels 收敛函数——Provider 形态内部断言，Unchanged 零写/Updated 单写/Conflict 可判（2B T2）"
```

---

### Task 3: 操作级 Capability（Operation/OperationSupport/CheckOperation + 派生）

**Files:**
- Create: `provider/operation.go`、`provider/operation_test.go`
- Modify: `provider/errors.go`（`ErrPermissionDenied` 哨兵，加在 `ErrClearNotSupported` 之后）
- Modify: `provider/divergence.go`（新增 Capability 感知 helper）

**Interfaces:**
- Produces（spec §2）: `Operation` 枚举（5 种子）、`OperationSupport`（Supported/Unsupported/CredentialDenied）、`OperationCapable` 接口、`DeriveOperationSupport(p, op)`、`CheckOperation(p, op) error`、`ErrPermissionDenied`、`IsStub(divs, capability, method) bool`。

- [ ] **Step 1: 写失败测试**

`operation_test.go` 覆盖（fake Provider 嵌入 nil 接口按需覆写 `Divergences()`）：
1. `IsStub`：Capability+Method 双匹配才 true（Method 同名不同 Capability 不误报——桩数据里造两条同 Method 异 Capability）；
2. 派生链：桩登记的 method → Unsupported；接口缺失（如不实现 NotificationManager）→ 该族 ops 全 Unsupported；都满足 → Supported；
3. `OperationCapable` 覆盖优先（fake 返回 CredentialDenied → 派生即 CredentialDenied，即使台账无桩）；
4. `CheckOperation`：Supported→nil、Unsupported→`Is(err, ErrNotImplemented)`、CredentialDenied→`Is(err, ErrPermissionDenied)`；
5. `ErrPermissionDenied` 的 `errors.Is` 可判性（Wrap 后）。

- [ ] **Step 2: 验证失败**

Run: `go test ./provider/ -run 'TestIsStub|TestDerive|TestCheckOperation'`
Expected: FAIL（未定义符号）。

- [ ] **Step 3: 实现 divergence.IsStub（Capability 感知）**

```go
// IsStub 报告台账是否登记了 capability 方法的 stub（2B）——与 Stubs 的
// 差别：同时匹配 Capability，防未来跨能力方法名撞车（Stubs 只按 Method）。
func IsStub(divs []Divergence, capability, method string) bool {
	for _, d := range divs {
		if d.Capability == capability && d.Method == method && d.Kind == DivergenceStub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 实现 errors.go 哨兵**

```go
	// ErrPermissionDenied 是凭据权限类哨兵（2B）：操作级声明判定
	// 「平台支持但当前 token 无权」时由 CheckOperation 返回。
	// 分界：ErrNotImplemented=平台不支持（台账锁定）；ErrForbidden=
	// 服务端 403（资源策略/权限 wire 不可分，不做猜测）；本哨兵=
	// 本库声明面的凭据轴。
	ErrPermissionDenied = errors.New("permission denied for this credential")
```

- [ ] **Step 5: 实现 operation.go**

```go
package provider

import (
	"context"

	oekiterrors "github.com/oekit/pkg/errors"
)

// Operation 是操作级能力的封闭枚举（2B spec）：只增不改，值即契约。
// 粒度案例：仓库级 vs 全局通知、单 webhook get/update——CapabilitySet
// 的 manager 级 bool 表达不了的层。
type Operation string

const (
	OpListRepoNotifications   Operation = "list_repo_notifications"
	OpListGlobalNotifications Operation = "list_global_notifications"
	OpUpdateGlobalInbox       Operation = "update_global_inbox"
	OpGetSingleWebhook        Operation = "get_single_webhook"
	OpUpdateSingleWebhook     Operation = "update_single_webhook"
)

// OperationSupport 是操作级三态（2B）：平台轴与凭据轴分开表示。
type OperationSupport uint8

const (
	Supported         OperationSupport = iota // 平台支持且未声明凭据限制
	Unsupported                               // 平台不支持——调用必得 ErrNotImplemented
	CredentialDenied                          // 平台支持但当前凭据无权限
)

// OperationCapable 是可选能力接口（规则二：新面走可选接口）：实现方可
// 覆盖派生（典型：凭据轴声明）。未实现的 provider 走 DeriveOperationSupport。
type OperationCapable interface {
	OperationSupport(op Operation) OperationSupport
}

// opRequirement 把种子操作映射到两个单一真源：门接口（iface 断言）与
// 台账 method（Capability 感知查 stub）。表只增不改。
type opRequirement struct {
	capability string // Divergence.Capability 字面（如 "NotificationManager"）
	method     string // Divergence.Method 字面（如 "ListNotifications")
	iface      func(Provider) bool // 门接口断言
}

var opRequirements = map[Operation]opRequirement{
	OpListRepoNotifications: {capability: "NotificationManager", method: "ListRepoNotifications",
		iface: func(p Provider) bool { _, ok := p.(NotificationManager); return ok }},
	OpListGlobalNotifications: {capability: "NotificationManager", method: "ListNotifications",
		iface: func(p Provider) bool { _, ok := p.(NotificationManager); return ok }},
	OpUpdateGlobalInbox: {capability: "NotificationManager", method: "MarkNotificationsRead",
		iface: func(p Provider) bool { _, ok := p.(NotificationManager); return ok }},
	OpGetSingleWebhook: {capability: "WebhookConfigManager", method: "GetWebhook",
		iface: func(p Provider) bool { _, ok := p.(WebhookConfigManager); return ok }},
	OpUpdateSingleWebhook: {capability: "WebhookConfigManager", method: "UpdateWebhook",
		iface: func(p Provider) bool { _, ok := p.(WebhookConfigManager); return ok }},
}
```

**Step 5 前置（必须现场核对）**：`go doc github.com/yi-nology/go-git-platform/provider WebhookConfigManager`（或读 iface_webhooks.go）钉死单 webhook 读/更新的**真实方法名**——若非 `GetWebhook/UpdateWebhook`，以上表按实改名并在报告记录。

派生与查询：

```go
// DeriveOperationSupport 从既有单一真源推导操作级支持态（2B spec 2.2）：
// OperationCapable 覆盖 > 台账 stub > 门接口缺失 > Supported。
// 凭据轴静态派生不了——只经 OperationCapable 声明。
func DeriveOperationSupport(p Provider, op Operation) OperationSupport {
	if oc, ok := p.(OperationCapable); ok {
		return oc.OperationSupport(op)
	}
	req, known := opRequirements[op]
	if !known {
		return Unsupported // 封闭枚举：未知 op 不在契约内，保守拒绝
	}
	if IsStub(p.Divergences(), req.capability, req.method) {
		return Unsupported
	}
	if !req.iface(p) {
		return Unsupported
	}
	return Supported
}

// CheckOperation 返回 nil（可执行）/ ErrNotImplemented（平台不支持）/
// ErrPermissionDenied（凭据无权限）。未知 op 按 Unsupported 处理。
func CheckOperation(p Provider, op Operation) error {
	switch DeriveOperationSupport(p, op) {
	case Supported:
		return nil
	case CredentialDenied:
		return oekiterrors.Wrap(ErrPermissionDenied, "operation denied for this credential", "op", string(op))
	default:
		return oekiterrors.Wrap(ErrNotImplemented, "operation not supported on this platform", "op", string(op))
	}
}
```

（`context` import 若未用则去掉——本文件无 ctx 面。）

- [ ] **Step 6: 跑测试 + Commit**

```bash
go test -race -count=1 ./provider/
git add provider/operation.go provider/operation_test.go provider/errors.go provider/divergence.go
git commit -m "feat(provider): 操作级 Capability——Operation 闭集三态 + DeriveOperationSupport 单一真源派生 + CheckOperation + ErrPermissionDenied 分界哨兵（2B T3）"
```

---

### Task 4: contracttest 五状态码错误分类套件

**Files:**
- Create: `backends/contracttest/errorclass.go`
- Modify: `backends/contracttest/contracttest.go`（Run 挂接一行 + t.Run）

**Interfaces:**
- Consumes: 既有 `Harness`/`baseCfg`/`h.NewProvider`（repo_templates.go 模板）。
- Produces: `ErrorClassSuite` 无条件挂接（无 Harness 配置字段——五状态是普适契约）；覆盖 401/403/409/429/5xx → `ErrAuthentication/ErrForbidden/ErrConflict/ErrRateLimited/ErrServerSide`。

- [ ] **Step 1: 读现场前置**

```bash
grep -n "func baseCfg" -A 15 backends/contracttest/contracttest.go
grep -rn "Retry" backends/contracttest/contracttest.go | head
```
确认：(a) `baseCfg(h, url)` 形态；(b) harness 的 provider 配置是否启用传输层重试——**429/5xx 会被重试**，套件必须单次尝试（若 `provider.Config` 有重试开关则在 cfg 关掉；若无开关，报告实测单状态用例耗时，>10s 则在报告记 ruling 并评估改用「首请求即断言」的形态）。

- [ ] **Step 2: 写套件**

```go
package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// ErrorClassSuite 钉「状态码 → 哨兵」分类经各后端真实错误链成立
// （2B spec §3.2）。分类单源在 provider.Wrap/classifyStatusCode——
// 本套件证明每个后端的 SDK/transport 错误都流经该单源。
// 自带 httptest（stubHandler 不能改状态码——既有模式）。
func testErrorClassSuite(t *testing.T, h Harness) {
	t.Helper()
	cases := []struct {
		status int
		want   error
		name   string
	}{
		{http.StatusUnauthorized, provider.ErrAuthentication, "401→ErrAuthentication"},
		{http.StatusForbidden, provider.ErrForbidden, "403→ErrForbidden"},
		{http.StatusConflict, provider.ErrConflict, "409→ErrConflict"},
		{http.StatusTooManyRequests, provider.ErrRateLimited, "429→ErrRateLimited"},
		{http.StatusInternalServerError, provider.ErrServerSide, "500→ErrServerSide"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
			}))
			defer srv.Close()
			p := h.NewProvider(t, baseCfg(h, srv.URL))
			im, ok := p.(provider.IssueManager)
			if !ok {
				t.Skipf("%s lacks IssueManager — no wire method to drive", h.Name)
			}
			_, err := im.ListIssues(context.Background(), provider.ListIssuesOptions{Owner: "o", Repo: "r"})
			if err == nil {
				t.Fatalf("status %d: want error, got nil", c.status)
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("status %d: want %v, got %v", c.status, c.want, err)
			}
		})
	}
}
```

（import 补 `errors`；`ListIssuesOptions` 字段名现场核对。）

- [ ] **Step 3: 挂接 Run**

contracttest.go 的 Run 内、既有套件 t.Run 旁加：

```go
	t.Run("ErrorClassSuite", func(t *testing.T) { testErrorClassSuite(t, h) })
```

- [ ] **Step 4: 跑测试**

```bash
go test -race -count=1 ./backends/github/ ./backends/gitlab/ ./backends/gitea/ ./backends/forgejo/ ./backends/gitee/ ./backends/gitcode/ ./backends/tencentcode/
```
Expected: 七后端全绿（每后端套件内 ErrorClassSuite 五子测过）。若某后端 SDK 对 429/5xx 的重试导致超时或分类漂移，按 Step 1 的 ruling 处理并报告。

- [ ] **Step 5: Commit**

```bash
git add backends/contracttest/errorclass.go backends/contracttest/contracttest.go
git commit -m "test(contracttest): 五状态码错误分类套件——401/403/409/429/5xx 经各后端真实错误链钉到哨兵（2B T4）"
```

---

### Task 5: contracttest 操作一致性套件（台账/接口 → CheckOperation → 行为）

**Files:**
- Create: `backends/contracttest/operation.go`
- Modify: `backends/contracttest/contracttest.go`（Run 挂接）

**Interfaces:**
- Consumes: Task 3 的 `Operation` 种子集、`DeriveOperationSupport`/`CheckOperation`、`IsStub`；Task 4 的自有 httptest 模式。
- Produces: `OperationSuite`——每个种子 op 上钉三层一致：(1) `CheckOperation` 的哨兵与「台账/接口直查」的期望一致；(2) stub 派生的 Unsupported 调用真实方法必得 `ErrNotImplemented`；(3) iface 缺失派生的 Unsupported 由 CheckOperation 直接给出。

- [ ] **Step 1: 写套件**

```go
package contracttest

import (
	"context"
	"errors"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// OperationSuite 钉操作级三层一致（2B spec §3.2）：
//  台账/接口（测试侧独立直查） → CheckOperation 哨兵 → stub 派生项的行为。
// 测试侧期望值不调 DeriveOperationSupport（否则同义反复）——直接用
// provider.IsStub + 接口断言计算。
func testOperationSuite(t *testing.T, h Harness) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // 任何真调用都该在 wire 前被 stub 拦下
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))

	// 种子 op 的测试侧期望（与 operation.go 表同构但独立书写——两份表
	// 不一致即套件红，正是要抓的漂移）。
	type expectation struct {
		op         provider.Operation
		capability string
		method     string
		hasIface   func(provider.Provider) bool
		invoke     func(provider.Provider) error // 仅 stub 派生项用于行为钉
	}
	expectations := []expectation{ /* 五条，invoke 形如： */
	// nm, ok := p.(provider.NotificationManager); if !ok { return errSkip }
	// _, err := nm.ListNotifications(context.Background(), provider.ListNotificationsOptions{}); return err
		...
	}
	for _, e := range expectations {
		t.Run(string(e.op), func(t *testing.T) {
			stubbed := provider.IsStub(p.Divergences(), e.capability, e.method)
			ifaceOK := e.hasIface(p)
			err := provider.CheckOperation(p, e.op)
			switch {
			case stubbed || !ifaceOK:
				if !errors.Is(err, provider.ErrNotImplemented) {
					t.Fatalf("expected ErrNotImplemented (stub=%v iface=%v), got %v", stubbed, ifaceOK, err)
				}
				if stubbed && e.invoke != nil {
					// 行为钉：真实方法调用也必须是 ErrNotImplemented（wire 前拦截）。
					if ierr := e.invoke(p); !errors.Is(ierr, provider.ErrNotImplemented) {
						t.Fatalf("stubbed method call: want ErrNotImplemented, got %v", ierr)
					}
				}
			default:
				if err != nil {
					t.Fatalf("expected Supported (nil), got %v", err)
				}
			}
		})
	}
}
```

（Step 1 落地时把 `...` 填满五条：三条 NotificationManager——`invoke` 分别驱动 `ListNotifications`/`MarkNotificationsRead`/（ListRepoNotifications 无台账桩则走 default 分支）；两条 WebhookConfigManager——`invoke` 驱动 `GetWebhook`/`UpdateWebhook`。**invoke 的方法名与参数以现场 iface 签名为准**（Task 3 Step 5 的核对结果复用）。`MarkNotificationsOptions`/`ListNotificationsOptions` 零值可用性现场核。）

- [ ] **Step 2: 挂接 Run + 跑测试**

```bash
# contracttest.go Run 内加：
#	t.Run("OperationSuite", func(t *testing.T) { testOperationSuite(t, h) })
go test -race -count=1 ./backends/github/ ./backends/gitlab/ ./backends/gitea/ ./backends/forgejo/ ./backends/gitee/ ./backends/gitcode/ ./backends/tencentcode/
```
Expected: 七后端绿。**gitcode 是关键阳性样本**（ListNotifications/MarkNotificationsRead 有真桩 → stub 分支 + invoke 行为钉必须命中）；某后端若 Skip 过多（iface 缺失且无桩），报告列明每个 op 的走向。

- [ ] **Step 3: Commit**

```bash
git add backends/contracttest/operation.go backends/contracttest/contracttest.go
git commit -m "test(contracttest): 操作级三层一致套件——台账/接口直查期望 vs CheckOperation vs stub 行为钉（2B T5）"
```

---

### Task 6: 错误分类矩阵文档 + CHANGELOG + 全量验证 + 发版确认

**Files:**
- Create: `docs/error-classification.md`
- Modify: `CHANGELOG.md`（Unreleased 段）

- [ ] **Step 1: 矩阵文档（与源码逐行核对后写）**

先读 `provider/errors.go:386-424`（ClassifyStatus doc + classifyStatusCode 全分支）与 `:217-224`（403-throttle 改道），文档结构：

```markdown
# 错误分类矩阵（provider）

（以 provider/errors.go classifyStatusCode 为准——改分类先改这里）

## 状态码 → 哨兵
| 状态码 | 哨兵 | 含义 |
（400/422→ErrInvalidInput；401→ErrAuthentication；403→ErrForbidden〔含
 throttle 改道 429 的例外，引 errors_ratelimit403_test〕；404→ErrNotFound；
 409→ErrConflict；410→ErrGone；429→ErrRateLimited；5xx→ErrServerSide；
 其余→裸 HTTP error）

## 三分界（2B）
- 平台不支持 = ErrNotImplemented（DivergenceStub 台账 + DivergenceSuite 锁定；
  CheckOperation 同哨兵）
- 凭据无权限 = ErrPermissionDenied（仅 CheckOperation 声明面；不做 wire 猜测）
- 服务端拒绝 = 状态码分类面（ErrForbidden 等）

## 既有行为契约清单
- 404：status 驱动（contracttest testIsNotFound）
- 400/422：**pre-wire 校验拒绝**断言（families_contract/notifications——
  不是 mock 状态码；本节按实描述）
- 401/403/409/429/5xx：ErrorClassSuite（本版新增）
- 操作层：OperationSuite

## 交叉引用
Divergence 台账（docs/divergence-ledger.md）、Operation/CheckOperation（provider/operation.go）
```

- [ ] **Step 2: CHANGELOG Unreleased**

Added（MutationOutcome+别名+两个收敛函数+Operation 闭集三态+CheckOperation+
ErrPermissionDenied）、Fixed（Ensure* 409 路径补 Conflict outcome——行为增强）、
Changed（错误分类矩阵文档 + 两新契约套件）。按仓内中文括注风格。

- [ ] **Step 3: 全量验证**

```bash
make check && make check-mcp
golangci-lint run ./...
cd mcp && golangci-lint run ./...
go tool cover -func=coverage.out | tail -1   # ≥45%
GOTOOLCHAIN=go1.27.4 govulncheck ./...
cd mcp && GOTOOLCHAIN=go1.27.4 govulncheck ./...
```
Expected: 全绿；失败修复入本 Task 提交。

- [ ] **Step 4: 文档提交**

```bash
git add docs/error-classification.md CHANGELOG.md
git commit -m "docs: 错误分类矩阵（源码逐行核对）+ 2B CHANGELOG 未发布段"
```

**暂停点**：切段 tag（以当时 main 最新版本顺延 minor，预计 v0.136.0）+ push 是对外动作——到此向用户确认后再执行（merge → 切段 → tag → push main+tag → 盯 CI/Release → 清理 worktree/分支/SDD 工作区；CHANGELOG 合并冲突按「2B 段落归 Unreleased/新版段，不动既有段」处理——2A 轮已验证的错位教训：**合并后必查段归属**）。

---

## Self-Review 记录

- Spec 覆盖：§1.1 类型+别名+Conflict→T1；§1.2 两收敛函数（Provider 形态裁定=spec 原文）→T2；§2 全部（含 IsStub helper、未知 op 保守 Unsupported）→T3；§3.1 矩阵→T6；§3.2 五状态套件→T4、操作一致性→T5；验收四条→各 Task 门 + T6。无缺口。
- 类型一致：`MutationOutcome`/四常量/`EnsureAction` 别名（T1）被 T2 引用；`Operation`/`OperationSupport`/`CheckOperation`/`IsStub`（T3）被 T5 引用；`ErrPermissionDenied`（T3 定义）T5 断言用。
- 关键陷阱已写入：429/5xx 重试预算（T4 Step 1 前置）；OperationSuite 期望值**不得**调 Derive（同义反复）；WebhookConfigManager 方法名现场核对（T3 Step 5 + T5 复用）；EnsureWebhook 冲突桩的方法集按现场适配；`want` 校验与缺能力哨兵零 HTTP。
- 四个勘察未知的裁定：①新收敛函数取 Provider 形态（spec 原文，与既有 Ensure* 直取接口有意不同并写明原因）②400/422 按实描述为 pre-wire 契约（不扩套件）③新套件自带 httptest（stubHandler 不可改状态码）④IsStub 新增 Capability 感知（不改既有 Stubs）。
- 并行会话：main `6b85ecb`（2B spec 已推）；每 task 前 git status 复核。
