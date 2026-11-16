# 版本策略（Versioning）

> 消费方口径的模块版本策略与版本语义汇总。兼容性承诺正文以
> [CONTRIBUTING.md](../CONTRIBUTING.md)「Versioning & compatibility commitment」
> 节为单一真源——本文引用其结论、不复制正文；兼容矩阵见
> [COMPATIBILITY.md](COMPATIBILITY.md)，迁移指南见 [MIGRATION.md](MIGRATION.md)。

## 1. 主模块 tag 语义

主模块 `github.com/yi-nology/go-git-platform` 按 SemVer 打 `vX.Y.Z` tag；tag
push 触发 Release workflow（测试 → `gh release create --generate-notes`，
见 `.github/workflows/release.yml`）。版本段含义：

| 版本段 | 内容 | 先例 |
|---|---|---|
| patch（vX.Y.**Z**） | bug 修复，无 API 面变化 | — |
| minor（vX.**Y**.Z） | 新能力（新增可选能力接口 / 字段 / 方法）；0.x 期 **breaking 亦由 minor 承载**（CONTRIBUTING 的 0.x convention），CHANGELOG 条目必须以迁移说明开头 | v0.73.0 删 `provider.ListAllPages`；v0.141.0 文件排布重构（BREAKING，迁移矩阵见 MIGRATION.md） |
| major（**vX**.Y.Z） | v1.0 起：破坏性变更 = major。什么算 breaking / 什么算 minor，以 CONTRIBUTING「What counts as breaking (v1.0 contract)」分类表为准 | — |

- **纯 docs / examples 提交不打 tag，直接 push main**（仓规——Release 只由
  `v*` tag 触发，docs 变更不产生发布物）。
- **就绪 ≠ 官宣**：v1.0 tag 由维护者择时执行；0.x → v1.0 无额外迁移（见
  MIGRATION.md 头部声明）。

## 2. mcp 子模块

- `mcp/` 是**独立 go module**（`github.com/yi-nology/go-git-platform/mcp`），
  **不发 `mcp/` 前缀 tag**（2026-11-17 实测：179 个 `v*` tag 全部无 `mcp/`
  前缀——既定实践）。
- `mcp/go.mod` 对父模块的 require **人工跟随**：父模块发版后由维护者把
  require 抬到新父 tag 并 `GOWORK=off go mod tidy`。
- **两步窗口**：父 tag 发布与 mcp require 抬版之间存在窗口期，期间
  `mcp/go.mod` 仍指上一版父模块；窗口随抬版闭合。从源码构建 mcp 时请基于
  require 已抬至目标父版本的提交。发布产物不受窗口影响——Release 构建段在
  workflow 内临时抬版闭窗、构建完即弃（release.yml 构建段）。
- **MCP 二进制挂父 tag 的 GitHub Release**（release.yml 构建段，第 4 期
  v1.0 准备落地）。二进制的运行时版本串经 Go 构建信息自动解析（父模块依赖
  版本），无额外注入依赖。
- 源码安装：`go install github.com/yi-nology/go-git-platform/mcp/cmd/go-git-platform-mcp@latest`
  ——mcp 无独立 tag，`@latest` 解析为 mcp 目录**最新提交的伪版本**
  （`v0.0.0-<时间戳>-<hash>`；2026-11-17 对 proxy.golang.org 实测）。需要
  确定性安装时用 `@<commit-sha>`，或直接取父 tag Release 的预构建二进制。

## 3. v1 金样纪律（API 面冻结）

- 公共 API 冻结面 = `docs/api-surface-v1.txt` 金样（15 包导出符号清单；
  `internal/` 与 `mcp/` 不进），机器守卫 `internal/apisurface` 挂
  `go test ./...`（即 `make check`）。守卫随第 4 期（v1.0 准备）落地，
  **v1.0 tag 起即为 v1 冻结契约的机器生效边界**。
- **v1 期任何 API 面变化 = 显式动作**：`go run ./internal/tools/gensurface`
  重生成金样 + CHANGELOG 条目（增 / 删 / 改签名同权，按 §1 版本段语义归位）。
- 金样更新**不挂 `go:generate`**——重生成是显式审查动作，不随 generate
  自动重写。

## 4. 下游引用口径

```sh
# 主模块（SDK）
go get github.com/yi-nology/go-git-platform@latest

# MCP 服务器（源码安装；伪版本语义见 §2）
go install github.com/yi-nology/go-git-platform/mcp/cmd/go-git-platform-mcp@latest
```

平台兼容面（平台 × 能力矩阵、Go 地板、平台实测覆盖面）见
[COMPATIBILITY.md](COMPATIBILITY.md)。
