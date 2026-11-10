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
  委托与错误面被动受益。下游反馈回流已全部被上游采纳落地（v0.37.0，当日往返）：#197 timex
  `ParseFirst`（已收编：gitcode/gitee due_on 手写循环删除）、#198
  bufiox `TailLines`（已收编：TailOfReader 委托，CRLF 归一改进）、
  #199 Slice.RemoveIf（无下游消费点，原语就位待用）、#210 MatchSimpleGlob
  跨 `/` 语义 doc 补全、#211 pagination WalkEach（收编否决：缺伪翻页
  检测——Forgejo 15.0.1 现场实证行为，pageiter 保留）、#212 secretx
  `NewPassphraseBox`（收编否决：PBKDF2 与本仓 v2 密文 argon2id 派生
  不同线，存量 wire 兼容优先）。第二轮新提六连：pkg#269 httpjson 条件
  请求（ETag/304 重放——ETagCache 现场硬化两年为证）、#270 webhook
  签名校验原语族（五验证器形态为证）、#271 `ParseFirstInLocation`
  （无时区 layout 非 UTC 服务端——Gitee CST 实证）；concurrency#198
  retry/httpx `ParseRetryAfter`（秒数+HTTP-date 双形态——transport
  手写解析为证）、#200 跨宿主重定向敏感头剥离（StripSensitiveHeaders
  为证）；pkg#272 SafeMap.Snapshot（Slice.Snapshot 对偶）。
- **pkg v0.36–v0.38 增量审计（2026-11-08，随深读补记；收编两处+定性三包，
  无新增依赖——pkg 已在白名单）**。收编：① `secretx.SecureString`——
  githubapp `InstallationTokenSource` 驻留 token 从明文 string 迁
  SecureString（XOR 混淆驻留+替换零化，内存扫描缓解；上游文档点名
  「git provider token」即此消费形态。混淆性质由上游自测保证、下游不窥
  `obfuscated` 未导出内部，只钉行为面：Get 独立副本往返/过期替换生效/
  导出字段直填零值经 nil 守卫可用；PrivateKeyPEM 是配置传入的公有字段
  不动——改类型即 breaking）；② `encoding.UnmarshalUseNumber`——
  `pkg/projection` 手写 decoder+UseNumber 删除（防 >2^53 精度丢失语义
  不变；上游「尾残报错」严于手写 Decode，但 raw 来自 json.Marshal 无
  尾残形态，零行为差异）。定性（无消费点，勿再评估）：`pack`（跨宿主
  迁移资产包——本仓无迁移资产导出/导入面）；`encoding` 其余面
  （Fnv1a64/Sha256/Base64 变体——HMAC 验签已下沉 oekit/webhook，仓内
  无独立哈希/编码消费点）；`jsonrepair`（LLM 宽容修复——本仓无 LLM
  输出解析面；#268/#275 修复被动受益）。其余 v0.36–v0.38 原语复核
  （无消费点）：`ctxscope`（仓内零 context.WithValue）、
  `cache.SingleValue`（无单值缓存需求）、`poll.RunCursor`（无 cursor
  长轮询形态）、`async.Spawn/Handle`（无旁路限时收结果形态——驻留
  循环已 loop.Run）、`httpjson.NormalizeBaseURL`（与自有
  DefaultBaseURL 不同域：上游管 scheme/空白规范化，自有管空值回退 +
  `/api/v1` 裁剪，链式引入反而给无 scheme 配置强补 `http://` 改变
  行为面，不收编）。第三轮新提两连（2026-11-08，均在 pagination）：#285
  伪翻页（无进展）检测原语——Forgejo 15.0.1 现场实证（pageiter v0.99.10
  为参考实现，上游长度判定识别不了「忽略 page 参数」形态，空转到预算
  耗尽丢全部数据）、#286 WalkEach fetch 缺 ctx 贯穿（同包 Walk 已是
  ctx 完整形态，包内不一致；pageiter PageFetch 签名为证）。上游落地
  后收编路径：pageiter 保留（ctx+无进展双安全网已自建），仅复核语义
  对齐。第四轮 ETag 域三连（同日）：#288 WithETag 无视
  Cache-Control: no-store（RFC 9111 §5.2.2.5 MUST NOT store 被违反
  ——2xx+ETag 无条件 Put 实证；提案只 gate no-store，no-cache 与条件
  请求天然兼容、private 不约束 private cache）、#289 NewMemETagStore
  无淘汰无上限（条目数+条目体积双无界，长驻轮询内存泄漏面；提案组合
  同模块 cache.LocalCache）、#290 WithETag doc 示例键漏 Authorization/
  Accept 分区（照文档写即跨凭据缓存串味；提案 CanonicalETagKey 构造器
  把纪律变构造期保证）——三条正是 #269 从本仓 ETagCache「逐条搬」时
  漏掉的硬化纪律（no-store/有界/键分区），上游落地后 transport/etag.go
  评估整体委托或只收键构造器；#291 URL path 低基数模板原语（ID 段折叠
  控制 metrics 标签基数——transport/metrics.ClassifyPath 为证，uuid
  盲区与保留清单可配两处诚实披露）。同期存档：oagentkit runtime#25/
  #26 已闭（procx ChildEnv 两缺口上游修复落地，gitbackend 收编评估
  待下一轮随 deps 升级一起做）。
