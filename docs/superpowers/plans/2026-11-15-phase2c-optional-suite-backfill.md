# Phase 2C：7 可选接口族 contracttest 补齐 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 路线图 §2.4 后半句验收——`uncoveredFamilies` 登记的 7 个可选能力族全部获得 wire 级行为契约套件，coverage registry 三向翻转（covered 行 + 挂接 + 族注册），机器可见缺口清零。

**Architecture:** 每族一个套件文件，严格复刻 `backends/contracttest/users.go` 形态（`XHarnessConfig` + `testXSuite` 双漂移开关〔config 非-nil ⟺ `Capabilities().X` 声明〕+ `RunXSuite` 自带 httptest）；`Harness` 增 7 个可选字段、`Run` 挂 7 行；`coverage_check_test.go` 每落地一族就把它从 `uncoveredFamilies`/`TestOptionalCapabilityFamiliesUncovered` 移进 `coveredFamilies`（该测试的报错信息即翻转指引），最后一族落地时删除翻转守卫与空注册表。能力仅在场的后端提供 fixture（github×6 族；TeamReviews=github/gitea/forgejo），其余后端 config 保持 nil → 双漂移开关跳过。

**Tech Stack:** Go 1.27.4；stdlib `net/http/httptest`；无新依赖；零生产代码改动（**本计划只动 `backends/contracttest/` + 各后端 `contract_test.go` fixture + CHANGELOG**——七族实现已存在，若勘察发现实现缺口只报告不擅自扩面）。

**Spec:** [docs/superpowers/specs/2026-11-12-review-roadmap-design.md](../specs/2026-11-12-review-roadmap-design.md) §2.4（「7 个未纳入共享行为套件的可选接口族补齐 contracttest 断言」）。**裁定：不另写设计 spec**——套件名由 `TestOptionalCapabilityFamiliesUncovered` 预设、形态由既有套件族约束、每族 wire 面在实现源码中现成，设计空间已闭合；本计划即真源。

## Global Constraints

- **只加断言不改语义**：七族生产实现（`backends/github/{gists,starred,migrations,deployments,release_assets,repo_extras,reviews}.go`、`giteafamily.RequestTeamReviewers`）零改动；发现实现 bug → 报告 + 另立单，不在本计划内修。
- **双漂移开关**：每族 config 非-nil ⟺ `DeriveCapabilities` 声明为真，两个方向都 error（缺 config / 多 config），声明为假才 skip——与 Labels/Issues 同形。
- **自带 httptest**：`stubHandler` 不能改行为；每套件自建服务器（既有模式）。
- **coverage 三向锁**：每族落地必须同提交完成 ①`coveredFamilies` 加行 ②`mountedSuites` 列表加名 ③从 `TestOptionalCapabilityFamiliesUncovered` 循环与 `uncoveredFamilies` 移除——任一方向不齐，`TestRunCoverageMatrix`/`TestCapabilityFamiliesAllRegistered` 红。
- **错误纪律**（AGENTS.md 规则一）：测试内 `fmt.Errorf` `%w`；禁 `%v` 包装、禁裸 `_` 丢错误、禁裸 `go func`。
- 提交中文 conventional（`test(contracttest): ...（2C T<N>）`），明确文件清单 `git add`；`mcp/` 零触碰。
- 每 task 门：`go test -race -count=1 ./backends/contracttest/ ./backends/github/ ./backends/gitea/ ./backends/forgejo/ ./backends/gitlab/ ./backends/gitee/ ./backends/gitcode/ ./backends/tencentcode/`。
- 并行会话纪律：worktree 隔离执行；每 task 动工前 `git status`；push/tag 走暂停点确认。
- 收尾（T5）：`make check` → `make check-mcp` → 双模块 `golangci-lint run ./...` → 覆盖率 ≥45% → 双模块 `GOTOOLCHAIN=go1.27.4 govulncheck ./...` → 批次评审。

## 现场勘察基线（写计划时核实，implementer 动工前复核）

