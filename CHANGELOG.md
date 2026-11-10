# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]


### Fixed（第四批 9 连修：SDK 限流分类 / ListIssues 分页统一 / Since-Until 显式报错 / projection 集合并 / branchfilter 逗号 / Reaction 桩三态 / State=merged / tag push 归一 / compare 截断）

- fix(provider,backends/github): **SDK 错误路的限流分类接通**（issue #50）——
  go-github 的 `*RateLimitError`（主配额耗尽）与 `*AbuseRateLimitError`
  （次级限流）恒为 HTTP 403，且既不实现 `StatusCode()` 也不携带 transport 的
  rateLimitedMarker ⇒ Wrap 的裸状态分类把 GitHub 最常见的节流信号丢回
  ErrForbidden，IsRateLimited/RateLimitRecovery 双双失灵。机制级修复：新增
  `RegisterThrottleClassifier`（与 RegisterStatusExtractor 并肩的注册面），
  Wrap 在状态分类之前采纳平台注册的权威节流判定并复制恢复窗口
  （Rate.Reset / RetryAfter）；GitHub 后端 init 注册，其他 SDK 路平台
  （Gitee/Gitea 等）可同面接入。补 SDK 错误路端到端 + 机制面 + 「普通 403
  仍 ErrForbidden」对照三测试。
- fix(backends,provider): **ListIssues 分页/total 语义全后端统一**（issue #51）——
  caller-paged 族契约落齐（见 NormalizePageOpts）：Page==0 一律单请求取
  首页、Page>0 恰取该页，int 返回一律 len(result)——不再是平台总数。
  对齐面：giteafamily（gitea/forgejo）与 gitcode ListIssues 旧 Page==0 走
  全部页、tencentcode 旧 caller 页保留平台总数（X-Total）都改为单请求 +
  len(result)；接口方法补成文契约注释（原来没有 doc），契约套件补
  「Page==0 恰 1 请求」「total==len(result)（夹具 X-Total:999 钉差异）」
  双断言。**迁移说明**：依赖 gitea/forgejo/gitcode/tencentcode
  ListIssues(Page==0) 全量行走语义的调用方请显式翻页。
- fix(provider,backends): **Since/Until 解析失败显式报错**（issue #52）——
  新增共享解析器 `provider.ParseTimestamp`（RFC3339；空=不过滤；其余一律
  ErrInvalidInput 并命名选项与版式），解析失败静默丢弃（日期写错悄悄返回
  全量历史、err=nil）改为报错：github/gitlab ListCommits（Since/Until）、
  github ListNotifications/ListRepoNotifications、giteafamily
  ListNotifications/ListRepoNotifications（Since）、gitee/giteafamily/gitcode
  ListCommits 与 gitee/gitcode/gitee-notifications 等裸串透传面（先校验后
  透传，接受/拒绝面全后端一致）；家族 SDK 不带 commit 时间过滤的面
  （giteafamily/gitcode ListCommits 合法值无法下传）按 DivergenceIgnore
  登记进台账；契约套件补「malformed Since/Until ⇒ ErrInvalidInput 且
  0 wire」全后端断言（notifications 含 repo-scoped 半边）。
- fix(pkg/projection): **字段集合并与列出顺序无关**（issue #53）——keepTree.add
  叶子分支遇既有嵌套子树时升级为整棵子树保留：祖先路径排在后代之后不再
  被静默忽略（旧实现同一组字段仅因次序不同投影结果不同，LLM 生成的字段
  清单顺序不稳定时静默丢字段）；补次序置换/深层祖先/兄弟路径共存表驱动
  测试（红↔绿）。
- feat(pkg/branchfilter): **NewPatterns 免切分构造器**（issue #54）——
  每个元素即一个完整 glob pattern，逗号是普通 pattern 字符：含逗号的合法
  git ref 名（`release,1.0`）首次可被过滤器匹配（旧字符串形态逗号恒为
  分隔符、无转义、本尊永不可匹配且无绕法）；New 保留逗号切分便利面并在
  包 doc 写明分隔符地位与表达力边界；两构造器同走 ErrBadPattern 预检。
- fix(backends/gitlab,backends/internal/giteafamily): **ReactionManager 桩
  三态统一**（issue #57）——gitlab 六个 note 级方法统一为
  `Wrap(ErrNotImplemented)` 桩（平台 award-emoji-on-note 端点需要父 issue
  IID/MR IID + note ID 双址，provider 接口只给 commentID 单址，接口面无法
  驱动），(nil,nil) 静默空与无哨兵文本错误两形态并存全部清零，六条
  DivergenceStub 登记 + 契约 dispatch 表配对（哨兵⇒台账机器互检、零 wire）；
  giteafamily 四个 CR 级方法**实装**（家族 CR 即 issue 共享 index、CR 评论
  即 issue 评论——ListCRComments 同一先例，github 后端同型委托）：CR/评论
  级反应读写走既有端点，不再返回 (nil,nil) 或文本错误；ReactionsSuite 补
  「每方法恰一形态」七方法断言（读 (nil,nil) / 无哨兵文本错误全部钉死）。
- fix(backends/gitcode,backends/github,backends/internal/giteafamily):
  **ListCRs State=merged 四家分化收口**（issue #59）——gitcode 走后过滤
  精确化（SDK PullRequest.Merged 标志筛掉 closed-but-unmerged，四家退化
  后端里唯一可精确的，补 wire 回归钉 merged:true/false 混合夹具）；
  github/giteafamily（gitea/forgejo）的 closed 超集近似按 mapping 登记
  进台账（closed∪merged 混返此前仅代码注释披露，违反「禁止文档-only
  偏离」），`go generate` 再生成 docs/divergence-ledger.md。
- fix(backends/github): **tag push 归一对齐跨后端词表**（issue #76）——
  PushEvent ref 带 refs/tags/ 前缀时归 `tag.created` 并填 Tag（对齐 gitee
  tag_push 与 forgejo/gitcode golden；同文件 CreateEvent/DeleteEvent 的
  ref_type=tag 早已归 tag.created/tag.deleted）：旧实现产出 "push" 且
  Branch 被污染成 "refs/tags/v1.0" 伪分支名，按 tag.created 驱动的下游
  漏掉 GitHub 标签事件；webhookcorpus 补 push_tag 夹具 + golden 钉形态。
- fix(backends/github,provider): **CompareCommits 静默截断收口**（issue #77）——
  commits 走分页拉全（GitHub compare 单次上限 250 条，旧实现 nil
  ListOptions 只取一页）；files 平台侧无分页线（仅首页返回、全量上限
  300 条）——`CompareResult` 新增 `FilesTruncated` 字段，len==300 时置
  true 截断可见；补 >250 提交两页拉全 + 恰 300 文件置位 + 小 compare
  单页短页终止三条 wire 回归（红↔绿）。


### Fixed（第三批 7 连修：Page==0 双模契约 / ListMilestones State 统一 / 422-410-5xx 哨兵 / 规则一残留 / janitor 身份检查 / 契约五族补白）

- fix(provider): **Page==0 分页语义双族契约钉死**（issue #60）——`NormalizePageOpts`
  升级为全 SDK Page 契约锚点：有界集族（labels/milestones/notifications）
  Page==0 走全页（预算封顶、撞页失败为响），Page>0 恰取该页；无界集族
  （CR；Issue 族 #51 另批）Page==0 单请求取首页，Page>0 恰取该页。对齐面：
  gitlab ListLabels/ListMilestones/ListNotifications/ListRepoNotifications（旧
  归一 page 1，只回 GitLab 默认 20 条）、gitee ListNotifications/ListRepoNotifications
  （旧 Page==0 不带分页参数 ⇒ 服务端默认页）、giteafamily ListLabels；tencentcode
  ListCRs 的 Page==0 全页行走按 divergence 登记（caller-paged 族例外），
  github ListCRs total=LastPage×PerPage 上界近似同批登记；契约套件补三族
  「Page==0 走全 / Page>0 单请求」wire 计数断言。
- fix(backends): **ListMilestones State 空值语义统一**（issue #61）——空 State
  一律显式落 "all"（github/gitee/gitcode/giteafamily 旧透传空值 ⇒ 平台缺省
  open-only，closed 里程碑静默漏项）；gitlab 空值本就不发过滤（= all）不变；
  tencentcode 列表端点无 State 参数，显式 State 落 DivergenceIgnore 台账；
  契约套件补「空值 wire ∈ {"", "all"}」「closed 显式转发（ignore 登记者豁免）」
  双断言（ignore 判据走台账机器可读，不上手写旗标）。
- fix(provider): **422/410/5xx 哨兵可判**（issue #64）——`classifyStatusCode`
  补 400/422→`ErrInvalidInput`、410→`ErrGone`（新哨兵）、5xx→`ErrServerSide`
  （新哨兵），`IsGone`/`IsServerSide` 判定助手同批；真实 wire 422 与本地构造
  校验拒绝（gitea repo_template、github deployments/repo_extras 等
  `provider.New(..., 422)`）现均 `errors.Is` 可判；其余状态码维持「仅文本、靠
  IsStatus 判」口径并在 `ClassifyStatus` godoc 写明；补全量映射表测试 +
  Wrap/New 端到端哨兵链测试。**迁移说明**：422/410/5xx 错误的 `Error()` 文本
  形态从 `HTTP 422` 变为 `HTTP 422: invalid input` 形态，精确匹配错误字符串的
  调用方请改用 `errors.Is`/`IsStatus`。
- fix(pkg/credential,transport,provider): **规则一残留面三连清**（issue #68）——
  pkg/credential sshkey 五处 `%v` 包装迁 `oekiterrors.Wrap`（cause 链恢复，
  `ssh.PassphraseMissingError` 等类型化原因重新可判，补 cause 链保真回归）；
  provider/errors.go `Wrapf` 与 classifyStatusCode 文本分支迁
  `oekiterrors.Errorf`（`fmt.Errorf` 收敛回双链豁免配额 3 处）；
  transport `AuthStrategy.Apply` 签名改返回 `error`（**breaking**，0.x minor
  承载）——`TokenSourceAuth.Apply` 旧 `_ =` 吞掉 TokenSource 刷新错误，直呼
  Apply 的调用方拿无凭据请求无信号；全仓实现与唯一调用点 `applyAuth` 同步，
  刷新错误现沿 plain-Apply 路径传播（补传播回归测试）。
- fix(provider): **janitor stopAndWait 补身份检查**（issue #69）——`j.done = nil`
  前加 `if j.done == done` 与 goroutine 侧自复位对称；旧无条件清零会在
  「Stop 持锁清 stop 后等待退出的窗口内并发 StartJanitor」时把新 janitor 的
  done 一并抹掉 ⇒ 下次 Stop 不等新 goroutine 退出即返回，「阻塞至退出」契约
  破窗；补窗口期交错回归测试（200 轮打点复现旧缺陷，红↔绿双向验证）+ 并发
  start/stop 混跑 -race 压测。
