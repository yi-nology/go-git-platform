# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.77.0] - 2026-10-06

### Changed

高内聚低耦合·强设计模式重构——"一个关注点只留一个实现"(勘察与决策记录见
`docs/superpowers/specs/2026-10-06-cohesion-patterns-refactor-design.md`):

- **transport 单一重试引擎**(原两套近乎复制的循环): `RetryConfig.retryLoop`
  为唯一引擎,`RetryConfig.Do`(签名不变)与 `NewRetryingRoundTripper` 退化为
  两个薄适配器。语义统一:响应体读取失败=硬错误;backoff 期间 ctx 取消会关闭
  最后一个响应并返回 `ctx.Err()`(原 Do 路径在此泄漏打开的 body)
- **transport 单一请求管线**: `Client.Do/DoJSON/DoRaw` 不再有私有编排,直接
  走与第三方 SDK 相同的 RoundTripper 链(limiter→auth→UA/hooks→ETag→retry),
  条件请求只留 `processRT` 一个实现(缓冲变体 `process` 删除)。Do 路径整体
  超时改为 ctx deadline(覆盖读体,等价原 `http.Client.Timeout`);每个
  attempt 重放 auth/hooks/限流等待(原来请求 hooks 只跑一次)
- **transport ClientOption Builder**(breaking,minor 承载): Client 的 10 个
  公开可变字段全部转私有,`NewClient(baseURL, auth, opts...)` 变参构造
  (WithTimeout/WithTransport/WithRetry/WithHooks/WithLogger/WithLimiter/
  WithETag/WithMaxBodySize);删除 `NewClientWithTransport`(被 WithTransport
  取代)。`backendutil.NewTransportClient`/`SDKHTTPClient` 一次完成后端装配
  (原 7 个后端各 ~35 行复制粘贴);tencentcode 的 TLS 1.2 定制 transport 抽为
  `tencentTLS12Transport` 经 extra 选项注入
- **webhook 验签归一**(安全加固): 7 个后端 `ValidateWebhookSignature` 全部
  委托 `provider.ValidateWebhookWithRegistry`——每个签名方案只有注册表里
  一个实现,删除各后端内联的 HMAC/常量时间比较。**空密钥语义从"放行"改为
  拒绝**(空 HMAC 密钥对可预测载荷可伪造;与注册表/契约测试既有断言一致);
  github 验签收窄为 X-Hub-Signature-256(不再经 go-github 的 sha1 回退)
- **分页迭代器收敛**: `backendutil.AllPages` 改为 `provider.CollectBounded`
  薄适配(预算仍 50 页),**触顶从"log+静默截断"改为报错**
  (`provider.ErrPageBudgetExceeded`)——平台分页 bug 显式失败而非悄悄丢数据;
  新增 `backendutil.PageList` 收敛"dual-mode 分页"惯用法,迁移 25 处复制点
  (tencentcode 需要 total-count 出参的 2 处保持显式实现)
- **平台探测开放注册**: `provider.RegisterHostAlias(host, platform, baseURL)`
  ——自建实例别名不再需要改 provider 核心;重复注册 panic(与 `Register`
  同约定)
- **gitbackend 内聚修复**: `Logger` 接口本地声明(消费方接口,删除对 provider
  的反向依赖);Fetch 结果分类抽为 `diffFetchRefs` 单一实现(native/gogit
  共用);gogit ~40 处 `PlainOpen`+`ErrRepoNotFound` 样板收敛为 `openRepo`
  助手;**`CheckoutDetached` 提升进 `BranchOps` 接口**(breaking,minor 承载)
  并补 native 实现(`git checkout --detach --force`)——修复 Repository 门面
  "文档说 detach、实际 attach"的语义 bug;删除恒返 `AuthNone` 的死代码
  `AutoDetectAuth`;`sshpincache.go` 更名 `sshpin.go`(名实相符,无代码变化)
- **mcp/tools.go 拆分**(555 行 → 6 个 toolset 文件): `tools_core/crs/issues/
  status/search/releases.go`,DTO 随各自 toolset 走;toolset 挂载门控单一化
  为接口类型断言(不再与 CapabilitySet 标志双通道,杜绝挂载/注册判据漂移)

### Fixed

