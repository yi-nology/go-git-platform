# 迁移指南：文件排布重构（v0.141.0）

> **v1 声明（2026-11-17）**：v1.0 起，0.x → v1.0 **无额外迁移**——本页
> v0.141.0 重构迁移矩阵继续有效，按其改完 import 即可用于 v1.0；v1.0 后的
> 破坏性变更走 major（[VERSIONING.md](VERSIONING.md) /
> [COMPATIBILITY.md](COMPATIBILITY.md) §5）。

> **无 shim、一次性迁移。** 本次重构（A 档纯布局拆包）**不提供任何 type alias
> 兼容层、双路径过渡或弃用期**——旧 import 路径自 v0.141.0 起全部失效，编译器
> 即迁移清单：旧引用全部报「undefined」，按本页改完即绿。0.x 破坏性变更由
> minor 承载（仓规），对应 CHANGELOG `### Changed（**BREAKING**：文件排布重构，v0.141.0）` 段。

**为什么**：把 `provider` 一个 90+ 文件的混合包拆成四层——`model`（纯域实体，
叶子）← `provider`（契约缝：接口/参数/哨兵）← `runtime`（机器与用户面助手）←
`adapter`（七平台实现）——依赖方向机器可校验（`internal/layering` 边表守卫），
下游 import 面随层收窄。**只动包边界与 import 路径，不改任何 API 语义、方法
签名与 wire shape**（包路径变更导致的引用改写除外）。

下文 `…` 一律代表 `github.com/yi-nology/go-git-platform`。

## 1. import 路径主表（spec §7.1 破坏矩阵）

| 旧 import 路径 | 新 import 路径 | 符号变化 |
|---|---|---|
| `…/provider`（实体类型） | `…/model` | `provider.{Platform,ChangeRequest,Issue,Label,Milestone,CommitStatus,Release,Webhook…}` → `model.*`（全量实体清单见 §2.1） |
| `…/provider`（机器助手） | `…/runtime` | `provider.{DetectPlatform,EnsureWebhook,EnsureBranchProtection,EnsureDeployKey,WaitForCommitStatus,ValidateWebhookWithRegistry,BatchOptions,Each,Collect,WebhookValidator…}` → `runtime.*`（全量清单见 §2.2） |
| `…/provider`（契约） | **不变** | `Provider`、`Config`、`NewProvider`、`Register`、`XxxManager` 接口、`XxxOptions`、`ErrNotFound`、`ResolvePatch`、`CapabilitySet`、`Divergence`、`Hooks`、`TokenSource`、`NormalizePageOpts`… 全部留在 `provider`（清单见 §2.3） |
| `…/backends/<平台>` | `…/adapter/<平台>` | 包名不变，仅路径（github/gitlab/gitea/forgejo/gitee/gitcode/tencentcode 七平台） |
| `…/backends/contracttest` | `…/contracttest` | 包名不变（上顶层） |
| `…/backends/all` | `…/adapter/all` | 包名不变 |
| `…/githubapp` | `…/auth/githubapp` | 包名不变 |
| `…/pkg/credential` | `…/auth/credential` | 包名不变 |
| `…/pkg/branchfilter` | `…/filter/branchfilter` | 包名不变 |
| `…/pkg/projection` | `…/model/projection` | 包名不变 |

`pkg/` 目录重构后删除（三子包全部迁出，不留空壳）。`backends/internal/{giteafamily,backendutil}`
→ `adapter/internal/{giteafamily,backendutil}`（保持 internal 可见性，外部本就不可见）。

## 2. 符号级迁移（`provider` 剖离明细）

### 2.1 实体 → `model`（spec §5.2 全清单，下游 grep 自查）

实体是平台**返回值/记录**（含其枚举值与纯函数方法）。按目标文件列出全量：

