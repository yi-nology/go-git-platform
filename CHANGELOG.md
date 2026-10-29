# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **tencentcode: 收编工蜂 SDK，移除 `github.com/studyzy/gongfeng-sdk-go` 外部依赖**：
  studyzy 上游删除了 v0.6.0 tag，外部引用不再可靠。SDK 源码（fork
  `zy84338719/gongfeng-sdk-go` main 分支，Apache-2.0）整体收编进
  `backends/tencentcode/internal/gongfeng/`，连同其单元测试与官方 API 文档镜像
  （`docs/api/`）。fork 相对 v0.6.0 是一批"对齐官方文档"的修正，随收编生效：
  - **Release 按 ID 寻址**（原 v0.6.0 按 tag 寻址是对 wire 的错误认知）：工蜂
    release 端点只认数值 ID 且无按 tag 取单条的原生面，`GetReleaseByTag`/
    `UpdateRelease`/`DeleteRelease` 改为先按 tag 扫 release 列表解析 ID（登记
    detour，查不到报 `provider.ErrNotFound`）；release 载荷为 `id`/`tag`/
    `title`/`description`（无 GitHub 式 `tag_name`/`name`），契约套件的工蜂
    fixture 据此修正，`CreateRelease` 现映射 `Title`，`Target`/`Draft`/
    `Prerelease` 登记忽略。
  - **路径修正**：commit status 列表/查询去掉 `repository/` 前缀
    （`commits/{sha}/statuses`）、MR commits 与 changed files 走
    `merge_request/{id}/…`（原 v0.6.0 路径与官方文档不符）。
  - **移除官方 API 不存在的端点**（`SearchProjects`/`DeleteIssue`，本仓无调用点）、
    webhook 增加 `review_events`/`token` 字段、分支保护/MR 创建 options 补齐
    官方字段（labels、necessary_reviewers、approver_rule 等）、时间解析兼容
    `+0000` 偏移格式。
  - 唯一新增直接依赖 `github.com/google/go-querystring`（原就是两模块共有的
    间接依赖，收编后转 direct）；`mcp` 子模块的 studyzy 间接依赖一并消失。
- **收编工蜂 SDK 的 oekit 适配层**（`internal/gongfeng`，偏离已在目录 README
  登记，升级 fork 时需重新套用）：
  - 错误面走 oekit/pkg errors：带格式构造（含 `%w`）转 `oekiterrors.Errorf`
    （setBaseURL / parseID / Time.UnmarshalJSON），静态错误转 `oekiterrors.New`；
  - 日志注入：`Client` 增加可选 `provider.Logger`（kv 风格，默认 noop），经
    `WithLogger` 注入；`Do` 记录请求 debug 行（method/path/status/耗时）与
    失败 error 行；tencentcode 后端默认把 `Config.Logger` 接进来，oekit 生态
    消费方经 `provider.NewLogxLogger("gongfeng")` 观测 SDK 请求流；
  - 并发约定成文：SDK 保持无 goroutine 的同步设计，有界并发由 provider 批量
    引擎（`provider.GetFileContents` / `GetCRs` / `SetCommitStatuses`，
    oekit/concurrency syncx.FanInLimitCtx 驱动）承担——tencentcode 后端因此
    开箱获得三个批量助手。

### Added

- **tencentcode: 官方 API 文档能力补齐到后端面**（此前仅存在于收编 SDK 内、
  未暴露）：
  - `provider.CollaboratorManager`（跨平台可选能力）——项目成员增删查，权限词表
    read/write/admin → 工蜂 access level 映射（登记 mapping），username 经
    Users API 解析（TTL 缓存）；
  - `TencentCodeOrgExtras`——项目组 CRUD + 组成员管理、命名空间列表；
  - `TencentCodeProjectExtras`——关注人 watch/unwatch/状态/列表、star/取消/
    状态/列表、fork 关系建立与解除、Tag 创建/查询/删除、项目↔群组共享、
    项目事件流；
  - `TencentCodeAccountExtras`——session 登录换 private token（引导期专用）、
    当前用户/指定用户详情、关注的项目、SSH 公钥与邮箱管理。
  - 契约套件协作者 stub 放宽为 PUT/POST/DELETE 三动词并支持 username→ID
    查找响应（ID 寻址成员 API 的平台形状；原有平台行为不受影响）。
- **tencentcode: 分支保护契约实现与 SDK 残余面全量暴露**（目标：后端完全
  替代 gongfeng-sdk-go 直连）：
  - `provider.BranchProtectionManager`——分支保护规则 CRUD（第 7 个实现平台）。
    平台无"列出保护分支"端点：List 走分支列表过滤 `protected` 行返回摘要
    （登记 detour），审批规则经 Get 取（`approver_rule` 原样映射，-1=全员
    审批）；protect PUT 只收访问级别 + force-push 标志，契约其余选项登记
    忽略；`allow_force_push` 只写不可读（读侧恒 false，登记 mapping）；
    unprotect 端点是 PUT 非 DELETE。`iface_branch_protection.go` 的平台
    支持注释同步更新。
  - `provider.NoteManager`——MR 评论改写（UpdateNote）与全量分页列出
    （ListNotes），GitLab 家族 MR/issue 注释分命名空间，同 gitlab backend。
  - `TencentCodeExtras` 增补仓库读取：单分支详情 `GetBranch`、原始文件流
    `GetRawFile`（blob 端点 + filepath query）、compare 变更文件集 zip 下载
    `DownloadCompareChangedFiles`。
  - `TencentCodeProjectExtras` 增补项目成员管理：单查 `GetProjectMember`、
    改级 `EditProjectMember`（协作者契约只覆盖增删列）。
  - `TencentCodeAccountExtras` 增补：用户目录检索 `ListUsers`（带 search）、
    按邮箱查用户 `GetUserByEmail`、SSH 公钥/邮箱单查、名下项目
    `ListOwnedProjects`。
  - 平台 API 原生缺失而未实现的可选能力维持缺席（工蜂无对应端点）：
    Search / Notifications / Reactions / DeployKeys / Gists / Migrations /
    ReleaseAssets；Starred 按接口文档仅 GitHub 实现。
- **tencentcode: 提交级原始文件读取与期望态收敛验证**：
  - `TencentCodeExtras.GetCommitRawFile`——commit 作用域的原始文件流
    （`repository/commits/{sha}/blob`，`GetRawFile` 的 commit 变体）；
  - `provider.EnsureBranchProtection` 跨平台期望态助手经新契约对工蜂生效
    （Created/Unchanged/Updated 三态有测试钉住）；已知形状：approver_rule
    不可经 protect PUT 写入，期望态的 `RequiredApprovingReviews` 需按平台
    上报值（或 0）表达才能收敛到 Unchanged。
