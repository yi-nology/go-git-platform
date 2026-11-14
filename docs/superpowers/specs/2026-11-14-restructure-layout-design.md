# 按架构思想的文件排布重构设计（2026-11-14）

来源：用户指令「请你按照架构思想 重构下当前这个项目的文件排布 可以让下游不兼容
强制让下游改 你来规划下」。设计呈现后用户以「可以」批准三个确认点：

1. **A 档纯布局拆包**——只动包边界与 import 路径，不改任何 API 语义/签名/接口面
   （2B「操作级 Capability + 统一 Mutation Result」保持独立路线，不在本轮范围）；
2. **`provider` 保留契约缝名**（不改成 `contract/`）；
3. **版本 v0.136.0**——0.x 破坏性变更按仓规由 minor 承载（不借机发 v1.0，
   v1.0 属第 4 期）。

## 1. 诊断

**依赖审计结论：全仓 import 边零违例**——`backends → provider,transport`、
`gitbackend → pkg/credential`、`githubapp → transport`、`transport → internal/version`，
没有任何一层反向引用。病灶不是依赖混乱，而是**包粒度**：

| 病灶 | 现状指标 | 后果 |
|---|---|---|
| `provider/` 上帝包 | 58 个非测试文件（+30 测试）：26 个接口文件与域实体、选项、错误、机器（manager/ttlcache/janitor）、工厂/注册表、webhook 验签、轮询等待混居一包 | 契约/域/机器无法分层引用；下游只能整包引入；依赖规则无法机器执行 |
| `backends/` 巨石 | 220 个 go 文件 = 7 平台（29–38 文件/平台）+ `internal/{giteafamily,backendutil}` + `contracttest`(27) + `all` | 共享契约套件与平台实现同目录树；内部件与公共件边界靠 `internal/` 巧合维持 |
| `pkg/` 杂物袋 | `credential` + `branchfilter` + `projection` 三个互不相干的包 | 「pkg」不表达职责；下游无法从路径判断用途 |

## 2. 目标 / 非目标

**目标**

- 分层可执行：`model`（纯域）← `provider`（契约缝）← `runtime`（机器）← `adapter`（平台实现），
  每层职责单一、边可机器校验；
- 下游按层引用：只要域类型引 `model`，只要机器引 `runtime`，契约面仍是 `provider`；
- 一次原子破坏性发布强制下游迁移：无 shim、无别名兼容层，`docs/MIGRATION.md` 给全量路径矩阵。

**非目标（红线）**

- 不改任何 API 语义、方法签名、wire shape（包路径变更导致的引用改写除外）；
- 不改接口面、不加减能力（2B/2C 路线原样保留）；
- 不动 `transport/`、`gitbackend/`、`mcp/`、`internal/` 的名称与内部结构；
- 不引入新依赖（AGENTS.md 规则一白名单不变）；
- 不做 type alias 兼容层（`type Issue = model.Issue` 之类一律禁止——违背「强制下游改」）。

## 3. 目标目录树与包职责

