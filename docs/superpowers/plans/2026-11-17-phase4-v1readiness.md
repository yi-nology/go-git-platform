# 第 4 期：v1.0 准备 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** v1 就绪态——`internal/apisurface` 金样守卫冻结公共面（机器强制）、`docs/COMPATIBILITY.md`/`VERSIONING.md` 落位、Release workflow 追加 MCP 6 目标二进制+SBOM+checksum+provenance 构建段。**不打 v1.0 tag**（就绪 ≠ 官宣）。

**Architecture:** T1 文档铺底 → T2 机器守卫（金样由新工具 gensurface 一次性生成，守卫与工具共享抽取核心）→ T3 发布段（抬版 dance 在 workflow 内完成、构建完即弃）→ T4 收口+发版暂停。全部 additive：docs/internal 守卫/workflow/新工具，零生产语义变化。

**Tech Stack:** Go 1.27.4（stdlib go/parser、零新依赖）；syft（SBOM，CI 工具）；actions/attest-build-provenance@v2；无 goreleaser（裁定②）。

**Spec:** [docs/superpowers/specs/2026-11-17-phase4-v1readiness-design.md](../specs/2026-11-17-phase4-v1readiness-design.md) —— 四裁定（①金样面 ②手滚 ③attest ④无 mcp tag）、容器镜像顺延裁定、包清单、6 目标矩阵、验收 §5 以 spec 为真源。

## Global Constraints

- **红线（spec §2）**：不打 v1.0 tag；不引 goreleaser；不发 mcp tag；不改 go.mod/go.sum 依赖面；金样不含未导出/注释；不动 release.yml 测试段语义；不做容器镜像（顺延裁定，spec §6）。
- **金样纪律**：v1 期 API 变化 = 显式更新 `docs/api-surface-v1.txt`（gensurface 重生成）+ CHANGELOG；**不挂 go:generate**（更新是显式动作，spec §3.1）。
- **守卫范围**：15 包金样（spec §3.1 清单）；`internal/` 与 `mcp/` 不进。
- **每 task 绿门**：`GOWORK=off go build ./... && GOWORK=off go test ./...` + `make check-mcp` + `golangci-lint run ./...`。
- **提交**：中文 conventional、明确文件清单、绝不 `-A`；并行会话纪律（动工前 git status）。
- **执行环境**：worktree 隔离；CI 首跑验证归发版序列（T3 的构建段红=修复循环归 T3 链）。

---

### Task 1: D——VERSIONING + COMPATIBILITY + MIGRATION v1 声明

**Files:**
- Create: `docs/VERSIONING.md`、`docs/COMPATIBILITY.md`
- Modify: `docs/MIGRATION.md`（头部 v1 声明段）

**Interfaces:**
- Produces（T5 收口消费）: 消费方可见的三文档；COMPATIBILITY 对「真源=adapter/all 机器测试」的引用关系。

- [ ] **Step 1: 现场核**
  - 通读 `CONTRIBUTING.md`（Versioning & compatibility commitment 节——VERSIONING 与其**零矛盾**，引用不复制承诺正文）；通读 `adapter/all/readme_matrix_test.go` 与 README 能力表（COMPATIBILITY 引用真源）；读 `docs/MIGRATION.md` 头部（v1 声明段插入位置）。

- [ ] **Step 2: 写 `docs/VERSIONING.md`**（spec §3.4 四节）
  - 主模块 tag 语义：`vX.Y.Z`（minor=breaking 承载，先例 v0.73.0 删 ListAllPages / v0.141.0 重构 BREAKING；patch=修复；**纯 docs 提交不打 tag 直接 push**）；
  - mcp 模块：require 人工跟随（发版后抬版 + GOWORK=off tidy——S8 套路；两步窗口规则）；**无 `mcp/` tag**（裁定④）；二进制挂父 tag Release（本期 C 段落地）；
  - 金样纪律：v1 期 API 变化 = `go run ./internal/tools/gensurface` 重生成金样 + CHANGELOG；
  - 下游引用：`go get github.com/yi-nology/go-git-platform@latest`；`go install github.com/yi-nology/go-git-platform/mcp/cmd/go-git-platform-mcp@latest`。

- [ ] **Step 3: 写 `docs/COMPATIBILITY.md`**（spec §3.2 五节）
  - 平台×能力矩阵：**引用真源**（「机器对账于 `adapter/all/readme_matrix_test.go` ↔ README 表」）+ 当前快照（从 README 表复制，注 明快照日期与对账测试名）；
  - Go 地板（go.mod `go 1.27.4`）；mcp↔父模块耦合（引用 VERSIONING）；平台实测覆盖面（e2e 沙箱 = gitea:28.4.0+forgejo:17.0.1；巡检六家族 = 七平台只读[secrets 待配]；contracttest = 七平台 httptest wire 面）；v1 语义段（破坏=major 引 CONTRIBUTING/VERSIONING）。

- [ ] **Step 4: MIGRATION v1 声明段**（头部、2-3 行）：v1.0 起 0.x→v1.0 无额外迁移；本矩阵（重构）迁移行继续有效；后续破坏走 major。