- **mcp: 新增 `protection` / `collaborators` 两个 toolset，crs 增补评论读改**：
  - `protection`（需 BranchProtections）——`list_branch_protections` /
    `get_branch_protection` / `ensure_branch_protection`（写，幂等期望态
    收敛）/ `delete_branch_protection`（写）；
  - `collaborators`（需 Collaborators）——`list_collaborators` /
    `add_collaborator`（写，read/write/admin 词表在工具边界校验）/
    `remove_collaborator`（写）；
  - `crs` 增补 `list_cr_comments` / `update_cr_comment`（写）——经
    NoteManager 门控，GitLab 家族 MR 注释独立命名空间的读写从此对 agent
    可达；
  - 七平台现共八个 toolset；`--read-only` / `--toolsets` 子集选择行为
    不变，有挂载/只读/子集/端到端/词表校验五类测试钉住。
- **contracttest: NoteManager 契约套件**（spec 空缺补齐）——`Harness.Notes`
  挂载 UpdateNote/ListNotes 双向漂移检查（实现而无配置、或配置而无实现都
  报错）；gitlab 与 tencentcode 挂 fixture，GitHub 家族（issue 即 PR，由
  IssueManager 覆盖）按设计跳过。README 能力矩阵同步（BranchProtections/
  Collaborators 的 Tencent 列转 ✅、平台缺席清单更正、CapabilitySet 计数
  13→17）。

## [0.99.7] - 2026-10-26

### Fixed

- **跨 host 重定向泄漏 URL query 里的凭据**：跨 host 剥凭据此前只覆盖**请求头**
  （Authorization / PRIVATE-TOKEN / Cookie…），但 go-gitee 的 `tokenTransport`
  把令牌放在 **URL query**（`access_token`），且**每跳都重新加**（它包在
  http.Client 的 Transport **外层**）⇒ 平台 302 到 CDN / 对象存储时，
  **令牌会跟着 query 一起发给第三方主机**。
  Go 的自动剥除只覆盖标准头，管不到 query；而剥除必须发生在 tokenTransport
  **之内** ⇒ http.Client 的 CheckRedirect 里剥是无效的。
  修法：transport 新增 `StripSensitiveQueryParams` / `StripSensitiveCredentials`，
  在 transport 层剥掉凭据类 query 参数（access_token / private_token / token /
  api_key / auth / signature…）；跨 host 才剥，同 host 的正常重定向继续带凭据。
- **Gitee 的归档下载 100% 失败**：go-gitee 的 `Client.Do` 内部是
  `defer resp.Body.Close()` ⇒ `DownloadTarball` / `DownloadZipball` 返回时
  body **已经关闭**，后端再读只会拿到 `http: read on closed response body`。
  修法：改用 SDK 同一个 http.Client 直接发请求（认证 / 重试 / 重定向策略
  与 SDK 完全一致），body 生命周期由本方法自己持有。
- **Gitee / GitCode 的文件内容返回 base64 串**：这两家内容端点返回 base64，
  后端原样返回 ⇒ 调用方按 GitHub 直觉读文件全错（解析 YAML / JSON 直接失败）。
  修法：新增 `provider.DecodeFileContent`，按响应里的 `Encoding` 决定解码还是
  原文返回；Gitee / GitCode / 工蜂统一走它。
- **`ListCRs` 的零值 State 在 giteafamily / Gitee / GitCode 被当成「只看 open」**：
  上层传「不过滤」（零值）时，这三家只返回 open 的 CR ⇒ closed / merged 的 PR
  全部丢失（备份、审计缺数据）。修法：零值 = 不过滤，显式给了状态才过滤。
- **`ListIssues` 的 Sort / Direction 在五个后端被丢弃**：增量备份依赖「按更新
  时间排序」，被丢弃后只能按创建时间取 ⇒ 增量窗口内的更新会漏。
  只有 GitLab 的词汇已确证（`order_by=created_at|updated_at` + `sort=asc|desc`）
  ⇒ 只在 GitLab 侧真传；Gitee / giteafamily / GitCode / 工蜂登记进 divergence
  账本（传错词汇会让 API 直接 400，比静默忽略更糟）。

## [0.99.6] - 2026-10-25

### Fixed

- **无共同祖先时 ahead/behind 不能报 (0,0)**：两侧历史无关（目标仓被别人强推
  改写过）时，gogit 侧直接 `return 0, 0, nil` ⇒ 上层（漂移检测 / 同步状态）
  **误判成「已同步」** ⇒ 目标仓被改写历史后，漂移再也检测不到。
  native 走 `git rev-list --left-right --count A...B`，这种情形下仍会给出两侧
  各自的提交数 ⇒ gogit 侧补齐同样的语义。
  这条路径在 git-ferry 的镜像漂移检测里被用到（core 的 `GetBranchSyncInfo`）。

## [0.99.5] - 2026-10-25

### Fixed

- **日志里的 URL 再脱敏一层**：
  - `redactURL` 只处理 query 里的 3 个参数名 ⇒ `https://user:pass@host/` 的**密码**
    会原样进日志（仓内构造带 userinfo 的 URL 的只有 `pkg/credential.BuildAuthURL`，
    目前无调用点 ⇒ 这是「改一行就会泄漏」的形状）。⇒ 秘密类查询参数补齐
    （password / secret / api_key / apikey / key…），**userinfo 一并脱敏**。
  - `Request.Path` **可以自带 query 串** ⇒ 日志的 `"path", req.Path` 与错误构造
    `NewStatusErrorWithHeaders(…, req.Path, …)` 会把调用方放在 query 里的 token
    原样带出去（错误信息还会进 HTTP 响应体）。⇒ 新增 `redactPath` 并接入。

## [0.99.4] - 2026-10-25

### Fixed

**P0 — 跨 host 重定向把平台令牌发给第三方主机**

认证是在 RoundTripper **内部**施加的（每一跳都重新加一次），而 Go 的 net/http 只对
「请求头已存在」的情况做跨 host 剥离（Authorization / Cookie / Www-Authenticate /
Cookie2）⇒ 平台 302 到第三方主机时，令牌会跟着走。而 `PRIVATE-TOKEN` /
`X-Auth-Token` 这类**非标准头 Go 本来就不剥**。

实测形态：GitLab 的 archive 端点 302 到对象存储 ⇒ 目标主机收到 `PRIVATE-TOKEN`。

- `transport`：跨 host 的重定向跳**不施加认证**，并剥掉敏感头（判据用**完整
  host（含端口）**：Go 的规则只比主机名，会把 `127.0.0.1:A → 127.0.0.1:B`
  当成同主机）。
- `backendutil.SDKHTTPClient`：加 `CheckRedirect`，跨 host 时剥敏感头（兜底第三方
  SDK 自带的认证头）。

**gogit 后端：会破坏仓库的六处**（两个后端行为不一致，而生产用的是 gogit；全部实测复现）

- **Checkout 半切换**（P0）：go-git 的 Checkout 先切 HEAD 再 Reset 工作区 ⇒ 工作区脏时
  Reset 失败，但 **HEAD 已经切走** ⇒ 调用方拿到错误却面对「半切换」的仓库。
  ⇒ 先自查脏，脏就不切。
- **CherryPick 静默覆盖未提交改动**（P0）：pick 是「按路径覆盖」式实现 ⇒ 未提交的修改
  被直接覆盖并**提交成功**（本地工作静默丢失）；native 会拒绝。⇒ 脏工作区 fail-closed。
