# 高内聚低耦合·强设计模式重构设计（v0.77.0）

日期：2026-10-06
状态：已实施（本文档为实施前设计，随（v0.77.0）提交）

## 背景与目标

对 v0.76.0 全仓做一次以设计模式为抓手的高内聚低耦合重构。三个探索代理对
provider/transport、backends、gitbackend/mcp/pkg 做了全量勘察，结论：仓库
骨架（registry/factory、strategy、contract test 双向能力一致性校验、webhook
golden corpus）是健康的，主要病灶是**同一关注点存在两套并行实现/编排**，
以及**构造装配逻辑复制 7 份**。本轮主题：**一个关注点只留一个实现**。

## 现状病灶（证据摘要）

| # | 病灶 | 证据 |
|---|------|------|
| F1 | 双重重试引擎 | `transport/retry.go:231-312`（`RetryConfig.Do`）与 `transport/client.go:558-656`（`retryingRoundTripper`）——同样的 body 缓冲、attempt 循环、门控谓词、日志文案 |
| F2 | 双管线编排 | `Client.do`（client.go:222-303）与 `clientRoundTripper.RoundTrip`（client.go:503-548）——limiter/auth/UA/hooks/etag 各写两遍；Do 路径 400+ 有 `transport.Error`，RT 路径没有 |
| F3 | 构造反模式 | `Client` 10 个公开可变字段 + "勿在交接后修改"注释，7 个后端各自 8 行字段赋值装配 + 相同的 SDK http.Client 拼装 |
| F6 | 双分页迭代器语义漂移 | `backendutil.AllPages`（50 页、静默截断+log）vs `provider.EachBounded`（100 页、报错）；"dual-mode 分页"惯用法复制 21 份 |
| F7 | webhook 验签漂移 | `provider/webhook.go` 注册表已内置 5 种验证器，4 个后端又内联重写 HMAC；**空密钥语义已漂移**：后端返回 nil（放行），注册表拒绝 |
| F14 | 探测封闭 | `knownHostPlatforms` 硬编码，自建实例别名要改 provider 核心 |
| G1 | gitbackend→provider 依赖倒置 | `gitbackend/logger.go:3` 仅为 4 方法 Logger 别名导入 provider |
| G2 | native/gogit 复制 | FetchResult 分类逻辑两份（native_core.go:97-133 / gogit_core.go:71-101）；`PlainOpen+ErrRepoNotFound` 包装样板 ×~40 |
| G3 | 死代码 | `gitbackend.AutoDetectAuth` 两分支都返回 AuthNone；`GoGitBackend.CheckoutDetached` 不在接口上、经 Repository 不可达 |
| M1 | mcp/tools.go 555 行 | 注册表 + 6 域 DTO + 校验 + handler 同文件；能力门控双通道（CapabilitySet 标志 + 类型断言） |

## 重构项

### R1+R2 transport：单一重试引擎 + 单一管线（Decorator/责任链）

- `RetryConfig.retryLoop` 成为唯一重试循环：attempt 回调 + classify 回调
  （"收到响应→读体/缓冲→是否可重试"由入口适配），body 复位统一走
  `ensureReplayable`/`resetBody`（GetBody 机制）。
- `RetryConfig.Do`（保持导出签名不变）= retryLoop 的 client.Do 适配器；
  `retryingRoundTripper.RoundTrip` = retryLoop 的 live-response 适配器。
  统一语义：读体失败=硬错误；backoff 期间 ctx 取消=关闭 lastResp 并返回
  ctx.Err()（此前 Do 路径泄漏打开的 body）。
- `Client.do` 不再有私有小管线：构造 `http.Request` 后直接走
  `pipeline()` = `retryingRoundTripper{clientRoundTripper}`（sync.Once 惰性
  构建一次），SDK 路径与 Do 路径同一条 Decorator 链。`ETagCache.process`
  （缓冲变体）随之消亡，只留 `processRT`。
- Do 路径整体超时改为 ctx deadline（覆盖读体，等价于原 `http.Client.
  Timeout`）；header 停滞防护仍在 `baseTransport`（ResponseHeaderTimeout）。
- 行为对齐收益：Do 路径每个 attempt 重放 auth/hooks/限流等待（原先请求
  hooks 只跑一次）；Do 与 SDK 两条路径的日志/钩子观测一致。

### R3 transport ClientOption + backendutil 装配收敛（Builder）