- mcp cmd 的 streamable HTTP server 补 `ReadHeaderTimeout`(Slowloris 加固)

## [0.76.0] - 2026-10-02

### Added

- **NoteManager 可选接口**(provider + gitlab backend):CR(MR/PR)评论的定点
  更新 `UpdateNote` 与全量分页列举 `ListNotes`,走 MergeRequest Notes API。
  动机:GitLab 的 MR 与 issue 是两个 iid 命名空间,IssueManager 的评论方法
  (Issues Notes API)对 MR 必 404——上层按评论 ID 做原地更新/扫描定位时全部
  失效,只能每轮新建造成重复评论。GitHub/Gitea 等 issue 即 PR 的平台由
  IssueManager 天然覆盖,无需实现本接口(与 DiffManager 同为类型断言的
  可选能力,不进 CapabilitySet)。

## [0.75.0] - 2026-10-02

### Added

- **`githubapp.InstallationTokenSource`(缓存式 TokenSource, v0.74 审查遗留项)**:
  `NewInstallationTokenSource(apiBase, appID, installationID, pem)` 实现
  `Token(ctx)`,可直接作为 `provider.Config.TokenSource`——按服务端
  `expires_at` 缓存(缺失时回退 1h 默认 TTL),到期前 1 分钟提前换新,
  互斥锁让并发调用收敛为单次抓取(防击穿);刷新失败保留旧缓存、下次
  调用照常重试。每小时一次 JWT 铸造 + 一次 POST,而非每请求一次
  (`FetchInstallationToken` 原签名不变,内部改共享带过期时间的实现)
- 端到端测试: github 后端 + TokenSource 接线实测"两次 API 调用只打一次
  token 端点、请求头带缓存 token";单测覆盖缓存命中/到期刷新/提前量/
  缺失过期回退/并发防击穿/失败可重试六条路径(race 检测下)

## [0.74.0] - 2026-10-02

### Fixed

v0.71→v0.73 全量代码审查(双流合流后)发现的 8 项问题修复,两项 P2 门禁级:

- **ETag 条件请求恢复流式契约与内存上界**(P2): RT 路径此前对带 ETag 的
  200 无界整读(绕过 MaxBodySize,大附件/归档端点全量进内存),且中途读错
  被吞成空 200。现在:ContentLength 已知超限直接流式放行;未知长度按
  cap+1 探读、超限用 MultiReader 回放"已读前缀+未读余量"零丢失;读错
  上浮为传输错误。附带两处加固:`Cache-Control` 指令列表中的 no-store
  (如 "private, no-store")不再误缓存;请求带显式 `Accept-Encoding` 时
  不缓存(仅透明 gzip 场景存解码体,防编码体被剥头回放)
- **304 无缓存条目改为报错**(原静默透传裸 304): 条目在 If-None-Match
  发出与响应到达之间被 LRU 逐出时,调用方拿到无 body 的 304 无法与空
  载荷区分——现两条路径均返回明确错误
- **Gitea/Forgejo `comment.created` 补上评论正文**(P2): 两后端此前丢弃
  issue_comment 载荷中的 comment 对象,事件发出但 `Comment` 恒空;已补
  解析并更新语料 fixture + golden
- **GitHub 评论 edited/deleted 不再误报 created**: `IssueCommentEvent`/
  `PullRequestReviewCommentEvent` 的 action 改为如实派生(词表新增
  `comment.edited`/`comment.deleted`,新增语料 fixture 锁定)
- **GitHub release asset 302 跟随客户端尊重 SkipTLS**: 原裸
  `&http.Client{}` 在 GHES 自签场景第二跳 x509 失败;现随 Provider 构造
  (仍不带鉴权,签名 URL 语义不变)
- **MCP HTTP 模式优雅关停**: SIGINT/SIGTERM 触发 10s 宽限 Shutdown,
  不再硬切在途工具调用

### Reviewed

- 全量审查范围 v0.71.0..HEAD(165 文件): tokensource 接线/retry 门控/
  githubapp JWT/gitbackend TransportAuth/语料驱动/ensure 语义均确认无恙;
  githubapp 的 installation token 按调用铸造,直接作 TokenSource 略贵,
  缓存适配器列为后续项

## [0.73.0] - 2026-10-02