```
go-git-platform/
├── provider/                 # 契约缝（包名 provider 不变）：接口、选项、错误、
│   │                         #   patch 三态解析、Capability、Divergence、工厂/注册表、
│   │                         #   Logger/Hooks/TokenSource、共享纯助手（状态映射/分页/
│   │                         #   文件解码/diff 统计）、Version()
│   ├── iface_*.go            #   27 个能力接口族（数据实体已剥离到 model）
│   ├── options.go            #   全部 XxxOptions 参数结构（实体类型已剥离到 model）
│   ├── patch.go errors.go capabilities.go divergence.go
│   ├── factory.go registry.go logger.go middleware.go retry.go tokensource.go
│   ├── stateutil.go diffutil.go files.go pagination.go version.go doc.go
│   └── provider.go           #   剥离实体后只剩 CapabilitySet + Provider 接口
│                             #   （Platform 与实体已下沉 model，见裁决 C1/C2）
├── model/                    # 纯域（新包，包名 model）：零内部依赖
│   ├── doc.go platform.go    #   Platform 枚举 + 七平台常量（裁决 C1）
│   ├── cr.go issue.go review.go repo.go event.go search.go notification.go
│   ├── gist.go release.go ci.go migration.go file.go
│   ├── commitstatus.go       #   ← provider/commitstatus.go 整文件
│   ├── deployments.go        #   ← provider/types_deployments.go 整文件
│   ├── mentions.go           #   ExtractMentions/isInsideURL（裁决 C2）
│   └── projection/           #   ← pkg/projection（包名 projection 不变，裁决 C6）
├── runtime/                  # 机器（新包，包名 runtime）：机器与用户面助手
│   ├── doc.go
│   ├── manager.go detect.go ensure.go webhook.go batch.go pageiter.go
│   ├── wait.go janitor.go ttlcache.go timeparse.go
│   └── convertutil.go        #   余下函数（SplitFullName/BuildEventRepo/ResolveMRSHAs…）
├── adapter/                  # ← backends/（包名逐目录不变）
│   ├── github/ gitlab/ gitea/ forgejo/ gitee/ gitcode/ tencentcode/
│   ├── internal/
│   │   ├── giteafamily/      #   ← backends/internal/giteafamily（包名不变）
│   │   └── backendutil/      #   ← backends/internal/backendutil（包名不变）
│   └── all/                  #   ← backends/all（空导入注册器 + README 矩阵对账测试）
├── contracttest/             # ← backends/contracttest（包名 contracttest 不变，裁决 C4）
├── auth/
│   ├── githubapp/            # ← githubapp/（包名 githubapp 不变）
│   └── credential/           # ← pkg/credential（包名 credential 不变）
├── filter/
│   └── branchfilter/         # ← pkg/branchfilter（包名 branchfilter 不变）
├── transport/ gitbackend/ mcp/ internal/ smoke/ examples/ docs/  # 名称不变
└── go.work go.mod Makefile AGENTS.md CHANGELOG.md README.md
```

## 4. 依赖规则（AGENTS.md 落点 + 机器守卫）

### 4.1 允许的 import 边（根模块内；只列仓内前缀，oekit/stdlib 不限）