| `model/` 文件 | 迁入类型（原 `provider.*` → `model.*`） |
|---|---|
| `platform.go` | `Platform` + 七平台常量（`PlatformGitHub`/`PlatformGitLab`/`PlatformGitea`/`PlatformForgejo`/`PlatformGitee`/`PlatformGitCode`/`PlatformTencentCode`） |
| `cr.go` | `CRState`（+常量）、`ChangeRequest`、`CRUser`、`CRComment`（含 `Mentions()`）、`CRCommit`、`MergeDiff`、`ChangedFile` |
| `repo.go` | `PlatformRepo`、`PlatformBranch`、`TagInfo`、`ReleaseInfo`、`CompareResult`、`CommitInfo`、`BranchProtection`、`Collaborator`、`DeployKey`、`Contributor` |
| `event.go` | `PlatformWebhook`、`NormalizedEvent`、`EventRepo`、`TestConnectionResult` |
| `issue.go` | `IssueState`（+`IssueStateOpen`/`IssueStateClosed`）、`Issue`、`IssueComment`（含 `Mentions()`）、`IssueLabel`、`MilestoneRef`、`MilestoneState`、`Milestone`、`Label` |
| `review.go` | `ReviewState`、`Review`、`ReviewComment`（含 `Mentions()`）、`ReviewResult`、`ReviewCommentResult` |
| `search.go` | `SearchRepoResult`、`SearchIssueResult`、`SearchUserResult` |
| `notification.go` | `Notification`、`NotificationSubject`、`Reaction` |
| `file.go` | `FileResult`（`FileOptions`/`FileDeleteOptions` 是参数，**留 `provider`**，裁决 C5） |
| `gist.go` | `GistFile`、`Gist` |
| `release.go` | `ReleaseAsset` |
| `ci.go` | `JobLog` |
| `migration.go` | `MigrationInfo`（`CreateMigrationOptions` 是参数，留 `provider`） |
| `commitstatus.go` | `CommitStatus`、`CommitStatusState`（+`Normalize`/`Combine`/`Terminal` 纯函数） |
| `deployments.go` | `Deployment`、`DeploymentStatus` |
| `mentions.go` | `ExtractMentions`、`isInsideURL`（裁决 C2：三个 Comment 实体的 `Mentions()` 与之同包） |
| `projection/` | 整包 `pkg/projection` → `model/projection`（裁决 C6，包名不变） |

下游自查（命中即还有旧引用）：

```bash
grep -rnE '\bprovider\.(Platform|PlatformGitHub|PlatformGitLab|PlatformGitea|PlatformForgejo|PlatformGitee|PlatformGitCode|PlatformTencentCode|CRState|ChangeRequest|CRUser|CRComment|CRCommit|MergeDiff|ChangedFile|PlatformRepo|PlatformBranch|TagInfo|ReleaseInfo|CompareResult|CommitInfo|BranchProtection|Collaborator|DeployKey|Contributor|PlatformWebhook|NormalizedEvent|EventRepo|TestConnectionResult|IssueState|IssueStateOpen|IssueStateClosed|Issue|IssueComment|IssueLabel|MilestoneRef|MilestoneState|Milestone|Label|ReviewState|Review|ReviewComment|ReviewResult|ReviewCommentResult|SearchRepoResult|SearchIssueResult|SearchUserResult|Notification|NotificationSubject|Reaction|FileResult|GistFile|Gist|ReleaseAsset|JobLog|MigrationInfo|CommitStatus|CommitStatusState|Deployment|DeploymentStatus|ExtractMentions)\b' --include='*.go' .
```

### 2.2 机器助手 → `runtime`（spec §5.1 + R1 + 2B/2C 新增）

机器是**带行为的用户面助手**（轮询/幂等 ensure/批量/检测/验签），按源文件：