### Changed

- **分页 API 收敛为一套**：删除 `provider.ListAllPages`（v0.72.0 引入，零调用方），
  `provider.Each` / `Collect` / `EachBounded` / `CollectBounded` 成为唯一泛型分页
  面。理由：两套并存会永久留在公共 API 上；且 ListAllPages 的"短页即末页"
  终止规则与仓库实测过的 Forgejo 行为相悖（服务端把页大小压到请求值以下时
  短页≠末页），Each/Collect 的"空页终止"规则与 backendutil.AllPages 一致。
  原测试中"迭代中途取消下一轮生效"用例已移植到 pageiter 测试
- **限流 403 判定收敛为单一谓词** `transport.isRateLimitedStatus`（429 恒真；
  403 + `X-RateLimit-Remaining: 0`；403 + 仅 `Retry-After` 即 GitHub secondary
  limit）——v0.72.0 中错误构造（`Error.IsRateLimited`）与重试门控
  （`canRetryResponse`）各写了一份且口径不一，现由同一函数支撑，两端口径
  统一为含 Retry-After-only 的最宽口径；新增 secondary-limit 测试用例

## [0.72.0] - 2026-10-02

### Added

- **可刷新 `provider.TokenSource`**（`Config.TokenSource`，优先于 `Config.Token`）：
  transport 层每请求取 token，GitHub App installation token / OAuth 轮换类凭证
  下一次请求即生效；七个后端统一接线（工蜂 SDK 构造期需非空 token，占位符
  由 RT 层每请求覆盖/清空）；`provider.StaticTokenSource` 提供静态等价物
- **ETag 条件请求（`Config.ConditionalRequests`，默认关）**：GET 自动携带
  `If-None-Match`，304 透明回放为缓存的 200（`Do` 与 SDK RoundTripper 双路径，
  缓存键含 URL/Accept/Authorization，LRU+单条目体积上限，`Cache-Control:
  no-store` 跳过）——轮询型负载（如 WaitForCommitStatus）在 GitHub 上不吃限流配额
- **`EnsureWebhook` / `EnsureBranchProtection` / `EnsureDeployKey`** 期望态
  收敛助手（created/updated/unchanged），幂等可重跑
- **Webhook 事件语料库（golden）**：每后端 `testdata/webhooks/` 固定载荷 ×
  规范化事件金样（`-corpus-update` 再生成），锁定 `cr./push/tag./branch./
  issue./comment.` 词表的跨平台一致性
- **`NormalizedEvent` 新增 `Issue` / `Comment` 字段**，七后端补齐 issue/comment
  事件解析（GitHub IssuesEvent/IssueCommentEvent/PR review comment；GitLab
  issue/note；Gitea/Forgejo/GitCode issues/issue_comment；Gitee Issue Hook/
  note；工蜂 issue/note）
- **版本戳与 User-Agent**：`provider.Version()`（buildinfo 解析，dev 回退），
  transport 默认追加 `go-git-platform/<ver>` 产品标识（已有 UA 不覆盖）
- **限流错误增富**：`transport.Error.RetryAfter/ResetAt`（Retry-After 与
  X-RateLimit-Reset 解析）+ `Error.IsRateLimited()`（429 或 403+Remaining:0）；
  `provider.RateLimitRecovery(err)` 一把取出恢复窗口
- **`transport/metrics` 包**：零依赖观测接口（`Recorder` + `ResponseHook`
  适配 + `ClassifyPath` 低基数路径模板），Prometheus/OTel 各自几行适配
- **MCP**：`releases` toolset（`list_tags`/`list_releases`/`get_release`/
  `create_release`(写)）；列表工具开放 `per_page`（1–100，默认 30）；
  `--http :8080` 切换 streamable HTTP（端点 `/mcp`，可选 `--http-token`
  Bearer 门禁）；server 版本号对齐主模块 `provider.Version()`
- **契约测试新增两节**：`TokenSource_Rotation`（轮换按请求生效）与
  `ConditionalRequests`（第二次 GET 恰好一次 304 回放）
- **`provider` / `pkg/credential` 补包文档（doc.go）**
- **provider 泛型分页 `ListAllPages[T]`**：循环翻页拉全量（短页即末页、
  `maxPages` 安全上限、ctx 逐页取消），供下游替换手写 page++ 样板