- feat(backends): **三处 mapping/detour 机器可读登记**（issue #62）——gitlab
  Milestone DueOn 日期截断（ISOTime 丢时分秒）与 ListNotifications/ListRepoNotifications
  映射 Todos API（threadID 即 todo ID、All/Since 无线路面）落 gitlab 台账；
  gitee ListNotifications/ListRepoNotifications 裸 HTTP detour（私有
  rawNotificationList、total_count 不外露）落 gitee 台账；`go generate ./...`
  再生成 docs/divergence-ledger.md。
- feat(backends/contracttest): **契约五大族补白**（issue #63）——CRSuite
  （Get/List 解析、Page==0 caller-paged 单请求契约 + tencentcode 例外与台账
  互检、Create wire 双词表断言、Close/Reopen state/state_event、Update title、
  Merge 端点）、BranchSuite（list/create/delete）、CommitSuite（get/list/compare
  解析）、DiffSuite（GetCRFiles/GetCRDiff 解析 + #44 同族全页、CreateNote wire）、
  FileSuite（GetFileContent 解码断言，raw/base64 双形态）挂进 Run()；七后端
  逐台配齐夹具（平台 wire 词表实锚：gitee additions 字符串、gongfeng changes
  包裹对象、giteafamily 无 hunk 文件列表与 raw 内容端点等）；补 Run() 覆盖矩阵
  × 方法族机器互检（TestRunCoverageMatrix 双向断言）与六个可选能力族
  （Gists/Starred/Migrations/ReleaseAssets/Deployments/ForkSync）「仅声明检查、
  无行为断言」缺口显式登记（TestOptionalCapabilityFamiliesUncovered）。


### gitbackend/gogit/credhelper 底座层

- fix(gitbackend): native `IsAncestor` 按退出码分流——`git merge-base
  --is-ancestor` 退出码 1 = 非祖先（正常结论）、≥2 = 致命错误；旧实现把
  一切错误吞成 `(false, nil)`，无效 ref/非仓库目录与「确实不是祖先」不可
  区分（调用方拿 false 当「未合入」继续执行——跳过 cherry-pick、误判漂移、
  放行合并）；argv 补 `--` 分隔与位置参数校验（issue #101）。
- fix(gitbackend): gogit `GetStatus` 补齐 Ahead/Behind——复用
  GetBranchSyncInfo 抽出的 `aheadBehindOf` 计数（merge-base 双侧统计，含
  无关历史回退），上游缺失保持 0/0 宽松口径（与 native 忽略 getAheadBehind
  错误一致）；旧实现零处赋值恒 0/0，漂移检测永远显示「已同步」；契约测试
  钉两后端同仓同数（issue #102）。
- fix(gitbackend): CommitInfo/TagInfo 两后端口径统一——native 格式串
  `%s`（仅主题行）→ `%B`（完整消息）、`%ai`（空格分隔偏移，RFC3339 解析
  必败）→ `%aI`；解析改 NUL 记录制（前导 NUL 开记录、消息置末位吸收分隔
  符），多行消息不再破坏解析；GetTagList 同步从 `%(subject)` 换
  `%(contents)`（for-each-ref 的 `%00` 十六进制转义可用，旧注释「不支持
  NUL」系语法混用），gogit 侧消息 TrimRight 归一；契约与 godoc 写明
  「完整消息 + RFC3339 author date」（issue #103）。
- fix(gitbackend): native `GetStatus` 改 `--porcelain=v1 -z` 解析——NUL
  分隔路径字节原样输出（中文/引号文件名不再被 core.quotePath 转义成
  `"\"\\346\\226\\207\""` 形态）、路径首尾空格不再被 TrimSpace 吃掉、
  rename 条目按「`XY <dst>` + `<orig>`」双记录拆分（旧实现 Path 是
  `old -> new` 整串）（issue #104）。
- fix(gitbackend): credhelper 脚本模板 `echo password=$(cat …)` 改
  `printf 'password=%s\n' "$(cat …)"`——命令替换进双引号后不做 IFS 分词/
  glob 展开、printf %s 不解释反斜杠（dash 的 echo 会把 `\t`/`\n`/`\c` 当
  转义），含空格/`*`/`$`/反斜杠/引号的密码按字节原样到达 git；askpass
  的 username 回答同步换 printf（issue #105）。
- fix(gitbackend): credhelper/askpass 加目标主机白名单——helper 读 stdin
  的 credential 协议取 `host=` 匹配（大小写归一）后才应答；askpass 从提示
  文本（`… for 'https://host':`）提取主机同样校验，未知提示一律静默（旧
  实现对任意提示回退输出 username，同属跨域泄漏面）；期望主机由
  `credentialHostsFor` 推导（argv URL + remote 名经仓库 config 解析，
  gitfile/worktree 布局兼容），推导不出时空白名单 fail-closed 拒绝一切；
  `FetchAll` 从单条 `git fetch --all` 改逐 remote 调用，多 remote 混合
  信任域时每个会话只对当前 remote 主机应答——为 A 准备的令牌不再发给 B；
  真实 `git credential fill` 集成测试钉「声明主机拿到令牌、异主机空应答」
  （issue #106）。
- fix(gitbackend): native `parsePushRefs` 重写——PushedRefs 是 push 报告的
  dst ref 名（`old..new  src -> dst` 取 dst、`- [deleted]` 行取末字段、
  `! [rejected]` 排除）；旧实现把 `To <url>`（内嵌凭据会进结果/日志）与
  old..new 区间当 ref 收集、`* [new branch]`/`- [deleted]` 行完全丢失；
  gogit Push 同步把请求 refspecs 归一成 dst 短名（去 `+`/`refs/heads|tags/`
  前缀）对齐口径，native 解析失败时按同一归一回填 RefSpecs（issue #107）。
- fix(gitbackend): gogit `GetFileAtRevision` 走 resolveRev——分支/标签/
  短 hash/rev 表达式统一解析（旧实现 `plumbing.NewHash` 对任意串盲 hex
  解码，非 hash 输入全失败）；空 ref 语义两后端统一为 HEAD（native 旧
  裸 `":path"` 读 index，index≠HEAD 时静默返回暂存内容而非提交内容；
  GetBlob 空 ref 同口径），契约 godoc 写明（issue #108）。
- fix(gitbackend): 位置参数统一防语义反转——branch/tag/remote 名与 rev
  类参数一律 `checkNameArg` 拒绝首字符 `-`（`ErrInvalidGitArg`），
  CreateBranch/DeleteBranch/RenameBranch/CreateTag/DeleteTag/AddRemote/
  GetRemoteURL/RemoveRemote/merge-base/cherry-pick/rebase/add 并加 `--`
  分隔（checkout/diff 系不吃 `--`，靠校验挡）；回归测试钉
  `CreateBranch("-D", victim)` 不再删分支、log/diff/show 的
  `--output=…` rev 不再落任意文件（issue #109）。
- fix(gitbackend): Diff 空 From/To 三方统一为「工作区 vs HEAD」unified
  patch（`git diff HEAD` 语义，staged+unstaged 均可见，untracked 不出现）
  ——旧 native 裸 `git diff` 只看 index 侧（staged-only 不可见）、旧 gogit
  返回 go-git 状态清单文本而非 patch；单边 From/To 两后端一致显式报错；
  gogit 侧经 go-git UnifiedEncoder 渲染（object.Patch 同管线），按
  index∪HEAD 选路径、index 跟踪性判定删除（rm --cached 同 git 呈现）
  （issue #110）。

### Fixed（第二批 14 连修：安全面凭据回灌 / 错误分类断链 / 读回反转 / 契约兑现）

- fix(transport): 跨 host 重定向剥凭据判据改对齐**重定向链起点**——旧判据只比紧邻
  上一跳，「平台→第三方→第三方同域再跳」链形的第二跳会把平台令牌**重新施加**到
  第三方主机（Bearer/PRIVATE-TOKEN/query token 三通道，探针实证）；backendutil
  `SDKHTTPClient.CheckRedirect` 同型修正（`via[len(via)-1]`→链头）；补多跳链
  回归测试（issue #65，P0）。
- fix(transport): 10MB 默认响应体上限**真正生效**——`NewClient` 缺省即带
  `DefaultMaxBodySize`，`WithMaxBodySize(0)` 显式表无限制、`-1` 取默认；
  旧缺省零值=无限制，文档宣称的 OOM 防护实际处于关闭状态（全仓 0 调用点）
  （issue #70）。
- fix(provider): `Manager` 缓存键补全全部影响 wire 行为的配置字段——
  TokenStyle/ConditionalRequests/RetryConfig 内容指纹 + TokenSource/Hooks/Logger
  指针身份（可比较值形态走 hasher 摘要，凭据不落键明文）；旧键漏字段 ⇒ 仅差
  TokenSource 的两个 Config 命中同一 provider＝**错凭据互撞**（探针实证）；
  补逐字段拆键回归测试 + 等值共享反断言（issue #66，P0 后果面）。
- fix(github): `GetArchive` 下载跳三连修——`NewRequestWithContext`（ctx 传导）、
  非 2xx 状态码门（错误页不再当归档字节返回，经 `provider.New` 归类）、下载走
  无 30s Timeout 的 token-less `followClient`（大归档不再被全局限时截断）
  （issue #72，P0）。
- fix(github): `GetFileContent` 检查 `DownloadContents` 第二跳的
  `resp.StatusCode`——go-github 契约明示两跳下载非 2xx 返回 `(错误页, nil err)`，
  旧实现把 429/5xx 错误页文本当文件内容静默返回（issue #71，P0）。
- fix(github): `MarkNotificationsRead`/`MarkRepoNotificationsRead` 的
  `LastReadAt` 解析失败即报 `ErrInvalidInput` 且**零 wire 触达**——旧实现静默
  丢错落零值，go-github 对零值的语义是「全部标已读」，一个格式笔误即不可逆
  全量副作用（issue #75）。
- fix(github): `CreateBranchProtection` 不再委托 Update 的 GET 前置（未保护分支
  必然 404 ⇒ create 路径死锁、`EnsureBranchProtection` 的 EnsureCreated 不可达），
  直落 PUT create-or-replace；`UpdateBranchProtection` 以 current 全字段回填兑现
  「nil 字段不变」契约——enforce_admins 透传当前值（旧硬编码 true）、
  AllowForcePushes/AllowDeletions 回填（omitempty 缺省下被复位为 false）、
  Restrictions 逐实体回填（旧 wire 成 null 清空 push/merge 限制）；补「只改一字段
  其余保持」契约测试（issue #73+#74 同族设计）。
- fix(gitea/forgejo): 适配层全端口（82 处/端）以 `withStatus` 把 SDK *Response 的
  HTTP 状态串进错误链——两 SDK 把非 2xx 拍平成纯文本错误、旧适配层整体丢弃
  response ⇒ `IsNotFound/IsRateLimited/IsConflict/IsForbidden` 恒 false（仅 4 处
  特判可判，「取不到就建/回退」分支静默走错）；family 层 `familyError` 统一收口
  四处特判；补「状态码 × 哨兵」矩阵 + 多方法 404 采样回归（红↔绿双向验证）
  （issue #55）。
- fix(gitee): `CreateCommitStatus` 的 in-flight 态（pending/running/未知动词）改写
  check-run 的 `in_progress` 形态（不带 conclusion）——旧写 `completed+neutral`
  被读侧 conclusion 归一成 **Success**，「写 pending 读回 success」静默反转令
  合并门禁提前放行；终态动词照常 completed+conclusion；补 round-trip 读写双侧
  测试；台账同步（issue #58）。
