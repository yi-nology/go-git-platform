# 文件排布重构 S1–S8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按 spec 的分层目标树完成 A 档纯布局拆包——`model`（纯域）← `provider`（契约缝）← `runtime`（机器）← `adapter`（平台实现）四层成形，依赖边机器可校验，一次性原子破坏性发布强制下游按 `docs/MIGRATION.md` 迁移（无 shim、无别名）。

**Architecture:** 只动包边界与 import 路径，不改任何 API 语义/签名/接口面（A 档红线）。S1/S2 先做「符号归属」剖离（provider → model/runtime），S3/S4 再做「路径大挪移」（backends→adapter、pkg/githubapp 归位），顺序不可倒置——避免同一提交混两类机械变更把冲突面翻倍。mcp/examples/smoke 随分期内联改写（C7），每期绿门保证 workspace 面全程绿；GOWORK=off 的 mcp tidy/install 是设计承认的两步窗口（§7.3），S8 闭合。

**Tech Stack:** Go 1.27.4；`go/parser`（layering 守卫）；`goimports -local github.com/yi-nology/go-git-platform`（`~/go/bin/goimports`）；无新依赖。

**Spec:** [docs/superpowers/specs/2026-11-14-restructure-layout-design.md](../specs/2026-11-14-restructure-layout-design.md) —— **本计划的文件映射表（§5）、裁决（C1–C8）、边表（§4.1）、破坏矩阵（§7.1）、分期预览（§8）全部以 spec 为真源**；执行者每个 task 都要读 spec 对应节。计划只补 spec 没有的执行细节（机械改写程序、2B/2C 新增文件归位、逐步验证命令）。

## Global Constraints

**spec 红线（§2/§10 逐字，每 task 隐含）：**
- 不改任何 API 语义、方法签名、wire shape（包路径变更导致的引用改写除外）；
- 不改接口面、不加减能力（2B/2C 已发布面原样保留）；
- 不动 `transport/`、`gitbackend/`、`mcp/`、`internal/` 的名称与内部结构；
- 不引入新依赖（AGENTS.md 规则一白名单不变）；
- 不做 type alias 兼容层（`type Issue = model.Issue` 一律禁止——违背「强制下游改」）。

**spec 之外本计划补充的裁决（记录在案）：**
- **R1（2B/2C 新增文件归位）**：`provider/operation.go`（Operation/OperationSupport/OperationCapable/DeriveOperationSupport/CheckOperation——能力声明=契约面）**留 provider**；`provider/ensure_issue.go`（EnsureIssueState/EnsureIssueLabels——Ensure 机器族）**随 ensure.go 迁 runtime**；对应测试 `operation_test.go` 留 provider、`ensure_issue_test.go` 迁 runtime；contracttest 的 2B/2C 新套件随 S3 整包走，无需单独处理。
- **R2（版本号）**：spec 中 `v0.136.0` 为撰写时占位——0.136/0.137/0.138 已被占。推荐序列：**2C 先独立发版（候选 v0.139.0，另一暂停点）→ 本重构 = 下一个空闲 minor（候选 v0.140.0）**。S7 发版时 `git ls-remote --tags origin` 实查，按 spec §7.4 顺延规则同步 spec §7.2/§8 的版本行。两批若被迫合并发版则只占一个号——不推荐，破坏矩阵与 2C 的 CHANGELOG 段会混。
- **R3（mcp 窗口预期）**：S1–S6 期间 mcp job 的 Tidy/install 两检查红 = 窗口（spec §7.3），不是回归；根 3 job 与 workspace 面必须全程绿。

**每期绿门（= task 的 Step「跑门」，逐字）：**
```bash
GOWORK=off go build ./... && GOWORK=off go test ./...
make check-mcp          # workspace 模式（go.work 在 checkout）
golangci-lint run ./... # 根模块全仓（含 examples）
```
S7 追加 spec §9 全部门（覆盖率 ≥45%、双模块 govulncheck、`go generate` 台账零 diff、旧路径 grep 归零）。