- **`githubapp` 顶层包**：GitHub App 凭证两步曲下沉——`MintJWT`（RS256，
  iss=app_id，PKCS#1/#8 PEM）与 `FetchInstallationToken`（经
  transport.Client 换 installation access token，自带重试/日志）
- **四个 GitHub 专属可选能力**（接口在 provider，实现在 backends/github，
  CapabilitySet 新增 Gists/Starred/Migrations/ReleaseAssets 四字段并登记
  contract suite）：
  - `GistManager.ListMyGists`——token 用户 gist 列表
  - `StarredManager.ListStarred`——`/user/starred`，双形状响应兼容
    （`star+json` 包裹与裸列表），返回统一 `PlatformRepo`
  - `MigrationManager.CreateMigration/GetMigration`——GitHub Migration
    API 用户级（org==""）与组织级两路径
  - `ReleaseAssetManager.DownloadReleaseAsset`——附件流式下载
    （302 跟随用裸 Client，签名 URL 不带 Bearer）
  - `ReleaseInfo` 新增 `Assets []*ReleaseAsset`（ListReleases 即带附件元数据）

### Changed

- **Webhook 词表归一（行为变化）**：tag 事件统一 `tag.created`（GitCode/Gitee
  此前发 `tag.push`；GitHub create/delete 按 `ref_type` 区分 tag/branch，此前
  一律 branch.*；Gitea/Forgejo create 同样支持 `ref_type`）；GitLab note 事件
  由非规范的 `cr.note` 改为 `comment.created`（CR/Issue 谁被评论看字段）
- **Webhook 注册表 ↔ 后端签名校验对齐（修复三处静默漂移）**：Forgejo 注册表
  改用双头 `ForgejoWebhookValidator`（X-Forgejo-Signature/X-Gitea-Signature）；
  GitCode 注册表改用 `GitCodeWebhookValidator`（HMAC 双头或 X-GitCode-Token
  静态）；Gitee 注册表在无时间戳时兼容后端 body-HMAC 方案；Gitea/Forgejo
  后端校验容忍可选 `sha256=` 前缀
- `NormalizeTagAction` 规范值改为 `created`（与三后端实际输出一致，原 `push`
  无调用方）
- **transport：GitHub 风格限流 403 纳入重试**——`403 +
  X-RateLimit-Remaining: 0` 或带 `Retry-After` 时按 429 同路退避
  （Retry-After 优先），仍受方法幂等门控；裸 403（权限拒绝）不重试。
  导出的 `ShouldRetry(status)` 语义不变
- README 能力矩阵补上述四项（仅 GitHub 支持）

## [0.71.0] - 2026-10-01

### Fixed

- **native 输出解析器加固**（深度审查发现的正确性缺陷，均实测复现）：
  - `GetCommitsBetween`/`GetCommit`/`GetFileHistory`：提交标题含 `|`
  时（如 "fix: handle a | b"），旧的管道分隔把 **Author/Date 解析
  错位**；改用 NUL（`%x00`）分隔 + 共享解析函数
  - `GetTagList`/`ListBranches`：TAB 分隔 + subject 置尾整体吸收
  （for-each-ref 类 format 不支持 `%x00`，TAB 是能做到的最强分隔）
  - **annotated tag 的 Author 恒空**（顺带修复）：tag 对象身份在
  tagger 而非 author，条件格式 `%(if)%(taggername)…%(else)
  %(authorname)%(end)` 兼容 annotated/lightweight 两种
  - `isConflictOutput` 收紧为 `CONFLICT (` 大写标记：合并不存在的
  "conflict-*" 分支等无关失败曾被误判为 `ErrMergeConflict`
- 回归测试：含 `|` 的提交标题、标签消息、分支 subject，及冲突判定
  分类用例表

### Changed

- 依赖复核：v0.69.0 后无新版直接依赖发布，全部仍为最新
  （go-git v6 仍为 alpha，不上）

## [0.70.0] - 2026-10-01

### Added

