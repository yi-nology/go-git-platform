# 第 4 期：v1.0 准备与生态交付设计（2026-11-17）

来源：路线图 [2026-11-12-review-roadmap-design.md](2026-11-12-review-roadmap-design.md) §第 4 期
（冻结 v1 公共 API 清单+正式兼容矩阵 / MCP 二进制发布+SBOM+provenance / 模块版本策略
文档化）。设计呈现后用户以「你来弄吧」批准全部四裁定并授权 spec→计划→SDD 全链执行：

1. **金样面冻结**——机器守卫比对提交在库的导出符号清单，不写文档-only 冻结；
2. **手滚 workflow 步骤，不引 goreleaser**——两模块+发版时 require 抬版的非标形态
   手滚更直，且与现有 Release workflow 风格一致；
3. **provenance = `actions/attest-build-provenance`**（GitHub 原生 artifact attestation）；
4. **mcp 不发独立 tag**——沿用 179 个 v* tag 无 `mcp/` 前缀的既定实践。

**本期完成 ≠ 打 v1.0 tag**——第 4 期交付的是「v1.0 就绪态」（冻结机制+发布物+文档）；
实际 v1.0 官宣由用户择时执行（v1.0 打 tag 时金样守卫即生效边界）。

## 1. 现状（实测，设计的事实基础）

- 导出面规模（`go doc` 计数）：provider 133 / model 54 / runtime 53 —— 三契约包
  合计 240 导出符号；另有 adapter 七平台、contracttest、auth/{githubapp,credential}、
  filter/branchfilter 的公共面。
- `adapter/all/readme_matrix_test.go`：平台×能力矩阵**已机器对账** README——
  兼容矩阵的真源已存在，本期只做 v1 化汇总文档。
- `docs/MIGRATION.md` 296 行（重构迁移矩阵）；CONTRIBUTING 有「Versioning &
  compatibility commitment」节（0.x breaking=minor 承载先例 v0.73.0）。
- 零 goreleaser 配置；mcp 单二进制入口 `mcp/cmd/go-git-platform-mcp/main.go`；
  tag 形态：179 个 `v0.*`、**零 `mcp/` tag**；mcp/go.mod require 人工跟随父 tag
  （发版后 S8 抬版——重构周期已实证的既定套路）。
- Release workflow（release.yml）：`v*` tag 触发 → 测试 → `gh release create
  --generate-notes`。**纯 docs 提交不打 tag 直接 push main**（仓规）。

## 2. 目标 / 非目标

**目标**

- v1 冻结机制机器化：`internal/apisurface` 金样守卫进 `go test ./...`；
- 正式兼容矩阵 `docs/COMPATIBILITY.md`（真源=既有机器测试，人读汇总）；
- Release 产出 mcp 多平台二进制 + SBOM + checksum + provenance，挂同一 Release；
- `docs/VERSIONING.md` 模块版本策略落地（消费方可见）；
- 全部为 additive：零生产代码语义变化（新增 internal 守卫包 + workflow 段 + 文档）。

**非目标（红线）**

- **不打 v1.0 tag**（就绪 ≠ 官宣——用户择时）；不做 v1 语义化承诺变更（CONTRIBUTING
  既有节为准，本期只落 VERSIONING 文档不改承诺）；
- 不引 goreleaser（裁定②）、不发 mcp 独立 tag（裁定④）、不改 `go.mod`/`go.sum`
  依赖面（syft/attest 是 CI 工具非 go 依赖）；
- 金样不含未导出符号与文档注释（裁定①）；不动 release.yml 的测试段（只追加构建段）。

## 3. 设计

### 3.1 A：`internal/apisurface` 金样守卫

- 测试 `internal/apisurface/apisurface_test.go`：
  - 用 `golang.org/x/tools/go/packages`？——**不引新依赖**：用 `go doc` 子进程输出
    逐包解析（`go doc -all github.com/yi-nology/go-git-platform/<pkg>` 逐行抽
    `^(func|type|const|var)` 签名行），或 `go list -json` + `go/parser` 读导出
    标识（build 包面）。**取 `go list -json`+parser 形态**（零子进程脆弱性、零新
    依赖：stdlib `go/parser`+`go/types`? 最稳的是 `go doc -all` 文本协议——
    现场两形态各自试、选稳者，报告记录裁定）。
  - 包清单（冻结面）：`provider model runtime adapter/{github,gitlab,gitea,
    forgejo,gitee,gitcode,tencentcode} adapter/internal/{giteafamily,backendutil}
    （internal 面冻结=v1 生态内约定） adapter/all contracttest auth/{githubapp,
    credential} filter/branchfilter model/projection`——以 commit 时刻实际包表
    生成；`internal/` 与 `mcp/` 不进金样（前者非公共、后者独立模块）。
  - 比对：当前面 → 规范化排序文本 ↔ `docs/api-surface-v1.txt` 逐行 diff；
    差异即红（增/删/改签名同权）。金样头注写明更新纪律（v1 期改动=conscious
    编辑本文件并写 CHANGELOG）。
  - 初始金样生成：一次性 `go run ./internal/tools/gensurface`（新工具，genledger
    同款形态）→ `docs/api-surface-v1.txt`；工具保留（金样更新用同一条命令）。
- **注意**：金样守卫挂 `go test ./...` 即 `make check`——本期发版后任何 PR 的面
  变化立即红。genledger 的 `go:generate` 不挂本工具（金样更新是 v1 期显式动作，
  不随 generate 自动重写——**写进金样头注**）。