**标准改写程序（S1–S4 每期复用，本节即「how」，task 内不再重复解释）：**

1. **加速器——按符号表 sed**（仅对**跨包引用**，符号表见各 task；`\b` 词边界防误伤 `IssueManager` 这类同前缀留守符号）：
   ```bash
   # 形态（macOS sed -i ''；逐符号执行，符号表来自 spec §5.2/本计划）：
   find . -name '*.go' -not -path './mcp/*' -not -path './.worktrees/*' \
     | xargs sed -i '' -E 's/\bprovider\.Issue\b/model.Issue/g'
   ```
   注意：`provider.` 前缀形态只用于**外部包**（adapter/contracttest/examples/smoke/mcp/genledger）；`provider/` 包**内部**的裸符号（`Issue`、`Platform`…）另走第 2 步。
2. **安全网——编译器驱动循环**（正确性的最终来源，sed 只是加速）：
   ```bash
   GOWORK=off go build ./...   # 每轮修掉全部报错再重跑，直至零错
   ```
   - provider 包内部裸符号：报错处逐个加 `model.`/`runtime.` 前缀（或按符号表预 sed：`s/\bIssue\b/model.Issue/g`——**执行前先审计字符串命中**：`grep -rn '"Issue"' provider/ backends/` 应为空，命中即先人工裁定再动）；
   - `goimports -local github.com/yi-nology/go-git-platform` 收 import（`~/go/bin/goimports`）。
3. **字符串/注释审计**：`git diff | grep -E '^[-+].*"[A-Z]'` 人工过一遍——wire 字符串、json tag、台账 Capability 字面量不得被改；注释里的旧路径归 S5 文档面（本期顺手改也不算错，但**存档结论不改写**）。
4. **测试**：`GOWORK=off go test ./...` + `make check-mcp` 双绿才许 commit。
5. **提交**：中文 conventional、明确文件清单 `git add`（绝不用 `-A`）、每期一（或一串）提交。

**并行会话纪律（spec §7.4）**：动工前 `git status` + 目标文件 mtime；每期按明确清单 add；合并后必查 CHANGELOG 段归属；发版前 `ls-remote` 查 tag 占用。

**mcp 内联改写（C7）**：mcp 是独立 module 但在 go.work 下引用本地父模块——每期把 mcp 源码中被迁符号/路径同步改写（sed 范围**包含** `mcp/`，但 `mcp/go.mod`/`go.sum` 本期不动）。

---

### Task 1: S1 model 剖离

**Files:**
- Create: `model/`（`doc.go` `platform.go` `cr.go` `issue.go` `review.go` `repo.go` `event.go` `search.go` `notification.go` `gist.go` `release.go` `ci.go` `migration.go` `file.go` `mentions.go` `commitstatus.go` `deployments.go` `projection/`〔整包平移〕）——类型来源表 = **spec §5.2 逐行**（含每类型来自 provider 的哪个文件）
- Modify: `provider/provider.go`（剥离后只剩 `CapabilitySet` + `Provider` 接口，`Platform`/实体下沉——C1/C2）、`provider/options.go`（剥离实体后剩 60 个 `XxxOptions` 参数结构——C5）、`provider/iface_gists.go` `iface_releases.go` `iface_ci.go` `iface_migrations.go`（剥离内嵌实体）、`provider/convertutil.go`（剥 `ExtractMentions`/`isInsideURL`——C2）
- 移动: `provider/commitstatus.go` → `model/commitstatus.go`、`provider/types_deployments.go` → `model/deployments.go`（整文件，改包名 `model`）、`pkg/projection` → `model/projection`（C6，`git mv`，包名不变）
- Rewrite: 全仓（含 mcp/examples/smoke/genledger）`provider.<实体符号>` → `model.<实体符号>`

