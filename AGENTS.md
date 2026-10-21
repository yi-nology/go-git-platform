# AGENTS.md — AI Agent 迭代规范

本文件是面向 AI Agent（与人类贡献者）的权威规范。oekit 生态的惯例是每仓根目录一份
AGENTS.md；本仓在其基础上叠加 go-git-platform 自身的契约与发布纪律。
不确定 API 时以源码与测试为准：`go doc github.com/oekit/<mod>[/<pkg>]`、
`go doc github.com/yi-nology/go-git-platform/<pkg>`。

## 规则一：优先引用 oekit（oekit-first，硬规则）

能用 oekit 底层三模块解决的，不得手写等价物。**依赖白名单**（只许这三个，按
oekit 五层架构取最底层，避免拖入传递依赖）：

| 模块 | 用途 | 本仓接入口 |
|---|---|---|
| `github.com/oekit/pkg` | errors（Wrap/BizError）等微工具 | `oekiterrors.Wrap(err, msg, kv...)` |
| `github.com/oekit/concurrency` | async 安全 goroutine、retry、GoSafe | `async.GoSafe` / `async.GoSafeWithErrChannel` |
| `github.com/oekit/logx` | 统一日志门面 | `provider.NewLogxLogger` / `transport.NewLogxLogger` |

**禁止引入** oekit 上层模块（httpx / bootstrap / hertz / kitex / gorm / redis /
registry 等）——本仓是公共 SDK，依赖会传递给全部下游。

三条具体规则：

1. **错误处理**：需要保留 cause 链的包装一律
   `oekiterrors.Wrap(err, msg, kv...)`（nil-safe；动态上下文进 kv，不要格式化进
   消息文本）。消息必须带动态值的格式化构造（含 `%w`，可多个）一律
   `oekiterrors.Errorf`（pkg ≥v0.19.0，与 fmt.Errorf 同型：nil-safe、空串返
   nil、多 `%w` 走 `Unwrap() []error`；需 kv 时先 Errorf 再 Wrap 叠加）。
   `fmt.Errorf` 只允许两种残留形态：
   - 纯 `%w` **双链**包装——`provider/errors.go` 的 3 处是**永久例外**（分类
     哨兵 + 原始 cause 双链，oekit Wrap 单 cause 语义不覆盖），别再"迁移"它们；
   - 无 cause 的新建错误（参数校验等一次性消息）——存量（2026-10-21 审计
     59 处）不强制迁移，新代码涉及格式化一律 Errorf。
   禁止 `%v` 包装、禁止 `_` 丢弃错误。
2. **日志**：SDK 包面向 `provider.Logger` / `transport.Logger` 注入（默认
   noop）；需要接 oekit 生态时用 `NewLogxLogger(component)`（基于
   `logx.Delegating` 懒解析，component 标识子系统，service name 属于消费方）。
   可部署的二进制（如 mcp）用 `logx.WithService("name")` 拥有 service name。
   禁止新增 stdlib `log` / `fmt.Println` 调试输出。
3. **并发**：**禁止裸 `go func`**——一律 `async.GoSafe`（自动 panic recover +
   堆栈日志）；需要收集 goroutine 错误用 `GoSafeWithErrChannel`。批量/有界并发
   沿用 `provider/batch.go` 的 semaphore + `BatchResult` 模式（panic 转 item
   Err，不许崩进程）。

**明确不引入**（写了避免反复重新评估）：

- `BizError` / `mw.Fail`——HTTP 服务层概念，本 SDK 没有这一层；
- `oekit/concurrency/retry` 替换 `transport/retry.go`——后者是 HTTP 特化
  （Retry-After 头、GitHub 403 次级限流判定，判定核心已收编
  `retry/httpx`），通用原语会丢能力；
- `logx` 的 hertz/zap/zerolog 子包——根包只多带 lumberjack 一个依赖，子包会把
  整个 Hertz 栈拖进 go.mod；
- **`oagentkit/*`（姊妹矩阵，2026-10-18 全量评估后不引入）**：tier2+ 的
  llm/mcpkit/rag/agent/tools/skillkit/toolgate/obs/queue 拖 eino 全家（公共
  SDK 依赖面不可接受）；tier0 的 `runtime/procx` 环境白名单会砍
  SSH_AUTH_SOCK/GIT_* 破坏 SSH agent 认证与 hermetic 测试（SDK 的 git 子进程
  操作用户自己的仓库，威胁模型与 agent 跑不可信代码不同）；`guardx/egress`
  SSRF 防护不适用（BaseURL 由消费方控制，self-hosted 必须允许私网）；
  `guardx/permgate` 运行时逐调用判定与本仓 mcp 注册期裁剪（mutating 工具不
  挂载）形态不同；`textx/diffx` 上游明确划界（解析 vs 本仓 diff 生成领域）。
  上游若新增 tier0 原语再评估。