- **Merge 快进只移动 ref**（P0）：工作区/索引仍是旧内容 ⇒ HEAD 与磁盘不一致。
  ⇒ 快进后同步 reset 工作区。
- **DeleteBranch 删当前分支**（P0）：HEAD 悬空 ⇒ 仓库直接不可用。⇒ 拒绝。
- **RenameBranch**（P0）：目标名已存在时**静默把别人的分支指针改掉**；重命名当前分支时
  HEAD 仍指向已删除的旧 ref ⇒ 仓库不可用。⇒ 两条都加保护。
- **Clone 忽略 Submodules**（P1）：生产的 gogit 后端下「递归同步子模块」**静默无效**
  （clone 出来 submodule 目录是空的）。⇒ 设 `RecurseSubmodules`。

**native 后端：凭据不再进 git 进程 environ**

`newCredSession` 失败时回退到 `GIT_CONFIG_VALUE_0=Authorization: Basic <base64>`
⇒ 同用户的 `ps eww` / `/proc/<pid>/environ` 可见 ⇒ 与本文件自述的安全模型直接矛盾。
⇒ **不再回退**：建不出凭证会话就直接报错中断命令。

## [0.99.3] - 2026-10-25

### Fixed

- **入站 webhook 事件 ID 永不为空**（GitHub / GitCode 的 envelope 没有
  `IDPrefix` ⇒ `NewEvent` 不写 ID ⇒ `NormalizedEvent.ID == ""`）。调用方按
  `event_id` 做唯一索引 + 去重 ⇒ **第 2 条起的事件全部被静默丢弃**
  （不建事件、不匹配规则、不触发同步）⇒ GitHub 入站同步从第 1 条之后就失效。
  现在 `NewEvent` 在没有显式前缀时用平台名兜底（`github-<unixnano>-0`）。
- **新增 `NewEventWithID`：优先用平台投递 ID 作事件 ID**（GitHub
  `X-GitHub-Delivery`、GitLab `X-Gitlab-Event-UUID`、
  Gitea/Forgejo `X-Gitea-Delivery`/`X-Forgejo-Delivery`、
  GitCode `X-GitHub-Delivery`/`X-GitCode-Delivery`、Gitee `X-Gitee-Delivery`）。
  投递 ID 是**真正的幂等键**（平台重投时值不变）⇒ 调用方能真正去重；
  没有该头时回落到本地合成的 ID（仍非空）。
  `WebhookEnvelope` 新增 `IDHeaders`（按顺序取第一个非空，与 `EventHeaders` 同风格）。

## [0.99.2] - 2026-10-25

### Removed

- **`gitbackend.hostKeyCallback(insecure bool)`**：旧签名的包装函数，注释自认
  "兼容旧签名"。实际只有测试在用它，生产代码走
  `hostKeyCallbackWithConfig(AuthConfig)`。函数未导出 ⇒ 对外无影响。
  主机密钥校验策略本身（`InsecureSkipTLS` / 指纹钉扎 / `known_hosts` 三档）
  一字未改。

## [0.99.1] - 2026-10-25

### Fixed

- **gogit 后端的 `GetCommitsBetween` 与 native 语义对齐（分叉场景）**：旧实现从 `to`
  往回走 log、**只在遇到 `from` 本身时停** ⇒ `from` 与 `to` **分叉**时永远走不到
  `from` ⇒ 一路往回走过 merge-base，把**共同祖先**也收进「目标侧独有提交」清单。
  native 后端走 `git log from..to`（git 的范围语义自带「排除 from 可达的一切」）⇒
  **两个后端在分叉场景给出不同的清单** ⇒ 同一个调用方拿到的「目标侧独有提交」
  随 `git.backend` 配置而变：清单里混进共同祖先 ⇒ 运维看到的「将被覆盖的提交」
  是误导的；用清单长度当「独有提交数」也会算多。
  ⇒ 改用 **merge-base 作遍历边界**：`from` 是 `to` 的祖先时 merge-base == from
  （旧行为**逐字不变** ✓）；分叉时停在共同祖先**之前** ⇒ 共同祖先不进结果 ✓。
  没有共同祖先（无关历史）⇒ 不设边界（走到底），与 git 的范围语义一致 ✓。
  ⇒ 附对拍用例 `TestGoGit_GetCommitsBetween_ForkParity`（gogit 与 native 逐条比对，
  并显式断言共同祖先不在结果里）——旧实现在这条用例下红（gogit 2 条 / native 1 条）。
- **两条门禁债**（`make check` 与 `golangci-lint run ./...` 恒红，已把 main 拖成红的）：
  `provider/ttlcache.go` 的 gofmt 对齐、`provider/ttlcache_test.go` 的 unparam
  （闭包签名由 `GetOrLoadWithSource` 的 func 类型钉死，error 位恒 nil 是本用例的事实）。

## [0.99.0] - 2026-10-25

### Added

- **`provider.ListIssuesOptions` 增加 `Sort` / `Direction`**：平台的 issue 列表默认按
  **created 倒序** ⇒ "上次备份后被改动的**老** issue"正好被挤在第一页之外，
  客户端再按 updated 过滤也捞不到 ⇒ 增量备份漏数据而调用方无从察觉。
  GitHub 后端透传（其 API 原生支持 `sort=updated`）；
  Gitea/Forgejo 家族当前 SDK 不暴露该参数 ⇒ 传入被忽略，调用方需自己兜底。

## [0.98.0] - 2026-10-25

### Added

- **`provider.Issue` 增加 `IsPullRequest bool`**：GitHub 的
  `GET /repos/{owner}/{repo}/issues` **同时返回 issue 与 PR**(载荷里带
  `pull_request` 字段的即是 PR)。此前 SDK 直接把它当 issue 返回,消费方
  没有任何字段能把两者分开 ⇒
  - 同一条记录既进 issue 快照、又进 pull_requests 快照(重复);
  - **占掉 issue 的数量预算**(默认 500)⇒ PR 多的仓里真实 issue 可能整批
    落在预算之外,快照缺项且无任何告警;
  - 审计口径失真(`counts["issues"]` 被 PR 撑大)。

  GitHub 后端按载荷的 `pull_request` 字段设置该标记;
  Gitea/Forgejo 家族的列表端点本就有 `type=issues` 过滤、GitLab 的 issues
  端点也不含 MR ⇒ 其余后端行为不变。

## [0.97.0] - 2026-10-24

### Fixed

- **git 传输加超时（gogit 后端）**：go-git 的默认 client 用 `http.DefaultTransport`
  —— **没有拨号超时、没有响应头超时**。于是"上游连不上/连上不回"时，git 操作会
  一直挂着，调用方只能等自己 context 的超时兜底。实测：GitHub 间歇性不可达时，
  一次克隆卡到操作系统默认的 TCP 重试超时（约 2 分钟）才失败，期间连接一直
  ESTABLISHED。
  修法：gogit 后端构造时给 http/https 协议装上有界传输
  （`GitHTTPDialTimeout` 30s / `GitHTTPResponseHeaderTimeout` 60s /
  `GitHTTPTLSHandshakeTimeout` 15s，可按需调整）。
  - **不设 `http.Client.Timeout`（总超时）**：大仓克隆的响应体可以流很久，
    设总超时会把正常的大克隆打断。只卡"连不上"和"连上不回"这两种真挂死。
  - **不影响 skip-TLS**：go-git 会在 Clone 出来的 transport 上叠加 endpoint 级设置
    （`InsecureSkipTLS` 等），超时随之保留 —— 自建实例（自签/过期证书）照常可连。

