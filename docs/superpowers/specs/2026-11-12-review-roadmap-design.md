# 评审后四期演进设计（2026-11-12）

来源：外部技术评测报告（基准 commit `387a083c`，已逐项现场核实）。目标：把
go-git-platform 从「功能丰富的 SDK」提升为「接口稳定、语义可信、安全可控、可
独立交付的基础组件」。四期顺序执行，每期独立验收，可独立发版。

## 第 1 期：稳定性与安全加固

### 1.1 MCP HTTP 默认安全（P0）

- `--http` 未显式给 host 时（如 `:8080`）默认绑定 `127.0.0.1`；显式
  `0.0.0.0`/公网地址时必须在启动期校验 `--http-token` 非空，否则拒绝启动。
- 校验逻辑抽成可单测的纯函数（如 `resolveHTTPAddr(addr, hasToken) (listen, error)`）。
- 启动日志不打印 token；`--skip-tls` 与对外监听组合时输出安全告警日志。

### 1.2 toolRegistry 并发竞争（P1）

- `toolRegistry` 从包级可变 map 改为构造期不可变注册表：工具集注册仍在
  `NewServer` 内完成，`recordTool` 的台账写入加 `sync.Mutex`（最小改动、保
  持 #122 的 README 对账机制），并新增并发构造 `NewServer` 的 `-race` 测试。

### 1.3 MCP 独立分发（P1）

- `mcp/go.mod` 移除 `replace github.com/oekit/... => ../`（require 已跟
  v0.128.0）；本地多模块开发改用根 `go.work`（不入库或入库后 CI 显式
  `GOWORK=off` 验证发布形态）。
- CI 新增 job：在干净临时目录 `go install
  github.com/yi-nology/go-git-platform/mcp/cmd/go-git-platform-mcp@<tag 或
  commit>` 并跑 `--help`，验证独立安装路径真实可用。

### 1.4 CI tidy 校验化（P2）

- mcp job 的 `go mod tidy` 改为「校验工作区无差异」（`go mod tidy &&
  git diff --exit-code`），让模块元数据漂移在 CI 显式失败而非被掩盖。
- 同步在 `make check` / AGENTS.md 记录 mcp 模块需要人工 tidy 跟随根模块的
  既有套路。

### 1.5 验收

P0 清零；匿名远程写操作不可达（新增单测钉死）；`go install` 独立安装可用；
`make check` + 全仓 lint + mcp 独立 lint 全绿。

## 第 2 期：API 契约收敛

### 2.1 Update 三态语义（P1）

- `provider/options.go` 的 Update 结构（`UpdateCROptions.Assignees` 等）引入
  显式三态：未指定 / 置空 / 置值。0.x 下不破环现有字段：新增 `*[]string`
  指针字段族或 `PatchField[T]` 泛型助手，旧字段标记 deprecated 并在文档写
  迁移说明；各后端适配层逐个接线，空值语义以平台 wire 真值为准（先实测再
  钉测试）。
- 目标：通用 SDK 能表达「清空全部指派人」。

### 2.2 操作级 Capability（P1）

- 保留 `CapabilitySet`，新增可选能力接口（如 `OperationCapability`）表达
  「仓库级通知支持、全局通知不支持」「当前 token 无写权限」等操作级粒度；
  平台不支持与凭据无权限分开表示（前者 `ErrNotImplemented` 哨兵，后者
  新的权限类错误）。
- 每个新面走 `iface_*.go` 可选能力接口模式，不加 `Provider` 核心方法。

### 2.3 错误分类与统一 Mutation Result（P2）

- 统一写操作返回语义：`Created / Updated / Unchanged / Conflict`，供自动化
  幂等执行。
- 错误分类矩阵文档化 + 行为契约进 contracttest。

### 2.4 验收

核心 Update 路径有三态语义 + 各后端行为契约；7 个未纳入共享行为套件的
可选接口族补齐 contracttest 断言；`go generate ./...` 分歧台账再生成。

## 第 3 期：兼容性与测试工程化

- 七平台环境矩阵 + 只读日常巡检 job（workflow_dispatch 手动触发，复用现有
  smoke.yml 形态扩面：GetRepo/ListRepos → CR、Webhook、分页、权限边界）。
- 沙箱写操作 E2E（gitea/forgejo docker 容器内）+ 异常注入（故障响应体、
  截断 JSON、伪翻页）。
- 关键域覆盖率目标 ≥80%（transport 重试/ETag、gitbackend、webhook 验签、
  provider 分页器），不是刷总覆盖率。

## 第 4 期：v1.0 准备与生态交付

- 冻结进入 v1 的公共 API 清单 + 迁移指南 + 正式兼容矩阵。
- MCP 二进制发布：goreleaser 或 CI 手工构建多平台二进制 + 容器镜像 +
  SBOM + checksum + provenance。
- 模块版本策略文档化（主模块与 mcp 模块 tag 跟随规则）。

## 明确不做

- 不新增第八个 Git 平台。
- 不把分布式任务队列/Webhook 持久化/审计放进 SDK（属于 git-ferry /
  MCP Gateway 的服务层）。
- 不引入 CloudWeGo 等上层框架到核心依赖（AGENTS.md 依赖白名单不变）。
