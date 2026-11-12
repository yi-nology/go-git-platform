# Phase 2A：Update 三态语义设计

来源：四期路线 spec 第 2 期 §2.1（[2026-11-12-review-roadmap-design.md](2026-11-12-review-roadmap-design.md)）。
目标：通用 SDK 能表达「清空」——`UpdateCROptions` / `UpdateIssueOptions` 的
空值语义从「空=不动」升级为三态（未指定 / 置空 / 置值），0.x 下零 breaking。

## 已锁定的裁决（spec 授权 + 用户批准）

1. **形态 = 指针字段族**（additive）：新字段 `*[]string` / `*string`，
   与 options.go 既有 `*bool`（UpdateRepoOptions.Private）、`*string`
   （UpdateReleaseOptions/UpdateWebhookOptions）惯用法一致。
   不引入 `PatchField[T]` 泛型——仓内无此先例，指针即三态。
2. **旧字段保留 + deprecated 注释**（0.x 不删，spec 锁定）。
3. **语义**：新指针字段 `nil` = 不动；非 nil（含空切片/空串）= 全量替换，
   空值 = 清空。旧字段语义逐字不变（空 = 不动）。
   **双字段同设时新指针字段优先**（文档写死，避免静默分歧）。
4. **平台 wire 表达不了清空时**：返回新哨兵
   `provider.ErrClearNotSupported`（`errors.Is` 分类，kv 带
   platform/op/field），并登记 divergence——**不静默丢弃**（评审批评的
   正是不可表达；静默 no-op 比不支持更糟）。
   旧字段路径行为不变（gitcode 等既有 divergence 保持原样）。
5. **先实测再钉测试**：各平台 SDK 请求 struct 的 omitempty/null 能力
   以 module cache 源码为准（勘察矩阵）；服务端是否真吃清空表达属
   平台兼容面，留 Phase 3 live 验证——2A 契约测试只钉**我们发出的
   wire 形状**。

## 范围

- `provider.UpdateCROptions`：`+ DescriptionPatch *string`（清空 = ptr("")）、
  `+ AssigneesPatch *[]string`（清空 = ptr 空切片）。
  Title/TargetBranch 不加 patch 字段——平台强制非空，「清空」无合法语义，
  加了就是陷阱。
- `provider.UpdateIssueOptions`：`+ BodyPatch *string`、
  `+ AssigneesPatch *[]string`、`+ LabelsPatch *[]string`、
  `+ MilestonePatch *string`（清空 = ptr("")，映射各后端的 null/0 表达）。
  Title/State 不加（Title 强制非空；State 是 closed/open 值语义无「清空」）。
- **命名裁决**：`XxxPatch` 后缀。仓内既有三态是「同名指针」
  （UpdateWebhookOptions.URL *string），本处旧字段同名占用，`Patch` 后缀
  是评审报告自己给的词（「显式 Patch 或三态字段」）；`*Assignees` 改类型
  = 删旧字段 = breaking，排除。
- **范围裁决**：Body/Description 纳入（spec §2.1 的「Assignees 等」授权
  ——清空描述/正文与清空指派人同类，模式建立后边际成本一行/字段；
  留着不清空会显得修一半）。
- 7 后端 Update 路径接线（github / gitlab / gitea+forgejo(giteafamily) /
  gitee / gitcode / tencentcode）。
- **共享解析面（定稿形态）**：provider 包泛型核心
  `ResolvePatch[T any](patch *T, legacy T, isEmpty func(T) bool) (val T,
  specified bool)`（泛型在 provider 已有先例：BatchResult[T]/Each[T]），
  外加 options 结构体上的每字段方法（`opts.ResolveAssignees()` 等，返回
  `(val, specified)`）——后端只调方法不直接摸旧字段（把对 deprecated
  字段的引用面收进 provider 包内）。语义：`specified=false` → 不动；
  `specified && isEmpty(val)` → 清空；`specified && !isEmpty` → 置值；
  patch 与 legacy 同设 → patch 立即返回（优先，spec 裁决 3）。
- **清空失败面**：`specified && isEmpty` 且矩阵 B 级 → 返回
  `oekiterrors.Wrap(provider.ErrClearNotSupported, ...)` 带 kv
  `platform/op/field`（`errors.Is` 可判；规则一形态：先 fmt.Errorf
  `%w` 再 Wrap 叠 kv，或 Wrap 直接包哨兵——实现时按仓内哨兵包装惯例）。
  仅清空尝试触发；set 永不触发（B 级字段 set 与今日同路径）。
- GitCode 侧端点接线（矩阵 A 级标注「未接线，本 spec 接」的两处）：
  `RemoveIssueAssignees`（assignees 清空）、`ReplaceIssueLabels`
  （labels 清空）。
- `provider/errors.go` 新增 `ErrClearNotSupported` 哨兵。
- **package doc 更新**：`options.go:10-16` 的「zero-value = leave
  unchanged，无法显式置零值」段落改写为三态说明（该声明对 patch 字段
  已不成立）。
- divergence 登记（B 级组合 + GitHub State 修复若需登记）+ `go generate
  ./...` 再生成 ledger。
- contracttest：三态 wire 形状行为契约（每 patch 字段 unset/set/clear ×
  各后端；B 级组合断言 `ErrClearNotSupported`）。
- CHANGELOG：Added（patch 字段族 + 哨兵）+ Fixed（GitHub State）+ 迁移
  说明（旧字段 deprecated 引导，0.x minor 承载 additive 面）。

## 不做（YAGNI）

- 不动 `UpdateRepoOptions` / `UpdateReleaseOptions` / `UpdateWebhookOptions`
  ——已是三态。
