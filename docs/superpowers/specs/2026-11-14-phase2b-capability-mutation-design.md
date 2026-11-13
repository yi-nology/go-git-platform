# Phase 2B：操作级 Capability + 统一 Mutation Result 设计

来源：四期路线 spec §2.2/§2.3（[2026-11-12-review-roadmap-design.md](2026-11-12-review-roadmap-design.md)）。
勘察基线：main `bdff4bc`（v0.135.0 后），CapabilitySet/EnsureAction/错误分类现状已由
module cache + 仓内源码级勘察定稿（2026-11-14 勘察报告，关键 file:line 见各节）。

## 已锁定的裁决（用户两问未答复，按推荐项裁定）

1. **Mutation Result = Ensure 家族扩展**（否决「后端平行方法」与「改签名」）：
   outcome 类型在 provider 层泛化交付，普通 Create/Update 签名零变更。
   代价（已接受）：只有 ensure/收敛型调用能拿到 outcome；真 `Unchanged`
   需读比对——这正是 ensure 语义的本义。
2. **操作级能力 = 类型化闭集 + CheckOperation**（否决字符串操作名与「仅
   做错误侧」）：封闭 `Operation` 枚举可静态校验；声明从**既有单一真源派生**
   （接口断言 ∪ DivergenceStub 台账），不引入 per-backend 手写表。
3. **范围 = spec §2.2 字面**：不含运行时探测实现（只留声明/派生面与
   `CredentialDenied` 枚举值位）、不含写侧条件更新（If-Match/预期 SHA
   归后续）、**MCP 面不动**（与 2A 一致，工具 schema 另审）。
4. **旧 API 零破坏**：`EnsureAction` 以类型别名保留；所有既有导出符号
   签名不变；新增面全部 additive。

## 一、统一 Mutation Result（Ensure 家族扩展）

### 1.1 类型

`provider/ensure.go`（EnsureAction 现址）新增：

```go
// MutationOutcome 是统一变更结果（2B spec）：自动化按 outcome 分支，
// 不必解析错误文本或比对返回对象。
type MutationOutcome string

const (
	OutcomeCreated   MutationOutcome = "created"   // 资源原本不存在，已创建
	OutcomeUpdated   MutationOutcome = "updated"   // 存在且发生了写入
	OutcomeUnchanged MutationOutcome = "unchanged" // 已达期望态，零写入
	OutcomeConflict  MutationOutcome = "conflict"  // 并发冲突（409），
	                                              // 伴随 ErrConflict 错误返回
)

// EnsureAction 是 MutationOutcome 的既有名（类型别名，源码完全兼容）。
// 既有 EnsureCreated/EnsureUpdated/EnsureUnchanged 常量保留。
type EnsureAction = MutationOutcome
```

- **Conflict 语义**：唯一同时携带 outcome 与非 nil error 的分支
  （`return OutcomeConflict, entity, err`，err 经 `IsConflict` 可判）；
  其余 outcome 错误一律 `("", nil, err)`。
- 既有三个 Ensure* 函数在 409 路径上改返 `OutcomeConflict`
  （今天它们把 409 当普通错误上抛——行为增强，CHANGELOG 记）。

### 1.2 新收敛函数（provider 层，一次实现，零接口变更）

沿 `EnsureWebhook` 的既有模式（包级函数 + 类型断言既有 manager）：

- **`EnsureIssueState(ctx, p, owner, repo, number, want IssueState) (MutationOutcome, *Issue, error)`**
  — GET 现状；已达 want → `Unchanged`（零写入）；否则 Close/Reopen → `Updated`。
  自动化「确保 issue 关闭」的幂等原语。
- **`EnsureIssueLabels(ctx, p, owner, repo, number, want []string) (MutationOutcome, []string, error)`**
  — GET 现状 labels；集合相等 → `Unchanged`；否则按差集 Add/Remove → `Updated`
  （返回收敛后的 labels）。全集替换语义（与 tri-state 的 LabelsPatch 同域但
  面向收敛而非单次写）。

两者都要求对应 manager 能力（断言失败 → `ErrNotImplemented` 包装），
contracttest 按后端桩做 wire 级行为契约（Unchanged 零写请求 / Updated
恰一次写 / 缺能力哨兵）。

### 1.3 不做（本节）

- 不给普通 `UpdateIssue/CreateIssue` 加 outcome（签名不动，裁决 1）。
- 不实现写侧 If-Match/预期 SHA（roadmap 表中另列，归后续周期）。
- 不新增 MCP 工具（`ensure_issue_state` 等留待 MCP 面单独评审）。

## 二、操作级 Capability（类型化闭集 + CheckOperation）

### 2.1 类型与查询面（新文件 `provider/operation.go`）

```go
// Operation 是操作级能力的封闭枚举（2B）：只增不改，值即契约。
// 粒度案例：仓库级 vs 全局通知、单 webhook get/update——CapabilitySet
// 的 manager 级 bool 表达不了的层。
type Operation string

const (
	// 种子集：覆盖评审点名的粒度案例，后续按需扩（枚举只增不改）。
	OpListRepoNotifications   Operation = "list_repo_notifications"
	OpListGlobalNotifications Operation = "list_global_notifications"
	OpUpdateGlobalInbox       Operation = "update_global_inbox"
	OpGetSingleWebhook        Operation = "get_single_webhook"
	OpUpdateSingleWebhook     Operation = "update_single_webhook"
)

// OperationSupport 是操作级三态（2B）：平台轴与凭据轴必须分开表示
// （评审硬要求）。
type OperationSupport uint8

const (
	Supported         OperationSupport = iota // 平台支持且未声明凭据限制
	Unsupported                               // 平台不支持——调用必得 ErrNotImplemented
	CredentialDenied                          // 平台支持但当前凭据无权限——
	                                         // CheckOperation 返回 ErrPermissionDenied
)

// OperationCapable 是可选能力接口（规则二：新面走可选接口）。
type OperationCapable interface {
	// OperationSupport 声明某操作的支持态。未实现本接口的 provider：
	// provider.CheckOperation 走派生回退（见 2.2）。
	OperationSupport(op Operation) OperationSupport
}
```