| 原 `provider/` 文件 | 新 `runtime/` | 迁入符号 |
|---|---|---|
| `manager.go` | `manager.go` | `Manager`、`Stats`、`ManagerOption`、`NewManager`、`WithMaxSize`、`WithHasher`、`HashToken` |
| `detect.go` | `detect.go` | `DetectPlatform`、`DetectResult`、`RegisterHostAlias` |
| `ensure.go` | `ensure.go` | `EnsureWebhook`、`EnsureBranchProtection`、`EnsureDeployKey`、`EnsureAction` |
| `ensure_issue.go`（R1） | `ensure_issue.go` | `EnsureIssueState`、`EnsureIssueLabels`（Ensure 机器族随 ensure.go 迁） |
| `webhook.go` | `webhook.go` | `WebhookValidator` 族（`HMACSHA256Validator`/`ForgejoWebhookValidator`/`GitCodeWebhookValidator`/`GiteeWebhookValidator`/`StaticTokenValidator`/`ValidatorFunc`）、`WebhookValidatorRegistry`、`NewWebhookValidatorRegistry`、`DefaultWebhookRegistry`、`ValidateWebhookWithRegistry`、`ReadAndRestoreBody`、`WebhookEnvelope`、`IsWebhookDeletionSHA`、`Normalize{Branch,CR,Issue,Tag}Action` |
| `batch.go` | `batch.go` | `BatchOptions`、`BatchResult`（semaphore 批量引擎）、`GetFileContents`、`GetCRs`、`SetCommitStatuses` |
| `pageiter.go` | `pageiter.go` | `PageFetch`、`Each`、`EachBounded`、`Collect`、`CollectBounded`、`ErrStopIteration`、`ErrPageBudgetExceeded` |
| `wait.go` | `wait.go` | `WaitForCommitStatus`、`WaitOptions`、`CombinedCommitStatus`、`DefaultWaitInterval`、`LatestCommitStatuses`、`ErrWaitTimedOut` |
| `janitor.go` | `janitor.go` | 后台清理循环（未导出，随宿主文件走） |
| `ttlcache.go` | `ttlcache.go` | TTL 缓存 + 自建 per-key flight（未导出） |
| `timeparse.go` | `timeparse.go` | `ParseTimestamp`（due_on 多格式解析） |
| `convertutil.go` | `convertutil.go` | 除 `ExtractMentions`/`isInsideURL` 外的全部：`SplitFullName`、`ExtractOwnerFromFullName`、`BuildEventRepo`、`ResolveMRSHAs` |

2B/2C 新增面按同一裁定归位（计划裁决 R1）：**机器结果语义进 `runtime`**——
`MutationOutcome`（`runtime/ensure.go`，`type EnsureAction = MutationOutcome` 别名
同处；Ensure 族返回值类型，原 `provider.MutationOutcome` → `runtime.MutationOutcome`）；
**能力声明留 `provider`**——`Operation`、`OperationSupport`、`OperationCapable`、
`DeriveOperationSupport`、`CheckOperation`、`AllOperations`。

三个**错误哨兵随机器迁 runtime**（`errors.Is` 引用同步改包前缀）：
`runtime.ErrWaitTimedOut`、`runtime.ErrStopIteration`、`runtime.ErrPageBudgetExceeded`。

下游自查：

```bash
grep -rnE '\bprovider\.(DetectPlatform|DetectResult|RegisterHostAlias|EnsureWebhook|EnsureBranchProtection|EnsureDeployKey|EnsureIssueState|EnsureIssueLabels|EnsureAction|MutationOutcome|WaitForCommitStatus|WaitOptions|CombinedCommitStatus|DefaultWaitInterval|LatestCommitStatuses|ErrWaitTimedOut|ErrStopIteration|ErrPageBudgetExceeded|ValidateWebhookWithRegistry|DefaultWebhookRegistry|NewWebhookValidatorRegistry|WebhookValidator|WebhookValidatorRegistry|ReadAndRestoreBody|WebhookEnvelope|IsWebhookDeletionSHA|BatchOptions|BatchResult|PageFetch|Each|EachBounded|Collect|CollectBounded|Manager|Stats|ManagerOption|NewManager|WithMaxSize|WithHasher|HashToken|GetFileContents|GetCRs|SetCommitStatuses|SplitFullName|ExtractOwnerFromFullName|BuildEventRepo|ResolveMRSHAs|ParseTimestamp)\b' --include='*.go' .
```

### 2.3 契约留守 `provider`（一切未被上两节点名的导出面）

- **核心契约**：`Provider` 接口、`Config`、`NewProvider`、`Register` 族
  （`Register`/`RegisteredPlatforms`）、`Hooks`（请求/响应生命周期钩子）、
  `TokenSource`/`StaticTokenSource`、`RetryConfig`/`DefaultRetryConfig`、
  `Logger`/`NewLogxLogger`/`NewNoopLogger`、`CapabilitySet`/`DeriveCapabilities`、
  `Divergence`/`DivergenceKind`、`ResolvePatch`（泛型三态解析）、
  `NormalizePageOpts`、`RateLimitRecovery`、`Version`；
- **能力声明（R1）**：`Operation`、`OperationSupport`、`OperationCapable`、
  `DeriveOperationSupport`、`CheckOperation`、`AllOperations`；