## [0.96.0] - 2026-10-21

### Changed

- **oekit 升到真最新**：`pkg` v0.17.0→**v0.20.0**、`concurrency`
  v0.18.0→**v0.19.0**、`logx` v0.7.0→**v0.7.1**（主模块与 mcp 同步）。
  逐项评估收编面：

  - **收编 `errors.Errorf`**（pkg v0.19.0）：带 `%w`（可多个）的格式化错误
    构造入口，规则一已固化——今后格式化构造一律 Errorf；存量无 cause 的
    `fmt.Errorf`（59 处）不强制迁移。审计确认 `%w` 仅存在于
    `provider/errors.go` 的 3 处永久例外，无 Sprintf-then-Wrap 形态。
  - **同族 bug 免疫核查**：concurrency v0.19.0（httpjson 退避移位溢出绕过
    MaxDelay 封顶）与 v0.18.1（retry 抖动幅度 0 时 `rand.Int63n(0)`
    panic）修复的两类 bug，本仓 `transport/retry.go` 的 float64 pow +
    双侧钳制（`> maxd || < 0`）+ 乘法抖动实现均不存在；新增
    `TestRetry_BackoffExtremeAttemptsStayClamped`（attempt 至 1000 钳制与
    正号）与 `TestRetry_BackoffRetryAfterZeroIsImmediate` 钉住免疫性。
  - **无消费点不引入**（结论固化 AGENTS.md）：`pathx.SanitizeRel` /
    `urlguard.WithExactHosts` / `jsonrepair.ExtractArray` / httpjson /
    `syncx.Gate` / `DoValue`——本仓无对应手写实现或调用形态，不硬凑。
- **go 指令 1.27.2→1.27.3**（主模块与 mcp 同步）：标准库漏洞库新收录
  GO-2026-6809（crypto/x509 不信任被攻陷的中间证书，go1.27.3 修复），
  CI govulncheck 腿随库更新转红；沿 v0.79.x 先例把 go 行升到修复版，
  CI `go-version-file` 自动带动 runner 工具链。

## [0.95.1] - 2026-10-21

### Fixed

