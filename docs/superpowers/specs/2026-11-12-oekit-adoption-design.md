# oekit 收编优化设计（2026-11-12）

来源：用户指令「尽可能使用 oekit 进行优化当前项目」。方法：仓内手写等价物
全量扫描（10 个模式族）× oekit 三模块当前 API 面盘点（pkg v0.46.0 34 包 /
concurrency v0.34.0 19 包 / logx v0.10.0，全部 `go doc` 实证）× AGENTS.md
既有存档交叉比对。**依赖白名单不变**（pkg/concurrency/logx 三模块，
AGENTS.md 规则一硬规则）；既有「定性不引入（勿再评估）」存档降级为待验证
假设，扫描后仍成立的维持，实锤手写重复的推翻。

## 扫描总结论

仓内 oekit 化程度已高：裸 `go func`（生产）、`%v` 丢 cause、rune 不安全
截断、多 layout 日期循环、错误聚合、sentinel 手写循环、新 LRU/TTL/ring/
bloom、ETag 键/路径分类/Retry-After/敏感头剥离——全部零命中或已收编。
真实收编点 = 下列 Tier 1/2；存档复核成立的 = Tier 3。

## Tier 1（明确收编）

| # | 现状 | 收编为 |
|---|---|---|
| A | `mcp/server.go` toolRegistry mutex+嵌套 map | 拍平 key 迁 `container.SafeMap[string,bool]` + 读取 helper（第 6 处注册表迁移） |
| B | 5 处 `GIT_SSH_COMMAND` 用 `%q` 拼 shell（native.go 注释对 `$`/反引号的保护声明不实——Go `%q` 不转义它们，git 走 `sh -c` 会展开） | `shellx.Quote`（credhelper 既有消费线同款）+ 新增含 shell 元字符路径的测试 |
| C | `redactURL`/`redactPath` 内联 9 项词表，与 strip 线 11 项**互有出入**（redact 独有 password/secret/key；strip 独有 auth/authorization/jwt/session/signature/sig） | 合并为 15 项超集单一真源 `sensitiveQueryParams`，strip 与 redact 共用（双向更安全；strip 行为变化记 CHANGELOG） |
| D | `provider/manager.go HashToken` 手写 sha256→hex；`deploykey.go` 手写 base64 | `encoding.Sha256(token)[:16]`；`encoding.Base64DecodeBytes` + `Base64RawEncodeBytes`（encoding 无 raw-sha256 字节原语，`sha256.Sum256` 保留——诚实部分收编） |
| E | branchfilter 逗号拆分样板、branchfilter/divergence/ensure 收集循环 | `stringx.SplitCSVNonEmpty`、`slicex.Filter`（首匹配/早退谓词类无对应原语，保留手写并注明） |
| F | `logtail.go TailOfReader` 手写行尾循环——**存档 #198 声称已委托 bufiox.TailLines 但现场未落地**（TailLines 文档自述即该场景两代实现的固化口径，签名逐字同形） | 委托 `bufiox.TailLines`；先 `git log` 核实存档与现场背离原因 |

## Tier 2（实施时评估，带采纳门）

| # | 内容 | 采纳门 |
|---|---|---|
| G | `provider/wait.go:75` `time.After`→NewTimer+Stop；评估 `retry.Poll(ctx, interval, timeout, cond)` 替换轮询环（签名已核：cond 立即首跑/错误中止透传/超时 ErrTimeout——与 wait 语义同构，值经闭包捕获） | `commitstatus_test.go` 25 处引用全绿；语义有出入则只做定时器卫生并记因 |
| H | `transport/logger.go SlogLogger` 经 logx 组合重写 | **预期不采纳**：值接收者 API 钉死现状，logx 组合需逐调用构造（每条日志分配）或改指针接收者（breaking）；评估后若维持，记「已评估不引入」 |
| I | `provider/ttlcache.go` flights map+mutex→`container.SafeMap.GetOrSet`、gen 计数→`sync/atomic`（**保留自建 flight**——#353 存档只禁委托、不禁换容器） | ttlcache 全部测试 + `-race` 绿（含 #135 世代、32 并发 1 构造）；任一失败即回滚并维持存档 |
| J | 用上游 `cache/cachetest.Contract/Run` 套件认证 ttlcache | 先 `go doc` 全签名核对 ttlCache 方法集与套件要求；不匹配则记「已评估不引入」 |

## 附带卫生（规则一「禁止 `_` 丢错误」，6 处非清理类）

`gogit_file.go:156` ForEach、`gogit_commit.go` rebase 状态写入 ×N、
`backendutil.go:38` 请求钩子、`ratelimit.go:79` WaitContext、
`ttlcache.go:160,170` 与 `idcache.go:43` 缓存写——逐处判定：能传播则传播，
确属 best-effort 则加显式注释把丢弃变为有意为之（不许静默）。

## Tier 3（存档复核成立，维持不引入）

retry.go 退避（HTTP 特化+免疫实证）、ratelimit 自适应节流（无匹配原语）、
gitee FixedZone（无原语）、examples/genledger 日志（SDK 范围外）、
contracttest 空密钥 HMAC（刻意负对照）、ptrx（内置 `new`+SDK helper 覆盖）、
首匹配类循环（slicex 无 Find/Any 原语）、`redact` 掩码本体（无 query 掩码
原语）、`slog` 适配（H 评估后若不采纳）。

## 验收

全部收编点有测试门（既有测试绿 + 新增元字符/词表用例）；`make check` +
`make check-mcp` + 双模块 lint + govulncheck 全绿；覆盖率不低于 45% 地板；
AGENTS.md 存档同步（新增收编条目、#198 背离修正、评估否决项记因）；
CHANGELOG 同步。