### 3.2 B：`docs/COMPATIBILITY.md` + MIGRATION v1 声明

- 结构：①平台×能力矩阵（**真源 = `adapter/all/readme_matrix_test.go` 对账的
  README 表**——本文档引用不复制，写「机器对账于 adapter/all 测试」+ 当前快照）；
  ②Go 地板（go.mod `go 1.27.4`）；③mcp↔父模块耦合（S8 两步窗口规则引用
  VERSIONING）；④平台实测覆盖面（e2e 沙箱 = gitea/forgejo；巡检 = 七平台只读
  [secrets 待配]；contracttest = 七平台 httptest wire 面）；⑤v1 语义（破坏=major、
  minor=能力、patch=修复——引用 CONTRIBUTING/VERSIONING）。
- MIGRATION.md 头部补 v1 声明段（0.x→v1.0 无额外迁移；重构迁移矩阵继续有效）。

### 3.3 C：MCP 二进制发布（release.yml 追加 build/upload 段）

- 触发不变（`v*` tag）。**构建前抬版**：`sed` `mcp/go.mod` require 父模块行 →
  本 tag（`v${GITHUB_REF##*/}`）+ `GOWORK=off go mod tidy`——解决「新 tag 的
  mcp require 还指上一版」的两步窗口（S8 套路的发布时自动化），构建完即弃
  （不 commit）。
- 6 目标矩阵：`linux/{amd64,arm64} darwin/{amd64,arm64} windows/{amd64,arm64}`，
  产物 `go-git-platform-mcp_<os>_<arch>[.exe]`，`-ldflags "-X
  github.com/yi-nology/go-git-platform/internal/version.Tag=<tag>"`（version
  包现场核变量名——若无则用 `go version -m` 自带元数据，裁定记录）。
- SBOM：`anchore/sbft-action`（或 syft 二进制）CycloneDX 对 mcp 模块；
- checksum：`sha256sum * > checksums.txt`；
- 上传：`gh release upload <tag> <binaries…> sbom checksums`；
- provenance：`actions/attest-build-provenance@v2` subjects = 6 二进制 + SBOM +
  checksums；
- workflow 语法 actionlint + 首跑验证（挂发版序列：**先发一次带构建段的 tag，
  红则修复循环**——同 e2e 首跑先例）。

### 3.4 D：`docs/VERSIONING.md`

- 主模块：`vX.Y.Z` tag 语义（minor=breaking 承载先例 v0.73.0/重构 v0.141.0；
  patch=修复；docs-only 不打 tag 直接 push）；
- mcp 模块：require 人工跟随（发版后 S8 抬版 + tidy；两步窗口规则）；无 `mcp/`
  tag；二进制挂父 tag 的 Release；
- 金样纪律：v1 期 API 变化 = 更新 `docs/api-surface-v1.txt` + CHANGELOG；
- 下游引用口径（`go get` 主模块、mcp 安装 `go install …/mcp/cmd/…@latest`）。

## 4. 分期与任务预览（SDD）

| 序 | 内容 | 主要文件 | 风险 |
|---|---|---|---|
| T1 | D：`docs/VERSIONING.md` + COMPATIBILITY + MIGRATION v1 声明 | `docs/` 三文件 | 低 |
| T2 | A：gensurface 工具 + 金样 + apisurface 守卫（RED 实证：加假符号→红） | `internal/tools/gensurface/`、`docs/api-surface-v1.txt`、`internal/apisurface/` | 中（面抽取形态选型） |
| T3 | C：release.yml build/upload/attest 段 + 抬版 dance | `.github/workflows/release.yml` | 中（首跑挂发版） |
| T4 | 收口：全量门 + 验收走查 + 批次评审 + 发版暂停点 | CHANGELOG Unreleased | — |

顺序理由：T1 文档先行（零风险铺底）；T2 机器守卫（T3 的发布物不进金样面——
mcp 是独立模块不冻结，无依赖但先立冻结再动 CI 更稳）；T3 最依赖发版场合实跑；
T4 收口。发版序列含「构建段首跑」验证（红则修复循环归 T3 链）。

## 5. 验收标准

1. `make check` 含 apisurface 守卫且绿；人为加假导出符号 → 红（负样例报告）；
2. `docs/api-surface-v1.txt` 与 `go doc` 三包计数对账（provider 133/model 54/
   runtime 53 为起点，最终以生成时刻为准）；
3. `docs/COMPATIBILITY.md`/`VERSIONING.md` 落位，矩阵真源引用成立（README 表与
   adapter/all 测试仍一致）；
4. release.yml 构建段语法 actionlint 绿；**真实 tag 首跑**产出 6 二进制+SBOM+
   checksums+provenance 全挂 Release（挂发版暂停点后的序列）；
5. 全量收尾门（make check/check-mcp/双 lint/覆盖率/双 govulncheck）绿；
6. 零生产语义变化（diff 面 = docs/internal 守卫/workflow/新工具）。

## 6. 明确不做

不打 v1.0 tag；不引 goreleaser；不发 mcp tag；不改 go.mod 依赖面；金样不含
未导出/注释；不动 release.yml 测试段语义；不做容器镜像（二进制+SBOM+provenance
已达路线图要求——**容器镜像若有需要属 v1.0 后追加**，本期裁定不做）。