**Interfaces:**
- Produces（后续 Task 消费）: `model.Platform`+七常量、`model.Issue`/`ChangeRequest`/… 全量实体（spec §5.2 清单）；`provider` 包对 `model` 的单向 import；`model` 零仓内依赖。

- [ ] **Step 1: 建包与整文件平移**
  - `mkdir -p model && git mv provider/commitstatus.go model/commitstatus.go && git mv provider/types_deployments.go model/deployments.go`；两文件包名 `provider` → `model`；`git mv pkg/projection model/projection`（包名不变）。
  - 按 spec §5.2 类型表新建 `model/*.go`：把类型（含其方法、常量组）从 `provider/provider.go`、`options.go`、5 个 iface 文件、`convertutil.go` **剪切**到对应 model 文件；每个 model 文件头部写包注释一行（`// Package model …`），`model/doc.go` 按 spec §3 职责写完整包注释（纯域层 + 依赖禁令）。
  - 现场核对清单：`grep -n 'type \|const (' provider/provider.go provider/options.go` ——spec 表列的每个类型都能定位；2B 新增的 `MutationOutcome`/`EnsureAction` **不是实体**（结果语义），随 R1 留待 S2 归 runtime（`MutationOutcome` 在 ensure 语境 → runtime；若 operation/契约侧引用它，编译器会指出，按「机器→runtime、契约→provider」裁定并在报告记录）。
- [ ] **Step 2: provider 剥离面**
  - `provider.go` 删实体后重写包注释（契约缝职责 + 「实体见 model」指引）；`options.go` 顶部加 import `model`；5 个 iface 文件同理（接口签名里的实体类型改 `model.Xxx`——这是**签名的包路径变更**，A 档允许）。
  - `convertutil.go` 中 `ExtractMentions`/`isInsideURL` 剪到 `model/mentions.go`（C2：三个 Comment 实体的 `Mentions()` 方法与之同包）。
- [ ] **Step 3: 标准改写程序第 1 步（跨包 sed）**
  - 符号表 = spec §5.2 全部导出类型 + 其导出常量（现场以 `grep -n '^type \|^const' model/*.go` 生成权威清单）；对 mcp/、examples/、smoke/、internal/tools/genledger/、backends/（含 contracttest）、provider/ 残留引用执行 `s/\bprovider\.SYM\b/model.SYM/g`。
  - **先审计**：`grep -rn '"\(Issue\|Platform\|ChangeRequest\|Label\|Release\|Gist\|MigrationInfo\|Deployment\|CommitStatus\)"' --include='*.go' .` ——命中先裁定（wire 字符串不许动）。
- [ ] **Step 4: 标准改写程序第 2–3 步（编译器循环 + goimports + 字符串审计）**
  - `GOWORK=off go build ./...` 循环至零错（provider 内部裸符号逐个 qualify）；`~/go/bin/goimports -local github.com/yi-nology/go-git-platform -w` 全部触碰文件；`git diff | grep -E '^[-+].*"[A-Z]'` 人工过。
  - **mcp 内联改写**：`cd mcp && GOWORK=off go build ./...` 修至零错（sed 范围已含 mcp，残余靠编译器）。
- [ ] **Step 5: 跑门**
  - `GOWORK=off go build ./... && GOWORK=off go test ./...`；`make check-mcp`；`golangci-lint run ./...`。Expected: 全绿（覆盖率若跌，先查是否丢测试——测试随宿主文件走：`provider/*_test.go` 中测实体的进 `model/*_test.go`、测机器的暂留）。
- [ ] **Step 6: Commit**
  ```bash
  git add model/ provider/ mcp/ examples/ smoke/ internal/tools/genledger/ pkg/
  git commit -m "refactor(model): S1 model 剖离——纯域实体下沉新包 model，provider 只剩契约面，全仓符号改写（重构 S1）"
  ```
  （`pkg/` 此时仅剩空目录或 projection 已走——按实际清单 add；`git status` 核对无意外文件。）

