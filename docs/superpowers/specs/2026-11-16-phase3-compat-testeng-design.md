# 第 3 期：兼容性与测试工程化设计（2026-11-16）

来源：路线图 [2026-11-12-review-roadmap-design.md](2026-11-12-review-roadmap-design.md) §第 3 期三条
（七平台环境矩阵+只读巡检 / 沙箱写 E2E+异常注入 / 关键域覆盖率 ≥80%）。设计呈现后
用户以「写 全都弄」批准全部四个裁定并授权 spec→计划→SDD 全链执行：

1. **不刷已达标域**——gitbackend/pageiter/transport 实测已 ≥80%，只保线；
2. **巡检不加 cron**——保持 `workflow_dispatch`（路线图原文），日常化由用户自行 schedule；
3. **E2E = push/PR 门禁**（path-filter）+ `make e2e` 本地双场合；
4. **异常注入归 contracttest**（httptest 面，不进 docker）——docker 沙箱只做真写 roundtrip。

## 1. 现状与基线（实测，设计的事实基础）

### 1.1 四域覆盖率（coverage.out @ v0.141.0，`go tool cover -func` 聚合）

| 域 | 现值 | 目标 | 结论 |
|---|---|---|---|
| webhook 验签（`runtime/webhook*.go` + 验签族，39 函数） | **55.6%** | ≥80% | **唯一真缺口，本期主战场** |
| transport（retry/etag/client，67 函数） | 81.2% | ≥80% | 贴线保线，零覆盖分支顺手补 |
| gitbackend（245 函数） | 83.8% | ≥80% | 已达标，不动 |
| provider 分页器（`runtime/pageiter.go`，4 函数） | 98.8% | ≥80% | 已达标，不动 |

手法沿用仓内既定方法论：**先出零覆盖函数清单（清单才是真缺口），逐函数补表驱动
单测**——不是刷行数。webhook 域的 wire 层已有 contracttest webhookcorpus 覆盖，
缺口在 unit 分支层：validator 注册表、envelope 归一化（`Normalize*Action`）、
HMAC 双形态（sha256= 前缀/裸 hex）、`ReadAndRestoreBody` 读写恢复、
`ValidateWebhookWithRegistry` 的 miss/多注册/unknown-event 分支。

### 1.2 巡检现状（`.github/workflows/smoke.yml` + `smoke/`）

- 已有形态：`workflow_dispatch`（platform 七选一 + owner/repo/url inputs），
  严格只读（GetRepo + 一页 ListRepos），env-gated——缺 secret 则 **SKIP 不红**。
- secrets 命名已定但**仓库当前零配置**（`gh secret list` 空）：
  `GIT_PLATFORM_SMOKE_TOKEN_<GITHUB|GITLAB|GITEA|FORGEJO|GITEE|GITCODE|TENCENTCODE>`
  + fallback `SMOKE_TOKEN`。放 token 前所有巡检 = no-op skip（现状即如此）。
- 本期扩面 = probe 家族从 2 个 → 6 个（§3.2），workflow 升 matrix；**不加 cron**（裁定②）。

### 1.3 E2E/异常注入现状

- 仓内**零 docker 基建**（无 Dockerfile/compose）；CI 三件套 ci.yml/release.yml/smoke.yml。
- 异常注入三形态（故障响应体/截断 JSON/伪翻页）目前在 contracttest **零覆盖**；
  transport 单测有部分 5xx/重试面但无「带 body 的故障体」「截断 JSON 解析」「
  伪翻页 Link 头」的显式契约钉。
- SDK 版本（钉沙箱镜像用，**计划期现场核对**）：`gitea.dev/sdk v1.5.1`、
  `codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3 v3.0.0`——镜像大版本须与 SDK
  编译面对齐（Gitea/Forgejo 同代 API，取两者交集的稳定 tag，先实测再钉）。

## 2. 目标 / 非目标

**目标**

- 四域覆盖率机器断言 ≥80% 进 `make check`（webhook 55.6→80+ 是主工作量）；
- 只读巡检 6 probe 家族 × 7 平台矩阵，一次 dispatch 可全跑；缺 secrets 沿用 skip 约定；
- gitea/forgejo 沙箱真写 roundtrip：本地 `make e2e` + CI push/PR 门禁（path-filter）双场合；
- 异常注入三形态成为 contracttest 契约（faultsuite）；
- 全部为 additive 测试面：零 API 变化、零生产代码语义变化。

**非目标（红线）**

- 不改任何 `provider`/`model`/`runtime`/`adapter` 公共 API 与 wire shape（E2E 只消费）；
- 不加 cron/定时（裁定②）、不碰 release.yml、不引入新 Go 依赖（docker 是 CI 基建非 go.mod 依赖）；
- 不在 E2E 里做破坏性/不可逆操作（沙箱容器内随便写，但**不指向真实平台写**——写操作
  只发生在一次性容器里）；真实平台永远只读（巡检）；
- 不刷已达标域覆盖率（裁定①）、不为 YAGNI 加防回退测试（cover 脚本本身即防回退）。