- **32 个 `XxxManager` 接口**：`IssueManager`、`RepoManager`、`WebhookManager`、
  `BranchProtectionManager`、`CommitStatusManager`、`DeploymentKeyManager`、
  `ChangeRequestManager`、`ReviewManager`、`ReleaseManager`、`GistManager`、
  `NotificationManager`、`SearchManager`、`FileManager`、`DiffManager`、
  `LabelManager`、`MilestoneManager`、`CollaboratorManager`、`BranchManager`、
  `CommitManager`、`CILogManager`、`DeploymentManager`、`ForkSyncManager`、
  `MigrationManager`、`NoteManager`、`ReactionManager`、`ReleaseAssetManager`、
  `RepoStatsManager`、`StarredManager`、`TeamReviewManager`、`TemplateRepoManager`、
  `UserManager`、`WebhookConfigManager`（自查：
  `grep -rn 'type [A-Z][A-Za-z0-9]*Manager interface' provider/`）；
- **全部 `XxxOptions` 参数结构（裁决 C5）**：iface 方法的入参归契约缝，留
  `provider`——含 `CreateWebhookOptions`、`FileOptions`/`FileDeleteOptions`、
  `CreateMigrationOptions`、`CreateDeploymentOptions` 等（自查：
  `grep -rn 'type [A-Z][A-Za-z0-9]*Options struct' provider/`）；
- **错误面**：`ProviderError`、`Wrap`/`Wrapf`/`New`/`WrapStatusError`，哨兵
  `ErrNotFound`、`ErrNotImplemented`、`ErrForbidden`、`ErrAuthentication`、
  `ErrRateLimited`、`ErrConflict`、`ErrGone`、`ErrInvalidInput`、`ErrServerSide`、
  `ErrPlatformNotSupported`、`ErrPermissionDenied`、`ErrDraftNotSupported`、
  `ErrWebhookValidation`、`ErrWebhookFiltered`、`ErrClearNotSupported`、
  `ErrConnectionFailed`，及 `IsNotFound`/`IsRateLimited`/… 分类器——**全部留
  `provider`**（仅 `ErrWaitTimedOut`/`ErrStopIteration`/`ErrPageBudgetExceeded`
  三个例外，见 §2.2）；
- **共享纯助手**：`MapStateToCR`/`MapMRStateToCR`/`MapBoolStateToCR`、
  `ClassifyStatus`、`DecodeFileContent`、`BuildRawDiff`/`CountDiffLines`/
  `SumDiffStats`、`FindByMethod`、`ParseTotalCountHeader`、`Ignores`/`Stubs`/
  `IsStub` 等（adapter 直接消费、非机器）。

### 2.4 `EnsureSecretMode` 留守 `provider`（S2 carry 裁决，勿找 `runtime.EnsureSecretMode`）

`EnsureSecretMode` 类型及常量 `EnsureSecretKeep`/`EnsureSecretAlwaysUpdate`
**留在 `provider/options.go`**（唯一结构消费方 `CreateWebhookOptions.EnsureSecretMode`
字段同处）：spec §5.1 曾把它列在 `ensure.go → runtime` 迁移行内，但移动会制造
被禁的 `provider → runtime` 反向边（`CreateWebhookOptions` 在 provider、
`EnsureWebhook` 在 runtime，编译器成环）。下游引用一律
`provider.EnsureSecretAlwaysUpdate` 等——**不存在 `runtime.EnsureSecretMode`**。

## 3. 五形态改写对照（before / after）

### 3.1 构造 provider

```go
// 旧（v0.139.0 及以前）
import (
	_ "github.com/yi-nology/go-git-platform/backends/all"
	"github.com/yi-nology/go-git-platform/provider"
)

p, err := provider.NewProvider(provider.Config{
	Platform: provider.PlatformGitHub,
	Token:    token,
})
```

```go
// 新
import (
	_ "github.com/yi-nology/go-git-platform/adapter/all"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

p, err := provider.NewProvider(provider.Config{
	Platform: model.PlatformGitHub, // Platform 枚举下沉 model（裁决 C1）
	Token:    token,
})
```

`NewProvider`/`Config` 不变；只有 `Platform`（及 `Config` 内引用的实体类型）
换包前缀。

### 3.2 读实体