- [ ] **Step 5: 绿门 + Commit**
  ```bash
  GOWORK=off go build ./... && golangci-lint run ./docs/... 2>/dev/null; make check-mcp
  git add docs/VERSIONING.md docs/COMPATIBILITY.md docs/MIGRATION.md
  git commit -m "docs: VERSIONING/COMPATIBILITY 落位 + MIGRATION v1 声明（4期 T1）"
  ```

---

### Task 2: A——gensurface 工具 + 金样 + apisurface 守卫

**Files:**
- Create: `internal/tools/gensurface/main.go`、`docs/api-surface-v1.txt`（生成物）、`internal/apisurface/apisurface_test.go`
- Modify: 无生产文件

**Interfaces:**
- Produces（T4 收口消费）: `go run ./internal/tools/gensurface` 重生成金样；`go test ./internal/apisurface/` 守卫绿。

- [ ] **Step 1: 抽取形态选型（现场双试、报告记录裁定）**
  - 形态甲：`go doc -all <pkg>` 子进程逐包抽取 `^(func|type|const|var)` 行（文本协议，含签名）；
  - 形态乙：`go list -json <pkg>` 取 GoFiles + `go/parser` 读 AST 导出标识（无签名渲染成本，需自拼签名）；
  - 判据：**可重现性**（两次运行字节同）× **签名保真**（改动签名必须可见）× 零依赖。选优者定型，另一形态的落选原因写进报告与工具头注。

- [ ] **Step 2: 写 gensurface 工具**
  - `main.go`：包清单硬编码 = spec §3.1 的 15 包（provider、model、runtime、adapter 七平台、adapter/internal/{giteafamily,backendutil}、adapter/all、contracttest、auth/{githubapp,credential}、filter/branchfilter、model/projection）；逐包抽取 → 规范化排序（`sort.Strings`）→ `docs/api-surface-v1.txt` 写盘（头注：生成方式 + 更新纪律 + 「不挂 go:generate」）。
  - **先 RED**：金样文件不存在 → 守卫测试红（Step 3 先写测试，此处只是生成物落地步骤）。

- [ ] **Step 3: 写守卫测试（TDD：负样例必须咬）**
  - `internal/apisurface/apisurface_test.go`：调用与工具**共享的抽取核心**（工具 main 与测试同用 `internal/tools/gensurface/surface.go` 导出函数，或测试子进程 `go run`——现场选，报告记录；**禁止测试内复制抽取逻辑**——两份实现漂移即守卫失真）；
  - 断言：当前面抽取结果 == `docs/api-surface-v1.txt`（逐行）；
  - **负样例（计划明令）**：临时向 `provider/version.go` 追加 `func ApisurfaceProbe() {}` → `go test ./internal/apisurface/` 必须 FAIL 且 diff 指向 `provider.ApisurfaceProbe`；删除 → 绿。两轮输出进报告；
  - **漂移防护**：抽取核心若为共享库形态，测试再加一条「工具输出 == 测试输出」的等价断言（若选子进程形态则天然同源，报告说明）。

- [ ] **Step 4: 生成金样 + 全门**
  ```bash
  go run ./internal/tools/gensurface && git add docs/api-surface-v1.txt
  wc -l docs/api-surface-v1.txt   # 与 go doc 三包计数对账（provider≈133/model≈54/runtime≈53 起点）
  GOWORK=off go build ./... && GOWORK=off go test ./... && make check-mcp && golangci-lint run ./internal/...
  ```

- [ ] **Step 5: Commit**
  ```bash
  git add internal/tools/gensurface/ internal/apisurface/ docs/api-surface-v1.txt
  git commit -m "feat(internal/apisurface): v1 金样面守卫——gensurface 工具+15 包冻结清单+负样例实证（4期 T2）"
  ```

---

### Task 3: C——release.yml 构建/上传/attest 段

**Files:**
- Modify: `.github/workflows/release.yml`（测试段语义不动，追加 build 段）

**Interfaces:**
- Produces（T4/发版序列消费）: tag push → Release 含 6 二进制 + SBOM + checksums + provenance。

- [ ] **Step 1: 现场读 release.yml 全文 + version 包**
  - `cat .github/workflows/release.yml`（现有 job 结构、gh release create 步骤位置）；
  - `cat internal/version/*.go`——**现场核版本注入变量名**：若有导出 `Tag`/`Version` 变量 → ldflags `-X` 注入；若无 → 裁定用 `go version -m` 自带元数据（构建信息已含 vcs tag），报告记录。