- `Client` 字段全部转私有；`NewClient(baseURL, auth, opts ...ClientOption)`
  （WithTimeout/WithTransport/WithRetry/WithHooks/WithLogger/WithLimiter/
  WithETag/WithMaxBodySize）；删除 `NewClientWithTransport`（被 WithTransport
  取代）。breaking 由 0.x minor 承载（CONTRIBUTING 版本承诺，先例 v0.73.0）。
- `backendutil.NewTransportClient(cfg, baseURL, style, extra...)` 一次完成
  logger/etag/retry/skipTLS/hooks 装配；`backendutil.SDKHTTPClient(tc,
  skipTLS)` 统一 SDK 侧 `http.Client`（Timeout=transport.DefaultTimeout +
  ChainTransport(HTTPTransport, NewRetryingRoundTripper)）。7 个后端构造器
  各瘦身 ~25 行；tencentcode 的定制 TLS transport 经 extra 选项注入。

### R4 webhook 验签归一（Strategy 单一实现）

- 7 个后端 `ValidateWebhookSignature` 全部委托
  `provider.DefaultWebhookRegistry().Get(platform)`，删除内联 HMAC/常量
  时间比较的重复实现。
- 语义修正（安全加固，CHANGELOG 标注）：空密钥从"放行"改为拒绝（与注册表
  及契约测试 `testWebhookSignature` 的既有断言一致）。
- 语料测试（webhookcorpus 35 fixtures golden）逐后端回归守护。

### R5 分页迭代器收敛（Iterator/Template Method）

- `backendutil.AllPages` 改为 `provider.CollectBounded` 的薄适配
  （预算仍 50 页）；**静默截断改为报错**（`ErrPageBudgetExceeded`），
  与公共迭代器语义一致。
- 新增 `backendutil.PageList[T](page, fetch)` 收敛"dual-mode 分页"惯用法
  （Page==0 全量 walk / Page>0 单页），迁移各后端 21 处复制点；
  tencentcode 需要 total-count 出参的两处保持显式实现。

### R6 平台探测开放注册（Registry 扩展）

- `provider.RegisterHostAlias(host, platform, baseURL)`：自建实例别名不再
  需要改核心；`knownHostPlatforms` 保持为内建默认，注册项优先匹配，
  重复注册 panic（与 `provider.Register`/`gitbackend.Register` 一致）。

### R7 gitbackend 内聚修复

- `logger.go` 本地声明 4 方法 `Logger` 接口，删除对 provider 的依赖
  （依赖倒置修正；结构化类型兼容既有实现方）。
- 抽取 `ClassifyFetchRefs`（native/gogit 共享 Fetch 分类，Template Method）；
  抽取 `openRepo` 助手消灭 gogit ×~40 的 PlainOpen 样板。
- 删除死代码：`AutoDetectAuth`（两分支同返回值）、`GoGitBackend.
  CheckoutDetached`（不在 GitBackend 接口上，Repository 门面不可达）；
  `sshpincache.go` 更名 `sshpin.go`（名实相符，无代码变化）。

### R8 mcp/tools.go 拆分（插件注册表归位）

- 按 toolset 拆为 `tools_core/tools_changes/tools_issues/tools_status/
  tools_search/tools_releases.go`，DTO 随各自 toolset 走；注册表机制不变。
- 能力门控单一化：toolset `enabled` 改为基于类型断言（capability 的唯一
  事实来源），挂载与注册不再双重判断。

## 明确不做（记入后续迭代）

- **Cluster A：gitea↔forgejo 全包合并**（~15% 归一化差异、~3.2k 重复行）——
  Template Method 收益最大但体量/风险最高，双侧契约测试已钉死，独立成轮。
- F9 `Provider` 47 方法门面 + CapabilitySet 双通道（删字段=breaking 面
  大，mcp 侧已单通道化）。
- F10 `provider.Manager` 五职责拆分（仓内无消费者）。
- F11 错误分类反射/字符串解析回退（需逐后端适配 SDK 错误类型）。
- F12 `ListCRs` total 语义显式化（`CRPage{Items, Total *int}`）。
- F13 webhook 信封解析模板方法（cluster D，~1.7k 行）。

## 验证

- 全仓 `go test ./...`（hermetic：`GIT_CONFIG_GLOBAL=/dev/null
  GIT_CONFIG_SYSTEM=/dev/null`）+ `-race` 抽样 transport。
- `golangci-lint run ./...` 全仓（先例教训：只 lint 子目录漏过 SA1019）。
- 总覆盖率 ≥45%（CI 地板）。
- 7 后端 contracttest + webhook corpus 全绿 = 行为钉死。
- CHANGELOG 补 0.76.0 缺段 + 新增 0.77.0 段；README 对齐。