- **pkg v0.39–v0.41 / concurrency v0.30–31 / logx v0.8.12 增量审计与七连
  回流收编（2026-11-10，先行审计再动工）**。提案八连全部闭（gitee#5
  仍 open）：#285/#286 → pkg v0.38.1（WalkEach/Walk 无进展检测 `page>1
  && DeepEqual(prev)` + fetch ctx 贯穿——与 pageiter 逐字同形）；
  #288/#289/#290 → v0.39.0（WithETag no-store 门禁/memETagStore LRU+
  体积双上限/CanonicalETagKey）；#291 → `pathx.ClassifyPath`（数字段 +
  hex≥7 + 词段保留，uuid 缺省不折叠、带符号数字不折叠——哨兵值取舍
  已收进缺省面）；concurrency#208 → `httpx.StripSensitiveQueryParamsWith`
  （raw-query 外科手术，不动无关参数字节免 re-encode 翻转转义）。
  **收编三处**：① `transport/client.go` StripSensitiveQueryParams 委托
  `StripSensitiveQueryParamsWith`（词表保留本仓 11 项——上游缺省 3 项
  的超集，api_key/jwt/session 等平台特化项不可丢）；②
  `transport/etag.go` etagKey 委托 `CanonicalETagKey`（\n 分段无歧义，
  vary 集合按本仓平台面固定：Accept/Authorization/PRIVATE-TOKEN——键
  字节形态变化零行为影响，内存缓存进程内私有）；③
  `transport/metrics.ClassifyPath` 委托 `pathx.ClassifyPath`（本仓手写
  启发式删除；上游「带符号数字不折叠」哨兵值取舍优于本仓 Atoi 形态，
  仓内无测试钉负数段、零行为回归）。**pageiter 保留不迁移**：上游
  WalkEach 现已带无进展+ctx 双安全网（语义逐字对齐复核完毕），但
  pageiter 的 ErrStopIteration 哨兵、budget 错误形态与 ctx PageFetch
  签名是自有公共面，迁移即 breaking——维持「同形不迁移，上游 bugfix
  被动受益」路线。**procx 维持不引入**：runtime#25/#26 修复消解了
  2026-10-18 的环境白名单顾虑，但依赖白名单硬规则（只许 pkg/
  concurrency/logx）未变，且 gitbackend 现行 GIT_CONFIG_COUNT 注入是
  git 原生机制（credhelper 文档明示令牌不进 argv/environ），procx 无
  消费痛感。logx v0.8.12 经 transport/provider NewLogxLogger 桥被动
  受益（bugfix 线）。第五轮建设性需求两连（2026-11-10，提报待回流）：
  #325 webhook 发端签名 helper 族（验签族 #270 的对称面——发端服务/
  测试夹具都在手算 HMAC，下游 contracttest signRequest 五处手写块为证
  ；签名器与验证器共享同一 HMACOption 语义面=单一真源不漂移）；
  #327 stringx rune 安全截断原语（字节切把 CJK 响应体切出非法 UTF-8
  ——transport truncateForLog/truncate 两处现场，httpjson 内部已有
  未导出先例）。上游落地后收编路径：contracttest signRequest 五处
  手写块删成调用；transport 两处截断委托（顺带修字节切缺陷）。
- **pkg v0.41.1 / concurrency v0.31.1–2 / logx v0.9.x 增量审计（2026-11-10
  ，随 patch 线升级，零新增引入；本条即结论存档）**。pkg v0.41.1 七连
  bugfix（#317 mathx 溢出/#318 pagination 伪翻页误判丢页/#319 httpjson
  304 空体当驱逐+#319 204/HEAD 双请求/#320 procx/#321/#323 jsonrepair/
  #322 urlguard）——两条特别复核：**#318 本仓 pageiter 不受影响**
  （`prev = batch` 直接持有 fetch 独立分配切片，无快照拷贝无缓冲复用
  ，与上游内部快照复用坑不同类）；**#319 不走 WithETag 的 Do 路径**
  （本仓只收 CanonicalETagKey 纯函数）。其余被动受益。concurrency
  v0.31.1 四连（#226 poll RunCursor 毒丸紧循环门/#227 budget
  WithClock(nil) panic/#228 httpx HTTPError 显式 0 秒 Retry-After 与
  ParseRetryAfter 口径统一——**本仓 ParseRetryAfter 委托线被动受益**
  /#229 poll doc）。logx v0.9.0 feat `ErrAttrs`（pkg/errors KVCarrier
  →slog.Attr 结构化出口）经 provider/transport NewLogxLogger 桥对
  接 logx 的消费方被动受益；#136/#137 slog 序列化与 Error() panic
  隔离；v0.9.1 floor sync。

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