| 族 | 套件名（预设） | 实现者 | wire 要点 |
|---|---|---|---|
| GistManager | GistsSuite | github | `GET /gists`（`GistListOptions` 分页）→ `convertGist` |
| StarredManager | StarredSuite | github | `GET user/starred?page&per_page` + `Accept: application/vnd.github.star+json`，`decodeStarredEntry` 包装体 |
| MigrationManager | MigrationsSuite | github | `POST user/migrations`（org 空/非空路由）+ `GET .../{id}`，Accept `application/vnd.github+json` |
| ReleaseAssetManager | ReleaseAssetsSuite | github | `DownloadReleaseAsset` 跟随 `p.followClient`，`rc==nil` 报错；有字节流则 `io.Copy(w, rc)` |
| DeploymentManager | DeploymentsSuite | github | `CreateDeployment` body（ref/environment/task/required_contexts…）；`ListDeployments` query（environment 分页）；`CreateDeploymentStatus` **pre-wire 状态白名单校验**（非法 state → `provider.New` 422） |
| ForkSyncManager | ForkSyncSuite | github | `MergeUpstream` body `{branch}`；**空 branch → pre-wire 422**（`provider.New`） |
| TeamReviewManager | TeamReviewsSuite | github + gitea + forgejo | github：`ReviewersRequest{TeamReviewers}`；gitea/forgejo：`giteafamily.RequestTeamReviewers`（形态现场核 ports） |

Coverage 现状：`coverage_check_test.go` 头注 + `uncoveredFamilies` + `TestOptionalCapabilityFamiliesUncovered`（循环恰为上表七个套件名）+ `TestCapabilityFamiliesAllRegistered`（族 ∈ covered∪uncovered）。

---

### Task 1: GistsSuite + StarredSuite（范式立基，github）

**Files:**
- Create: `backends/contracttest/gists.go`、`backends/contracttest/starred.go`
- Modify: `backends/contracttest/contracttest.go`（Harness 字段 ×2 + Run 挂接 ×2）
- Modify: `backends/contracttest/coverage_check_test.go`（coveredFamilies 行 ×2、mountedSuites 名 ×2、`TestOptionalCapabilityFamiliesUncovered` 循环删 2 项、`uncoveredFamilies` 删 2 行）
- Modify: `backends/github/contract_test.go`（`Gists`/`Starred` fixture）

- [ ] **Step 1: 读模板与现场**：通读 `backends/contracttest/users.go`（套件范式）、`coverage_check_test.go`（三向锁）、`backends/github/gists.go`/`starred.go`（wire + `provider.Gist`/`PlatformRepo` 字段）、`decodeStarredEntry`（包装体形态）、github `contract_test.go` 既有 fixture 风格。
- [ ] **Step 2: 写失败测试**：`gists.go`/`starred.go` 按 users.go 形态落地——`testXSuite` 双漂移开关 + `RunXSuite` 自带 httptest。子测最低面：
  - Gists：`ListMyGists` 解析（fixture 首元素 `id`/`description` 非空）+ 分页参数在 query（`page`/`per_page` 归一化后出现）。
  - Starred：`ListStarred` 解析（starred_at 包装体与裸 repo 体都过 `decodeStarredEntry`——fixture 用实现真实消费的形态，现场核）+ `Accept: application/vnd.github.star+json` 请求头断言 + 分页 query。
  - 两套件：capability 为假且无 config → skip；github 声明为真 → config 必须在场（否则 error）。
- [ ] **Step 3: 挂接与 registry 翻转**：Harness 加 `Gists *GistsHarnessConfig`、`Starred *StarredHarnessConfig`（doc 注释沿既有字段风格）；`Run` 加两行 `t.Run`（放 `RepoTemplatesSuite` 旁）；coverage 三件套同提交翻转（GistsSuite/StarredSuite）。
- [ ] **Step 4: 跑门**：`go test -race -count=1 ./backends/contracttest/ ./backends/github/ ...七后端`——`TestRunCoverageMatrix`/`TestCapabilityFamiliesAllRegistered`/`TestOptionalCapabilityFamiliesUncovered`（剩 5 项）全绿。
- [ ] **Step 5: Commit**：`git add` 上述文件 → `git commit -m "test(contracttest): Gists/Starred 两可选族 wire 套件——双漂移开关+分页/Accept 钉，coverage 注册表翻转 2/7（2C T1）"`。

---

### Task 2: MigrationsSuite + ReleaseAssetsSuite（github）

**Files:**
- Create: `backends/contracttest/migrations.go`、`backends/contracttest/releaseassets.go`
- Modify: `contracttest.go`、`coverage_check_test.go`、`backends/github/contract_test.go`（同 T1 三件套 + fixture）