- [ ] **Step 2: 追加构建段（测试段语义逐字不动）**
  ```yaml
    # ---- MCP binary build（4期 T3；两步窗口在构建时自动闭合：require 抬到本 tag ----
    mcp-dist:
      needs: test        # 以现有测试 job 名为准（现场读）
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v7
        - uses: actions/setup-go@v7
          with: { go-version-file: go.mod, cache: true }
        - name: Bump mcp require to this tag（构建时自动闭合两步窗口，构建完即弃）
          run: |
            TAG="${GITHUB_REF_NAME}"
            sed -i "s|github.com/yi-nology/go-git-platform v[0-9.]*|github.com/yi-nology/go-git-platform ${TAG}|" mcp/go.mod
            cd mcp && GOWORK=off go mod tidy
        - name: Build 6 targets
          run: |
            TAG="${GITHUB_REF_NAME}"
            mkdir -p dist
            for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
              os="${t%%/*}"; arch="${t##*/}"
              out="dist/go-git-platform-mcp_${os}_${arch}"
              [ "$os" = "windows" ] && out="${out}.exe"
              (cd mcp && GOWORK=off CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
                -ldflags "-s -w ${LDFLAGS_X}" -o "../$out" ./cmd/go-git-platform-mcp)
            done
        - uses: anchore/sbft-action@v0   # SBOM（CycloneDX）——现场核 action 名/参数；若无合用 action 则用 syft 二进制步（curl 安装 + syft dir:dist -o cyclonedx-json）
          with: …（现场按 action 文档）
        - name: Checksums
          run: cd dist && sha256sum * > ../checksums.txt && mv ../checksums.txt .
        - name: Upload to release
          run: gh release upload "${GITHUB_REF_NAME}" dist/* --clobber
          env: { GH_TOKEN: ${{ github.token }} }
        - uses: actions/attest-build-provenance@v2
          with: { subject-path: | dist/go-git-platform-mcp_* … }（现场按 action 文档 subjects 形态）
  ```
  **注意**：以上是**骨架**——LDFLAGS_X（version 变量名现场核）、sbft 参数、attest subjects、needs 链名全部以 Step 1 现场读为准；`release.yml` 现有 `gh release create` 步骤与构建段的先后（create 先于 upload——upload 需 Release 已存在）按现有结构编排。

- [ ] **Step 3: 语法验证 + 文档同步**
  - `actionlint .github/workflows/release.yml`（无则 yaml parse）；VERSIONING.md 的「二进制挂父 tag Release」句补「构建段在 release.yml mcp-dist job」引用；
  - **真跑验证挂发版序列**：本 task 完成定义 = 语法绿 + 逻辑推演记录；首跑红 = 修复循环归本 task 链（发版暂停点后的序列执行）。

- [ ] **Step 4: Commit**
  ```bash
  git add .github/workflows/release.yml docs/VERSIONING.md
  git commit -m "ci(release): MCP 6 目标二进制+SBOM+checksum+provenance 构建段——构建时自动抬版闭两步窗口（4期 T3）"
  ```

---

### Task 4: 收口——全量门 + 验收走查 + 批次评审 + 发版暂停点

**Files:**
- Modify: `CHANGELOG.md`（Unreleased）

- [ ] **Step 1: CHANGELOG Unreleased**：`### Changed（4 期：v1.0 准备）` 段——金样守卫（15 包冻结面+负样例）/ COMPATIBILITY+VERSIONING 落位 / release 构建段（6 二进制+SBOM+checksums+provenance，构建时抬版闭窗）；「零生产语义变化」+「不打 v1.0 tag（就绪≠官宣）」注记。
- [ ] **Step 2: 全量门**（make check / check-mcp / 双 lint / 覆盖率 / 双 govulncheck 逐腿）。
- [ ] **Step 3: spec §5 六项验收走查**（进报告）：重点 §5.1 负样例（T2 已证，收口复跑）、§5.2 金样计数对账、§5.4 构建段=语法绿+首跑挂发版、§5.6 diff 面=docs/internal/workflow/新工具。
- [ ] **Step 4: Commit + 批次评审**：`docs: 4 期 CHANGELOG 未发布段（4期 T4）`；controller 派全分支批次评审（base=本分支起点）→ findings 修复 → **发版暂停点**：`ls-remote` 实查（候选 v0.144.0）→ 推 main → CI 绿（构建段首跑归发版 tag 的 run）→ 切段/tag/push（等用户确认）。

---

## Self-Review 记录

- **Spec 覆盖**：§3.1→T2（包清单/金样纪律/不挂 generate 逐条）；§3.2→T1 Step3；§3.3→T3（抬版 dance/6 矩阵/SBOM/checksum/attest/首跑归发版）；§3.4→T1 Step2；§5.1 负样例→T2 Step3；§5.2 计数对账→T2 Step4；§5.4→T3 Step3+T4；§5.6→T4 Step3；§6 不做→Global Constraints。**无缺口。**
- **占位符扫描**：T3 的 YAML 为显式骨架+「以现场读为准」的五个定点（LDFLAGS 变量名/sbft 参数/attest subjects/needs 名/create-upload 顺序）——执行期实测项（release.yml 首读是 Step 1），非 TBD；T2 两形态选型同先例。无 "TODO/later/similar"。
- **类型一致**：包清单（15 包）在 spec §3.1 与 T2 Step2 逐字一致；`go-git-platform-mcp_<os>_<arch>` 命名在 T3 各处一致；`docs/api-surface-v1.txt` 路径在三处一致。
- **顺序依赖**：T2 依赖 T1 的 VERSIONING 金样纪律句（先有文档后有机器）；T3 的 VERSIONING 引用回填依赖 T1；T4 汇总。