- **native 后端 SSH 指纹钉扎真正生效**（`HostKeyFingerprint`）：
  - 此前 native 路径的钉扎只退化为 `StrictHostKeyChecking=yes` + 已有
    known_hosts，指纹从未被比对；gogit 路径却严格匹配——同一配置两个
    后端语义不一致，native 用户"钉了个寂寞"
  - 现在跑 git 前解析本次命令要连的主机（argv 的 ssh:// URL，或经
    `git remote get-url` 解析 remote，支持 `fetch --all` 多 remote），
    `ssh-keyscan` 抓公钥、在 Go 里比对 SHA256 指纹，匹配的行写入
    `0600` 临时 known_hosts（用后即删），git 强制校验
  - **fail-closed**：联网命令解析不出主机、keyscan 不可用、指纹零匹配
    （可能 MITM/pin 过期）均直接报错；非联网子命令（status 等）不受影响
  - TOFU-then-pin：密钥经网络获得后立即与钉扎比对，MITM 假密钥不会命中
  - 端到端测试内嵌 x/crypto/ssh 服务端 + 真实 ssh-keyscan（正向/反向/
    remote 解析/clone argv 四路）

## [0.69.0] - 2026-10-01

### Changed

- **依赖全量保鲜**（`go get -u ./...`）：
  - 直接：`gitlab client-go/v3` v3.12.0 → **v3.15.0**；`gitea.dev/sdk`
    v1.2.0 → **v1.3.0**
  - 间接（crypto/传输类）：`ProtonMail/go-crypto` v1.5.2、
    `cloudflare/circl` v1.6.5、`pjbgf/sha1cd` v0.7.0、
    `skeema/knownhosts` v1.3.3、`x/net` v0.59.0、`go-openapi` 全家 0.29.x
  - 验证：全量 `go test -race`、hermetic 模拟环境、lint、govulncheck
    （0 可达漏洞；`x/crypto/openpgp` GO-2026-5932 为不可达且无修复版本
    的"无人维护"公告，升级前即存在）
  - 其余直接依赖已最新：go-github v92（无 v93）、go-git v5.19.2
    （v6 仅 alpha）、forgejo-sdk v3

## [0.68.2] - 2026-10-01

### Fixed

- **Lint 修复（CI Lint 腿红）**：`gitlab.Ptr` 已被 go-gitlab 废弃（SA1019），
  10 处全部改用 go 1.26 内置 `new(value)`；`tailOfReader` 参数 `max`
  遮蔽内建标识符改名 `budget`；注释 `//` 后补空格；`types.go` gofmt。

## [0.68.1] - 2026-10-01

### Fixed

- **测试夹具 hermetic 化补全（CI/Release 连续全红的根因）**：夹具的
  `git commit/merge/rebase/cherry-pick` 依赖本机全局 git 身份；CI runner
  无全局身份 → `git` exit 128，自 09-27 起（含 v0.65.0~v0.68.0）所有
  CI 与 Release 工作流失败。
  - `gitOutput` 统一注入 `GIT_AUTHOR_*`/`GIT_COMMITTER_*`，失败时带出 stderr
  - `createTestRepo` 设置仓库本地身份——被测 backend 的 Merge/Rebase
    也不依赖机器环境
  - 裸 `exec.Command(...).Run()`（吞错）的夹具调用全部改走 `gitOutput`
  - 本地以 `GIT_CONFIG_GLOBAL=/dev/null` 模拟 runner 复现并验证归零

## [0.68.0] - 2026-10-01

### Changed

- **认证路径重构（credential helper）**：HTTPS 令牌改经**临时 credential helper +
  GIT_ASKPASS** 注入 git（对标 gickup）：
  - 令牌写入 `0600` 临时文件；helper 脚本读文件回 `password=`
  - **不再**把 `Authorization: Basic …` 放进 `http.extraheader` / `GIT_CONFIG_VALUE_*`
  - **令牌不进 argv、不进 git 进程 environ 明文**；会话目录 RAII 清理
  - `configureAuth` 返回 cleanup；`runGitEnv` 修复 extraEnv 覆盖认证 env 的问题
  - 建会话失败时回退旧 extraheader（保持可用性，并 `logger.Warn` 留痕）
  - env 注入**空串 `credential.helper=` 先于本次 helper**：清空主机已配置的
    helper 列表（osxkeychain / store / cache）。已实测两处回归风险：
    (a) 主机 helper 先被查询，个人凭证会**遮蔽**本次令牌（GitHub/GitLab
    推送成错误身份）；(b) 认证成功后 git 对全部 helper 执行 `store`，
    一次性令牌会被**持久化进用户钥匙串 / 明文凭证文件**。
  - 移除无效的 `credential.useHttpPath`；helper/askpass 路径 `ToSlash`
    以兼容 Windows 上的 msys sh
  - 集成测试用真实 git 钉死上述两条（主机 helper 不被 get/store 触达）