- 不做 Create 路径（create 无「不变」语义）。
- 不做运行时能力探测 / 操作级 Capability（Phase 2B 域）。
- 不做统一 Mutation Result（Phase 2B 域）。
- **MCP 工具入参不透传 patch 字段**（工具 schema 面另审，2A 只动 SDK）。
- **giteafamily PR 路径 `title:""/base:""` 与 issue 路径注释矛盾**：
  服务端语义 UNKNOWN 且生产未见标题被清的证据（大概率服务端对 PR edit
  跳过空串）——2A 不动代码，登记为 Phase 3 live 验证项（实测 PR edit
  对 `title:""` 的处理；若实测为清空则按 issue 路径同款 backfill 修复）。

## 平台能力矩阵（勘察定稿，证据 = module cache 源码）

**分级规则（本 spec 核心裁决）**：
- **A 级 wire 可表达**：SDK struct 能放出区别于「省略」的清空值 → 接线，
  contracttest 钉 wire 形状；服务端是否真吃 = Phase 3 live 验证。
- **B 级结构不可表达**：SDK struct 放不出清空值且无侧端点 → 返回
  `ErrClearNotSupported` + divergence 登记。
- **既有登记 no-op 尊重**：已登记「空集不可达」的组合维持 B 级（不凭结构
  分析推翻 live 结论；Phase 3 实测后再升级）。
- 表内 **A-wire** = 结构上属 A 级（清空值能上 wire）但语义不确定性更高
  （csv 空串 / 数值 0 是否被服务端当清空），处置同 A：接线 + Phase 3 优先
  实测；csv 空串若服务端 422 也是诚实失败（错误可见），不是静默 no-op。

### UpdateIssueOptions

| 平台 | Body | Assignees | Labels | Milestone |
|---|---|---|---|---|
| GitHub | A `new("")`→`"body":""` | A `[]string{}`→`[]`（`omitzero`，SDK 注释明写 clears） | A 同左 | **B** `*int omitempty` 发不出 null → ErrClearNotSupported |
| GitLab | A custom marshal 保非 nil 指针 | A `&[]int64{}`→`[]` | A `&LabelOptions{}`→`""`/`null` | A 一等公民 `ResetMilestoneID`→字面 `null`（SDK 测试钉住） |
| Gitea/Forgejo | A `new("")`→`"body":""`（nil→null=今日不变） | A `[]string{}`→`[]`（nil→null=不变） | A `IssueLabelsOption{[]int64{}}`→`{"labels":[]}` PUT | A-wire `ptr(0)`→`"milestone":0`（0-vs-null 语义 Phase 3 验） |
| Gitee | A `String("")`→`"body":""` | A-wire csv `""`（clear-or-422，422=诚实失败） | A-wire csv `""` | A-wire `Int(0)`→`0`（null 不可达，Phase 3 验） |
| GitCode | **B** `string omitempty` 丢 `""`，无 body 侧端点 | A 侧端点 `RemoveIssueAssignees`（未接线，本 spec 接） | A 侧端点 `ReplaceIssueLabels`→`[]`（未接线，本 spec 接） | **B** `int64 omitempty` 丢 `0`，无 detach 端点 |
| TencentCode | A `Ptr("")`→`"description":""` | A-wire csv `""`（无反证登记） | **B** 既有登记 no-op（`RemoveIssueLabel` 空集不可达，divergence.go:27 尊重） | A-wire `Ptr(0)`→`0` |

### UpdateCROptions

| 平台 | Description | Assignees |
|---|---|---|
| GitHub | A `pullRequestUpdate.Body *string`→`""` | A issues 线 `[]string{}`→`[]` |
| GitLab | A `new("")` | A `&[]int64{}`→`[]` |
| Gitea/Forgejo | A `Body: new("")` | A `[]string{}`→`[]` |
| Gitee | A `String("")`→`"body":""` | A RMW 删光当前（同一 add/remove 机制，去掉 `len>0` 门即得清空） |
| GitCode | **B** `string omitempty` 丢 `""` | **B** wire 无 assignee 字段（divergence.go:36 既有 ignore，`/assignees` 路由管 reviewers） |
| TencentCode | A `Ptr("")` | A-wire 单值 `Ptr(0)`→`"assignee_id":0`（Phase 3 验） |

Title/TargetBranch/State 无 patch 字段（标题/基线强制非空；State 是值语义）。
服务端对 A 级清空值（`[]`/`""`/`0`/`null`）的实际接受度**全部 Phase 3
live 验证**——2A 只钉我们发出的 wire。

## 附带 bug 修复（勘察发现，同 theme「静默丢弃」）

- **GitHub `UpdateIssue` 丢 `opts.State`**：`backends/github/issues.go:86-116`
  无 `req.State` 赋值（其余 6 后端都映射；Close/Reopen 是独立方法但
  UpdateIssueOptions.State 是公共契约）；无 divergence 登记 = 违约而非
  偏离。2A 修复：映射 State（`new(string(opts.State))`）+ contracttest
  断言 + CHANGELOG Fixed。调用方依赖此 bug 的可能性为零（要关 issue 的
  人会用 CloseIssue，静默 no-op 不构成可用行为）。

## 验收（对齐 spec 第 2 期 2.4）

- 三态语义有单测 + 各后端 contracttest 行为契约（set/clear/unset）。
- 能表达清空的平台：wire 形状被契约测试钉死；不能表达的：返回
  ErrClearNotSupported + divergence 登记。
- `go generate ./...` ledger 再生成无漂移；make check + check-mcp +
  全仓 lint 全绿；覆盖率不低于 45% 地板。