- **transport 轮询日志分级**：状态 ≥400 不再一律 Warn——5xx 与 429（服务端
  故障/限流信号，与重试引擎的可重试分类一致）保留 Warn，其余 4xx 降为
  Debug；kv 字段（method/url(redacted)/status/duration）不变
  （[#25](https://github.com/yi-nology/go-git-platform/issues/25)）。

  背景：4xx 在本 SDK 契约里不是错误——`RoundTripper` 正常返回完整响应，
  严重度判定权归有上下文的调用方。argus 等消费方的规范文件存在性探测
  （404 即预期缺省）在 215 试点环境单次进程启动即产生 25+ 条噪声 WARN，
  淹没真正的限流/权限告警。路径写错的 404 仍可在 Debug 级别看到，响应
  对象不变，调用方可自行按业务告警。

## [0.95.0] - 2026-10-20

### Added

- **`provider.SkipValidationSecret`：显式 parse-only 入口**。宿主方已自行完成
  签名校验、只想把 SDK 当 payload 解析器用时，把该哨兵传给
  `ParseWebhookEvent` / `WebhookEnvelope.Read`，SDK 跳过内部签名校验，只做
  body 还原与事件头读取。

  背景：v0.77.0 起各平台校验器不再接受空 secret（`StaticTokenValidator` 与
  `HMACSHA256Validator` 均在空 secret 或缺签名头时直接报错），此前"传空
  secret 让 SDK 隐式跳过校验"的写法全面失效，依赖该写法的消费方（如
  review-service 的 `internal/service/webhook`）在解析入口报
  `missing token header`——**五个平台的 webhook 解析同时失效**，且因为是
  空 secret 语义变更而没有破坏性提示，只能靠消费方测试暴露。

  哨兵取值含 NUL 字节，不可能是真实 secret（HTTP 头与配置文件都承载不了），
  只在 `WebhookEnvelope.Read` 内生效：`ValidateWebhookWithRegistry` 与
  `Provider.ValidateWebhookSignature` 仍走真校验器，**不构成签名绕过开关**。
  该变更向后兼容：非哨兵 secret 的行为完全不变。

## [0.94.0] - 2026-10-20

### Changed

- **oekit 升到真最新**: `concurrency` v0.14.0→**v0.18.0**、`pkg`
  v0.14.0→**v0.17.0**(主模块与 mcp 同步;logx v0.7.0 无新版)。pkg
  v0.17.0 的 stringx.Masker BREAKING(短值改整段打码)对本仓无影响
  (不消费 stringx);concurrency v0.18.0 的 `DoValue` 泛型伴生暂无
  消费点(本仓重试走 transport/retry.go 自持路径)

### 姊妹矩阵 oagentkit 全量评估(不引入,结论备案)

oagentkit(github.com/oagentkit/*,13 模块 agent/LLM 工具链,依赖 oekit
底层)逐模块评估后**不引入**,避免后续重复评估(完整结论已固化
AGENTS.md"明确不引入"):

- tier2+ 的 llm/mcpkit/rag/agent/tools/skillkit/toolgate/obs/queue 拖
  eino 全家——公共 SDK 依赖面不可接受
- tier0 `runtime/procx`: 子进程环境白名单会砍 SSH_AUTH_SOCK/GIT_*——
  SDK 的 git 子进程操作用户自己的仓库,威胁模型与 agent 跑不可信代码
  不同,且会破坏 SSH agent 认证与 hermetic 测试基建
- `guardx/egress` SSRF 防护不适用(BaseURL 消费方控制,self-hosted 必须
  允许私网);`guardx/permgate` 运行时逐调用判定与本仓 mcp 注册期裁剪
  形态不同;`guardx/invoke` 解决"装饰被绕过",本仓注册期裁剪已是结构
  保证;`textx/diffx` 上游明确划界(解析 vs 本仓 diff 生成领域)

## [0.93.1] - 2026-10-18

### Changed

- `oekit/pkg` v0.13.0→**v0.14.0**(主模块与 mcp 同步;concurrency
  v0.14.0/logx v0.7.0 无新版)——cache 包增强(动态回填 TTL +
  stale-if-error),本仓不消费 cache 包,纯依赖同步

### 优化盘点结论(本轮无够格优化点)

候选逐一评估后全部排除,记录在案避免后续重复盘点: batch 引擎已收编
`syncx.FanInLimitCtx`(v0.93.0);重试判定已收编 `retry/httpx`(v0.92.0);
`provider/manager.go` 薄层结构干净;`transport/etag.go` 的 etagKey 每
GET 重复计算约 5 allocs(重构调用链传递 key 的复杂度大于收益);
`Manager.buildKey` 的 fmt.Sprintf 与 `ProviderError.Error()` 均为冷路径
纳秒级。等待上游 issue(当前无在途)或新的真实热点再动。

## [0.93.0] - 2026-10-18

### Changed

- `oekit/concurrency` v0.13.1→**v0.14.0**(主模块与 mcp 同步;pkg v0.13.0/
  logx v0.7.0 无新版)——**上游采纳本仓
  [concurrency#27](https://github.com/oekit/concurrency/issues/27) 落地
  `FanInLimitCtx`**,批量引擎整体收编: `provider/batch` 删除自维护
  worker pool(atomic index + GoSafeLabeled + RunSafeRet 组合,约 40 行),
  换 `syncx.FanInLimitCtx(ctx, n, limit, fn)` 调用(约 10 行胶水)。
  三条取消契约(已取消项 fn 一次不调/剩余槽一次遍历快速排空 ctx.Err/
  在飞项跑完落槽)与 panic→typed `*async.PanicError` 均由引擎契约测试
  与本仓语义钉子双向锁定,全部原样通过;batch 语义从此跟随上游引擎
  演进,不再自维护

## [0.92.1] - 2026-10-18

### Changed

- `oekit/concurrency` v0.12.0→**v0.13.1**(主模块与 mcp 同步)——**上游修复
  本仓提的 [concurrency#26](https://github.com/oekit/concurrency/issues/26)**:
  `RetryableWithHeaders` 对 `X-RateLimit-Remaining`/`Retry-After` 头值套
  `TrimSpace`——自造头(测试桩/代理注入/配置还原)与非 textproto 客户端的
  带空白值统一语义。v0.92.0 翻为 false 的 padded 用例恢复 true(原防御
  行为回归);`pkg` v0.13.0/`logx` v0.7.0 无新版

### 需求反馈

- `syncx.FanInLimit` 无 ctx 参数,表达不了「取消后剩余槽位引擎级
  fail-fast」——已提 [concurrency#27](https://github.com/oekit/concurrency/issues/27)
  (提议 `FanInLimitCtx`);采纳后本仓 batch 的自维护 worker pool 可整体
  删除收编

## [0.92.0] - 2026-10-18

### Changed

oekit 升级(pkg v0.13.0 / concurrency v0.12.0 / logx v0.7.0 不变)+ HTTP
重试判定谓词收编上游:

- **transport 重试判定收编 `retry/httpx`**(上轮提的
  [concurrency#22](https://github.com/oekit/concurrency/issues/22) 已在
  v0.12.0 落地,方案与本仓提议一致): `ShouldRetry` 的 429/5xx 核心换
  `httpx.Retryable`;错误构造与重试门控共用的限流谓词
  `isRateLimitedStatus` 换 `httpx.RetryableWithHeaders`(403 +
  Retry-After / X-RateLimit-Remaining: 0 可重试)。语义并集逐项核对等价
  (`ShouldRetry ∪ isRateLimitedStatus ≡ RetryableWithHeaders ∪ 自定义
  Statuses`),幂等方法门控/RetryWrite/body rewind 仍本仓自持。
  单一谓词包装保留(错误构造与门控不漂移的契约不变)
- **已知上游差异**(表驱动用例钉住): httpx 对
  `X-RateLimit-Remaining` 精确比较不做 OWS trim——真实 net/http 解析按
  RFC 7230 已剥离,仅人造头受影响;已提
  [concurrency#26](https://github.com/oekit/concurrency/issues/26) 建议
  补 TrimSpace

## [0.91.0] - 2026-10-18

### Changed

oekit 升级(pkg v0.12.0 / concurrency v0.10.0 / logx v0.7.0,主模块与 mcp
同步)+ 新能力收编:

- **provider/batch panic 收编上游化**: 手写 per-item recover 的
  `fmt.Errorf("batch: fn panicked: ...")` 换成 `async.RunSafeRet`——
  panic 转为 typed `*async.PanicError`(携带原始值与捕获堆栈,可
  `errors.As` 结构化检查);worker goroutine 经 `GoSafeLabeled` 启动,
  panic 汇报带 `task=batch-worker` 归属;janitor 同步换
  `GoSafeLabeled("janitor")`。行为增强:panic 转为 item Err 的同时经
  PanicHandler 汇报堆栈,不再静默
- **errors 双 import 收敛**: pkg v0.12.0 的
  `oekiterrors.New/Is/As/Unwrap` 与 stdlib 同型透传(纯转发,哨兵类型
  逐字节一致)——gitbackend/provider/mcp 共 7 个文件删除 stdlib
  `errors` import,统一经 oekiterrors 执行"类型同一性红线";
  `provider/errors.go` 的哨兵 var 块保持原样(公共契约,文件内本就
  单一 errors 包)

### 需求反馈

- 本仓 transport/retry.go 的 GitHub 403 次级限流判定
  (`403 + X-RateLimit-Remaining: 0 / Retry-After` 可重试)在
  `concurrency/retry/httpx` 中无对应物,已提
  [concurrency#22](https://github.com/oekit/concurrency/issues/22)——
  采纳前保留自实现(HTTP 特化红线不变)

## [0.90.1] - 2026-10-17

### Changed

- `oekit/logx` v0.5.0→**v0.5.1**(主模块与 mcp 同步)——**上游修复本仓
  提的 [logx#11](https://github.com/oekit/logx/issues/11)**: `NewStdLogger`
  无参默认从 `LevelDebug`(iota 零值陷阱)改为 `LevelInfo`,与文档"默认
  INFO"及 `NewSlogLogger` 行为对齐。本仓测试已自备状态不受影响,仅更新
  过期注释;`pkg` v0.11.0/`concurrency` v0.5.4 无新版

> 说明:该版本发布时 goproxy.cn/proxy.golang.org 尚未同步 v0.5.1,本仓经
> 上游 tag tarball 手工构造模块 zip(剔除 hlog/kitexlog/hertzlog/sink 四
> 个嵌套模块,与代理规范一致)本地安装;go.sum 的 `h1:` hash 仅含文件路径
> 与内容,后续 proxy 同步后 hash 一致,下游无需任何操作。

## [0.90.0] - 2026-10-17

### Changed

oekit 升级适配(主模块与 mcp 同步)。上轮提的三个 issue 本轮全部随新版
落地,实现与本仓建议一致:

- **依赖**: `oekit/pkg` v0.9.0→**v0.11.0**、`oekit/concurrency`
  v0.4.0→**v0.5.4**、`oekit/logx` v0.4.0→**v0.5.0**
- **concurrency#4 落地**: `retry.Config` 新增 `Exponential bool` +
  `Jitter time.Duration`(指数退避+抖动,MaxDelay 封顶,溢出饱和)——
  本仓暂无收编点(transport/retry.go 为 HTTP 特化,红线保留;gitbackend
  无手写退避),待未来场景出现直接可用
- **pkg#5 落地**: `MultiError.Errors()` 改 `slices.Clone` 防御拷贝
- **logx#9 落地**: 新增 `LevelEnabler` 接口 + `logx.Enabled` 判定——
  **本仓收编**: transport 新增 `DebugEnabled(logger)`(nil/未实现→保守
  true;logx-backed/SlogLogger 按级别判定),`Client.do` 的 debug 事件
  块抽为 `logRequestDebug` 并以 `logger != nil && DebugEnabled` 守卫——
  真实 logger 但 Debug 被级别过滤时,kv 参数(含响应体拷贝)不再求值;
  `SlogLogger` 补 `Enabled(logx.Level)` 方法(struct 方法,非接口变更,
  映射到 handler 级别)

### Fixed

- transport `Client.do` 复杂度超限(gocyclo 16>15)——debug 事件块抽成
  `logRequestDebug` 方法

### Known upstream issue

- 升级中发现 logx 进程默认 logger 起始级别实为 DEBUG(与文档"默认
  INFO"矛盾,iota 零值陷阱),已提 [oekit/logx#11](https://github.com/oekit/logx/issues/11);
  本仓测试已改为显式自备状态,不依赖文档承诺的默认值

## [0.89.0] - 2026-10-17

### Changed

oekit 升级适配(主模块与 mcp 同步;本次升级的三个变化全部源自本仓在
oekit 提的 issue 被上游采纳):

- `oekit/pkg` v0.7.0→**v0.9.0**、`oekit/logx` v0.2.0→**v0.4.0**
- **logx#8 落地**: `logx.Delegating` 变参改单参必填(误用编译期报错)——
  本仓 `provider/transport.NewLogxLogger` 的编译期断言同步改为
  `Delegating("")`;对外 API 无变化
- **logx#7 落地**: FileSink/RotatingSink 拆出根包,`logx` 根包不再携带
  lumberjack 传递依赖——SDK 消费方的依赖面净减一个
- **pkg#3 落地**: `errors.Wrap` 奇数个 kv 不再静默丢弃,渲染
  `!BADKEY=<value>` 对齐 slog 约定(本仓 104 处调用均为偶数 kv,行为
  不受影响)

## [0.88.2] - 2026-10-17

### Changed

- `oekit/pkg` v0.6.0→**v0.7.0**(主模块与 mcp 同步)。所用 API
  (`errors.Wrap`)签名与行为不变;新 API `In`/`Chain` 不影响既有调用
- **适配**: `gitbackend.IsNotFound` 改用 `oekiterrors.In`——四哨兵一次
  链遍历替代四个独立 `errors.Is` 的 `||` 链,语义严格等价(oekit-first
  规则:能用 oekit 底层解决的不得手写等价物)

## [0.88.1] - 2026-10-16

### Changed

依赖升级(合入 Dependabot #22/#23/#24,统一提交保持线性历史):

- `github.com/go-git/go-git/v5` v5.19.2→**v5.19.3**(commondir 句柄关闭/
  redirect 凭证收敛等上游修复)
- `gitlab.com/gitlab-org/api/client-go/v3` v3.15.0→**v3.18.0**(3 个
  minor,后端全量测试通过)
- `golang.org/x/crypto` v0.57.0→**v0.58.0**,联动 `x/net` v0.59.0→
  **v0.60.0**(清掉 GO-2026-6617/6612/6611 的 require 级命中)+ x/sys、
  x/text;`mcp/` 同步

## [0.88.0] - 2026-10-16

### Changed

oekit 依赖升级到最新 + 热点路径深层优化(无 API 变化):

- **依赖**: `oekit/pkg` v0.1.3→**v0.6.0**、`oekit/concurrency` v0.1.5→
  **v0.4.0**、`oekit/logx` v0.1.10→**v0.2.0**(主模块与 mcp 同步)。逐个
  核对所用 API 签名(`errors.Wrap`/`async.GoSafe*`/`logx.Delegating`/
  `WithService`)全部未变,logx 根包依赖面仍只有 lumberjack 一个传递依赖
- **provider/batch 批处理引擎重写为固定 worker pool**: 原"每 item 一个
  goroutine + semaphore"改为 workers 个常驻 goroutine 拉取 index——
  goroutine 创建从 O(items) 降到 O(concurrency)。语义逐项保持(输入序
  结果/并发上界/取消 fail-fast/panic 转 item Err),既有测试全部通过。
  benchmark(M5, GetFileContents 空 fn): 10k 项 **9.86ms→1.29ms(7.6×)**,
  分配 30004→10012 allocs(-67%),1.84MB→563KB(-69%);1k 项 7.2×
- **transport/client Do 路径的 debug 日志守卫**: noop logger(默认)下
  不再求值 debug kv 参数——原先每次成功请求都执行 `truncateForLog` 的
  响应体字符串拷贝(截断上限 2048B)+ 10 个 kv 装箱分配。挂了真实
  logger 的调用方行为不变

### Added

- benchmark 基线: `transport` Do/DoJSON(noop logger, ~36KB payload)、
  `provider` batch 1k/10k 项 × 默认/最大并发

## [0.87.0] - 2026-10-16

### Changed

oekit/pkg/errors 迁移收尾——剩余 74 处带动态参数的 `%w` 包装点全部换成
`oekiterrors.Wrap(err, msg, kv...)` 结构化形式:

- **gitbackend**(24 处): 哨兵增强型(`ErrInvalidGitArg`/`ErrConfigKeyNotFound`/
  `ErrRepoNotFound`/`ErrAuthFailed` 等)与工作树写盘路径——动态值全部收进
  kv(`key`/`path`/`file`/`host`/`port`/`segment`)
- **provider**(18 处): webhook 验证器矩阵(HMAC/StaticToken/Gitee 双模式/
  Hmac 任意算法)+ wait/factory/detect/pageiter/batch——kv 化
  `header`/`platform`/`ref`/`last_state`/`max_pages`/`drift`
- **backends/pkg**(4 处): github resolve-ref、enterprise client、
  branchfilter bad-pattern、projection item
- **保留 3 处双 `%w`**(provider/errors.go): `Wrap` 分类链需同时挂哨兵与
  原始错误双链,oekit Wrap 单 cause 语义不覆盖,oekit 错误指南对 `%w`
  形式明确认可
- 错误字符串形状变化(动态值从内嵌改为 kv 后缀,如
  `invalid git argument: "ext::"` → `invalid git argument: exec/config
  flag is not accepted from callers arg=ext::`)——分类判定
  (`errors.Is` 哨兵/HTTP status)不受影响,匹配错误消息**文本**的调用方
  需要改用哨兵判定

## [0.86.0] - 2026-10-16

### Changed

接入 oekit 生态第三批——`pkg/errors` 错误包装（对齐其 llms.txt 错误处理
规范）:

- 30 处纯前缀 `fmt.Errorf("...: %w", err)` 包装点统一换成
  `oekiterrors.Wrap(err, "...")`(gitbackend/transport/backends×7/
  provider/githubapp/pkg/mcp)——获得 nil-safe 语义,消息形状
  `msg: cause` 与 `errors.Is/As` 链完全不变
- **不动公共契约**: 哨兵错误/`ProviderError`/`Wrap`/`Wrapf`/`New` 签名
  原样保留;带动态参数的 74 处 `fmt.Errorf` 保留 `%w` 形式(oekit 错误
  指南明确允许),后续再逐步迁移为 kv 结构化参数

## [0.85.0] - 2026-10-16

### Added

接入 oekit 生态第二批——`logx` 日志门面（对齐其 llms.txt 日志规范）:

- **`provider.NewLogxLogger(component)` / `transport.NewLogxLogger(component)`**:
  基于 `logx.Delegating` 的懒解析 logger——消费方在任意时刻
  `logx.SetDefault`/`UseSlog` 后自动跟随;component 属性标识子系统
  (service name 属于消费方)。`logx.Logger` 与本 SDK 的
  `provider.Logger`/`transport.Logger` 方法集结构同构,附编译期断言防漂移

### Changed

- **mcp 命令**: stdlib `log` 换成 `logx` kv 风格
  (`logx.WithService("go-git-platform-mcp")`——二进制是服务方,拥有
  service name;启动横幅改为结构化 kv)

## [0.84.0] - 2026-10-16

### Changed

接入 [oekit](https://oekit.github.io) 生态第一批——`concurrency/async`
安全 goroutine（对齐其 llms.txt AI Agent 硬规则"禁止裸 go func"）:

- **provider/batch**: worker goroutine 改经 `async.GoSafe` 启动,内层
  recover 把 `fn` 的 panic 转换为该 item 的 `Err`(其余 item 不受影响);
  此前 panic 会直接崩溃调用方进程
- **provider/janitor**: 后台清理 goroutine 改经 `async.GoSafe`;`cleanup`
  panic 时 defers 照常执行(done 关闭/状态复位),janitor 干净停止且可被
  `start` 重新拉起,不再拖垮宿主进程
- **mcp**: HTTP 监听 goroutine 改经 `async.GoSafeWithErrChannel`,panic
  转为 error 汇入 errCh

### Fixed

- `GetFileContents`/`GetCRs`/`SetCommitStatuses` 批量原语: 单项 panic 不再
  使进程崩溃或结果缺失,新增 `TestBatchPanicBecomesItemError` 回归测试

## [0.83.0] - 2026-10-13

### Changed

设计台账 F13 + F11（`docs/superpowers/specs/2026-10-06-cohesion-patterns-
refactor-design.md`）:

- **F13 webhook 信封骨架**: 新增 `provider.WebhookEnvelope`——平台方言
  描述符(EventHeaders 有序头链/IDPrefix)+ `Read`(验签→读体→事件头,错误
  op 与手写路径逐一对齐)+ `NewEvent`(Source/Timestamp/RawPayload 骨架 +
  `<prefix>-<nano>-<key>` ID)。6 处接入:gitlab/gitee/gitcode/
  tencentcode/github + giteafamily 引擎——各平台 ParseWebhookEvent 只留
  payload 解码与事件分派(平台方言),7 平台 35 个 golden corpus 逐字节通过
- **F11 错误分类去 reflection**(breaking 面,minor 承载):
  `provider.RegisterStatusExtractor(platform, fn)`——每后端注册一个基于
  `errors.As` 的 SDK 错误类型提取器(github 5 类型/gitlab 1/gitee 3/
  gitcode 7(gotcha:wrapper 无 Unwrap 须逐个断言)/tencentcode 1);
  Wrap 分类链收敛为 statusCoder 接口 → 平台注册 extractor。
  **删除** `httpStatusFromError`(reflect 字段/方法扫描)与
  `parseStatusFromString`(错误消息数字解析)+ 9 码白名单。gitea/forgejo
  的 SDK 错误不携带 status(平台限制),不注册——其 404 分类经既有的
  *Response 状态保真路径(statusCoder 快路径)

### Fixed

- provider 单测改写到新契约: 5xx 原因保留经注册 extractor 路径;
  "错误消息含状态数字不再分类"新增负向回归测试

## [0.82.0] - 2026-10-13

### Changed

设计台账 F9 + F12（`docs/superpowers/specs/2026-10-06-cohesion-patterns-
refactor-design.md`）:

- **F9 能力声明单通道**: 新增 `provider.DeriveCapabilities(p)`——从
  可选 manager 接口的类型断言派生 CapabilitySet,7 个后端的
  `Capabilities()` 从手写 12–17 bool 字面量改为一行派生。断言成为能力
  声明的唯一事实来源,声明与实现的漂移不可能再发生(contracttest 的
  双向校验保留,防绕过派生手写字面量)
- **F12 ListCRs total 语义显式化**(breaking,minor 承载):
  `ListCRs(...) ([]*ChangeRequest, *int, error)`——`total` 为 nil 表示
  "平台未提供权威总数",非 nil 是平台自身元数据(X-Total-Count/X-Total/
  Link header 推算)。各后端逐一对齐:gitcode 明确"SDK 无 total"从返回
  页长改为 nil;tencentcode 全量合并模式保留 &len(合并即全量,构造即
  精确)、单页模式 header 缺失改 nil;gitee/gitlab/github 的"页长兜底"
  删除。mcp list_crs 的 DTO 保持 int(nil 渲染为 0)

## [0.81.0] - 2026-10-13

### Changed

Gitea 家族合并第 4 批（收尾）——**22 个能力文件全部迁入 `giteafamily`
引擎,Cluster A 完成**:

- **releases**: tags/releases 全量分页、CreateRelease 的 target_commitish
  空串=默认分支语义、UpdateRelease 的 id 解析 + title/note 回填
  (EditReleaseOption 无 omitempty,draft/prerelease 指针透传)、
  GetArchive 格式映射
- **reactions**: 端口语义统一为"全量列出"——gitea 适配器内部分页、
  forgejo 单次不分页,差异完全留在适配器;reactionID=响应 UserID、
  删除按 content 字符串的 N+1 扫描语义收进引擎;CR/CR-comment 级
  不支持语义按平台名报告
- **branch_protection**: 全字段迁移(AllowDeletions↔
  BlockOnRejectedReviews 逆映射、AllowDeletions 无直接字段时的
  no-op 语义原样保留)
- **gitea/forgejo 的 `types.go` 整体删除**: 转换器(convertRepo/convertPR/
  convertRelease/mapState/parseTotalCount/timeOrZero)全部由引擎的
  Family 线格式转换取代
- 两包 22 个能力文件的归一化差异收敛到 4–26 行(注释措辞),真实平台
  差异只剩两端 ports.go 这一处(设计使然);全仓 go test 通过,覆盖率
  48.5%,golangci-lint 主模块+mcp 0 issues

## [0.80.0] - 2026-10-13

### Changed

Gitea 家族合并第 3 批（承接 v0.78.0 架构,台账见
`docs/superpowers/specs/2026-10-06-cohesion-patterns-refactor-design.md`）:

- **4 能力迁入 `giteafamily` 引擎**: reviews(GitHub 事件词表→家族 review
  状态词汇映射,行号 side 语义收编)、diffs(GetCRDiff 的 raw diff + 文件
  分解 + 总数聚合,DeleteNote 的 404 容忍)、files(GetFile 的 404 经
  *Response 状态保真,FileResult 的 commit SHA)、repos(ListRepos 的
  owner 过滤分歧、GetRepo 的 404 保真、CreateRepo 的 user/org 分流、
  ForkRepo、DeleteRepo、UpdateRepo)。两后端各 4 文件收为一行委托
- **issues 完整迁入**(含 title-backfill——家族 EditIssueOption 无
  omitempty,标题回填经一次 GET;labels/assignees 经共享 TTL 缓存解析;
  comments 全量分页;close/reopen 状态归一)
- **crs.go + types.go 收敛**: `PullRequest` 线格式逐字段核验(两 fork 同
  构字段 30+,gitea 独有 RequestedReviewers/RequestedReviewersTeams/
  Draft/ReviewComments 四个),引擎按 Family 方言参数
  `CRPointersHaveReviewers`/`PRWireHasDraft` 取活子集;forgejo 侧
  convertPR 的 Reviewers 回填 Assignees、Draft 恒 false 原样保留;
  `mapListState` 归一化统一(forgejo 原透传 "opened" 是历史 bug,一并修)
- 引擎新增 PRPort(含 ListPullRequestCommits)、IssuePort(含 comments)、
  DiffPort(含 comment 创建/删除)、ReviewPort、FilePort、RepoPort;
  Family 线格式 PullRequest/FamilyComment/FamilyChangedFile/
  FamilyFileResult 等自持
- 净删 ~2100 行、增 ~1130 行;gitea/forgejo 两包从 ~6.4k 行降到 ~1.3k 行

### Fixed

- forgejo 的 `ListCRs` 状态过滤修正(原透传 "opened" 触发家族"未知值返
  回全部"行为,现与 gitea 一致归一到 open/closed)

## [0.79.3] - 2026-10-13

### Fixed

- **CI 链配套升级**(v0.79.2 的 Release 已发但 CI 腿红,govulncheck 与 lint
  两个门禁都卡在"工具链比目标旧"): golangci-lint v2.12.2 → v2.14.0(其
  二进制以 go1.27 编译,跟上 1.27 目标);govulncheck v1.1.4 → v1.9.0(修复
  对 go1.27 AST 的 panic: unexpected expr: *ast.KeyValueExpr);vuln job
  去掉显式 GOTOOLCHAIN=local 的隐式锁定,让工具读 go.mod 的 toolchain。
  无 API 变化

## [0.79.2] - 2026-10-13

### Changed

- **Go 版本线升级 1.26.3 → 1.27.2**(主模块 + mcp 模块,`go`/`toolchain`
  同步):跟进上游稳定线;CI 全部 job 经 `go-version-file: go.mod` 自动跟随,
  govulncheck 门禁在新线干净(1.27.2 已含 crypto/tls、net/http 系列修复)。
  无 API 变化

## [0.79.1] - 2026-10-13

### Fixed

- **toolchain 升级 go1.26.6 → go1.26.9**(主模块 + mcp 模块): 修复标准库
  net/http 的 3 个 HTTP/2 CVE(GO-2026-6617 HPACK encoder race、
  GO-2026-6613、GO-2026-6612 双倍流量控制退款),CI 的 govulncheck 门禁
  从红转绿;v0.79.0 的 Release 已发但 CI 腿红,本版为纯构建链修复,
  无 API 变化

## [0.79.0] - 2026-10-06

### Changed

Gitea 家族合并第 2 批（承接 v0.78.0 的 ports & adapters 架构,台账见
`docs/superpowers/specs/2026-10-06-cohesion-patterns-refactor-design.md`）:

- **7 个能力迁入 `giteafamily` 引擎**: labels(含 name→ID 的 TTL 缓存
  解析器,issues.go 的 assignee 编辑改共享)、branches、collaborators
  (含每用户 CollaboratorPermission 回填)、repo_stats(forks/stargazers)、
  notifications(read/unread/pinned 词表收编,forgejo 侧 `NotifyStatus`
  映射)、commits(含 CreateCommitStatus/ListCommitStatuses,状态词表统一
  到 provider 归一化)、search(全局仓库/用户 + repo 内路由 ListRepoIssues)。
  两后端各 7 个能力文件收为对引擎的一行委托,净删 ~940 行
- **端口扩展**: `sdkPorts` 增 7 个端口(Label/Branch/Collaborator/RepoStats/
  Notification/Commit/Search),gitea 直接委托、forgejo 去 ctx + fork 类型
  逐字段转换(各字段已核验;forgejo 通知词表 `NotifyStatus` 与 gitea
  `NotificationStatus` 同值不同名,适配器归一)
- 引擎侧新增 `ListCommitStatuses` + `ListStatuses` 端口方法(commits 能力
  的双接口覆盖 CommitManager + CommitStatusManager)
- gitea 适配器从已弃用的 flat `client.Xxx` 收敛到 `Repositories` 服务
  (staticcheck SA1019)

### Fixed

- 引擎把 `Issue.State` 的 provider 类型对齐(`provider.IssueState`,修正
  第 1 批遗留的 string 转换)
- SearchIssueResult 补 Comments/CreatedAt(原实现携带,第 1 批合并遗漏)

## [0.78.0] - 2026-10-06

### Changed

Gitea 家族合并第 1 批 + Manager 拆分（承接 v0.77.0 设计文档的后续清单,
架构与迁移台账见
`docs/superpowers/specs/2026-10-06-cohesion-patterns-refactor-design.md`）:

- **`backends/internal/giteafamily` 家族引擎(ports & adapters)**: 勘察确认
  forgejo SDK 是 gitea SDK 的 fork——类型逐一同构但名义类型不同。引擎零
  SDK 依赖,自持最小线格式 DTO 与端口接口,拥有全部行为(选项构建/分页/
  provider 转换/错误包裹/webhook 解析/分歧台账);每后端一个 `sdkPorts`
  聚合适配器:gitea 近直接委托,forgejo 去 ctx + fork 类型逐字段转换。
  Family 描述符携带平台名/事件 ID 前缀/事件 header(X-Gitea-Event vs
  X-Forgejo-Event)/页大小
- **第 1 批迁移 4 个能力**: webhooks(解析器归一化后逐字节相同,含 payload
  结构体与事件映射)、divergence(台账文本经 displayName 参数化)、
  deploy_keys、milestones。gitea/forgejo 两包合计净删 ~870 行;双侧
  contracttest + webhook golden corpus 全程钉死
- **`provider.Manager` 五职责拆分**: 泛型 `ttlCache[V]`(TTL+LRU 驱逐+
  原子计数,getOrBuild 双检收敛并发构建)与 `janitor`(start/stop 生命
  周期,保留 reset-before-done 重启语义)独立成件;Manager 收敛为 key
  派生 + 工厂调用 + 委托,公开 API 不变

### Fixed

- forgejo/gitea 的 webhook corpus 测试改用真实构造的 provider(此前用
  零值 `&Provider{}` 仅取解析器,引擎化后需 Family 描述符)

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
