# 兼容矩阵（Compatibility）

> 平台 × 能力、Go 版本地板与平台实测覆盖面的消费方口径。能力矩阵**真源 =
> README 表 + 机器对账测试**，本文快照仅供参考、不构成承诺。版本语义见
> [VERSIONING.md](VERSIONING.md)，破坏分类表见
> [CONTRIBUTING.md](../CONTRIBUTING.md)「What counts as breaking (v1.0 contract)」。

## 1. 平台 × 能力矩阵

**真源**：README「Capability-gated optional APIs」矩阵 ↔
`adapter/all/readme_matrix_test.go` 的
`TestReadmeMatrixMatchesCapabilities`——按七个平台真实构造 provider，逐格
对账 `Capabilities()` 声明（矩阵格非 `—` ⇔ 该平台声明该能力），反向断言
`CapabilitySet` 每个字段都有矩阵行。矩阵漂移会被 `go test ./...` 红灯拦下。

`Provider` 组合 8 个核心子接口（repos / change requests / webhooks /
branches / diffs / commits / files / releases）+ 23 个可选能力接口（经
`Capabilities()` 声明、类型断言消费；缺席 = 不声明，永不以 stub 表达）。

**快照**（快照日期 2026-11-17；对账测试 `adapter/all.TestReadmeMatrixMatchesCapabilities`）：

| Capability | GH | GitLab | Gitea | Forgejo | Gitee | GitCode | Tencent | Interface |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|---|
| Issues | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `IssueManager` |
| Search | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — | `SearchManager` |
| Labels | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `LabelManager` |
| Milestones | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `MilestoneManager` |
| Reviews | ✅ | ✅ | ✅ | ✅ | — | ✅ | ✅ | `ReviewManager` |
| CommitStatuses | ✅ | ✅ | ✅ | ✅ | ✅ | ❌¹ | ✅ | `CommitStatusManager` |
| Notifications | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️² | — | `NotificationManager` |
| Reactions | ✅ | ✅ | ✅ | ✅ | — | ❌¹ | — | `ReactionManager` |
| BranchProtections | ✅ | ✅ | ✅ | ✅ | ✅ | ❌¹ | ✅ | `BranchProtectionManager` |
| Collaborators | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `CollaboratorManager` |
| WebhookConfig | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `WebhookConfigManager` |
| DeployKeys | ✅ | ✅ | ✅ | ✅ | ✅ | ❌¹ | — | `DeploymentKeyManager` |
| RepoStats | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `RepoStatsManager` |
| Users | — | ✅ | — | — | — | — | ✅ | `UserManager` |
| Gists | ✅ | — | — | — | — | — | — | `GistManager` |
| Starred | ✅ | — | — | — | — | — | — | `StarredManager` |
| Migrations | ✅ | — | — | — | — | — | — | `MigrationManager` |
| ReleaseAssets | ✅ | — | — | — | — | — | — | `ReleaseAssetManager` |
| CILogs | ✅ | ✅ | ✅ | — | — | ✅⁵ | — | `CILogManager` (`Capabilities().CILogs`) |
| Deployments | ✅ | — | — | — | — | — | — | `DeploymentManager` (`Capabilities().Deployments`) |
| RepoTemplates | ✅ | — | ✅ | — | — | — | — | `TemplateRepoManager` (`Capabilities().RepoTemplates`) |
| ForkSync | ✅ | — | — | — | — | — | — | `ForkSyncManager` (`Capabilities().ForkSync`) |
| TeamReviews | ✅ | — | ✅ | ✅ | — | — | — | `TeamReviewManager` (`Capabilities().TeamReviews`) |

脚注（¹ GitCode 部分能力 live 404 已摘除；² 通知仅仓库域可用；⁵ CI 失败
日志走 v8 Actions 面）的完整语义与证据链以 README 对应节与
[divergence-ledger.md](divergence-ledger.md) 为准。对账测试按「非 `—` 即
声明」归一判定脚注格（`❌` 前缀 = 未声明），受限细节由
`contracttest/capabilities.go` 双向断言与分歧台账承载。

## 2. Go 版本地板

主模块与 mcp 模块的 `go.mod` 均声明 `go 1.27.4`——消费方需要
**Go ≥ 1.27.4**。CI 以 `go-version-file: go.mod` 取工具链版本；go.mod 顶行
即真源。

## 3. mcp ↔ 父模块耦合

- mcp 是独立 go module，对父模块的 require **人工跟随**（父模块发版后抬版
  + `GOWORK=off go mod tidy`）；两步窗口规则与发布形态见
  [VERSIONING.md](VERSIONING.md) §2。
- 两者独立升级：SDK 消费方 `go get` 主模块不需要同步动 mcp；从源码构建
  mcp 时注意两步窗口（require 未抬版期间仍指上一版父模块）。
- mcp 不进主模块的 v1 金样冻结面（独立模块；冻结面见 VERSIONING.md §3）。

## 4. 平台实测覆盖面

| 层 | 形态 | 覆盖面 | 入口 |
|---|---|---|---|
| wire 契约 | `contracttest.Run` 挂在七平台 `adapter/<platform>/contract_test.go`（httptest wire 面） | GitHub / GitLab / Gitea / Forgejo / Gitee / GitCode / TencentCode | `go test ./...` 常跑 |
| 沙箱写路径 E2E | `make e2e`（`-tags=e2e ./e2e/...`）；镜像钉死 Makefile 变量 `GITEA_IMAGE ?= gitea/gitea:28.4`、`FORGEJO_IMAGE ?= codeberg.org/forgejo/forgejo:17.0.1`（可覆盖）；本地一次性起容器（无 Docker 跳过并说明），CI 注入 `E2E_GITEA_URL` / `E2E_FORGEJO_URL` + tokens | Gitea / Forgejo 写往返 | `make e2e` / e2e workflow |
| 巡检（只读探针） | smoke workflow 手动 dispatch，7 平台 × 6 家族（repo / cr / webhook / pagination / errors / connection）；fail-fast=false，单平台红不串扰 | 七平台只读；`GIT_PLATFORM_SMOKE_TOKEN_<PLATFORM>` secrets 未配时 SKIP（待配——无 secrets 的 dispatch 是 no-op） | `.github/workflows/smoke.yml` |
| 能力声明对账 | README 矩阵 ↔ `Capabilities()` 逐格机检（见 §1） | 七平台声明面 | `go test ./adapter/all/` |
| 公共面冻结 | 金样 `docs/api-surface-v1.txt` + `internal/apisurface` 守卫 | 15 包导出符号（见 VERSIONING.md §3） | `go test ./...`（第 4 期落地） |

平台偏离（divergence）登记在各后端 `divergence.go` 并由
`go generate ./...` 再生成 `docs/divergence-ledger.md`，漂移由机器检查
拦——平台差异的细粒度事实以台账为准，禁止文档-only 偏离。

## 5. v1 语义

- 0.x 期：breaking 由 minor 承载，CHANGELOG 条目以迁移说明开头（先例
  v0.73.0 删 `provider.ListAllPages`、v0.141.0 文件排布重构）；
- **v1.0 起：破坏 = major，minor = 新能力，patch = 修复**。什么算
  breaking / 什么算 minor 以
  [CONTRIBUTING.md](../CONTRIBUTING.md)「What counts as breaking (v1.0
  contract)」分类表为准，版本策略见 [VERSIONING.md](VERSIONING.md)；
- 0.x → v1.0 无额外迁移（[MIGRATION.md](MIGRATION.md) 头部声明）；平台
  偏离登记纪律（divergence 台账 + 对账测试）在 v1 期不变。