## 3. 设计

### 3.1 A：四域覆盖率先行

- `scripts/cover-domain.sh`：解析 `go tool cover -func=coverage.out`（或
  `go test -coverprofile` 现跑），按 §1.1 四个正则聚合，任一域 <80% 则非零退出并
  打印域名/现值/缺口；挂进 `Makefile` 的 `check`（`vet lint test cover-domains`）。
- webhook 域打法：零覆盖清单 → 表驱动补测（新文件 `runtime/webhook_*_test.go`
  与既有测试同包），逐清单项打勾；目标不是 100% 而是域聚合 ≥80%，优先补
  **分支密集**函数（归一化/注册表/HMAC 校验），跳过纯转发壳（报告注明即可）。
- transport 贴线：只补清单里 0% 的函数（如有），不追行。
- 验收：`make check` 含 cover-domains 且绿；`go tool cover -func` 四域快照进报告。

### 3.2 B：只读巡检扩面（smoke → matrix patrol）

**probe 家族（6，全部只读）**：

| family | 断言面 |
|---|---|
| `repo`（现有） | GetRepo + ListRepos 一页解析 |
| `cr` | GetCR/ListCRs：状态词表、分页字段、SourceBranch 过滤口径（#78 契约的真平台面） |
| `webhook` | ListWebhooks 解析 +（fixture 侧）ValidateSignature 已验签头——**不创建不删除** |
| `pagination` | 同一 endpoint 连续 2 页 walk：页参数生效、第二页可空、无死循环（真平台的伪翻页探测） |
| `errors` | 不存在资源 → `ErrNotFound`、无权/私有 → 401/403 分类哨兵（错误分类矩阵的真平台复核） |
| `connection` | TestConnection + TokenSource 轮转冒烟 |

**实现形态**：

- `smoke/` 包内加 `probes.go`：`map[string]ProbeFunc` 注册表（family → func
  `(ctx, provider.Provider, opts) error`），env `GIT_PLATFORM_SMOKE_FAMILIES`
  （逗号分隔，缺省 `repo` 保持现状；`all` = 六个）。**沿用 env-gated：缺 secret →
  SKIP；probe 内部任何写方法一律禁止（lint 级：probe 代码不 import 写接口）。**
- `smoke.yml` 升级：`inputs.families`（default `all`）+ `strategy.matrix.platform`
  七平台——一次 dispatch 跑全矩阵（每 matrix 单元仍走该平台 secret，缺则 skip）；
  保留单平台快跑（platform 输入非 all 时收窄 matrix）。
  **勘误（T2 评审）**：workflow `families` input 缺省取 `repo` 保持既有
  dispatcher 零行为变化，非本节上行的 `all`——env 侧缺省同为 `repo`
  （`smoke.ParseFamilies` 空串→`["repo"]`）。
- **secrets**：spec 附清单（§5），workflow 注释同步「如何加」。放 token 前全 skip。
- 验收：本地 `GIT_PLATFORM_SMOKE_*=… go test -tags=smoke ./smoke/` 六 family 在
  至少 github 真 token 下手跑一次（**token 由用户提供；未提供则以 httptest 假
  provider 跑 family 单测证明接线**，真平台手跑留给用户）；workflow 语法
  `actionlint`（若无则 yaml parse + 人审）。

### 3.3 C2：异常注入 faultsuite（contracttest，不进 docker）

新 `contracttest/fault.go` + 挂进 `Run`（`FaultSuite`，三子测，自带 httptest——
既有模式；capability 无关故无 drift 开关，七后端全跑）：

1. **故障响应体**：5xx + 各平台真实错误 body 形态（JSON 错误对象/纯文本/空体）→
   断言 `ErrServerSide` 链成立且 **body 不进错误文本**（防泄漏/防文本匹配依赖）；
2. **截断 JSON**：200 + 被腰斩的 JSON（半截数组/半截对象/非法 UTF-8 尾部）→
   断言错误可判（非 panic、非静默空集——**静默空集是最危险形态**，逐 manager 断言
   返回 err 非 nil 或显式空约定，以现场实现为准记录 ruling）；
3. **伪翻页**：`Link: <…page=2>; rel="next"` 指向忽略 page 参数的 stub（第二页
   返回与第一页相同内容）→ 断言 pageiter 的无进展检测在 N 页内止损（#285 语义的
   wire 级钉），预算耗尽返回错误而非丢数据。

验收：三子测七后端绿；与 `Pagination_Normalized`/`ErrorClassSuite` 无断言冲突。

### 3.4 C1：沙箱写 E2E（gitea/forgejo docker）

- `e2e/` 新包，文件头 `//go:build e2e`——`go test ./...`（make check）**不含**，
  `make e2e` 与 CI 显式 `-tags=e2e` 才跑。