- fix(webhooks/gitlab 家族): 分支删除归一 `branch.deleted`——GitLab/Gitee/
  TencentCode/GitCode 四后端的 push + 全零 after 投递此前一律落 `push`（#47 只修了
  tag_push 案），同一物理动作在 github/gitea/forgejo 产出 `branch.deleted`，消费方
  switch 静默漏掉全部分支删除；四后端补 #47 同款守卫（before 非零确认真删除），
  webhookcorpus 补四份 delete_branch 夹具 + golden（issue #67）。
- fix(repo_stats): 四处桩方法改 `provider.Wrap(platform, op, ErrNotImplemented)` 并
  按 DivergenceStub 强制登记台账——giteafamily ListContributors、gitlab
  ListStargazers、tencentcode ListForks/ListContributors 旧裸 `Wrapf` 使
  `IsNotImplemented` 恒 false；契约套件改「stub ⇒ IsNotImplemented + 台账配对断言、
  实作 ⇒ 夹具解码断言」，删除任意错误一律 skip 的空转形态；DivergenceSuite
  dispatch 表补三案（issue #56）。
- feat(github/gitea/forgejo): 新增可选能力接口 `TeamReviewManager`
  （`RequestTeamReviewers`，团队 review request 落 `team_reviewers` 独立 wire 通道）
  ——统一 ReviewManager 只收用户名，组织仓库按 CODEOWNERS 请求团队评审被迫绕开
  SDK；`CapabilitySet.TeamReviews` 登记 + DeriveCapabilities 派生 + 契约一致性
  断言 + 双后端 wire 测试（issue #79，按可选能力族惯例不 breaking）。

### Fixed（后端语义面 11 连修：wire 静默丢参 / 幂等破窗 / 跨后端口径错位）

- fix(github): `ListDeployments` 的 int 返回值从 `Response.LastPage`（页码，
  单页恒 0）改为本页条数——对齐 #51/#60 选定的 IssueManager total 口径并在
  接口文档写明（issue #89）。
- fix(gitee): `UpdateCR`/`UpdateRepo` 空串字段不再照发 wire——`*string`+omitempty
  只省略 nil 指针，`gitee.String("")` 非 nil ⇒ 单字段更新会清空对侧字段或
  422；改条件落线（非空才 `gitee.String`），与同后端 `UpdateIssue` 对齐（issue #88）。
- fix(gitcode): `ForkRepo` 不再整体吞掉 `ForkRepoOptions`——`opts.Name` 落
  fork body 的 name 字段（改名 fork 生效）；`opts.Organization` 无线路面登记
  DivergenceIgnore（issue #87）。
- fix(gitcode/gitee): `CreateDiscussion` 行内锚点落线——gitcode 切
  `CreatePullRequestInlineComment`（path/line/side/commit_id，reviews.go 同款
  先例），gitee 落 PR 评论 wire 的 path/position/commit_id；base/start SHA 对
  与多行区间无线路面，双侧登记 divergence（issue #86）。
- fix(gitcode/provider): webhook 事件幂等双修——`gitcodeHookEventFlags` 一
  flag 一名（note_events 只读回 `note`，不再 `["comment","note"]`）+
  `canonicalWebhookEvents` 排序后 `slices.Compact` 去重；含 note 事件的
  `EnsureWebhook` 不再每次 delete+recreate（issue #85）。
- fix(gitee/gitcode): 路径段转义纪律补全——gitee `RemoveIssueLabel` 标签名
  裸传修正；gitcode 全量 SDK 调用点（约 25 处 owner/repo，另含 branch/tag/
  sha/username/label 名与多段 file path 的逐段转义）过 `backendutil.Esc`
  （新增 `escPath` 助手，镜像 gitee methods.go），补 escape 同款回归测试
  （issue #84）。
- fix(gitlab): `MarkRepoNotificationsRead` 桩改带 `ErrNotImplemented` 哨兵
  （裸 `Wrapf` 使 `IsNotImplemented` 恒 false）并登记台账；DivergenceSuite
  dispatch 表补 `NotificationManager.MarkRepoNotificationsRead` 案（issue #83）。
- fix(gitlab): `ListCRCommits` 的 SHA 落全 SHA（`c.ID`）——原取 `ShortID`
  （7 位）与六家后端及本后端 commits.go 的 40 位口径自相矛盾，join/去重键
  漂移；补 40-hex 形状断言（issue #82）。
- fix(gitcode): `ListCRComments` 走 `ListIssueCommentsWithOptions +
  AllPages` 全页——原单发裸 helper 被 SDK 缺省 page=1/per_page=20 截断，
  CR 侧对齐 issue 侧「exhausts the platform's pagination」契约（issue #81）。
- fix(github): `ListRepos(Owner=…)` 在 `/orgs` 端点 404 时回退
  `ListByUser`——个人命名空间不再被误判「无仓库/不存在」，mapping 登记台账
  （issue #80）。

## [0.124.0] - 2026-11-10

### Added（契约套件三扩展：milestone Get / release List+Tags / BP Get+Update 全矩阵接线）

**spec 论证**（契约测试是 spec）：本版只**新增**断言、不修改任何既有 wire
shape 断言；唯一行为变更是三处历史违规实现的修正（见下），依据是
DivergenceSuite 的既有强制规则「登记为 DivergenceStub 的方法必须返回
ErrNotImplemented」——github/gitee 的 BP list/get 桩一直 `return nil,
nil` 违反该规则且未登记台账，本次补登记并修正返回形态。新发现的
gitee BP GetResponse 键形修正同理（gitlab wire 用 `name` 键，原夹具
`branch_name` 永远解出空名——旧套件只断言非 nil 故漏检，新 Get 步骤
断言 BranchName 后如实暴露）。

- feat(contracttest): MilestonesSuite 增 `Get_ParsesAndNormalizes`
  步骤（`GetResponse` 夹具字段，缺省跳过）；ReleaseSuite 增
  `List_Parses`（列表路由复用 ByTagResponse 形）与
  `ListTags_Parses`（`TagsListResponse` 夹具——六平台 tags wire 形
  状逐一对 SDK 源码核实：gitea 族/ github/gitee 用 `commit.sha`、
  gitlab/gongfeng 用 `commit.id`）；BranchProtectionsSuite 增
  `GetBranchProtection_Shape`（断言 BranchName 往返）与
  `UpdateBranchProtection_AcceptsMutation`。
- feat(contracttest): BP 套件增 `ListUnsupported`/`GetUnsupported`
  豁免旗标（机器可读的「平台无此端点」声明，镜像
  UpdateSendsDescriptionOnly 先例）——github 设 List、gitee 设
  List+Get。
- fix(backends): github `ListBranchProtections`、gitee
  `ListBranchProtections`/`GetBranchProtection` 从 `return nil, nil`
  修正为 `provider.Wrap(Platform, op, provider.ErrNotImplemented)`
  ——对齐 DivergenceSuite 既有强制与台账 DivergenceStub 语义
  （`errors.Is` 判定形态不变：原本就不承诺成功，现在如实报错）。
- chore(divergence): 三处 stub 登记进 github/gitee 台账并再生成
  `docs/divergence-ledger.md`（规则二：平台偏离机器可读登记）。
- 效果：giteafamily 共享实现零覆盖函数 41→36，GetMilestone/
  ListReleases/ListTags/GetBranchProtection/UpdateBranchProtection
  脱离零覆盖（71–87%）；全仓覆盖率 50.1%→50.8%。

## [0.123.0] - 2026-11-10

### Changed（oekit 三件套 patch/minor 线拉齐：pkg v0.41.1 / concurrency v0.31.2 / logx v0.9.1）

- **deps: pkg v0.41.0→v0.41.1、concurrency v0.31.0→v0.31.2、logx
  v0.8.12→v0.9.1**（根 go.mod 与 mcp go.mod 双升）。收益为被动
  bugfix 线：
  - pkg v0.41.1 七连修复（#317–#323：mathx 溢出/pagination 伪翻页
    误判/httpjson 304 空体当驱逐/procx/jsonrepair/urlguard）——两条
    与本仓面相邻的特别复核：pageiter 的 `prev = batch` 无快照拷贝
    无缓冲复用，与 #318 的内部快照复用坑不同类，**不受影响**；
    本仓不走 httpjson WithETag 的 Do 路径（只收 CanonicalETagKey
    纯函数），#319 **不受影响**。
  - concurrency v0.31.1 四连（#226–#229）——retry/httpx 的 #228
    （HTTPError 显式 0 秒 Retry-After 与 ParseRetryAfter 口径统一）
    经本仓 `ParseRetryAfter` 委托线被动受益。
  - logx v0.9.0 `ErrAttrs`（pkg/errors KVCarrier → slog.Attr 结构化
    出口）——经 provider/transport NewLogxLogger 桥对接 logx 的
    消费方被动受益；#136/#137 slog 序列化与 Error() panic 隔离。
- docs(AGENTS.md): v0.41.1/v0.31.1–2/v0.9.x 增量审计存档（含两条
  「相邻面不受影响」的复核结论）。

## [0.122.0] - 2026-11-10

### Changed（上游提案八连全量回流：三件套拉齐 v0.41.0/v0.31.0/v0.8.12 + 三处收编）

上游提案飞轮完整闭环——本仓提报的 pkg#285/#286/#288/#289/#290/#291 与
concurrency#208 全部当日/隔日修复落地，本版随依赖升级全量收编：

- **deps: oekit 三件套拉齐**——pkg v0.38.0→**v0.41.0**、concurrency
  v0.29.0→**v0.31.0**、logx v0.8.11→**v0.8.12**（根 go.mod 与 mcp
  go.mod 双升，mcp replace ../ 需人工跟随已做）。