## [0.67.1] - 2026-10-01

### Fixed

- **GitLab `convertBasicMR` 补 `Draft`/`HeadSHA` 映射**：poller 列表路径
  恒零值导致上层兜底逻辑全静默失效。

## [0.67.0] - 2026-10-01

### Added

- **GitLab CI 失败日志能力 + pipeline 失败事件**：`CILogManager` 可选接口，
  pipeline 失败终态进入事件流，供上层拉取失败作业日志。

## [0.66.0] - 2026-09-28

### Added

- **GitLab diff 感知行内定位**：评论 position 经 hunk 映射到新旧行号，
  定位失败降级为 file 级 position。

## [0.65.0] - 2026-09-27

### Added

- **GitLab `CreateReview` 行内评论（discussions）**：支持 position 级
  行内 discussion。

### Fixed

- 行内 discussion 失败留痕（`logger.Warn`），不再静默全丢。
- `diff_refs` 降级与行内路径日志；清理编辑残渣。

## [0.64.0] - 2026-09-27

### Added

- **PlatformRepo 仓库元数据**: `Archived`/`Fork`/`Stars`/`Language`(供导入过滤);
  `CloneOptions` 部分克隆(`Filter: blob:none` 等)与 `Submodules`。

### Fixed

- **webhook 验证对齐真实平台协议**: GitCode 改用文档规定的 `X-GitCode-Token`
  头; Gitee 实现 sign(Base64 HMAC-SHA256 + 时间戳新鲜度)与 password 双模式。
- **gitbackend**: Fetch 结果两后端对称(新增/更新/删除分支分类); `FetchAll`
  改单次全量(原逐分支 N+1); worktree 写回保留可执行位/符号链接、不再吞错;
  工厂回退 gogit 时留痕; `branchfilter` 拒绝坏模式; 两后端默认启用 SSH
  主机密钥校验; tag/rebase/checkout(`CheckoutDetached`)一批语义修复。
- **transport**: RoundTrip 并发竞态(共享 Client 不再被改写); 大响应体不再
  截断; 限流器锁内不再休眠(剩余 0 也节流、预约定防惊群); `Retry-After`
  封顶; 重试幂等感知(POST 仅 DNS/dial 失败可重试); 拒绝型请求 hook 在
  SDK 路径同样生效。
- **provider**: `DetectPlatform` 精确主机匹配(自托管域名不再误路由公共云);
  Manager janitor 可重启; `Wrap` 保留原始错误链; 状态码提取白名单化;
  batch 取消快速失败。
- `CommitWithIdentity` 显式设 `GIT_AUTHOR_*/GIT_COMMITTER_*` 压过进程环境;
  测试 hermetic 化第一轮(修复 4 个受本机身份污染的用例)。

### Changed

- `transport.Error.StatusCode` 改为方法(启用 statusCoder 快路径)。

## [v0.62.0] - 2026-09-20

### Added

- **无分页参数的列表方法全量翻页**: 14 个 List 方法此前只回服务器默认首页
  (10-30 条), 统一走 `backendutil.AllPages`; 带 `Page` 字段的方法获得双语义
  (`Page==0` 全量、`>0` 调用方驱动单页), 契约套件钉死翻页走查。
- **`CommitStatusManager.ListCommitStatuses`**(七平台)+ 统一
  `CommitStatus`/`CommitStatusState` 词表。
- **Agent 原语落地**: `provider.WaitForCommitStatus`(CI 门禁轮询)、
  batch 有界并发读(`provider/batch.go`)、字段投影(`pkg/projection`)。
- **`mcp/` 模块**: MCP server(独立 go.mod, toolset 能力门控 + 读写分离)。
- `examples/capabilities` 能力巡检; CONTRIBUTING 落地 SemVer 契约。