---

### Task 2: S2 runtime 剖离

**Files:**
- 移动（整文件 11+1，包名 `provider` → `runtime`）: `provider/{manager,detect,ensure,webhook,batch,pageiter,wait,janitor,ttlcache,timeparse,convertutil}.go` → `runtime/`；**+R1：`provider/ensure_issue.go` → `runtime/ensure_issue.go`**；对应测试 `provider/{manager,detect,ensure,webhook,batch,pageiter,wait,janitor,ttlcache,timeparse,convertutil,ensure_issue}_test.go`（以实际 `ls provider/*_test.go` 对应宿主为准）随宿主迁
- Create: `runtime/doc.go`（机器层职责 + 依赖 `provider+model` 的包注释）
- Modify: `provider/convertutil.go` 剩余已空则删除（`ExtractMentions` 已在 S1 走）；引用机器符号的全部文件
- Rewrite: 全仓（含 mcp）`provider.<机器符号>` → `runtime.<机器符号>`

**Interfaces:**
- Consumes: S1 的 `model.*`（runtime 文件大量引用实体——`EnsureWebhook` 返回 `*PlatformWebhook` 等）。
- Produces: `runtime.{DetectPlatform,EnsureWebhook,EnsureBranchProtection,EnsureDeployKey,EnsureIssueState,EnsureIssueLabels,WaitForCommitStatus,ValidateWebhookWithRegistry,BatchOptions,BatchResult,Each,Collect,ErrStopIteration,WebhookValidator,Manager,Stats,RegisterHostAlias,ParseFirst,…}`（= 移动文件的导出面，签名逐字不变）。

- [ ] **Step 1: 整文件平移**
  - 按 spec §5.1「→ runtime」表 `git mv` 11 文件 + R1 的 `ensure_issue.go`；包名改 `runtime`；测试文件随宿主 `git mv`（测试的 `package provider` → `package runtime`；测试若引用留守 provider 的符号——如 `provider.ErrConflict`——由编译器指出后加前缀）。
  - `runtime/doc.go` 新写（spec §3 职责 + 「依赖 provider+model，禁止反向」）。
- [ ] **Step 2: 符号改写（标准程序 1–3 步）**
  - 跨包符号表 = 移动文件的导出名（`grep -h '^func [A-Z]\|^type [A-Z]\|^const\|^var [A-Z]' runtime/*.go` 现场生成；含 `EnsureAction`/`EnsureSecretMode`/`WaitOptions`/`PageFetch` 等）；`s/\bprovider\.SYM\b/runtime.SYM/g` 全仓（含 mcp）。
  - **方向审计**：`grep -rn 'model\.' runtime/ | grep import` 反向不存在性靠 build 保证；确认无任何 `runtime` 文件 import `adapter`/`contracttest`（边表 S6 才机器化，本期先 build+人工）。
  - `MutationOutcome` 按 R1 裁定落位（若落 runtime：`provider.` 引用点同步改 `runtime.`；报告记录裁定）。
- [ ] **Step 3: 编译器循环 + goimports + 字符串审计 + mcp 内联**
  - 同标准程序 2–3 步；`cd mcp && GOWORK=off go build ./...` 零错。
- [ ] **Step 4: 跑门** —— 同 Global Constraints 三件套，全绿。
- [ ] **Step 5: Commit**
  ```bash
  git add runtime/ provider/ mcp/ examples/ smoke/ internal/tools/genledger/
  git commit -m "refactor(runtime): S2 runtime 剖离——机器与用户面助手下沉新包 runtime，全仓符号改写（重构 S2）"
  ```

---

### Task 3: S3 adapter 拆分（路径大挪移）