- **harness**（`e2e/harness_test.go`）：
  - 本地：`make e2e` 用 `docker run` 起 `gitea`/`forgejo` 官方镜像（版本计划期
    按 SDK 实测钉，写进 Makefile 变量），health 轮询 `/api/v1/version` 就绪，
    初始化：建 admin token、建测试仓库，跑完 `docker rm -f`（t.Cleanup 兜底）。
  - CI（`e2e.yml`）：`on.push/pull_request` path-filter
    （`adapter/**` `transport/**` `contracttest/**` `e2e/**` `Makefile` `.github/workflows/e2e.yml`），
    用 Actions **`services:`** 声明 gitea/forgejo 镜像（同版本变量），service
    健康 check 步骤后跑 `go test -tags=e2e ./e2e/...`。**纯 docs/CHANGELOG push 不触发。**
  - 两场合共用同一测试代码；差异只在容器编排（本地 docker run vs services），
    以 env `E2E_GITEA_URL`/`E2E_FORGEJO_URL` 注入端点（测试内自起容器的形态
    仅本地用——CI 用 services 时 URL 由 env 给）。
- **写族 roundtrip（每平台）**：Issue(create→get→update→close/reopen→delete)、
  Label(add→list→remove)、CR(create→get→update title→close)、Ensure 族
  （EnsureIssueState/EnsureIssueLabels 收敛二段：Unchanged 零写 + Updated 写一次）、
  BranchProtection(create→get→delete，gitea/forgejo 支持面内)。每个 roundtrip
  断言 wire 结果与 `model` 实体回读一致；**只在一次性容器内写**。
- 失败诊断：t.Log 容器日志（`docker logs` 尾部）入失败输出。
- 验收：`make e2e` 本地两平台绿一次（开发机有 docker 时；无 docker 则 CI 验证并
  报告）；`e2e.yml` 首次 push 触发绿；`go test ./...` 默认面仍不含 e2e。

## 4. 分期与任务预览（SDD）

| 序 | 内容 | 主要文件 | 风险 |
|---|---|---|---|
| T1 | A：cover-domain.sh 进 make check + webhook 域补测至 ≥80% | `scripts/cover-domain.sh`、`Makefile`、`runtime/webhook_*_test.go` | 低（纯仓内） |
| T2 | B：smoke probes 注册表 + workflow matrix | `smoke/probes.go`、`.github/workflows/smoke.yml`、smoke 测试 | 低（skip 约定兜底） |
| T3 | C2：faultsuite 三形态 | `contracttest/fault.go` + `Run` 挂接 | 中（截断 JSON 语义需现场 ruling） |
| T4 | C1：e2e 包 + harness + `make e2e` + `e2e.yml` | `e2e/**`、`Makefile`、`.github/workflows/e2e.yml` | 高（镜像版本/就绪时序/CI services） |
| T5 | 收口：全量门 + 批次评审 + 发版暂停点 | CHANGELOG Unreleased、验证 | — |

顺序不可倒置的理由：T1 纯仓内先把机器门立起来；T2 依赖现有 smoke 形态最小扰动；
T3 不依赖 docker 可先行；T4 最重建放最后（前三个 task 的 contracttest/coverage
教训可复用）。每 task 独立绿门 + SDD 评审；发版 = 普通 minor（v0.142.0 候选，
`ls-remote` 实查），无 breaking、无 MIGRATION 变更。

## 5. secrets 清单（用户提供，spec 存档）

| secret 名 | 用途 |
|---|---|
| `GIT_PLATFORM_SMOKE_TOKEN_GITHUB` … `_GITLAB/_GITEA/_FORGEJO/_GITEE/_GITCODE/_TENCENTCODE` | 巡检对应平台（7 个） |
| `SMOKE_TOKEN` | 任一平台的 fallback |

放好后：`smoke.yml` dispatch → platform+`families=all` 即跑六家族矩阵；未放 = skip。
**E2E 不需要 secrets**（本地/CI 一次性容器自建 token）。

## 6. 验收标准

1. `make check` 含 cover-domains 且四域 ≥80%（webhook 实测从 55.6 起步）；
2. `FaultSuite` 三子测 × 七后端绿，挂进 `Run` 且 coverage mirror 登记；
3. `smoke.yml` matrix 语法有效、六 family 单测绿（httptest 假 provider 面）；
   真平台手跑至少 1 平台 1 次（用户 token 就位后，或报告注明待跑）；
4. `make e2e` + `e2e.yml` 双场合至少一边实跑绿（本地有 docker 则本地，否则 CI），
   默认 `go test ./...` 不含 e2e；
5. 全量收尾门同仓规：make check / check-mcp / 双 lint / 覆盖率地板 / 双 govulncheck；
6. 零 API 变化（`git diff` 不触 provider/model/runtime/adapter 生产文件——T1 只加
   测试文件与 Makefile/scripts）。

## 7. 明确不做

不加 cron；不碰 release.yml；不进真实平台写；不刷已达标域；不为 YAGNI 加
防回退测试；不引入 go.mod 新依赖；不改公共 API/wire；不做多平台 docker 沙箱
（只 gitea/forgejo——路线图原文；其余五平台的写面由 contracttest httptest 承担）。