| from \ to | model | provider | runtime | adapter/* | contracttest | auth | filter | transport | gitbackend |
|---|---|---|---|---|---|---|---|---|---|
| **model/**（含 projection） | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **provider/** | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **runtime/** | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **adapter/*** | ✓ | ✓ | ✓ | ✓（仅 `adapter/internal/*`、`adapter/all` 的平台空导入、各平台 `_test.go` 夹具） | ✓（仅 `_test.go`） | ✓（仅 `_test.go`） | ✗ | ✓ | ✗ |
| **contracttest/** | ✓ | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **auth/githubapp** | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✓ | ✗ |
| **auth/credential** | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **filter/branchfilter** | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **gitbackend/** | ✗ | ✗ | ✗ | ✗ | ✗ | ✓ | ✗ | ✗ | ✗ |
| **transport/** | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |

补充边（消费方杂项，同样进守卫）：`examples/*|smoke|internal/tools/genledger →
{model, provider, runtime, adapter/*, contracttest, auth, filter, gitbackend, transport}`；
`internal/version` 可被任何仓内包引用（`internal/` 本身不受限，守卫跳过
`internal/` 与 `mcp/` 前缀）。mcp 是独立 go module，不进本守卫（其 go.mod 依赖面
由 tidy/install CI 守）。

规则一句话（写进 AGENTS.md 规则四）：**`model` 是叶子；`provider` 只准向下看
`model`；`runtime` 看 `provider+model`；`adapter` 看 `provider+runtime+model` +
自家 `internal`；契约套件看 `provider+runtime+model`；反向一律禁止。**

### 4.2 机器守卫

新增 `internal/layering/layering_test.go`：`go/parser` 遍历根模块全部 `.go` 文件，
抽取 `github.com/yi-nology/go-git-platform/...` 前缀 import，按 §4.1 边表断言；
违例打印 `from → to` 并 fail。边界口径：`mcp/` 下文件不检（独立模块）；`internal/`
下文件作为**来源**不检（genledger 等工具自由），`internal/*` 作为**目标**对全部包
放行；`_test.go` 的边用同一张表的 test-only 列校验（非测试文件出现 test-only 边即
违例）。该测试挂进 `go test ./...`，即 `make check` 自动覆盖，杜绝「文档-only 的
分层规则」（与规则二 divergence 机器登记同一精神）。

## 5. 文件级映射

### 5.1 provider/ 的 58 个非测试文件（30 个测试文件随宿主文件走）

**→ model/（整文件 2 个）**

| 源文件 | 目标 | 内容 |
|---|---|---|
| `provider/commitstatus.go` | `model/commitstatus.go` | `CommitStatus`/`CommitStatusState` + `Terminal`/`Normalize`/`Combine` 纯函数（零内部依赖，整文件平移） |
| `provider/types_deployments.go` | `model/deployments.go` | `Deployment`/`DeploymentStatus` |

**→ runtime/（整文件 11 个）**

| 源文件 | 目标 | 备注 |
|---|---|---|
| `provider/manager.go` | `runtime/manager.go` | `Manager`/`Stats`/`ManagerOption` |
| `provider/detect.go` | `runtime/detect.go` | `DetectPlatform`/`DetectResult`/`RegisterHostAlias` |
| `provider/ensure.go` | `runtime/ensure.go` | `EnsureWebhook`/`EnsureBranchProtection`/`EnsureDeployKey` + `EnsureAction`/`EnsureSecretMode` |
| `provider/webhook.go` | `runtime/webhook.go` | `WebhookValidator` 族/registry/`ReadAndRestoreBody`/`ValidateWebhookWithRegistry`/`WebhookEnvelope`/`Normalize*Action` |
| `provider/batch.go` | `runtime/batch.go` | `BatchOptions`/`BatchResult` + semaphore 引擎 |
| `provider/pageiter.go` | `runtime/pageiter.go` | `PageFetch`/`Each`/`Collect`/`ErrStopIteration` |
| `provider/wait.go` | `runtime/wait.go` | `WaitForCommitStatus`/`WaitOptions`/`CombinedCommitStatus` |
| `provider/janitor.go` | `runtime/janitor.go` | 后台清理循环 |
| `provider/ttlcache.go` | `runtime/ttlcache.go` | TTL 缓存 + 自建 per-key flight（#353 不变） |
| `provider/timeparse.go` | `runtime/timeparse.go` | due_on 多格式解析（ParseFirst 收编线） |
| `provider/convertutil.go` | `runtime/convertutil.go` | **除** `ExtractMentions`/`isInsideURL`（下沉 model，裁决 C2）外的全部：`SplitFullName`/`ExtractOwnerFromFullName`/`BuildEventRepo`/`ResolveMRSHAs` |

**留 provider/（45 个：18 非接口 + 27 接口文件）**

- 文档与契约核心：`doc.go`（包注释重写）、`provider.go`（**剥离实体后**只剩
  `CapabilitySet` + `Provider` 接口）、`errors.go`、`capabilities.go`、`divergence.go`
  （`//go:generate go run ../internal/tools/genledger` 路径不变）、`factory.go`
  （`Config`/`NewProvider`）、`registry.go`（`Register` 族）、`logger.go`、
  `middleware.go`（`Hooks`）、`retry.go`（`RetryConfig`）、`tokensource.go`；
- 参数与解析：`options.go`（**剥离实体后**只剩 60 个 `XxxOptions` 参数结构）、
  `patch.go`（`ResolvePatch` 泛型 + 六解析方法）；
- 共享纯助手（adapter 直接消费、非机器）：`stateutil.go`（`Map*StateToCR`）、
  `diffutil.go`、`files.go`（`DecodeFileContent`）、`pagination.go`
  （`NormalizePageOpts` 33 个 adapter 文件在用）、`version.go`；
- `iface_*.go` ×27：接口全部留下；其中 5 个文件剥离内嵌实体（见 5.2）。

### 5.2 model/ 文件布局（类型来源表）

| 目标文件 | 类型（来源） |
|---|---|
| `model/doc.go` | 新写包注释（纯域层职责 + 依赖禁令） |
| `model/platform.go` | `Platform` + 七平台常量（provider.go:10-19） |
| `model/cr.go` | `CRState`+常量、`ChangeRequest`、`CRUser`、`CRComment`（含 `Mentions()`）、`CRCommit`、`MergeDiff`、`ChangedFile`（provider.go） |
| `model/repo.go` | `PlatformRepo`、`PlatformBranch`、`TagInfo`、`ReleaseInfo`、`CompareResult`、`CommitInfo`、`BranchProtection`、`Collaborator`、`DeployKey`、`Contributor`（provider.go + options.go） |
| `model/event.go` | `PlatformWebhook`、`NormalizedEvent`、`EventRepo`、`TestConnectionResult`（provider.go） |
| `model/issue.go` | `IssueState`、`Issue`、`IssueComment`（含 `Mentions()`）、`IssueLabel`、`MilestoneRef`、`MilestoneState`、`Milestone`、`Label`（options.go） |
| `model/review.go` | `ReviewState`、`Review`、`ReviewComment`（含 `Mentions()`）、`ReviewResult`、`ReviewCommentResult`（options.go） |
| `model/search.go` | `SearchRepoResult`、`SearchIssueResult`、`SearchUserResult`（options.go） |
| `model/notification.go` | `Notification`、`NotificationSubject`、`Reaction`（options.go） |
| `model/file.go` | `FileResult`（options.go；`FileOptions`/`FileDeleteOptions` 是参数，留 provider） |
| `model/gist.go` | `GistFile`、`Gist`（iface_gists.go 剥离） |
| `model/release.go` | `ReleaseAsset`（iface_releases.go 剥离） |
| `model/ci.go` | `JobLog`（iface_ci.go 剥离） |
| `model/migration.go` | `MigrationInfo`（iface_migrations.go 剥离；`CreateMigrationOptions` 是参数，留 provider） |
| `model/commitstatus.go` | 整文件平移（5.1） |
| `model/deployments.go` | 整文件平移（5.1） |
| `model/mentions.go` | `ExtractMentions`、`isInsideURL`（convertutil.go 剥离，裁决 C2） |
| `model/projection/` | 整包平移 `pkg/projection`（裁决 C6） |

`iface_repo_extras.go` 的 `CreateDeploymentOptions`/`DeploymentStatusOptions`/
`CreateRepoFromTemplateOptions` 是方法参数，**留 provider**（选项归契约缝，原设计）。
剥离后 provider → model 的方向合法（§4.1），全仓无反向边（本轮已 grep 实证：
留在 provider 的文件对 runtime 候选类型零引用）。

### 5.3 backends/ → adapter/（纯路径平移，包名全部不变）

| 源 | 目标 | 文件数 |
|---|---|---|
| `backends/github` 等 7 平台目录 | `adapter/{github,gitlab,gitea,forgejo,gitee,gitcode,tencentcode}` | 29–38/目录 |
| `backends/internal/giteafamily` | `adapter/internal/giteafamily` | 15 |
| `backends/internal/backendutil` | `adapter/internal/backendutil` | 12 |
| `backends/all` | `adapter/all` | 2 |
| `backends/contracttest` | `contracttest/`（顶层） | 27 |

`adapter/internal/*` 保持 `internal` 可见性（裁决 C3）；`contracttest` 上顶层后
其唯一仓内消费方是各 adapter 的 `_test.go`（已 grep 实证：contracttest 自身只 import
`provider`，不依赖 giteafamily/backendutil）。

### 5.4 pkg/ 与 githubapp 归位

| 源 | 目标 | 包名 | 仓内消费方（改写面） |
|---|---|---|---|
| `pkg/credential` | `auth/credential` | `credential` | `gitbackend/native.go`、`examples/credential` |
| `githubapp/` | `auth/githubapp` | `githubapp` | `adapter/github/githubapp_test.go`（+ 下游 git-ferry 等，见 MIGRATION） |
| `pkg/branchfilter` | `filter/branchfilter` | `branchfilter` | 仓内零消费方（纯公共包），只改路径与文档 |
| `pkg/projection` | `model/projection` | `projection` | `mcp/tools_core.go`、`mcp/tools_issues.go` |

`pkg/` 目录重构后删除（三子包全部迁出，不留空壳）。

### 5.5 内部消费方与文档改写面

- **mcp/（独立模块）**：`provider.<实体>` → `model.<实体>`、机器助手 → `runtime.*`、
  `backends/tencentcode` → `adapter/tencentcode`、`pkg/projection` →
  `model/projection`；随各分期**内联改写**（workspace 模式全程绿，裁决 C7）；
- **examples/（7 个）、smoke/、internal/tools/genledger**：同上内联改写
  （根模块 `go test ./...` GOWORK=off 形态必须每分期绿）；
- **genledger**：import `backends/*` → `adapter/*`；`provider/divergence.go` 的
  `go:generate` 相对路径不变；重跑 `go generate ./...` 后 `docs/divergence-ledger.md`
  应字节不变（台账内容无 import 路径），若非零 diff 必须查明；
- **README.md / provider/doc.go 包注释 / AGENTS.md**：全部旧路径
  （`backends/…`、`pkg/…`、`githubapp`、`provider.<实体>` 示例代码）改写——
  AGENTS.md 中历史存档条目里的**路径引用**（如 provider/ttlcache.go、
  backends/contracttest）同步更新，存档的**结论**不改写；
- **字符串路径面**：已 grep 实证——仓内无运行时计算的 `backends/` 字符串路径
  （仅注释与 doc 注释），Makefile/CI 无 backends 路径。

## 6. 关键裁决记录

- **C1 `Platform` 下沉 model**：`PlatformRepo.Platform`、`NormalizedEvent.Source`
  引用它，而 iface 又引用这些实体 → `model` 若反向 import `provider` 即成环；
  `Platform` 是纯枚举，归 model 后 `provider/runtime/adapter` 全部合法向下引用。
  下游影响：`provider.PlatformGitHub` → `model.PlatformGitHub`。
- **C2 `ExtractMentions` 下沉 model**：`CRComment/IssueComment/ReviewComment`
  三个实体的 `Mentions()` 方法调用它；方法必须与类型同包，`model` 零依赖禁止
  它 import runtime。`convertutil` 余下函数（`BuildEventRepo` 返回 `*EventRepo`
  但方向是 runtime→model，合法）整体留 runtime。
- **C3 `adapter/internal` 保持内部性**：`backends/internal/*` 现状是
  「仅 backends 树可 import」；若按目录树字面平铺成 `adapter/giteafamily`（公共），
  等于**扩张 API 面**，违背 A 档「只动布局不动面」。故收进 `adapter/internal/`，
  可见性逐字保持。
- **C4 `contracttest` 上顶层**：它是 spec 级 harness，被 7 个 adapter 的测试 +
  `adapter/all` 矩阵测试消费；自身只依赖 `provider(+runtime+model)`，无平台引用，
  顶层后依赖边干净（`adapter --test--> contracttest` 单向）。
- **C5 options/patch 留 provider（原设计保持）**：**iface 方法**的参数结构
  （`XxxOptions`）与三态解析是契约缝的一半；实体（返回值/记录）归 model。
  runtime 自有助手的参数类型（`WaitOptions`/`BatchOptions`）随整文件迁移留
  runtime，不属此条——C5 只约束 provider 包自身的文件切分。方向
  `provider → model` 单向，无环。
- **C6 `projection` 归 `model/projection`**：其价值是按 model 的 JSON 契约
  （snake_case 字段）裁剪载荷；包名不变，仅路径变。`pkg/` 目录清空删除。
- **C7 mcp 随分期内联改写**：mcp 在 go.work 下用本地父模块——若 mcp 源码滞留旧路径，
  从第 1 分期起 `make check-mcp` 即红；内联改写使 workspace 面全程绿。
  代价见 §7.3：GOWORK=off 的 mcp tidy/install 两个 CI 检查进入发布窗口红。
- **C8 包名 `runtime` 与 stdlib 撞名**：仓内 runtime 候选文件对 stdlib `runtime`
  零引用（已 grep 实证）；下游同文件同时需要两者时给 stdlib 加别名（罕见），
  MIGRATION.md 注明。不改设计名。

## 7. 下游迁移与发布故事

### 7.1 破坏矩阵（MIGRATION.md 主表）

| 旧 import 路径 | 新 import 路径 | 符号变化 |
|---|---|---|
| `…/provider`（实体类型） | `…/model` | `provider.{Platform,ChangeRequest,Issue,Label,Milestone,CommitStatus,Release,Webhook…}` → `model.*`（全量实体清单见 §5.2） |
| `…/provider`（机器助手） | `…/runtime` | `provider.{DetectPlatform,EnsureWebhook,EnsureBranchProtection,EnsureDeployKey,WaitForCommitStatus,ValidateWebhookWithRegistry,BatchOptions,Each,Collect,WebhookValidator…}` → `runtime.*` |
| `…/provider`（契约） | 不变 | `Provider`、`Config`、`NewProvider`、`Register`、`XxxManager` 接口、`XxxOptions`、`ErrNotFound`、`ResolvePatch`、`CapabilitySet`、`Divergence`、`Hooks`、`TokenSource`、`NormalizePageOpts`… 全部留在 `provider` |
| `…/backends/<平台>` | `…/adapter/<平台>` | 包名不变，仅路径 |
| `…/backends/contracttest` | `…/contracttest` | 包名不变 |
| `…/backends/all` | `…/adapter/all` | 包名不变 |
| `…/githubapp` | `…/auth/githubapp` | 包名不变 |
| `…/pkg/credential` | `…/auth/credential` | 包名不变 |
| `…/pkg/branchfilter` | `…/filter/branchfilter` | 包名不变 |
| `…/pkg/projection` | `…/model/projection` | 包名不变 |

MIGRATION.md 另含：典型改写前后代码块（构造 provider / 读实体 / 轮询等待 /
webhook 验签 / 空导入注册五形态）、stdlib `runtime` 别名指引、「无 shim、
一次性迁移」声明、按 §5.2 的实体全清单（下游可 grep 自查）。

### 7.2 原子发布

单分支单 release：全部分期合入 → 全量门（§9）→ tag `v0.136.0` → push main+tag
→ Release workflow。**不设中间兼容版本**；CHANGELOG `## [0.136.0]` 段以
「Breaking」开篇并指向 MIGRATION.md（0.x 破坏性 = minor 承载，仓规）。

### 7.3 mcp 两步窗口（CI 红集合与时序，预期内）

mcp/go.mod 按版本 require 父模块（Phase 1 起无 replace），窗口必然存在：

1. **分支期与合并后**：mcp 源码已改写新路径，workspace 模式（mcp job 的
   build/vet/test，go.work 在 checkout 里）绿；`GOWORK=off` 的两个检查红——
   **mcp job 的 Tidy check**（新路径不在 require 的旧版本里）与
   **mcp-install job**（`@sha` 装出的 mcp 要求父模块新路径）。根模块
   Lint/Vuln/Test 三 job 必须全程绿。
2. **父模块 v0.136.0 发布后立即**：第二步提交——`mcp/go.mod` require 抬到
   `v0.136.0` + `go mod tidy`（GOWORK=off）+ `make check-mcp`，合入后窗口闭合、
   CI 全绿。此步记为分期 S8。

窗口是设计承认的状态，不是事故：任何分支 CI 报告若红在且仅在上述两个检查，
按窗口解释；根 job 红 = 真回归。

### 7.4 并行会话防线（2A 教训前移）

- 动工前 `git status` + 文件 mtime 核对（今日已见并行会话的 2B spec 落盘）；
- 每分期提交按明确文件清单 `git add`；合并/rebase 后**必查 CHANGELOG 段归属**
  （不只看冲突——v0.135.0 曾被自动合并静默错归）；
- tag `v0.136.0` 推送前查远端 tag，若被并行会话占用则顺延 `v0.137.0` 并同步
  本 spec 版本号行。

## 8. 执行分期（SDD 任务预览；每期绿门 = 根 `GOWORK=off go build ./... &&
go test ./...` + `make check-mcp`）

| 分期 | 内容 | 改写面 |
|---|---|---|
| **S1 model 剖离** | 建 `model/`，按 §5.2 迁类型 + `commitstatus/types_deployments` 整文件 + `mentions`；`provider.go`/`options.go`/5 个 iface 文件剥离；新写 `model/doc.go` | 全仓 `provider.<实体>` → `model.<实体>`（7 adapter、contracttest、mcp、examples、smoke、genledger、provider 自身留用文件） |
| **S2 runtime 剖离** | 11 文件整迁 + `convertutil` 剥离 `ExtractMentions`；新写 `runtime/doc.go` | 全仓 `provider.<机器>` → `runtime.*` |
| **S3 adapter 拆分** | `git mv backends/* → adapter/*`（含 `internal/` 收进 `adapter/internal/`）、`contracttest/` 上顶层 | 全仓 import 路径 `backends/…` → 新路径（包名零变化）；genledger import + `go generate` 台账复核 |
| **S4 auth/filter/projection 归位** | `git mv` 三处 + `pkg/` 删除 | `gitbackend`、`examples`、`mcp`、`transport` 注释、`adapter/github` 测试的 import 改写 |
| **S5 文档面** | README 代码块、`provider/doc.go`/`model`/`runtime` 包注释、AGENTS.md 路径引用全量更新（结论不改写）、其余 `docs/` 旧路径 | 纯文档/注释，不改行为 |
| **S6 守卫 + 迁移文档** | `internal/layering` 边表守卫测试；新建 `docs/MIGRATION.md`；CHANGELOG `0.136.0` Breaking 段 | 新增文件 + CHANGELOG |
| **S7 全量验证 + 发版暂停点** | §9 全部门 → 暂停待用户确认 → tag/push/Release | — |
| **S8 窗口闭合**（父版发布后） | `mcp/go.mod` require `v0.136.0` + tidy + `make check-mcp` → 合入 → CI 全绿复核 | mcp/go.mod、mcp/go.sum |

分期顺序不可倒置的约束：S1/S2 的方向（`provider/runtime → model`）先于 S3 的路径
大挪移，避免同一提交里同时改「符号归属」和「import 路径」两类机械变更、把冲突面
翻倍。每期一个（或一串）提交，提交信息中文 conventional commits；版本号只在 S7
出现（`v0.136.0` 标题行）。

## 9. 验收标准

1. `make check`（GOWORK=off 根全系）+ `make check-mcp` 全绿；
2. 全仓 `golangci-lint run ./...`（含 examples 与 mcp/）0 issues；
3. 覆盖率 ≥ 45% 地板（测试随宿主平移，比率应近似不变；跌破必查是丢测试还是
   结构性稀释）；
4. `GOTOOLCHAIN=<go.mod 版本> govulncheck ./...` 双模块无漏洞；
5. `internal/layering` 守卫绿（§4.2 边表零违例）；
6. `go generate ./...` 后 `docs/divergence-ledger.md` 零 diff（或差异已解释）；
7. `docs/MIGRATION.md`、AGENTS.md、README、CHANGELOG 全部落位，旧路径 grep 归零
   （历史存档结论中的路径引用已更新且结论未被改写）；
8. CI 6 job 全绿（根 3 job 全程绿；mcp Tidy/install 在 S8 后绿）+ Release 成功；
9. 下游破坏面与 §7.1 矩阵逐行对应——矩阵外的公共符号零移动（S1/S2/S3 的改写
   diff 即证据）。

## 10. 明确不做（红线复述）

不加兼容别名/shim；不改接口与语义；不动 transport/gitbackend/mcp 名称；
不引入新依赖；不借机做 2B 的 Capability/Result 改造；不发 v1.0；不做第八平台、
不进服务层（评审 spec 既定红线不变）。