### 2.2 派生（单一真源，不写 per-backend 表）

`provider/operation.go` 提供包级派生（与 `DeriveCapabilities` 同哲学）：

```go
// DeriveOperationSupport 从既有两个单一真源推导操作级支持态：
//  1. 门接口的类型断言（如 OpUpdateSingleWebhook ← WebhookConfigManager）；
//  2. DivergenceStub 台账（方法级「平台不能服务」登记 → Unsupported）。
// 凭据轴（CredentialDenied）静态派生不了：由 provider 实现 OperationCapable
// 时自行声明（如未来 TestConnection 探测接入后），缺省不出现。
func DeriveOperationSupport(p Provider, op Operation) OperationSupport
```

- op → 门控要求的映射表**一份**（在 provider 内：接口断言 或
  `Capability, Method` 对用于查 DivergenceStub）。
- 七个后端无需改一行（回退路径自动生效）；有特殊声明的后端可选实现
  `OperationCapable` 覆盖。
- `CheckOperation` 帮助函数：

```go
// CheckOperation 返回 nil（可执行）/ ErrNotImplemented（平台不支持）/
// ErrPermissionDenied（凭据无权限）。未声明且无接口 → Supported 乐观
// 缺省（实际调用是最终真相，避免误杀）。
func CheckOperation(p Provider, op Operation) error
```

### 2.3 权限哨兵（错误侧分离的另一半）

`provider/errors.go` 新增：

```go
// ErrPermissionDenied 是凭据权限类哨兵（2B）：操作级声明/探测判定
// 「平台支持但当前 token 无权」时由 CheckOperation 返回。
// 与 ErrForbidden 的分界：ErrForbidden = 服务端 403（资源策略/权限
// 混同，wire 不可分）；ErrPermissionDenied = 本库声明面的凭据轴。
// 真实调用的 403 仍归 ErrForbidden——不做 wire 猜测。
var ErrPermissionDenied = errors.New("permission denied for this credential")
```

**分界文档化**（写进哨兵 doc + 错误矩阵）：平台不支持 = `ErrNotImplemented`
（+ DivergenceStub 台账，已有套件锁定）；凭据无权限 = `ErrPermissionDenied`
（CheckOperation 声明面）；服务端 403 = `ErrForbidden`（分类面，2B 补行为契约）。

## 三、错误分类矩阵文档 + 行为契约

### 3.1 文档

新建 `docs/error-classification.md`：
- 状态码 → 哨兵全表（400/422→InvalidInput、401→Authentication、403→Forbidden、
  404→NotFound、409→Conflict、410→Gone、429→RateLimited、5xx→ServerSide、
  未映射→裸 HTTP err——以 `classifyStatusCode` 源码为准逐行核对）；
- 三轴分界节：平台不支持 / 凭据权限 / 服务端拒绝 的判定路径与示例；
- 与 Divergence 台账、OperationSupport 的交叉引用。

### 3.2 contracttest 行为契约（新共享套件）

`backends/contracttest` 新增错误分类套件（桩返回指定状态码 → 断言
`errors.Is` 到对应哨兵），每后端挂接，覆盖 **401/403/409/429/5xx** 五类
（404/400/422 已有既有契约）。外加：
- `CheckOperation` 一致性：声明 Unsupported 的操作，调用对应 manager 方法
  必得 `ErrNotImplemented`（与 DivergenceSuite 联动，防声明与行为漂移）；
- `CredentialDenied` 声明的后端（若有）→ CheckOperation 必得
  `ErrPermissionDenied`。

## 四、验收（对齐 roadmap 2.4「核心 API 语义清晰；关键 Mutation 路径有行为契约」）

- `MutationOutcome` 四值 + `EnsureAction` 别名兼容（既有编译零改）；
  三个既有 Ensure* 的 409 路径返 Conflict；两个新收敛函数有 wire 级行为契约
  （Unchanged 零写 / Updated 单写 / 缺能力哨兵）。
- `Operation`/`OperationSupport`/`OperationCapable`/`CheckOperation` 落地，
  七后端零改动即可派生；Unsupported 声明与 ErrNotImplemented 行为双向钉。
- `ErrPermissionDenied` 落地且与 ErrForbidden 分界成文。
- `docs/error-classification.md` 与源码逐行一致；五类状态码行为契约全后端绿。
- 收尾：`make check` + `make check-mcp` + 双模块 lint + 覆盖率 ≥45% +
  双模块 govulncheck 全绿；CHANGELOG 同步；发版走常规暂停点确认。

## 明确不做

- 运行时探测实现（只有枚举值位与派生面；探测接入是后续周期）。
- 写侧条件更新（If-Match/预期 SHA）。
- MCP 工具/schema 任何变更。
- 普通 Create/Update 签名变更（含「平行 WithOutcome 方法」形态）。
- per-backend 手写操作支持表（派生是唯一真源；特例走 OperationCapable 覆盖）。