### Changed

- **三后端 SDK 大迁移**(统一面不变): GitHub go-github v72→v92、
  GitLab client-go v2→v3.12、Gitea SDK v0.25→新版; 其余依赖追平。

### Fixed

- **gitbackend 注入加固**: `runGit` 子命令白名单 + `-c`/`--exec` 等
  exec 类 flag 拒收; `GetConfig`/`SetConfig` 配置键注入防护。

## 历史版本摘要(v0.38.0 – v0.61.0)

> 更早版本的完整条目见各版 GitHub Release 与 git 历史;此处保留一行摘要
> 供检索迁移线索。⚠️ = 含破坏性变更。

- **v0.61.0**(2026-09-03): Webhook 改进、`ListCRReactions`(CR 表情读面)、CI 加固。
- **v0.60.0**(2026-09-03): Gitea/Forgejo `convertUser` 补 `Name` 字段。
- **v0.59.0**(2026-09-03): 全后端空值防护/错误包装/分页修复/去重。
- **v0.58.0**(2026-09-02): `UserManager` 可选能力(username→平台 ID 解析)。
- **v0.57.0**(2026-09-01): 标签名→ID 批量解析 + per-provider TTL 缓存(原逐标签一次调用)。
- **v0.56.0**(2026-09-01): 去重/错误处理标准化/webhook 事件归一(`NormalizedEvent`)。
- **v0.55.0**(2026-09-01): Gitee `DeploymentKeyManager` + `CommitStatusManager`(Checks API 映射)。
- **v0.54.0**(2026-09-01): Gitee 后端弃用 swagger 生成 SDK, 换 `next-bin/go-gitee`(22 处 raw 绕行重落新 SDK)。
- **v0.53.0**(2026-09-01): `BranchProtection/Collaborators/DeployKeys/RepoStats` 四项可选能力 + `ChangeRequest.Assignees`。
- **v0.52.0**(2026-08-31): `NotificationManager` + `ReactionManager` 可选能力。
- **v0.51.0**(2026-08-31): GitLab `TokenStyle`(bearer, 如 `CI_JOB_TOKEN`)。
- **v0.50.0**(2026-08-30): `ListIssueLabels`/`ListIssueComments` 七平台翻页补全(原只回首页)。
- **v0.49.0**(2026-08-30): `IssueManager.UpdateIssueComment` 七平台。
- **v0.48.0**(2026-08-29): 依赖刷新(go-github v72 等)+ toolchain 钉 go1.26.6(修 7 个 stdlib CVE)。
- **v0.47.0**(2026-08-28): 工蜂 issue assignees 生效; gitbackend 9 个 bug 修复(gogit merge/fetch/rebase、native 冲突判定等), 覆盖率 15.8%→76.9%; Dependabot + 覆盖率地板 45%。
- **v0.46.0**(2026-08-27): Manager 真 LRU 驱逐; RateLimiter 改 `x/time/rate`; 移除 `pkg/encoding`。
- **v0.45.0**(2026-08-26): GitLab `RequestReviewers`/issue assignees 真正生效(username→ID 解析)。
- **v0.44.0**(2026-08-24): GitLab `ListRepos` 限定 token 用户范围。
- **v0.43.0**(2026-08-16): GitCode SDK v0.7.0, 解锁 `GetArchive`/`CreateCommitStatus` 最后两个桩。
- **v0.42.0**(2026-08-16): 工蜂声明 `Reviews` 能力(MR notes 映射, 四项登记限制)。
- **v0.41.0**(2026-08-16) ⚠️: CR/DiffManager 13 个方法 `number` int→string。
- **v0.40.0**(2026-08-15) ⚠️: IssueManager 8 个方法与 `Issue.Number` int→string; Milestone 选项/`MilestoneRef.Number` string 化; `CreateReview` 移入新的 `ReviewManager`。
- **v0.39.0**(2026-08-15) ⚠️: `Issue.Milestone` string→`*MilestoneRef{Number,Title}`。
- **v0.38.1**(2026-08-15): `provider.Wrap` 对非 struct 错误不再 panic。
- **v0.38.0**(2026-08-15) ⚠️: `IssueManager`/`SearchManager` 移出 `Provider` 接口(可选能力化)。