- **feat(concurrency#208 收编): `transport.StripSensitiveQueryParams`
  剥离机制委托 `httpx.StripSensitiveQueryParamsWith`**——raw-query
  外科手术（不动无关参数的字节，免 re-encode 翻转转义形态）；词表保留
  本仓 11 项（上游缺省 3 项的超集：api_key/jwt/session 等平台特化项），
  跨 host 重定向 token-in-query 泄漏面的既有行为零变化。
- **feat(pkg#290 收编): `transport/etag.go` 缓存键构造委托
  `httpjson.CanonicalETagKey`**——\n 分段无歧义键；vary 集合按本仓
  平台面固定（Accept/Authorization/PRIVATE-TOKEN），跨凭据缓存串味
  防线从手写纪律变为上游构造期保证；键字节形态变化零行为影响
  （内存缓存进程内私有）。
- **feat(pkg#291 收编): `transport/metrics.ClassifyPath` 委托
  `pathx.ClassifyPath`**——手写 ID 段折叠启发式删除（同一发源：数字
  段+hex≥7+词段保留）；上游把「带符号数字不折叠」的哨兵值取舍一并
  收进缺省面（优于本仓 Atoi 形态，无测试钉负数段、零行为回归）；
  uuid 扩展需求走 `pathx.PathClassifier` 可配置面。
- **pkg#285/#286 收编路径裁定（pageiter 保留不迁移）**：上游
  WalkEach/Walk 现已带伪翻页无进展检测（`page>1 && DeepEqual(prev)`
  与 pageiter 逐字同形）+ fetch ctx 贯穿——语义对齐复核完毕；pageiter
  的 ErrStopIteration 哨兵、budget 错误形态与 ctx PageFetch 签名是
  自有公共面，迁移即 breaking，维持「同形不迁移，上游 bugfix 被动
  受益」。
- **procx 维持不引入**（runtime#25/#26 已闭，环境白名单顾虑消解，
  但依赖白名单硬规则未变且 gitbackend 的 GIT_CONFIG_COUNT 注入是
  git 原生机制，无消费痛感）——评估结论存档 AGENTS.md。
- docs(AGENTS.md): pkg v0.39–v0.41 / concurrency v0.30–31 / logx
  v0.8.12 增量审计存档。

## [0.121.0] - 2026-11-08

### Changed（pkg/projection + githubapp 小收编轮：SecureString 驻留混淆 + UseNumber 委托）

- **feat(security): `githubapp.InstallationTokenSource` 驻留 token 迁
  `secretx.SecureString`**（pkg v0.35 引入的内存态敏感凭证持有原语，
  上游文档点名「git provider token」即此消费形态）——XOR 混淆驻留 +
  替换零化，防堆扫描拿到连续明文；明文仍在每次请求物化进
  Authorization 头（HTTP 协议必需），硬化的对象是最长 1 小时的驻留
  副本。nil 守卫保持导出字段直填的零值形态可用；`PrivateKeyPEM` 为
  配置传入的公有字段不动（改类型即 breaking）。新增行为级测试：
  Get 独立副本往返、过期刷新替换生效、零值字面量可用。
- **refactor: `pkg/projection` 手写 decoder+UseNumber 委托
  `encoding.UnmarshalUseNumber`**——防 >2^53 精度丢失语义不变
  （json.Number 保留原文字面量）；上游「尾残报错」严于手写
  Decode，但 raw 来自 json.Marshal 无尾残形态，零行为差异。
- docs(AGENTS.md): pkg v0.36–v0.38 增量审计存档——pack/encoding/
  jsonrepair 定性无消费点；ctxscope/cache.SingleValue/poll.RunCursor/
  async.Spawn/Handle/httpjson.NormalizeBaseURL 逐项复核结论（含
  NormalizeBaseURL 与自有 DefaultBaseURL 不同域的收编否决理由）。

## [0.120.0] - 2026-11-08

### Changed（oekit 三件套拉齐 v0.38.0/v0.29.0/v0.8.11 + 验签面全面下沉 oekit/webhook）

- **refactor: `provider/webhook.go` 验签核心迁 `oekit/webhook` 子包**
  （pkg#270 落地收编）——HMAC 计算/hex 解码/常量时间比较/多模消歧
  全部下沉上游维护；本文件保留平台 header 约定、secret 逐调用的
  多租户形态（oekit 验签器为构造期绑密钥，适配层按调用构造）与
  `ErrWebhookValidation` 哨兵链（wrapValidation 统一收编，下游
  errors.Is 契约不变）。五验证器逐字映射：
  - HMACSHA256/StaticToken/HmacValidator（任意算法+前缀）→ oekit
    同名原语（空密钥构造期 panic 由适配层先行守卫，保持本仓错误形态）；
  - Forgejo 双 header 回退 → oekit 构造器内置 + **双形态容忍补
    `WithSignaturePrefix`**（契约夹具签前缀形、真 wire 发裸 hex，
    两者都收——旧验证器的既有语义超集）；
  - GitCode 三模/Gitee 签名+密码双模消歧 → oekit 构造器内置
    （Fallback「签名在场即裁决、错签硬失败」与旧实现同哲学）；
    Gitee 无时间戳分支保留本仓的 body 签名探测优先消歧；容忍窗
    映射：0=本仓缺省 10min（显式传，oekit 缺省 1h 不同）、负值=
    饱和关闭。**294 行既有验签测试零改动通过**（行为等价实证）。
- **`httpx.ParseRetryAfter` 收编**（concurrency#198 落地）：
  transport `parseRetryAfter` 手写双形态解析删除，委托原语（签名
  同形；交付版带 OWS 修剪改进）。HTTP-date 形态回归测试零改动通过。
- **`httpx.StripSensitiveHeadersWith` 收编**（concurrency#200 落地）：
  头部剥离委托原语（宿主判定一并下沉，prev/current 同判据）；
  词表经 With 变体保留本仓多出的 X-Access-Token/X-Gitlab-Token
  平台形态。
- **`timex.ParseFirstInLocation` 收编**（pkg#271 落地）：Gitee
  due_on 无时区 layout 按 Gitee 服务端时区（CST/UTC+8）解析——
  关闭按 UTC 解析偏 8 小时的误报窗口（pkg#271 的下游实证场景）。
- deps: pkg v0.37.1 → **v0.38.0**、concurrency v0.28.1 → **v0.29.0**、
  logx v0.8.6 → **v0.8.11**。

## [0.119.0] - 2026-11-08
### Fixed（webhook/鉴权/分页九连——审计 issue #40-#48 清零）

- **[P0] Gitee 分发键对齐真实 wire 值（#41）**：`X-Gitee-Event` 实际是
  "Push Hook" 形态（带空格、首字母大写），此前 switch 只认小写无空格键
  （测试语料自造形态——语料驱动漂移的样本），push/PR/tag/note 四族真实
  投递全部落空返回 `Type=""` + nil err。分发前归一（大小写不敏感 + trim
  兜底变体），历史小写键保留兼容；语料五文件改真实 wire 值 + 真实词表
  contracttest 钉住。
- **EnsureWebhook 词表归一收敛（#40）**：GitLab/TencentCode 写侧接受
  pull_request/cr，读侧 convertHook 读回 merge_request——归一缺失时
  同族判漂移，每次 ensure 都 delete+recreate（钩子 ID 翻动、删建窗口丢
  投递、重建失败即无钩子裸奔）。比对两侧统一过
  `canonicalWebhookEvents` 归一表（未登记名透传）。
- **GitHub parse 路径空 secret 收口（#43）**：go-github ValidatePayload
  对空 secret 不设防（`HMAC(key="")` 可自算）且缺 SHA-256 头回落接受
  SHA-1；parse 路径改走平台注册表单源（与 ValidateWebhookSignature 同
  策略），空 secret/空串自签/错签三例拒绝测试钉住。
- **403 节流优先归类 ErrRateLimited（#42）**：GitHub/Gitea 把最常见节流
  信号发成 403 + X-RateLimit-Remaining:0，transport 已判限流但 provider.Wrap
  只按裸状态码分类丢掉该判定。新增 `rateLimitedMarker` 接口探针，裸 403
  仍保持 ErrForbidden（授权失败不放大负载）。
- **GetCRDiff/GetCRFiles 全页行走（#44）**：GitLab diffs 端点 per_page
  服务端缺省 20（nil options 只取一页）、Gitea/Forgejo 固定 Page:1
  PageSize:50——>20/>50 个变更文件的 CR 评审覆盖面无声缩水。两处走
  backendutil.AllPages 预算模式（GitLab 显式 per_page=100 上限），两页
  夹具回归测试钉住。
- **pipeline 噪声过滤显式化（#45）**：GitLab success/running 态此前
  `return nil, nil`——只判 err 的调用方在 ev 上空指针。新增
  `provider.ErrWebhookFiltered` 哨兵，噪声态 (nil, 哨兵) 可 errors.Is 判定。
- **InstallationTokenSource stale-if-error（#46）**：margin 窗口内刷新
  失败不再丢弃仍有效的缓存 token（此前窗口内每次 API 调用都报错）；
  真实过期后无 stale 可用才报错。
- **tag_push 全零 after 归一 tag.deleted（#47）**：GitLab 家族删除事件
  与 GitHub/Gitea 对齐（after 全零 + before 非零；全零+全零异常形态不硬
  归；删除事件不携带删除前 SHA）；新增 `WebhookDeletionSHA` 常量单点
  判定，corpus golden 重生成（仅 +action 一行）。
- **TencentCode 投递 ID 头 + RawDiff（#48）**：envelope 补
  IDHeaders（X-Gitlab-Event-UUID/X-Request-ID，重投不再合成新 ID）；
  GetCRDiff 补 RawDiff（其余六家都有，工蜂此前静默空串）。

## [0.118.0] - 2026-11-08

### Changed（oekit 全量拉齐 v0.37.1/v0.28.1/v0.8.6 + 下游收编第二轮）

- **deps: oekit 三件套全量拉齐**——pkg v0.35.1 → **v0.37.1**、concurrency
  v0.23.4 → **v0.28.1**、logx v0.8.2 → **v0.8.6**。上游 v0.36–v0.37 期间
  本仓所提六 issue（#197/#198/#199/#210/#211/#212）全部落地，下游收编：
  - **`timex.ParseFirst` 收编**（#197）：`parseGitCodeDueOn`/
    `parseGiteeDueOn` 两处手写多 layout 循环删除，委托原语（语义逐字
    同构：first-match + (time.Time, bool)）；
  - **`bufiox.TailLines` 收编**（#198）：`backendutil.TailOfReader` 委托
    原语（签名同形），删 30 行手写行扫描；交付版带 CRLF 归一（较手写
    版改进）；四后端 CI 调用点零改动；
  - **收编否决（语义依据）**：`pagination.WalkEach`（#211）停止规则为
    长度单调递减+空页，缺 `EachBounded` 的**伪翻页检测**（Forgejo
    15.0.1 现场实证：同页逐值重复 = 列表已尾，优雅返回已收集数据）——
    AllPages 委托 WalkEach 会把现场硬化行为从优雅降级变报错丢数据，
    `provider/pageiter` 保留；`secretx.NewPassphraseBox`（#212）KDF 为
    PBKDF2，与本仓 v2 密文的 argon2id 派生不同线——存量密文 wire 兼容
    优先，收编留给下一代密文版本位；`Slice.RemoveIf`（#199）暂无消费
    点（本仓无注销 API）；`ctxscope`/`cache.SingleValue` 无消费点。
- **refactor: janitor 轮次纪律迁 `loop.Run`**（concurrency v0.28 新原语
  `IntervalAfterCompletion`，#140）——消灭「panic 后静默停摆」失败
  形态：旧 GoSafeLabeled+ticker 下 cleanup panic 后循环死亡、清理永久
  停摆直到手动 start；loop.Run 单轮 panic 收编、janitor 存活。间隔从
  「本轮结束」起算（长清扫不重叠不双发）；FirstDelay=interval 保持旧
  首扫时机；生命周期状态机（重启/双停/竞测）契约不变，全部既有测试
  零改动通过。

## [0.117.0] - 2026-11-07

### Fixed（gitbackend：Branches 含 refspec 条目的透传兼容）

- **`Fetch` 的 `Branches` 条目含 `:` 时原样透传为 refspec**（native 与
  gogit 同规则）：`:` 是 git refspec 的 src:dst 分隔符，调用方手写完整
  refspec（如 `pull/1/head:refs/heads/pr-1`）的形态在 v0.116.0 收紧
  Branches 语义时被误当分支名展开成 `refs/heads/<refspec>` 而炸掉旧调用方
  （code-review-service 的 PR head 抓取实测命中）。现分支名展开只适用
  纯分支名，refspec 透传恢复 v0.116 前行为，两后端不分叉。

## [0.116.0] - 2026-11-07

### Fixed（gitbackend：native Fetch 分支语义对齐 gogit）

- **native `Fetch` 传 `Branches` 时展开为显式 refspec**
  （[go-git-platform#37](https://github.com/yi-nology/go-git-platform/issues/37)，
  review-service 生产评审空跑根因）：此前裸 `git fetch origin <branch>` 有
  两个叠加问题——工作区 clone 恰好检出同名分支时 git 直接拒绝
  （`refusing to fetch into branch ... checked out`）；即便不撞检出冲突，
  无 refspec 的分支 fetch 只写 FETCH_HEAD 不更新 remote-tracking ref，
  after 快照与 `FetchResult` 分支更新检测全部失真。现镜像 gogit 后端
  `buildFetchRefSpecs` 语义（`+refs/heads/<b>:refs/remotes/<remote>/<b>`，
  SHA 过滤、`refs/` 前缀、全 SHA 回退通配），两后端行为不再分叉。
  调用方拿到的可见结果（refs/remotes 下的新值）恰是本该发生而此前没发生的。

## [0.115.0] - 2026-11-06

### Added（CI 失败诊断第四家：GitCode 接入）

- **`CILogManager` 扩到 GitCode**——go-gitcode v0.15.0 落地了下游反馈
  （[go-gitcode#2](https://github.com/yi-nology/go-gitcode/issues/2)，
  2026-11-05 关闭）：`FindActionRunByHeadSHA`（wire 无 head_sha 过滤，
  SDK 按最新序客户端扫描）+ `ListActionRunJobs`/`DownloadActionJobLogs`
  （日志线路）。GitCode 成为 CI 失败诊断第四家（GitHub/GitLab/Gitea/
  GitCode）。mapping 分歧登记两条语义差异：
  - SDK 只返回单条最新命中 run——同 SHA 其他 workflow 的 run 不归因
    （GitHub/Gitea 是每 workflow 取最新）；
  - job status 词表为大写（SDK 夹具实证 COMPLETED/RUNNING），失败判定
    大小写不敏感并放宽到 failure/error/timeout 家族（词表未知时宁可多
    归因），live 词表实证后收紧。

### Changed（deps）

- **deps: go-gitcode v0.14.0 → v0.15.0**——issue#2 修复线（见上）。
- **deps: go-gongfeng v0.2.1 → v0.3.0**——
  [go-gongfeng#1](https://github.com/yi-nology/go-gongfeng/issues/1)
  （2026-11-05 关闭）反馈闭环：
  - **MR 多指派确认不可表达**（平台 wire 单 `assignee_id`，SDK 已在
    字段注释标注与 GitLab `assignee_ids` 不同源）——下游 first-wins
    mapping 分歧维持不变；
  - **`GetReleaseByTag` 收编进 SDK**（Paginate 扫全量匹配，带
    `ErrReleaseNotFound` 哨兵）。下游**刻意保留本地有界扫描**（50 页
    预算；SDK 版无界）——预算保护是已登记行为，切换会丢；detour 本身
    的分歧台账条目不变。

### Docs

- README 能力矩阵 CILogs 行 GitCode 列 ✅（脚注⁵）、中文要点、结构注释、
  mcp README 工具集表与工具描述、iface_ci.go 文档——六处同步 GitCode。

## [0.114.0] - 2026-11-06
### Added

- **`EnsureWebhook` 的 `EnsureSecretMode`（issue #34）**：平台 webhook secret
  write-only（List/Create 响应都不返回），URL+事件集 matched 的 hook 无法
  比对 secret——宿主侧轮换后 ensure 报 unchanged 并保留平台旧 secret，宿主
  用新 secret 验签 fail-closed 全拒、事件静默丢失。新增
  `CreateWebhookOptions.EnsureSecretMode`（仅 EnsureWebhook 消费，
  CreateWebhook 忽略）：缺省 `EnsureSecretKeep` 保持现状；
  `EnsureSecretAlwaysUpdate` 在 matched 且 opts.Secret 非空时经
  `WebhookConfigManager.UpdateWebhook` 原地补写 secret（保 hook 平台 ID，
  一次 API 往返，报 EnsureUpdated）——argus bootstrap 的「ensure 后无条件
  UpdateWebhook」workaround 收编进 SDK。manager 未实现
  WebhookConfigManager 时显式报错而非静默保留旧 secret；空 secret 无可
  收敛，跳过补写报 unchanged。五组单测（原地补写/缺省保持/空 secret
  跳过/裸 manager 报错 + 既有回归）。
- **`IssueManager.DeleteIssueComment`（issue #33）**：删除评论收进能力接口。
  签名 `(ctx, owner, repo, number, commentID int64) error`，与
  UpdateIssueComment 同一地址空间与 number 语义：comment 直址平台
  （GitHub、Gitea、Forgejo、GitCode、Gitee）忽略 number，GitLab 与工蜂经
  issue 路由必须携带 issue number。各后端底层删除 API 均现成（工蜂 SDK 无
  该端点，沿用 DeleteNote 的 NewRequest/Do raw 模式）；Gitea/Forgejo 经
  giteafamily 新增 `DeleteIssueComment` 引擎 helper（走既有 DiffPort 的
  DeleteIssueComment 端口）。doc 注明平台 authorship 门禁与「删除永久，
  要保留历史请用 UpdateIssueComment」。contracttest 的 assertIssueComments
  补删除断言（DELETE 动词 + issue 路由平台复检 notes 路径形状）。

## [0.113.0] - 2026-11-05

### Changed（简化收尾：退化封装清理）

- **`signBody` 合并到 `signBodyHash`**：`signBody`（SHA256 专用）是
  `signBodyHash("sha256")` 的退化形式，唯一调用点 `HMACSHA256Validator`
  直接用通用形式；删掉 `signBody` 减少一层间接。
- 同轮全面扫描确认代码库高成熟度：`IDCache` 已全量委托 oekit
  `pkg/cache.LocalCache`、`labelresolve` 分层合理（无缓存版 + 缓存版）、
  giteafamily 引擎无重复、gitbackend 双实现（native/gogit）实现方式根本
  不同不重复、webhook 5 validator 各平台签名格式不同不合并、transport
  42 函数无退化封装。

## [0.112.0] - 2026-11-05

### Changed（契约套件补齐：RepoTemplates 行为锁定）

- **RepoTemplates 契约套件**（`contracttest/repo_templates.go`）：
  `CreateRepoFromTemplate` 返回 `*provider.PlatformRepo` 且 Name 非空——
  GitHub/Gitea 双平台接线。此前只有 `assertCap` 声明断言，无行为锁定。
  契约套件覆盖对照 CapabilitySet 21 字段：15 项有行为套件、6 项单平台
  能力（Gists/Starred/Migrations/ReleaseAssets/Deployments/ForkSync）
  由后端线级测试覆盖——单平台能力不需要跨平台契约。

## [0.111.0] - 2026-11-05

### Changed（重复助手提取 + CILogManager 契约套件补齐）

- **`esc()` 三处重复助手提取到 `backendutil.Esc`**：gitcode/gitee/
  tencentcode 各有一个一行别名包装 `url.PathEscape`——统一替换为已有
  的 `backendutil.Esc`，删掉三份副本（gitee 20+ 文件、gitcode 5+ 文件、
  tencentcode 3+ 文件的调用点同步更新）。
- **CILogManager 行为契约套件**：`contracttest/cilogs.go` 补齐 CI 失败
  诊断的跨平台契约——绿提交返回空切片（非 error）、失败提交返回至少
  一条 JobLog 且 Name/Stage 非空；GitHub/GitLab/Gitea 三家接线配置；
  Gitea contract_test 版本串 1.22.0→1.26.0（Actions 端点版本门槛）。
  此前 CILogManager 只有声明断言（`assertCap`），无行为锁定——声明了
  能力但实现语义漂移不会被契约套件抓住。

## [0.110.0] - 2026-11-05

### Changed（oekit container 满配：detect.go 最后一个手写 mutex 收编）

- **oekit/pkg v0.33.0 → v0.34.1**——issue#178 的
  `container.Slice.AppendIfAbsent`（原子「查重+追加」，对应
  `SafeMap.GetOrSet` 的有序版承诺）落地后，`provider/detect.go` 的
  hostAliases 别名表从手写 RWMutex+slice 迁移到 `container.Slice`：
  - `RegisterHostAlias` 用 `AppendIfAbsent(entry, EqualFold-eq)` 做原子
    查重+追加，重复注册的 fail-fast panic 契约保留；
  - `classifyHost` 用 `Range`（容器快照语义，读侧不持锁）做 first-match
    子域分类——有序性（注册顺序敏感）由 Slice 保持，正是当初保留手写
    实现的两条理由都被容器原语覆盖。
- **provider 包从此零 `sync` 导入**：全仓注册表形态（SafeMap 五处 +
  Slice 一处）容器化收官。
- 迁移顺带修正测试契约：容器是 `*Slice` 指针类型，原 `hostAliases = nil`
  的测试注入会致 nil 指针解引用——改 `Clear()`（零值/清空语义由容器
  承接）。
- AGENTS.md 存档更新（container 例外项转为已收编）、README 生态描述
  补 `container.Slice`（有序注册表）。
- **顺带修复另一个调度敏感的重试测试**：`TestRetry_RateLimit403
  RetryAfterZeroRetriesImmediately` 的 50ms 上界把网络往返算进了「Retry-
  After: 0 不消费指数底延时」的语义——CI runner 高负载（race 检测器）会
  破。改用 `elapsed >= BaseDelay` 的相对判定：消费了 100ms 底延时必红
  （语义保持），真实调度开销天然满足（schedule 无关）；`-race ×200`
  压测验证。

## [0.109.0] - 2026-11-05

### Changed（重复维护面简化 + 上游反馈闭环）

- **mcp 工具集清单单一事实源化**：`knownToolsets` 从 `toolsets` 注册表
  init 派生（此前手工复本已随 ci/repo-extras 落地漂移过一次）；
  `NewServer` 的 unknown-toolset 报错清单改由 `ToolsetNames()` 派生
  （此前每次加工具集都要手工同步报错文案）。注册表此后是唯一手工点。
- **巡检示例声明打印改有序切片**：map 迭代无序导致每次运行输出顺序
  随机（对 diff 不友好）；CapabilitySet 字段展示顺序现在稳定。
- **上游反馈四连**（本轮迭代的实证问题回流）：
  [oekit/pkg#178](https://github.com/oekit/pkg/issues/178)
  （container.Slice 原子查重追加原语——detect.go 迁移阻断点）、
  [go-gitcode#2](https://github.com/yi-nology/go-gitcode/issues/2)
  （Actions 缺 head_sha 过滤与 job logs 线路——CI 能力接入阻断点）、
  [go-gongfeng#1](https://github.com/yi-nology/go-gongfeng/issues/1)
  （MR 单 assignee_id 与 release 无 by-tag 端点）、
  [go-gitee#5](https://github.com/next-bin/go-gitee/issues/5)
  （issue assignee 逗号 join 语义确认请求）。

## [0.108.0] - 2026-11-04

### Changed（oekit 纪律收口：fmt.Errorf 存量容忍区取消）

- **无 cause 格式化错误全量迁移 `oekiterrors.Errorf`**：主模块 50 处 +
  mcp 9 处（2026-10-21 审计的 59 处存量容忍区在此取消——「能用尽用」指令
  下的机械迁移，行为兼容：Errorf 与 fmt.Errorf 同型，nil-safe 为增益）。
  迁移后 `fmt.Errorf` 在 SDK/mcp 源码中仅存于 `provider/errors.go` 例外区
  （3 处双 `%w` 永久例外 + 同文件内 3 处错误机内部实现）。规则一同步更新：
  此后 SDK/mcp 源码出现 `fmt.Errorf` 即违规。
- 同轮复核确认全仓注册表形态的 mutex+map 已清零（githubapp token 缓存 /
  encrypt material / ratelimit / janitor / ttlcache baseline 均为单键状态
  字段，不在容器范畴）。

## [0.107.0] - 2026-11-04

### Changed（breaking：`JobLog.Truncat` 重命名）

- **`JobLog.Truncat` → `JobLog.Truncated`**（公共 API 拼写修正，Go 字段名
  变更；**wire 不变**——json tag 一直是 `truncated`）。迁移：改字段引用即
  可，序列化形态无变化。
- mcp `get_failed_ci_logs` 工具改为直接返回 `provider.JobLog`（此前逐字段
  复制到私有 out 类型——无语义差异的重复面）。
- GitHub/Gitea 两份相同的 CI 失败判定词表提取为
  `backendutil.IsFailedCIConclusion`（failure/timed_out/startup_failure 计
  失败；canceled/skipped 不归因）。
- mcp 写侧词表校验的 `fmt.Errorf` 迁移 `oekiterrors.Errorf`（规则一：新
  代码格式化一律 Errorf；`tools_status.go` 存量同形顺带迁移）。
- capabilities 巡检示例补 `shortSHA` 守卫（COMMIT_SHA 短于 7 字符时此前
  会切片越界）。

## [0.106.0] - 2026-11-04

### Added（部署/模板/Fork 同步三能力收官）

- **`DeploymentManager`（GitHub）**：`CreateDeployment` / `ListDeployments`
  （可按 environment 过滤）/ `CreateDeploymentStatus`（写侧词表在工具与
  后端双层守卫：pending|in_progress|queued|success|failure|error|canceled|
  inactive）。GitLab 的 deployment 记录是 CI 产物且无手工状态线——不声明
  （absence-by-not-declaring），不硬凑。
- **`TemplateRepoManager`（GitHub / Gitea）**：`CreateRepoFromTemplate`
  ——GitHub `POST /repos/{o}/{r}/generate`、Gitea 同路径 generate 线；
  **Gitea 要求显式目标 owner**（generate 端点无认证用户缺省，空值以统一
  422 拒绝并在能力文档标注）。
- **`ForkSyncManager`（GitHub）**：`SyncFork` 走 merge-upstream 端点——
  fork 分支从上游同名分支快进/合并；冲突不代解，报错留给手工。
- `CapabilitySet` 增 `Deployments` / `RepoTemplates` / `ForkSync` 三字段
  （非 breaking），经 `DeriveCapabilities` 统一推导 + 契约套件双向锁定。
- **mcp 新增 `repo-extras` 工具集**：`create_deployment`(写)
  `list_deployments` `create_deployment_status`(写)
  `create_repo_from_template`(写) `sync_fork`(写)，逐能力门控挂载。
- 契约面：GitHub/Gitea 线级测试（四路由 body 断言 + generate 要求 owner
  的守卫）；巡检示例补 Deployments/RepoTemplates/ForkSync 声明与探针；
  README 矩阵三行、mcp README 工具表同步。

## [0.105.0] - 2026-11-04

### Added（CI 失败诊断扩面：GitLab 独占 → GitHub/GitLab/Gitea 三家）

- **`CILogManager`（CI 失败 job 日志尾部，agent 归因的核心原语）扩到
  GitHub 与 Gitea**——三家线路：
  - **GitLab**：原生 pipeline trace（既有实现，零改动）；
  - **GitHub**：runs-by-SHA（`ListRepositoryWorkflowRuns` 的 head_sha
    过滤）→ 失败 job（failure/timed_out/startup_failure 计失败，
    canceled/skipped 不归因）→ presigned job log 拉取尾部；
  - **Gitea**：`ListRepoRuns` 的 head_sha 过滤 → 失败 job → job log
    reader 拉尾部。
- **`CapabilitySet` 增 `CILogs` 字段**（结构体加字段=非 breaking）——此前
  CILogManager 是唯一不在机器可读声明里的可选能力（type-assert-only），
  现经 `DeriveCapabilities` 统一推导，契约套件双向锁定。
- **结构重构**：GitLab 的 trace 尾部截断提取为
  `backendutil.TailOfReader`+`CILogTailBytes`（20KB），三后端共享。
- **mapping 分歧登记 ×2**（GitHub/Gitea）：一个 SHA 可扇出多个 workflow
  run——按 workflow 取最新 run 收集失败 job（而非单一 pipeline 语义）。
- **mcp 新增 `ci` 工具集**：`get_failed_ci_logs`（gated CILogManager，
  自动随平台挂载）。
- 契约面：capabilities 套件补 `CILogs` 双向断言；GitHub/Gitea 线级测试
  锁 workflow 去重（旧 run 的 jobs 路由绝不被查询）与日志尾部拉取；
  README 矩阵/要点/巡检探针同步。

## [0.104.0] - 2026-11-04

### Added（CR 指派人写面）

- **`UpdateCROptions` 增 `Assignees []string`**（结构体加字段=非 breaking）：
  全量替换 CR 指派人（logins），空 = 不变（与 `UpdateIssueOptions.Assignees`
  同款 len>0 口径，清空不可表达）。七平台接线形态与分歧登记：
  - **GitHub**：PR 编号即 issue 编号，走 `Issues.Update` assignees 线
    （PR edit wire 不带该字段），替换语义完整；
  - **GitLab**：logins 经 Users API 解析为 `assignee_ids`（复用
    `resolveUserIDs` 模式）；
  - **Gitea/Forgejo**：家族 PR edit body 增 `Assignees`（两成员 SDK 均原生
    支持）；
  - **Gitee**：PATCH pull 无 assignee 字段——读改写收敛差集到 add/remove
    端点（mapping 分歧：并发编辑有丢更新窗口）；
  - **工蜂**：MR update 线路收单个 `assignee_id`——多派取第一个（mapping
    分歧）；
  - **GitCode**：pull-update wire 无字段且其 `/pulls/{n}/assignees` 路由是
    评审人语义——accept-and-drop（ignore 分歧）。
- **顺带修正**：Gitee issue 指派人的逗号 join 存量行为此前**未登记**分歧
  （违反台账纪律），本轮补 `CreateIssue`/`UpdateIssue` mapping 登记。
- **mcp 新增 `update_cr` 工具**（写）——编辑 title/description/target
  branch/assignees，随各后端分歧形态走。
- 契约面：giteafamily 引擎透传断言 + GitHub/Gitee 线级测试（issue 线
  pre-step、RMW 差集收敛）；分歧台账校验套件守住登记完整性。

## [0.103.2] - 2026-11-04

### Fixed

- **provider: 修 `TestTTLCache_ConcurrentMissCountsEachLookup` 调度竞态**
  （v0.103.1 CI macos 腿红的原因，测试自身缺陷非产品回归）——builder 原本
  只 sleep 10ms，CI runner 高负载时 goroutine 启动延迟可超过 10ms，晚到的
  查找命中已回填的缓存（实证 7 miss + 1 hit）。改为「到齐门闩 + 沉降窗口」：
  回源函数等全部 8 个调用方完成「计数→进入查找」后才回填（计数后、查找前
  的纯用户态间隙由 5ms 沉降覆盖——纯门闩在 race 检测器下压测仍会露出该
  间隙，实证 6 miss + 2 hit；2s 超时兜底防挂死）；`-race ×300` 压测验证。
  锁定的契约不变：并发未命中每次查找各计一次 Miss。

## [0.103.1] - 2026-11-04

### Security

- **deps: golang.org/x/net v0.60.0 → v0.62.0**——关掉两个 module 级漏洞
  GO-2026-6866 / GO-2026-6860（x/net html 渲染的命名空间遍历与文本节点合并
  平方复杂度；本仓代码不直接消费 html 渲染路径，属 require 面卫生）。零
  API 适配面。
- **deps: golang.org/x/crypto v0.58.0 → v0.58.1-0.20261009181718-2f1f834330b2**
  （公告钉的 master 修复提交，tag 未发）——关掉 GO-2026-5932（openpgp 包
  失修告警；本仓不 import openpgp，同为 require 面卫生）。升级后主模块
  与 mcp 模块 govulncheck 均「No vulnerabilities found」。

## [0.103.0] - 2026-11-04

### Changed（deps：git 平台 SDK 全量升级）

- **deps: oekit 三件套**——pkg v0.28.0 → v0.33.0、concurrency v0.22.0 →
  v0.23.4、logx v0.7.1 → v0.8.2。零 API 面变化，收益为被动 bugfix：pkg/cache
  六连修 + `HashKey` 长度前缀无歧义编码（经 provider/ttlcache.go 全量委托
  传导）、logx `WithAttrs` 脱敏修复（经 `NewLogxLogger` 桥传导）。oekit
  新原语增量审计已存档 [AGENTS.md](AGENTS.md)（零新增引入）。
- **deps: go-github v92.0.0 → v93.0.0**（major 线跃迁）——上游选项结构改
  指针字段并改名 `AddCollaboratorRequest`（原 `RepositoryAddCollaboratorOptions`），
  backends/github 已全部适配；provider 层 wire shape 与公共契约不变。
- **deps: gitlab client-go v3.40.0 → v3.43.0**、**gitea.dev/sdk v1.4.0 →
  v1.5.1**——零适配面（backends/gitlab、backends/gitea 零文件改动）。
- 其余 git 相关依赖经查已在上游最新：go-git v5.19.3、forgejo-sdk v3.0.0、
  go-gitcode v0.14.0、go-gongfeng v0.2.1、go-gitee（master 分支 2026-05-21
  后无新提交）、golang.org/x/*。
- mcp 模块 go.mod 同步 tidy 跟随（Dependabot 只更根 go.mod）。

### Changed（refactor：注册表面全面接入 oekit `container.SafeMap`）

- 五处手写 mutex+map 注册表迁 `oekit/pkg/container.SafeMap`：provider 平台
  注册表、`WebhookValidatorRegistry`、`statusExtractors`、
  `pkg/credential` SSH hostKeys（`GetOrSet` 顺带把 TOFU 双检锁并成一步）、
  `gitbackend` 后端注册表。公共契约不变（`Register` 仍查重 panic、
  `WebhookValidatorRegistry.Register` 仍覆盖语义）。
- 例外存档：`provider/detect.go` hostAliases 保持有序 slice——子域
  first-match 语义依赖注册顺序，且「查重+追加」原子性 `container.Slice`
  无对应原语；判定依据见 [AGENTS.md](AGENTS.md)。

### Migration notes（github 平台）

- 直接构造 `github.Repository` 传给 create 的下游：改用
  `CreateRepositoryRequest`；`CreateRepoOptions.DefaultBranch` 在 GitHub 上
  此前即被 API 忽略（v92 载荷发出即被忽略），v0.103.0 起不再发出——需要改
  默认分支用 `UpdateRepo`。

## [0.102.0] - 2026-11-04

### Changed（breaking：gitcode 平台能力面再收缩）

- **deps: go-gitcode v0.13.0 → v0.14.0**（SDK live 探针第二轮：kanban 面
  整面重路由到真 wire `/org/{org}/kanban/*`；分支保护面收缩）。

### Fixed（live 实证驱动的 gitcode 能力收缩·续）

live 探针第二轮（body 分类法：GitCode 业务错误 body = 路由存在）证实：
分支保护真 wire 只有 `PUT/DELETE /branches/{name}/setting`（要求已有保
护；`/protect-branches` 有文档页但 GET/POST live 404，`/setting/new` 与
`/wildcard/setting` 死——保护管理是 web-UI 管的，API 无创建无列表）。
gitcode 后端的 `BranchProtections` 能力依赖 list+create+delete（converge
流），不可用即摘除：

- **BranchProtectionManager 摘除**（branch_protection.go 整文件 +
  convertBranchProtectionRule）——`Capabilities().BranchProtections` 不
  再为真，mcp `protection` 工具集对 gitcode 自动隐藏；
- 契约夹具同步摘除（被摘能力的 mock 路由在真实服务不存在，保留夹具等
  于为死路由维护绿色测试——同 v0.101.0 先例论证）；
- 顺带修正：SDK 的 git/blobs 上轮误用 commit SHA 探测，本轮以真实 blob
  SHA 复探 200（compare/raw/contents/search/discuss 亦全部 live 200），
  文档化对照面（commits/issues/pulls/labels/milestones/releases/hooks/
  collaborators/branches）全部 live 200。

## [0.101.1] - 2026-11-04

### Fixed

- **gitlab: `CreateWebhook`/`UpdateWebhook` 补 note（评论）事件族**（#32，
  by zy84338719）——事件映射此前只有 push/merge-request/tag-push 三族，
  Events 里的 "note"/"comment"/"issue_comment" 被静默丢弃，`EnsureWebhook`
  建出的钩子永远收不到评论事件（argus 的 @bot 评论面在 GitLab 上无法经
  SDK 完成 webhook 自动化）。现 Create 显式置 `NoteEvents`（含别名
  comment/issue_comment），Update 将 note 加入「非 nil Events 显式置全
  部派生位」集合——note 不在集合中时发显式 false 表达移除，与 push/MR/
  tag-push 族的既有语义对齐；`convertHook` 反向映射本就带 note，读回一
  致。两个线测钉住：create 别名映射与显式 false、update 表达/移除 note。

## [0.101.0] - 2026-11-03

### Changed（breaking：gitcode 平台能力面收缩）

- **deps: go-gitcode v0.8.0 → v0.13.0**（SDK 四轮 wire 纠偏：v0.9 删虚构
  org hooks、v0.10 Actions 重写 v8、v0.11 Enterprise 重写 v8、v0.12/13
  sitemap 全量 diff 纠偏 + live 探针回滚拼写）。

### Fixed（live 实证驱动的 gitcode 能力收缩）

以有效 token 对真实服务只读探针（真实仓库/真实 commit SHA/真实 issue，
文档化对照端点同轮 200 证实方法有效，证据存档于 gitcode_api
docs/wire-provenance.md）后，移除 gitcode 后端对死路由的能力宣称：

- **CommitStatusManager 摘除**（`CreateCommitStatus`/`ListCommitStatuses`
  方法删除）——`/repos/{o}/{r}/statuses/{sha}` 404；`Capabilities()
  .CommitStatuses` 不再为真，mcp `status` 工具集对 gitcode 自动隐藏
  （工具注册按能力断言门控，README 门控语义不变）；
- **ReactionManager 摘除**（reactions.go 整文件）——issue/评论/PR
  reactions 路由全 404（连文档形的 user-reactions 也 404）；
- **DeployKeyManager 摘除**（deploy_keys.go 整文件）——`/keys` 404；
- **GetArchive 改 ErrNotImplemented**——archive 三种路径形式全 404；
  ReleaseManager 是整包接口（releases 族全部存活），方法保留但显式
  stub，`provider.IsNotImplemented` 可判；
- **NotificationManager 全局半边改 ErrNotImplemented**——全局
  /notifications 与 threads 路由 404，仓库域（文档化 + live 200）保留；
  接口是整包设计，全局三方法显式 stub。

契约套件同步（AGENTS.md 规则二，提交内含论证）：NotificationsHarness
新增 `GlobalUnsupported` 旋钮——置位时全局三方法断言
`IsNotImplemented`、仓库域断言保持绿（整包接口的"半边能力"从此有了
机器可读表达）；gitcode harness 摘除 CommitStatus/Reactions/DeployKeys
夹具、Notifications 夹具置位旋钮并修正 id 类型（int64）。新增三个线测
钉死：GetArchive ErrNotImplemented、CommitStatus 能力不宣称、
Reactions/DeployKeys 不再实现。

divergence 台账新增 4 条 DivergenceStub（GetArchive + 全局通知三方法），`docs/divergence-ledger.md` 再生成。mcp 模块 tidy 跟随。

### Migration notes（gitcode 平台）

- 依赖 `Capabilities().CommitStatuses/Reactions/...` 做发现的调用方：
  gitcode 现在返回 false（此前宣称但线上必 404——虚假可用性比诚实的
  不可用更危险）；
- 直接类型断言 `provider.CommitStatusManager`/`ReactionManager` 的调用
  方：对 gitcode 断言会失败，属预期；
- `GetArchive` 与全局通知三方法：改判 `provider.IsNotImplemented(err)`。

## [0.100.2] - 2026-11-02

### Fixed

- **transport: `ETagCache` 304 与 LRU 驱逐竞态不再硬失败**（#31）——
  有界缓存（默认 128 条）在并发翻页下，验证器可能在
  `applyConditional` 与 304 到达之间被驱逐，旧实现直接报错
  `conditional request ... answered 304 but the cached entry is gone`，
  未配重试的 Client 把纯本地记账竞态当真实 API 错误抛给调用方。现
  RoundTrip 检测哨兵后**进程内自愈**：剥掉 If-None-Match 无条件重发
  一次——重发经过限流器与响应钩子（时长并入本次交换），200 回填缓存
  使后续条件请求恢复工作；不依赖调用方配置重试，协议违例的裸 304
  原样上抛（无循环）。
- 新增导出哨兵 `transport.ErrETagEntryGone`：自愈前的 304-miss 条件
  现在可被自定义 RoundTripper/观察方 `errors.Is` 识别（stdlib Is 链，
  经 oekiterrors.Errorf %w 包装）。

## [0.100.1] - 2026-11-02

### Fixed

- **giteafamily: `MergeCR` 把非 2xx 当成功**（#30，by @zy84338719）——
  go-gitea 的 `MergePullRequest` 走 `getStatusCode`，非 2xx 返回
  `(status, nil)`；空 diff PR 的 405（"Please try again later"）此前
  会漏过 `err != nil` 判断、落到末尾 `return GetCR(...)`，调用方拿到
  open 状态的 CR 且无错误——上层状态机谎报 MERGED。现补非 2xx 兜底转
  错误，插在 405 恢复分支之前：已合并 PR 的 405 幂等恢复语义不变；
  附 3 个单测钉住（非 2xx 必败 / 405 已合并可恢复 / 2xx 成功）。
  合并后规范跟进：错误构造按规则一改走 `oekiterrors.Errorf`。

## [0.100.0] - 2026-11-02

0.x 序列的第 100 个 minor。本版以依赖线收口为主，无 API 变化；主仓自
0.99.12 起无功能增量，随版收入两个排队中的 dependabot 升级（PR CI 均
全绿：ubuntu+macos 测试 / lint / vuln / mcp 模块）：

### Changed

- **deps: gitlab client-go v3.18.0 → v3.40.0**（#28）——跨 22 个 minor
  的常规升级，覆盖 3.x 线全部累计修复；
- **deps: gitea.dev/sdk v1.3.0 → v1.4.0**（#29）。

## [0.99.12] - 2026-11-02

### Changed

- **deps: go-gitcode v0.7.2 → v0.8.0**（SDK 对照 docs.gitcode.com 的 wire
  契约修正，gitcode backend 随之回归 SDK 主路）：
  - **webhook 密钥真修**——wire 密钥字段是 `password`（SDK 旧 `Secret` 发的
    `secret` 键服务端不认，密钥从未生效）；Create/Update 改走 `Password`；
  - **webhook 事件布尔化**——GitCode hook wire 没有 `events` 数组，订阅是
    `push_events`/`tag_push_events`/`issues_events`/`note_events`/
    `merge_requests_events` 五族布尔（gitee 同构）。backend 以
    `gitcodeHookEventFlags` 映射中性事件名，非 nil Events 显式置全族
    旗标（true 成员/false 其余）表达"清空"；Get/Update/List 回读旗标映射
    回中性名。**撤销该能力的 divergence 登记**——空事件集此前不可表达；
  - **milestone**——SDK create/update 的 `due_on` 改 omitempty（未设值不再
    以 `""` 上 wire 清空日期），响应结构对齐文档（`number` 寻址 + date-only
    `due_on`，新增 `Number`/`RepositoryID`/`URL`/issue 计数；`DueDate`
    → `DueOn string`）。backend 撤除 milestone create/update 的 raw 绕行，
    `convertMilestone` 以 id 优先/number 兜底解析寻址值、双格式解析
    due_on（RFC3339 与 date-only）；
  - **分页**——issue 评论（+order/since）、仓库 label、hook 列表三端点
    文档化 page/per_page 落地，backend 撤除对应 raw 绕行并让
    LabelManager.ListLabels 走双模分页（此前台账误记"不分页"）；
  - **raw 面清零**——`Provider.rawClient` 与 `doJSON` 助手整体移除，
    gitcode backend 不再有绕过 SDK 的路径；owner/repo/label-name 路径段
    统一 `esc()` 转义（SDK 逐字插值，`#`/`?`/`%`/空格会截断 URL）；
  - 契约夹具同步真 wire：milestone `due_on` date-only（旧 `due_date`
    键从未在文档 wire 上存在）、webhook GetResponse 改旗标形态、labels
    撤 `IgnoresListPagination` 豁免（提交内含旧形状为何是错的的论证）；
  - divergence 台账 9 项 → 3 项（label 无 description ×2、issue 列表
    无 sort/direction），`docs/divergence-ledger.md` 再生成。

## [0.99.11] - 2026-11-01

### Added

- **tencentcode: `TencentCodeExtras` 补齐工蜂工作流面**（SDK 已有、后端未暴露
  的最后一批中价值端点）：
  - **订阅族 ×6**——issue/MR 的 watch 状态查/订/退订（`GetIssueSubscription`/
    `SubscribeIssue`/`UnsubscribeIssue`、`GetMRSubscription`/`SubscribeMR`/
    `UnsubscribeMR`）。wire 词汇：issue 读取返回裸 bool，订阅/退订是独立 PUT
    路径（`/issues/{id}/subscribe` 与 `/unsubscribe`）；MR 读取/订阅共用
    `/merge_request/{id}/subscribe`（退订为独立 `/unsubscribe` PUT），三个 MR
    变更都回显 `{"subscribed":b}`，方法签名 accordingly 返回 bool；
  - **跨项目全局 issue 列表** `ListUserIssues`（工蜂全局 `/issues` 端点，
    `ListUserIssuesOptions` 支持 State/Labels/Page/PerPage；provider 的
    open 词表映射工蜂 `opened`，复用 `convertIssue`）——项目域的
    `provider.IssueManager.ListIssues` 没有跨项目模式；
  - **里程碑关联 issue 列表** `ListMilestoneIssues`（caller 驱动单页；里程碑
    线型是无里程碑回引的瘦身 Issue、单一 assignee，`convertMilestoneIssue`
    映射）；
  - **归档下载 ×2**——`DownloadMergeRequestChangedFiles`（
    `merge_request/{id}/changed_files`）与 `DownloadCommitReviewChangedFiles`
    （`review/{id}/changed_files`），流式写调用方 writer；
  - wire 测试 ×3 钉请求形状（订阅路径动词表、全局列表 query、归档流回读）；
- **mcp: 新增 `tencentcode` 工具集**（仅工蜂挂载，注册期断言
  `tencentcode.TencentCodeExtras`）——工蜂原生工作流面向 agent 开放：原生
  代码评审五件套（list/get/create/submit/reopen，submit 的 verdict 词表
  approve|comment|require_change|deny 在工具边界校验）、跨项目
  `list_user_issues`、`list_milestone_issues`、issue/MR 订阅四件套
  （get/set ×2；set 合并订/退订为 `subscribed` 布尔参数）；只读模式丢全部
  写工具，其余平台永远看不到该工具集；mcp README 工具面表同步。

### Changed

- **deps: go-gongfeng v0.2.0 → v0.2.1**——重试耗尽哨兵接线（`Do` 预算耗尽
  现按契约返回 "retries exhausted" 包装，cause 链保留、错误分类不受影响）
  + examples/quickstart + 选项/错误面补测；本仓无消费点变化（后端不消费
  SDK 重试），纯依赖跟进。

## [0.99.10] - 2026-10-31

### Fixed

- **provider: 伪翻页无进展检测**（`EachBounded`/`Each`/`Collect*` 单一迭代器）——
  与上一页逐值相同的页视为服务器未推进分页（实测 Forgejo 15.0.1 评论列表忽略
  `page` 参数、每页返回同一批全量数据），按"已到列表末尾"软收手并**返回已收集
  数据**；此前同页重复会一路翻到 `ErrPageBudgetExceeded` 报错、数据整体丢弃
  （调用方如 argus 的报告扫描被逼进"新建评论"降级，总结评论碎片化）。首页即
  全量（这正是服务器无法分页的原因），截断风险仅存在于"忽略 page 但遵守
  limit"的服务器形态，此时首页数据也优于整体报错。`ErrPageBudgetExceeded`
  语义收窄为真超长列表越界；`backendutil.AllPages` 随单一迭代器同步受益，
  回归用例改钉软收手语义（首页数据 + 两次 fetch 判停）。

## [0.99.9] - 2026-10-30

### Added

- **`provider.WebhookConfigManager`（跨平台可选能力，7 平台全实现）**——单个
  webhook 的读取与更新（`GetWebhook`/`UpdateWebhook`），补齐核心
  `WebhookManager`（create/delete/list 全平台本就有）缺的"查配置、改事件
  订阅"面：
  - `UpdateWebhookOptions` patch 语义（nil 字段不动），唯一例外是非 nil
    `Events` 切片**全量替换**事件集（空切片=清空）——GitLab/Gitee/工蜂的
    事件订阅是布尔标志位（省略=不变），增量表达不存在，故契约定义为全量、
    实现侧显式置位（含 false）来落地"移除事件"；GitHub/GitCode 的 SDK 对
    空 `Events` 带 `omitempty`（空切片上不了 wire，清空不可表达），登记
    `DivergenceIgnore`；
  - 平台词汇映射：secret 按平台落 key（GitHub `config.secret`/GitLab
    `token`/Gitee `password`/工蜂 `token`/Gitea 家族 `config.secret`/
    GitCode `secret`），事件派生 scope 与各家 create 完全一致（工蜂/
    GitLab：push/MR/tag 三族）；secret 在所有平台均为只写，`GetWebhook`
    永不回显；
  - Gitea 家族经 `giteafamily` 引擎统一实现（`HookPort` 扩展
    `GetRepoHook`/`EditRepoHook`）：两家 SDK 的 edit 端点都不回读 hook，
    引擎 PATCH 后补一次 GET 水合，两平台观察语义一致；`GetRepoHook` 按家族
    惯例回带 HTTP status 供 `IsNotFound` 分类（family 错误类型拍平状态码）；
  - 契约套件新增 `WebhookConfigSuite`（get/update/404 分类三例，mock 按
    "寻址到单个 hook 资源"路由，PATCH/PUT 动词漂移也会被抓住），`Harness`
    新增 `WebhookConfig` 配置项，7 后端全部接线；GitHub/GitLab/Gitee/
    GitCode/工蜂/Gitea 另各有 wire 级单测钉请求体形状（config 只在
    URL/Secret 变更时挂载、标志位显式 false、password/token key 等）；
  - `CapabilitySet` 新增 `WebhookConfig`，`contracttest` 双向漂移检查与
    `provider` 派生同步扩展；README 能力矩阵补行。
- **mcp: `webhooks` 工具集**——`list_webhooks`/`create_webhook`(写)/
  `delete_webhook`(写) 走核心 `WebhookManager`（必有工具集），
  `get_webhook`/`update_webhook`(写) 走 `WebhookConfigManager` 能力门控
  （注册期断言，平台不支持则根本不挂载）；`update_webhook` 是 patch 语义
  （至少给一个字段），README 工具面表同步。

## [0.99.8] - 2026-10-30

### Changed

- **tencentcode: 工蜂 SDK 抽离为独立组件
  [go-gongfeng v0.1.0](https://github.com/yi-nology/go-gongfeng)，本仓改为
  依赖 `github.com/yi-nology/go-gongfeng`**（工蜂逻辑单独成库，架构对齐
  go-gitcode 等姊妹 SDK 模式；本发布周期内曾短暂以收编目录承载，同周期内
  被抽离取代）：
  - go-gongfeng 承载工蜂 SDK 全量源码、单元测试与官方 API 文档镜像
    （22 个能力域），lineage：studyzy v0.6.0（上游已删 tag）→
    zy84338719/gongfeng-sdk-go 文档对齐修正 → 本组件（oekit 适配层）；
  - SDK 的日志注入改为其**自有最小 kv 风格 `Logger` 接口**（4 方法，默认
    noop，与 `oekit/logx` 及本仓 `provider.Logger` 结构兼容，注入零适配），
    打断 SDK → 本仓的依赖方向（SDK 是最底层，不得反向依赖 adapter）；
  - 后端 17 个文件 import 切换 + `go get @v0.1.0`，收编目录删除；对
    0.99.7 的净依赖变化：`studyzy/gongfeng-sdk-go v0.6.0` →
    `yi-nology/go-gongfeng v0.1.0`；
  - **wire 修正随组件对本仓生效**（fork 相对 v0.6.0 的"对齐官方文档"批次，
    均有真实环境集成测试背书）：
    - **Release 按 ID 寻址**（原 v0.6.0 按 tag 寻址是对 wire 的错误认知）：
      工蜂 release 端点只认数值 ID 且无按 tag 取单条的原生面，
      `GetReleaseByTag`/`UpdateRelease`/`DeleteRelease` 改为先按 tag 扫
      release 列表解析 ID（登记 detour，查不到报 `provider.ErrNotFound`）；
      release 载荷为 `id`/`tag`/`title`/`description`（无 GitHub 式
      `tag_name`/`name`），契约套件的工蜂 fixture 据此修正，
      `CreateRelease` 现映射 `Title`，`Target`/`Draft`/`Prerelease` 登记
      忽略。
    - **路径修正**：commit status 列表/查询去掉 `repository/` 前缀
      （`commits/{sha}/statuses`）、MR commits 与 changed files 走
      `merge_request/{id}/…`（原 v0.6.0 路径与官方文档不符）。
    - **移除官方 API 不存在的端点**（`SearchProjects`/`DeleteIssue`，本仓
      无调用点）、webhook 增加 `review_events`/`token` 字段、分支保护/MR
      创建 options 补齐官方字段（labels、necessary_reviewers、
      approver_rule 等）、时间解析兼容 `+0000` 偏移格式。
- **工蜂 SDK 的 oekit 适配层**（随组件迁入 go-gongfeng，偏离已在其 README
  登记，升级上游时需重新套用）：
  - 错误面走 oekit/pkg errors：带格式构造（含 `%w`）转 `oekiterrors.Errorf`
    （setBaseURL / parseID / Time.UnmarshalJSON），静态错误转 `oekiterrors.New`；
  - 日志注入：`Client` 增加可选日志出口（SDK 自有最小 kv 风格 `Logger`
    接口，默认 noop，与 `provider.Logger`/logx 结构兼容），经 `WithLogger`
    注入；`Do` 记录请求 debug 行（method/path/status/耗时）与失败 error 行；
    tencentcode 后端默认把 `Config.Logger` 接进来，oekit 生态消费方经
    `provider.NewLogxLogger` 观测 SDK 请求流；
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

- **GitLab: `CollaboratorManager` 落地（能力矩阵补上最后一个 GitLab 空格）**——
  项目成员 API 承载协作者增删查（与 TencentCode 同构：成员按数值 user ID
  寻址，username 经 Users API 精确过滤解析 + TTL 缓存）；read/write/admin →
  access level 映射登记 mapping（GitLab 加成员**无平台默认**、缺
  access_level 直接 400，空权限映射为 developer）。`iface_collaborators.go`
  平台支持注释同步更新；契约套件挂 Collaborators fixture（GitLab 的用户
  过滤走 /users 的 query 参数，成员 fixture 兼作用户载荷）。`examples/
  capabilities` 巡检程序为通用实现，自动覆盖新能力。主 README 矩阵同步。
- **mcp: `--toolsets` 旗标帮助文本改为从注册表动态生成**（新增
  `mcpserver.ToolsetNames()`）——toolset 名单不再可能与注册表漂移，
  protection/collaborators 随之进入 `--help`。
- **tencentcode: 批量助手端到端 wire 测试**——`provider.GetFileContents` /
  `GetCRs` / `SetCommitStatuses`（oekit/concurrency 有界并发引擎）在工蜂
  wire 上钉住：输入序保持、单条失败不扩散为批量失败、缺失文件按
  per-item Err 呈现、状态批量按 2 次 POST 落线。

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