- [ ] **Step 1: 读现场**：`backends/github/migrations.go`（org 空/非空双路由、Accept 头、`provider.MigrationInfo` 字段）、`release_assets.go`（`followClient` 行为——**空 body 必报错**的契约、`io.Copy` 到 w）、`DownloadReleaseAsset` 在既有测试/调用方的用法。
- [ ] **Step 2: 写套件**：
  - Migrations：`CreateMigration` org="" → POST `user/migrations`（路径断言）+ 响应解析；`GetMigration` org 非空 → `orgs/{org}/migrations/{id}` 路径断言 + 解析；两调用都带 Accept 头（现场核实现设置的值）。
  - ReleaseAssets：`DownloadReleaseAsset` 服务端回 200 + 已知字节体 → `w` 收到逐字节相同内容；服务端回 200 空体 → **错误**（rc==nil 契约）；404 → `ErrNotFound` 链。`followClient` 的重定向形态若 stub 单机无法表达，按现场裁：直连形态先钉、重定向分支在报告记 ruling。
- [ ] **Step 3: 挂接 + registry 翻转 4/7**（同 T1 形态；fixture 进 github contract_test）。
- [ ] **Step 4: 跑门**（同 T1 命令）。
- [ ] **Step 5: Commit**：`test(contracttest): Migrations/ReleaseAssets 两可选族 wire 套件——org 双路由+资产下载字节/空体契约钉，coverage 翻转 4/7（2C T2）`。

---

### Task 3: DeploymentsSuite + ForkSyncSuite（github 写侧）

**Files:**
- Create: `backends/contracttest/deployments.go`、`backends/contracttest/forksync.go`
- Modify: `contracttest.go`、`coverage_check_test.go`、`backends/github/contract_test.go`

- [ ] **Step 1: 读现场**：`backends/github/deployments.go`（三方法 body/query、**CreateDeploymentStatus 非法 state pre-wire 422**——`isValidDeploymentState` 白名单）、`repo_extras.go` SyncFork（空 branch pre-wire 422）、`provider.Deployment`/`CreateDeploymentOptions` 字段。
- [ ] **Step 2: 写套件**：
  - Deployments：`CreateDeployment` POST body 携带 `ref`+`environment`、响应解析（id 非零）；`ListDeployments` query 携带 `environment`（非空时）+ 分页 + 解析；`CreateDeploymentStatus` 合法 state → POST body `state` 在场；**非法 state → 零请求 + `Is(err, ErrInvalidInput)`**（pre-wire 校验钉，负例是本族最高价值断言）。
  - ForkSync：`SyncFork` 正常 → POST `merge-upstream` 类路径携带 `{branch}`（路径现场核 `MergeUpstream` 实际 URL）；空 branch → **零请求 + 错误可判**（`provider.New` 422 → 核哨兵，现场核 `ErrInvalidInput` 是否命中）。
- [ ] **Step 3: 挂接 + registry 翻转 6/7**。
- [ ] **Step 4: 跑门**（同 T1）。
- [ ] **Step 5: Commit**：`test(contracttest): Deployments/ForkSync 两可选族 wire 套件——写侧 body/query 钉 + pre-wire 负例（非法 state/空 branch 零请求），coverage 翻转 6/7（2C T3）`。

---

### Task 4: TeamReviewsSuite（三平台 wire 差异）

**Files:**
- Create: `backends/contracttest/teamreviews.go`
- Modify: `contracttest.go`、`coverage_check_test.go`、`backends/{github,gitea,forgejo}/contract_test.go`（三家 fixture）

- [ ] **Step 1: 读现场（本 task 的关键勘察）**：`backends/github/reviews.go:124`（`ReviewersRequest{TeamReviewers}` → POST 形态）；`backends/internal/giteafamily` 的 `RequestTeamReviewers` 全链（ports → SDK → **gitea/forgejo 各自请求 body 键名**——`team_reviewers` vs `reviewers` 现场核，勿凭记忆）；三家 `contract_test.go` 的 Reviews fixture 现状。
- [ ] **Step 2: 写套件**：`RequestTeamReviewers` 正常路径 → 请求携带 teams 数组（键名按平台：套件从实现形态推导断言面——三家键名若不同，用「body 含全部 team 值」的平台无关断言 + 每家 fixture 提供 teams 值）；`number` 非法 → pre-wire `ParsePRNumber` 拒绝（零请求，负例）。双漂移开关同形（`TeamReviews` 字段 ⟺ `Capabilities().TeamReviews`）。
- [ ] **Step 3: 挂接 + registry 翻转 7/7**（最后一批：`TestOptionalCapabilityFamiliesUncovered` 循环此时应为空——若 T1-T3 已逐项删除，本步只删剩余 TeamReviews 项；`uncoveredFamilies` 同步清空但**注册表变量与 `TestCapabilityFamiliesAllRegistered` 保留**（收尾归 T5））。
- [ ] **Step 4: 跑门**（同 T1，**重点看 gitea/forgejo/gitlab/gitee/gitcode/tencentcode 的 skip 方向**：无 config 且未声明 → skip 不 error）。
- [ ] **Step 5: Commit**：`test(contracttest): TeamReviews 三平台 wire 套件——teams 载荷+非法 number 零请求钉，coverage 翻转 7/7（2C T4）`。

