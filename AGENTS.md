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
   `fmt.Errorf` 只允许一种残留形态：
   - 纯 `%w` **双链**包装——`provider/errors.go` 的 3 处是**永久例外**（分类
     哨兵 + 原始 cause 双链，oekit Wrap 单 cause 语义不覆盖），别再"迁移"它们。
   （2026-11-04 起：无 cause 格式化错误的存量容忍区已取消——当日
   「能用尽用」指令下 50 处主模块 + 9 处 mcp 全量迁移 `oekiterrors.Errorf`，
   仅余上述 3 例外；此后 `fmt.Errorf` 在 SDK/mcp 源码中出现即违规。）
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
  - **2026-11-06 再评估（结构演进后复核）**：oagentkit 已拆为 13 个独立
    发版仓（`oagentkit/agentkit` 门面废弃）。三点结论更新：① `guardx/egress`
    新增 `PrivateNoLoopback` 具名策略并明确「内网自建服务是合法场景」
    ——本仓 self-hosted 顾虑已被上游消化，2026-10-18 的「不适用」结论
    由「不适用但上游已支持」取代（仍不引入：无消费点）；② `procx.ChildEnv`
    新增 extra 按名/字面透传（白名单顾虑部分缓解），但仍有两缺口已提
    [runtime#25](https://github.com/oagentkit/runtime/issues/25)（硬编码
    尾部追加与 extra 字面注入的重复 environ key 冲突）与
    [runtime#26](https://github.com/oagentkit/runtime/issues/26)（缺
    inherit-all-except 模式——git 包装 SDK 的可信父进程场景）；③
    `guardx/permgate` 新增注册期 `Capability` 自报（与运行时 Check 组合），
    形态与本仓 mcp 注册期裁剪趋近但仍偏运行时。tier2+ 拖 eino 结论不变。
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
- **pkg v0.28.0 / concurrency v0.22.0 全量子包重评估（2026-11-02，无新增
  引入；先行审计再动工，本条即结论存档）**。当前基线（已接入的上游新原
  语，后续迭代直接沿用、勿手写等价物）：`syncx.FanInLimitCtx`（batch 引
  擎，panic→item Err）、`pkg/cache`（`LocalCache`+`SingleFlightCache`，
  provider/ttlcache.go 全量委托）、async v0.22 变体（`GoSafeLabeled`=
  provider/janitor、`GoSafeWithErrChannel`=mcp serve）、
  `concurrency/retry/httpx`（transport 重试判定）。本轮逐包对照后确认无
  手写等价物/不适用：`errors` 新增的 `Append`/`Chain`/`In`（仓内无错误聚
  合、无清理链、无多哨兵 Is 链）；`cache.Checker`（无健康检查回源形态）；
  `pagination`（服务端 PageRequest/PageResponse，与本仓客户端翻页器
  provider/pageiter.go 不同域）；`convert`（any 强转，本仓 Parse* 是校验
  型拒绝语义）；`timex`（日期算术/格式化，无多 layout 解析原语，
  gitcode/gitee 的 due_on 多格式解析保留为领域助手）；`shellx`（POSIX
  shell 转义，gitbackend 走 argv 直 exec 不经 shell）；`container`
  （**2026-11-04 起采纳 SafeMap + Slice 全量**：用户授权 oekit 全量化的
  breaking 重构后存量裁定更新——provider 注册表/webhook validator 注册表/
  statusExtractors/credential hostKeys/gitbackend 后端注册表五处
  mutex+map 已迁 SafeMap，
  GetOrSet 的原子性顺带把 credential 的 TOFU 双检锁与注册表的查重 panic
  简化为一步；detect.go hostAliases 在 v0.34.1 的 `AppendIfAbsent`
  （原子查重+追加，issue#178）落地后已迁 `container.Slice`——子域
  first-match 的有序性由 Slice 保持，查重+追加的原子性由原语承诺；
  janitor 的 mutex 是状态字段
  非 map，不在容器范畴）；`bloomx`/`pack`/`strmap`/`encoding`/`cli`/
  `contextx`/`bufiox`/`enumx`/`mathx`/`stringx`/`slicex`/`ptrx`（无匹配
  形态：本仓无布隆/迁移资产包/CLI 分发需求，指针助手用外部 SDK 自带的，
  contains 类循环用 stdlib slices）。
- **pkg v0.29–v0.33 / concurrency v0.23.x / logx v0.8.x 增量审计（2026-11-04，
  随「git 相关依赖全量升级」先行审计再动工，无新增引入；本条即结论存档）**。
  本轮依赖线：pkg v0.28→v0.33、concurrency v0.22→v0.23.4、logx v0.7.1→
  v0.8.2，另 go-github v92→v93 / gitlab client-go v3.40→v3.43 / gitea sdk
  v1.4→v1.5.1（go-git/forgejo/gitcode/gongfeng/gitee/x/* 已在最新）。pkg
  侧逐版本对照：v0.29 为大批 bugfix 发版（cache 六连修 + `HashKey` 长度前缀
  无歧义编码——经 provider/ttlcache.go 全量委托**被动受益**，无引入点）；
  v0.30 `singleflighttest` 契约套件（无自研 singleflight）、`convert` 增
  nil 安全 `Opt*`/`AnyToStrOr`/`AnyToInt32`（mcp 层经 MCP SDK 类型化解码，
  无 any→T 手写解析点）；v0.31 `cli` 子命令别名/Table/Report（SDK 无 CLI
  分发面）；v0.32 `stringx.LastN`、`cache.NXSetter`/`SetNX`（无 NX 语义
  消费点）；v0.33 `stringx.Filter`/`KeepFunc`/`DeleteFunc`、cache 选择性
  stale（无字符串过滤与 stale 兜底消费点）。concurrency v0.23.x 全是
  singleflight（DoChan 收尾）/budget（溢出+`Used`）/gopool（Close 竞态）/
  breaker（重复样本）修复——本仓不消费这四个包（batch 引擎 `syncx.FanInLimitCtx`
  未受影响），被动受益。logx v0.8.0 `Nop()`/`LevelOff`/`ParseLogLevel("OFF")`
  ——SDK 注入面默认 noop 自维护（引 logx.Nop 会让 logx 变硬依赖，违反注入
  纪律）；mcp 二进制无「彻底关日志」需求。v0.7.2 `WithAttrs` 脱敏修复经
  transport `NewLogxLogger` 桥被动受益。全量结论：零新增引入，升级收益为
  被动 bugfix（pkg/cache、async、logx 底座）。
- **pkg v0.34–v0.35 增量审计（2026-11-06，随上游反馈闭环补记）**：v0.34.1
  `container.Slice.AppendIfAbsent`（#178 下游提案落地，detect.go hostAliases
  已迁 Slice——见 container 条款）与 `ErrActionRunNotFound` 配套面；v0.35.0
  新原语 `pathx.MatchSimpleGlob`（单星+次星字面量——`pkg/branchfilter` 需
  多星 + filepath.Match 语义（`*` 不跨 `/`、`feature/*/*` 多层），形态
  不匹配不引入，branchfilter 保留 stdlib）；`stringx.MaskJSONFields`
  （JSON 字段级脱敏——暂无消费点，webhook 日志脱敏若长出需求再评估）；
  v0.35.1 全量 bugfix（mathx NaN 防护/stringx FirstN 口径/Union 保序/
  cron 方言/cache Key 契约警示/httpjson 204 空体对偶面）——经 ttlcache
  委托与错误面被动受益。下游反馈回流三连：timex 多 layout 解析（#197）、
  bufiox 行粒度尾部窗口（#198）、Slice.RemoveIf（#199）——均出自本仓
  真实消费形态（due_on 解析/CILogManager 尾部截断/别名注销），上游采纳
  后按「能用尽用」收编。

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