**Files:**
- 移动（包名全部不变，纯路径）: `backends/{github,gitlab,gitea,forgejo,gitee,gitcode,tencentcode}` → `adapter/` 同名；`backends/internal/{giteafamily,backendutil}` → `adapter/internal/`（C3 可见性逐字保持）；`backends/all` → `adapter/all`；`backends/contracttest` → `contracttest/`（顶层，C4）
- Modify: 全仓 import 路径 `github.com/yi-nology/go-git-platform/backends/…` → 新路径；`provider/divergence.go` 的 `//go:generate go run ../internal/tools/genledger` **相对路径不变**；`internal/tools/genledger` 的 import `backends/*` → `adapter/*`
- 删除: `backends/` 空目录

**Interfaces:**
- Consumes: S1/S2 的 `model.*`/`runtime.*`（adapter 已在用）。
- Produces: `adapter/*` 七平台 + `adapter/internal/*` + `adapter/all` + 顶层 `contracttest`（包名零变化，下游 import 路径变更进 MIGRATION 矩阵）。

- [ ] **Step 1: git mv 全量**
  ```bash
  mkdir -p adapter/internal
  git mv backends/github backends/gitlab backends/gitea backends/forgejo \
         backends/gitee backends/gitcode backends/tencentcode adapter/
  git mv backends/internal/giteafamily backends/internal/backendutil adapter/internal/
  git mv backends/all adapter/all
  git mv backends/contracttest contracttest
  rmdir backends/internal backends   # 若空
  ```
- [ ] **Step 2: import 路径改写（路径级，非符号级）**
  - `find . -name '*.go' -not -path './.worktrees/*' | xargs sed -i '' -E 's#github.com/yi-nology/go-git-platform/backends/contracttest#github.com/yi-nology/go-git-platform/contracttest#g; s#github.com/yi-nology/go-git-platform/backends/internal/#github.com/yi-nology/go-git-platform/adapter/internal/#g; s#github.com/yi-nology/go-git-platform/backends/#github.com/yi-nology/go-git-platform/adapter/#g'`
  - **顺序敏感**：contracttest 与 internal 两条规则必须在 `backends/` 通用规则**之前**（同 sed 多表达式按序执行即满足）；mcp 内的 `backends/tencentcode` 引用同轮改掉。
  - `goimports -local … -w` 全部触碰文件。
- [ ] **Step 3: genledger 与台账复核**
  - `GOWORK=off go build ./...` 循环零错；`go generate ./...` 后 `git diff --stat docs/divergence-ledger.md` **必须零 diff**（台账内容无 import 路径——非零 diff 立即停下查明，spec §5.5）。
- [ ] **Step 4: 跑门** —— 三件套 + `cd mcp && GOWORK=off go build ./...`（mcp 的 `adapter/tencentcode` 引用）。
- [ ] **Step 5: Commit**
  ```bash
  git add adapter/ contracttest/ provider/ internal/ mcp/ examples/ smoke/ docs/
  git commit -m "refactor(adapter): S3 backends 拆分——七平台+internal 收进 adapter/，contracttest 上顶层，包名零变化（重构 S3）"
  ```

---

### Task 4: S4 auth/filter/projection 归位

**Files:**
- 移动: `githubapp/` → `auth/githubapp/`；`pkg/credential` → `auth/credential`；`pkg/branchfilter` → `filter/branchfilter`；（`pkg/projection` 已在 S1 走）；删除空 `pkg/`
- Modify（仓内消费方 import 改写，包名零变化）: `gitbackend/native.go`（credential）、`examples/credential`、`adapter/github/githubapp_test.go`（githubapp）、`mcp/`（projection → `model/projection` 若 S1 已改则本步仅复核）、`transport/` 注释中的 `pkg/…` 字样

**Interfaces:**
- Consumes: S1 的 `model/projection`。
- Produces: `auth/{githubapp,credential}`、`filter/branchfilter`（import 路径进 MIGRATION 矩阵）。

- [ ] **Step 1: git mv + 目录清理**
  ```bash
  mkdir -p auth filter
  git mv githubapp auth/githubapp
  git mv pkg/credential auth/credential
  git mv pkg/branchfilter filter/branchfilter
  rmdir pkg   # 三子包全走后必须空（不许留壳——spec §5.4）
  ```