---

### Task 5: 收口——registry 清理 + CHANGELOG + 全量门 + 批次评审

**Files:**
- Modify: `backends/contracttest/coverage_check_test.go`（删 `TestOptionalCapabilityFamiliesUncovered`；`uncoveredFamilies` 清空后按其用途裁定：保留空表+注释「历史缺口已清零」或整体移除——以 `TestCapabilityFamiliesAllRegistered` 仍绿为准）
- Modify: `CHANGELOG.md`（Unreleased）

- [ ] **Step 1: registry 收口**：确认七行全部在 `coveredFamilies`、`mountedSuites` 七名齐、`uncoveredFamilies` 零残留；删除翻转守卫测试；跑 `go test ./backends/contracttest/ -run 'TestRunCoverageMatrix|TestCapabilityFamiliesAllRegistered|TestOptional' -v` 绿（第三个不存在即为预期）。
- [ ] **Step 2: CHANGELOG Unreleased**：`### Changed（2C：7 可选接口族 contracttest 补齐）` 一段——七套件名 + 双漂移 + 三向锁翻转 + pre-wire 负例点名（非法 deployment state / 空 branch / 非法 number）+ coverage registry 缺口清零。
- [ ] **Step 3: 全量验证**：
  ```bash
  make check && make check-mcp
  golangci-lint run ./...
  cd mcp && golangci-lint run ./...
  go tool cover -func=coverage.out | tail -1   # ≥45%
  GOTOOLCHAIN=go1.27.4 govulncheck ./...
  cd mcp && GOTOOLCHAIN=go1.27.4 govulncheck ./...
  ```
- [ ] **Step 4: Commit**：`docs: 2C CHANGELOG 未发布段 + coverage registry 缺口清零收口（2C T5）`。
- [ ] **批次评审**（全分支 diff 派 code reviewer，含七套件 spec 合规 + 三向锁 + wire 断言真实性）→ findings 修复 → **暂停点**：切段（v0.139.0 起，发版前 `ls-remote` 实查；重构 spec 地板 ≥v0.139.0 同步顺延）+ tag + push 待用户确认。

---

## Self-Review 记录

- Spec 覆盖：roadmap §2.4「7 个未纳入共享行为套件的可选接口族补齐 contracttest 断言」→ T1-T4 各族 + T5 registry 收口；「`go generate` 分歧台账再生成」句属 2A 语境（本计划零 divergence 变更，无需再生成——若 T 中途动了台账则跑）。验收缺口：无。
- 套件名权威：`TestOptionalCapabilityFamiliesUncovered` 预设七名与勘察表逐一吻合（GistsSuite/StarredSuite/MigrationsSuite/ReleaseAssetsSuite/DeploymentsSuite/ForkSyncSuite/TeamReviewsSuite）——计划不发明新名。
- 三向锁时序：行/挂接/守卫循环必须同提交（Direction1/2 与翻转守卫三方互锁）——每 task Step 显式列出。
- 关键陷阱已写入：followClient 空体契约（T2）；pre-wire 负例是高价值断言（T3 非法 state、T4 非法 number）；gitea/forgejo team wire 键名现场核勿凭记忆（T4）；skip 方向只在未声明端（T4 Step4）。
- 三个裁决：①不另写设计 spec（理由见开头——套件名/形态/wire 全部现成）；②零生产代码改动，实现 bug 另立单；③`uncoveredFamilies` 注册表 T5 裁定去留（以 `TestCapabilityFamiliesAllRegistered` 语义完整性为准）。
- 并行会话：动工前 `git status`；worktree 隔离。