```go
// 旧
var iss *provider.Issue
iss, err = mgr.GetIssue(ctx, owner, repo, "42") // mgr 是 provider.IssueManager
for _, name := range iss.Labels { // Labels []string
	fmt.Println(provider.IssueStateOpen, name)
}
```

```go
// 新
var iss *model.Issue // 实体类型全量换 model.*
iss, err = mgr.GetIssue(ctx, owner, repo, "42") // 接口与方法名不变，返回类型路径已换
for _, name := range iss.Labels {
	fmt.Println(model.IssueStateOpen, name)
}
```

`IssueManager` 接口本身留 `provider`（`GetIssue` 返回 `*model.Issue`——签名的
包路径变更，A 档允许）。

### 3.3 轮询等待（CI 门）

```go
// 旧
st, err := provider.WaitForCommitStatus(ctx, p, owner, repo, sha, provider.WaitOptions{
	Interval: 2 * time.Second,
	Timeout:  10 * time.Minute,
})
if errors.Is(err, provider.ErrWaitTimedOut) {
	// 超时 ≠ 失败：st 仍是 pending
}
```

```go
// 新
st, err := runtime.WaitForCommitStatus(ctx, p, owner, repo, sha, runtime.WaitOptions{
	Interval: 2 * time.Second,
	Timeout:  10 * time.Minute,
})
if errors.Is(err, runtime.ErrWaitTimedOut) { // 哨兵随机器迁 runtime
	// 超时 ≠ 失败：st 仍是 pending
}
```

`st` 的类型两侧都是 `CommitStatusState`——旧 `provider.CommitStatusState` →
新 `model.CommitStatusState`。

### 3.4 webhook 验签

```go
// 旧
if err := provider.DefaultWebhookRegistry().Validate(
	provider.PlatformGitHub, req, body, secret,
); err != nil {
	return err
}
// 一次性形态：provider.ValidateWebhookWithRegistry(provider.PlatformGitHub, r, secret)
```

```go
// 新
if err := runtime.DefaultWebhookRegistry().Validate(
	model.PlatformGitHub, req, body, secret,
); err != nil {
	return err
}
// 一次性形态：runtime.ValidateWebhookWithRegistry(model.PlatformGitHub, r, secret)
```

验签哨兵 `provider.ErrWebhookValidation` 留 `provider`（`errors.Is` 不用改）。

### 3.5 空导入注册

```go
// 旧
import _ "github.com/yi-nology/go-git-platform/backends/all"
// 或单平台：import _ "github.com/yi-nology/go-git-platform/backends/github"
```

```go
// 新
import _ "github.com/yi-nology/go-git-platform/adapter/all"
// 或单平台：import _ "github.com/yi-nology/go-git-platform/adapter/github"
```

`contracttest` 上顶层：`…/backends/contracttest` → `…/contracttest`（仅测试
依赖方需要改）。

## 4. stdlib `runtime` 撞名（裁决 C8）

仓内新包名 `runtime` 与标准库 `runtime` 同名。**只用本仓 `runtime` 的文件无需
任何处理**；同一文件两者都要用时，给标准库加别名（罕见，标准库侧加别名而非本仓）：

```go
import (
	stdruntime "runtime" // 裁决 C8：同文件需要 stdlib runtime 时给 stdlib 加别名

	"github.com/yi-nology/go-git-platform/runtime"
)

_ = stdruntime.NumCPU()             // 标准库
_, _ = runtime.DetectPlatform(url)  // 本仓机器层
```

## 5. 迁移自查清单

1. **路径**：§1 主表逐行——`backends/`、`githubapp`、`pkg/` 前缀在你的
   `go.mod` 消费面归零（`grep -rn 'go-git-platform/\(backends\|pkg\)/\|go-git-platform/githubapp' --include='*.go' .`）。
2. **实体**：§2.1 末尾 grep 归零。
3. **机器**：§2.2 末尾 grep 归零（注意 `ErrWaitTimedOut` 等三个哨兵的包前缀）。
4. **契约不动**：`NewProvider`/`Register`/`XxxManager`/`XxxOptions`/`ErrNotFound`
   等仍从 `provider` 导入，无需改。
5. **`EnsureSecretMode`**：从 `provider` 引（§2.4），不要找 `runtime`。
6. 全部改完 `go build ./...` 即绿——没有过渡层可绕，编译器就是清单。