- [ ] **Step 2: 消费方改写**
  - `sed -i '' 's#go-git-platform/githubapp#go-git-platform/auth/githubapp#g; s#go-git-platform/pkg/credential#go-git-platform/auth/credential#g; s#go-git-platform/pkg/branchfilter#go-git-platform/filter/branchfilter#g; s#go-git-platform/pkg/projection#go-git-platform/model/projection#g'` 于全部 `.go`（含 mcp、注释面顺手）。
  - `grep -rn 'go-git-platform/pkg/\|go-git-platform/githubapp' --include='*.go' --include='*.md' .` → 归零（`.worktrees` 除外）。
- [ ] **Step 3: 跑门** —— 三件套 + mcp build，全绿。
- [ ] **Step 4: Commit**
  ```bash
  git add auth/ filter/ pkg/ gitbackend/ transport/ mcp/ examples/ adapter/
  git commit -m "refactor(auth,filter): S4 githubapp/credential/branchfilter 归位，pkg/ 目录清空删除（重构 S4）"
  ```

---

### Task 5: S5 文档面

**Files:**
- Modify: `README.md`（代码块与路径全量）、`provider/doc.go`（包注释重写：契约缝职责 + 分层图 + 「实体见 model、机器见 runtime」）、`model/doc.go`/`runtime/doc.go`（S1/S2 已写初版——本步按 spec §3 复核补依赖禁令）、`AGENTS.md`（**路径引用**改写：如 `backends/contracttest` → `contracttest`、`provider/ttlcache.go` → `runtime/ttlcache.go`——**存档结论一字不改**，只改路径）、`docs/*.md` 中旧路径（`docs/divergence-ledger.md` 若 genledger 输出含路径则随 generate 自动更新，人工复核）、`CHANGELOG.md` **不动**（S6 写）

**Interfaces:**
- Consumes: S1–S4 落位的最终路径。
- Produces: 旧路径 grep 归零的文档面（验收 §9.7 的前半）。

- [ ] **Step 1: 旧路径全量清点**
  ```bash
  grep -rn 'backends/\|pkg/credential\|pkg/branchfilter\|pkg/projection\|go-git-platform/githubapp' \
    --include='*.md' --include='*.go' . | grep -v '.worktrees\|CHANGELOG.md:\|docs/superpowers/' | tee /tmp/oldpaths.txt
  ```
  `CHANGELOG.md` 历史段与 `docs/superpowers/`（spec/plan 存档）**不改**（历史记录）；其余逐条改写。
- [ ] **Step 2: 逐文件改写**
  - README：示例 import/路径块全部新路径（`model`/`runtime`/`adapter`/`contracttest`/`auth`/`filter`）；分层小节按 spec §3 树补一段。
  - `provider/doc.go`：按 spec §3 provider 行职责重写（契约缝：接口/选项/错误/patch/Capability/Divergence/工厂/Logger/Hooks/TokenSource/共享纯助手/Version）。
  - AGENTS.md：规则四仓库结构段、历史存档中的路径字面量（`provider/ttlcache.go`、`backends/contracttest`、`pkg/projection` 等）改新路径；**所有结论、裁决、审计记录正文不改写**。
- [ ] **Step 3: 复核**
  - 重跑 Step 1 清点（应只剩 CHANGELOG/存档豁免行）；`gofmt`/build 不受影响（纯注释）。
- [ ] **Step 4: Commit**
  ```bash
  git add README.md provider/doc.go model/doc.go runtime/doc.go AGENTS.md docs/
  git commit -m "docs: S5 文档面——README/包注释/AGENTS 路径引用全量换新，存档结论不改写（重构 S5）"
  ```

---

### Task 6: S6 layering 守卫 + MIGRATION + CHANGELOG