- **pkg v0.20.0 的 `pathx.SanitizeRel` / `urlguard.WithExactHosts`、
  v0.18.0 的 `jsonrepair.ExtractArray`、v0.19.0 的 httpjson 退避修复
  （2026-10-21 评估，均无消费点）**：本仓无用户可控仓库相对路径入口
  （gitbackend 只解析 git 输出），SSRF 面同 `guardx/egress` 结论不适用，
  jsonrepair/httpjson 不在白名单消费路径上；
- **concurrency v0.19.0 的 `syncx.Gate`、v0.18.0 的 `DoValue`（无消费点）**：
  本仓无进程级全局在飞配额需求（batch 是逐调用 semaphore），带返回值的
  重试无调用方。v0.19.0/v0.18.1 修的移位溢出与抖动 0 panic 两类 bug，
  `transport/retry.go` 的 float64 pow + 双侧钳制 + 乘法抖动实现天然免疫，
  已有回归测试钉住（TestRetry_BackoffExtremeAttemptsStayClamped）。

## 规则二：公共契约红线

- `provider` 包的哨兵错误（`ErrNotFound` 等）、`ProviderError`、`Wrap`/`Wrapf`/
  `New` 签名一律不动；分类判定只走 `errors.Is` 哨兵——**禁止匹配错误消息文本**
  （v0.87.0 起动态值是 kv 后缀，文本不稳定）。
- 给 `Provider` 或核心子接口加方法 = breaking（major 口径）；新面一律走可选
  能力接口（`iface_*.go` + `CapabilitySet`）。
- **契约测试是 spec**：改 `backends/contracttest/` 断言必须在提交信息里论证旧
  wire shape 为什么是错的。
- 平台偏离必须机器可读登记进后端 `divergence.go`，`go generate ./...` 再生成
  `docs/divergence-ledger.md`——禁止文档-only 的偏离。

## 规则三：迭代与发布套路

- 提交信息：中文 conventional commits，版本号写在标题末尾 `（vX.Y.Z）`，打同名
  git tag，push main+tag 触发 Release（测试 → `gh release create
  --generate-notes`）。**纯 docs/examples 提交不打 tag**，直接 push main。
- `CHANGELOG.md` 必须同步（Keep a Changelog 格式）；0.x 下 breaking 用 minor
  承载并在 CHANGELOG 写迁移说明。
- 收尾验证顺序：`make check` → **全仓** `golangci-lint run ./...`（含
  examples；本机 lint 版本可能比 CI 新，多报的小项顺手修）→ 覆盖率不低于 45%
  地板 → `GOTOOLCHAIN=<go.mod 版本> govulncheck ./...`。CI 有 ubuntu+macos
  矩阵，本机只跑 darwin 时更要盯 CI 结果。

## 规则四：仓库结构与验证陷阱

- **`mcp/` 是独立 go module**：主模块的 `go test ./...` /
  `golangci-lint run ./...` 不覆盖它，必须在 `mcp/` 里单独 build / test /
  lint / govulncheck。goimports 用 `-local github.com/yi-nology/go-git-platform`。
- 测试夹具不得依赖本机 git 全局身份：
  `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null go test ./gitbackend/`。
- git 行为先实测再钉测试（临时脚本验证语义后再写成集成测试）。
- 可能有其他会话并行开发本仓：动工前 `git status` + 看文件 mtime（几分钟内被
  改过 = 对方活跃）；批量改前重新 Read；提交按明确文件清单 `git add`，提交后在
  临时 worktree 验证暂存树可编译可测再 tag/push。
- go.mod 的 `toolchain` 行升级要配套 CI 工具链（golangci-lint / govulncheck
  版本钉死关系见 git log 的 v0.79.x 教训），vuln 测试要真跑目标 Go 版本。

## 与 oekit 上游的关系

- 本仓消费 oekit，不修改 oekit；oekit 各仓的 CI 规则改动只去 oekit/.github 仓，
  与本仓无关。
- oekit API 以模块源码为准（`go doc` / module cache），不要凭文档记忆写调用。
- 版本策略：`go get github.com/oekit/<mod>@latest`，不 pin 旧路径（ekit 单体 /
  yi-nology 路径均已迁移）。