**Files:**
- Create: `internal/layering/layering_test.go`（spec §4.2 边表守卫——**表内容 = spec §4.1 全表逐字**，含补充边与边界口径）、`docs/MIGRATION.md`（spec §7.1 破坏矩阵 + 五形态代码块 + stdlib runtime 别名指引 + 实体全清单〔§5.2〕+ 无 shim 声明）
- Modify: `CHANGELOG.md`（Unreleased 加 `### Changed（**BREAKING**：文件排布重构，vX.Y.Z）` 段——开篇 Breaking、指向 MIGRATION.md、三行摘要：四层分层成形 / 实体进 model、机器进 runtime、路径 adapter 系 / 下游按 MIGRATION 一次性迁移）

**Interfaces:**
- Consumes: S1–S5 的最终布局（守卫扫的就是它）。
- Produces: `internal/layering` 守卫（挂进 `go test ./...` = `make check` 自动覆盖）；MIGRATION（S7 发版材料）。

- [ ] **Step 1: 写守卫测试（TDD：先对违规样例红）**
  - `internal/layering/layering_test.go`：`go/parser` 遍历根模块 `.go`（跳过 `mcp/` 前缀文件与 `.worktrees`；`internal/` 文件作**来源**不检、作**目标**放行），抽取 `github.com/yi-nology/go-git-platform/...` import，按 spec §4.1 边表断言；`_test.go` 用 test-only 列（`adapter --test--> contracttest|auth`）。
  - **负样例验证**：临时在 `model/doc.go` 加 `import _ "github.com/yi-nology/go-git-platform/provider"` → `go test ./internal/layering/` 必须 FAIL 打印 `model → provider`；删掉样例复绿。
- [ ] **Step 2: 写 MIGRATION.md**
  - §7.1 矩阵逐行（含 2B/2C 新增符号的归属：`MutationOutcome`→`runtime`、`Operation`/`CheckOperation`→`provider`——按 R1/实际落位填）；五形态 before/after 代码块（构造 provider / 读实体 / 轮询等待 / webhook 验签 / 空导入注册）——代码块用新旧路径真实可编译片段；`runtime` 撞名别名指引（C8）。
- [ ] **Step 3: CHANGELOG Breaking 段**（Unreleased；版本号 S7 再切段）。
- [ ] **Step 4: 跑门** —— 三件套 + `go test ./internal/layering/ -v`（含负样例手工复验记录进报告）。
- [ ] **Step 5: Commit**
  ```bash
  git add internal/layering/ docs/MIGRATION.md CHANGELOG.md
  git commit -m "feat(internal/layering): S6 依赖边表机器守卫 + docs/MIGRATION.md 破坏矩阵 + CHANGELOG Breaking 段（重构 S6）"
  ```

---

### Task 7: S7 全量验证 + 发版暂停点

**Files:** 无新文件（验证 + 可能的修复小提交）

- [ ] **Step 1: spec §9 全部门逐条**
  ```bash
  make check && make check-mcp
  golangci-lint run ./...            # 根含 examples
  cd mcp && golangci-lint run ./... && cd ..
  go tool cover -func=coverage.out | tail -1    # ≥45%；跌破必查丢测试 vs 稀释
  GOTOOLCHAIN=go1.27.4 govulncheck ./...
  cd mcp && GOTOOLCHAIN=go1.27.4 govulncheck ./... && cd ..
  go test ./internal/layering/ -v    # 边表零违例
  go generate ./... && git diff --exit-code docs/divergence-ledger.md
  grep 旧路径归零复核（同 S5 Step1，豁免清单：CHANGELOG 历史段、docs/superpowers 存档）
  ```
  任何失败：修复（小提交 `fix:`/`test:`）后重跑该腿，全绿才继续。
- [ ] **Step 2: 破坏面复核（验收 §9.9）**
  - `git diff main...HEAD --stat` 全览；对照 spec §7.1 矩阵逐行确认「矩阵外公共符号零移动」——抽查 `provider` 包导出面：`go doc github.com/yi-nology/go-git-platform/provider | grep -E '^(func|type|var|const)'` 与矩阵「不变」列一致；`go doc …/model`、`…/runtime` 与「迁移」列一致。
- [ ] **Step 3: 暂停点（对外动作，停下等用户）**
  - 汇报全门结果 + 破坏面复核 + 推荐版本号（`git ls-remote --tags origin` 实查后顺延；同步 spec §7.2/§8 版本行——R2）→ **等用户确认** 才执行：CHANGELOG 切段 → tag → push main+tag → 盯 CI/Release（根 3 job 必绿；mcp Tidy/install 红 = R3 窗口，按 spec §7.3 解读）。
  - 提交（若有修复）：信息按实际内容。

---

### Task 8: S8 窗口闭合（父版发布后）

**Files:**
- Modify: `mcp/go.mod`（require 父模块抬到 S7 实际 tag 版本）、`mcp/go.sum`（`GOWORK=off go mod tidy` 生成）

**Interfaces:**
- Consumes: S7 已发布的父模块版本（如 `v0.140.0`——以实际 tag 为准）。

- [ ] **Step 1: 抬版本 + tidy**
  ```bash
  cd mcp
  GOWORK=off go get github.com/yi-nology/go-git-platform@<S7实际tag>
  GOWORK=off go mod tidy
  ```
- [ ] **Step 2: 窗口验证**
  ```bash
  make check-mcp                 # workspace 模式仍绿
  cd mcp && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... && cd ..
  git diff --exit-code go.work    # go.work 不动
  ```
  Expected: GOWORK=off 三连绿 = Tidy/install 两 CI 检查恢复绿。
- [ ] **Step 3: Commit + push**
  ```bash
  git add mcp/go.mod mcp/go.sum
  git commit -m "chore(mcp): S8 窗口闭合——require 抬到 <tag>，GOWORK=off tidy/install 恢复绿（重构 S8）"
  ```
  push 后核 CI 6 job 全绿（含此前窗口红的两个）→ 汇报闭环。

---

## Self-Review 记录

- **Spec 覆盖**：§3 目标树→T1–T4 落位；§4.1 边表→T6 守卫（表逐字进测试）；§4.2 守卫口径→T6 Step1；§5.1/5.2/5.3/5.4 文件映射→T1–T4 各自 Files 节（类型表/移动表直接引用 spec 行）；§5.5 消费方与文档→T1–T4 内联改写 + T5；§6 C1–C8→各 task 括注；§7.1 矩阵→T6 MIGRATION；§7.2 原子发布→T7 暂停点；§7.3 窗口→R3+T8；§7.4 并行防线→Global Constraints；§8 分期表→T1–T8 一一对应；§9 验收→T7 Step1 逐条 + T6 Step4；§10 红线→Global Constraints 逐字。**无缺口。**
- **2B/2C 增量**（spec 撰写后落地）：`operation.go`/`ensure_issue.go`/`MutationOutcome` 归位→R1；contracttest 37 文件随 S3 整包→无需单列；版本占位→R2。
- **Placeholder 扫描**：无 TBD/TODO；机械步骤全部带命令；符号表指定「以 spec §5.2 + 现场 grep 生成权威清单」——这不是占位，是权威源指针（spec 即真源，类型名逐个抄进计划反而制造两份清单漂移风险），且编译器循环兜底任何遗漏。
- **类型一致性**：`model.*`/`runtime.*` 符号名在 T1/T2 产出节与 T6 MIGRATION 要求处一致；`adapter/*`、`contracttest`、`auth/*`、`filter/*` 路径在 T3/T4/T5/T6 间一致。
- **顺序不可倒置**：T1→T2→T3→T4（spec §8 约束）已按此编号；T5 文档依赖前四期落位；T6 守卫扫最终布局；T7 验收；T8 依赖 T7 的发布。
