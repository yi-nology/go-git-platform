# 第 3 期：兼容性与测试工程化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 四域覆盖率机器断言 ≥80%（webhook 55.6→80 主战役）、只读巡检六家族×七平台 matrix、faultsuite 异常注入三形态、gitea/forgejo 沙箱写 E2E 双场合——全部 additive 测试面，零 API/wire 变化。

**Architecture:** 按 spec §4 五 task 顺序（A 覆盖率 → B 巡检 → C2 异常注入 → C1 沙箱 → 收口），风险递增。覆盖率目标进 `make check`（机器强制）；巡检沿用 env-gated skip 约定（secrets 未配 = no-op）；faultsuite 走 contracttest 既有自带 httptest 模式；E2E 用 build tag 隔离 + CI services 编排，与本地 `make e2e` 共用测试代码。

**Tech Stack:** Go 1.27.4；bash（cover-domain.sh）；GitHub Actions services（docker）；无 go.mod 新依赖。

**Spec:** [docs/superpowers/specs/2026-11-16-phase3-compat-testeng-design.md](../specs/2026-11-16-phase3-compat-testeng-design.md) —— 四裁定、覆盖率基线、probe 家族表、faultsuite 三形态、E2E 双场合、secrets 清单、验收 §6 以 spec 为真源。

## Global Constraints

- **红线（spec §2 逐字）**：不改 `provider/model/runtime/adapter` 公共 API 与 wire；不加 cron；不碰 release.yml；真实平台只读；不刷已达标域；不引 go.mod 新依赖；docker 仅 gitea/forgejo；默认 `go test ./...`（make check）**不含** e2e。
- **四域阈值**：webhook/transport/gitbackend/pageiter 均 ≥80%（基线 55.6/81.2/83.8/98.8 —— spec §1.1）。
- **巡检铁律**：probe 代码只读——lint 级不 import 写接口；缺 secret → SKIP 不红（沿用 smoke 现约定）。
- **每 task 绿门**：`GOWORK=off go build ./... && GOWORK=off go test ./...` + `make check-mcp` + `golangci-lint run ./...`（T4 追加 `make e2e` 或 CI 实跑，见该 task）。
- **提交**：中文 conventional、明确文件清单（绝不 `-A`；`.video_agent/` 已 gitignore 但仍不 add）、每 task 一（或一串）提交；版本号只在收口/发版出现。
- **并行会话纪律**：动工前 `git status` + mtime；发现非己修改 STOP 报告；发版前 `ls-remote`。
- **执行环境**：worktree 隔离（SDD setup 时建）；本机 darwin，CI 以 push 后结果为准。

---

### Task 1: A——cover-domains 机器门 + webhook 域补测至 ≥80%

**Files:**
- Create: `scripts/cover-domain.sh`、`runtime/webhook_coverage_test.go`（或按拆分多个 `runtime/webhook_*_test.go`，与既有 `webhook_test.go`/`webhook_gitee_test.go` 同包）
- Modify: `Makefile`（`check` 链加 cover-domains；新 target）
- Test: 本 task 产出即测试

**Interfaces:**
- Produces（T5 收口消费）: `make check` 含 `cover-domains`；四域 ≥80% 断言脚本。

- [ ] **Step 1: 出零覆盖清单（方法论第一步）**
  ```bash
  go test -coverprofile=/tmp/cov.out ./runtime/ ./transport/ >/dev/null 2>&1 || go test -coverprofile=/tmp/cov.out ./runtime/ ./transport/
  go tool cover -func=/tmp/cov.out | awk -F'\t+' '$NF=="0.0%" {print $1}' | grep -E "webhook|retry|etag" > /tmp/zero.txt
  cat /tmp/zero.txt   # 权威缺口清单——本 task 只打 webhook 域的 0% 行；transport 0% 行（若有）顺手补，gitbackend/pageiter 不动
  ```
  把清单贴进报告；按「分支密集优先、纯转发壳跳过并注明」排序。

- [ ] **Step 2: 写 cover-domain.sh（先于补测落地——门先立）**

  `scripts/cover-domain.sh`：
  ```bash
  #!/usr/bin/env bash
  # 四域覆盖率断言（spec §1.1/§6.1）：任一域 <80% 非零退出。make check 调用。
  set -euo pipefail
  OUT="${1:-coverage.out}"
  [ -f "$OUT" ] || { echo "cover-domain: $OUT missing — run 'go test -coverprofile=$OUT ./...' first" >&2; exit 2; }
  fail=0
  check() { # name regex
    local name="$1" rx="$2"
    local agg
    agg=$(go tool cover -func="$OUT" | grep -E "$rx" | awk -F'\t+' '{gsub("%","",$NF); s+=$NF; n++} END {if(n) printf "%.1f", s/n; else print "NA"}')
    if [ "$agg" = "NA" ]; then echo "cover-domain: $name NO FUNCS MATCHED ($rx)" >&2; fail=1; return; fi
    local ok
    ok=$(awk -v a="$agg" 'BEGIN{print (a+0 >= 80) ? 1 : 0}')
    if [ "$ok" = "1" ]; then echo "cover-domain: $name ${agg}% OK"; else echo "cover-domain: $name ${agg}% < 80%" >&2; fail=1; fi
  }
  check "webhook"   'runtime/webhook|provider/webhook'
  check "transport" 'transport/(retry|etag|client)\.go:'
  check "gitbackend" 'gitbackend/'
  check "pageiter"  'pageiter'
  exit $fail
  ```
  Makefile：`cover-domains:` 依赖 `test`（生成 coverage.out——现 `cover` target 已产 coverage.out，复用其命令行），`check: vet lint test cover-domains`。**先用当前 coverage.out 跑一次：webhook 应红（55.6<80）——这就是本 task 的 RED。**

- [ ] **Step 3: RED 验证**

  Run: `make cover-domains`（基于现 coverage.out）
  Expected: `webhook 55.6% < 80%` 非零退出；其余三行 OK。贴输出进报告。

- [ ] **Step 4: webhook 域补测（GREEN 循环，逐清单项）**
  - 对 `/tmp/zero.txt` 中每个 webhook 0% 函数：读实现 → 表驱动测试（`runtime` 同包）。
  - 重点面（spec §1.1）：`ValidateWebhookWithRegistry`（无注册/多注册/unknown-event/secret 不匹配）、`Normalize*Action` 全词表、HMAC 校验双形态（`sha256=` 前缀 vs 裸 hex）与 `WithSignaturePrefix`、`ReadAndRestoreBody`（body 恢复/二次读/大 body 截断）。
  - 既有夹具复用：`runtime/webhook_test.go` 的 helper 直接用；错误判定只走 `errors.Is` 哨兵（规则二）。
  - 每补 3-5 个函数跑一次 `go tool cover -func=coverage.out | grep runtime/webhook | …` 看聚合爬升；**目标 = 域聚合 ≥80% 即停（不追 100%）**，剩余 0% 转发行壳在报告列明跳过理由。

- [ ] **Step 5: 全门 + 脚本真跑**
  ```bash
  make check          # 现在应含 cover-domains 且绿（coverage.out 由 check 内 test 步骤现产）
  golangci-lint run ./runtime/
  ```
  Expected: 全绿；贴四域聚合数值进报告。

- [ ] **Step 6: Commit**
  ```bash
  git add scripts/cover-domain.sh Makefile runtime/webhook_ coverage-test.go  # 按实际测试文件名
  git commit -m "test(runtime): webhook 验签域补测至 ≥80% + cover-domains 四域断言进 make check（3期 T1）"
  ```

---

### Task 2: B——smoke 六家族 probes + workflow matrix

**Files:**
- Create: `smoke/probes.go`、`smoke/probes_test.go`
- Modify: `smoke/smoke_test.go`（现有入口改为走注册表；env 读 `GIT_PLATFORM_SMOKE_FAMILIES`）、`.github/workflows/smoke.yml`（families input + matrix）
- Test: `probes_test.go`

**Interfaces:**
- Produces（T5 消费）: `smoke.ProbeFunc` 类型与 `smoke.Probes` 注册表；workflow `families` input。

- [ ] **Step 1: 现场读现状**
  - 通读 `smoke/smoke_test.go`（env 约定、skip 逻辑、provider 构造）与 `.github/workflows/smoke.yml` 全文——probes 必须长在现有形态上。
  - `grep -n "func.*ListCRs\|func.*ListWebhooks\|func.*GetCR" provider/iface_*.go` 核对只读方法签名（CR/Webhook 读面）。

- [ ] **Step 2: 写失败测试（probes 注册表）**

  `smoke/probes_test.go`：
  ```go
  package smoke

  import "testing"

  func TestProbesRegistry_SixFamiliesReadOnly(t *testing.T) {
  	want := []string{"repo", "cr", "webhook", "pagination", "errors", "connection"}
  	for _, f := range want {
  		if _, ok := Probes[f]; !ok {
  			t.Errorf("family %q missing from Probes registry", f)
  		}
  	}
  	if len(Probes) != len(want) {
  		t.Errorf("Probes has %d entries, want exactly %d (register new families here consciously)", len(Probes), len(want))
  	}
  }

  func TestParseFamilies(t *testing.T) {
  	cases := []struct{ in, want []string }{
  		{"", []string{"repo"}},                       // 缺省保持现状
  		{"all", []string{"repo", "cr", "webhook", "pagination", "errors", "connection"}},
  		{"repo,cr", []string{"repo", "cr"}},
  	}
  	for _, c := range cases {
  		got := ParseFamilies(c.in)
  		if len(got) != len(c.want) { t.Fatalf("ParseFamilies(%q)=%v want %v", c.in, got, c.want) }
  		for i := range got { if got[i] != c.want[i] { t.Errorf("ParseFamilies(%q)[%d]=%q want %q", c.in, i, got[i], c.want[i]) } }
  	}
  }
  ```
  Run: `go test ./smoke/ -run 'TestProbesRegistry|TestParseFamilies'` → **FAIL（undefined: Probes/ParseFamilies）**。

- [ ] **Step 3: 实现 probes.go**

  ```go
  // Package smoke: 只读巡检 probe 注册表（spec 3期 §3.2）。
  package smoke

  import (
  	"context"
  	"strings"

  	"github.com/yi-nology/go-git-platform/provider"
  )

  // ProbeFunc 是一个只读探针家族；实现内禁止调用任何写方法
  // （创建/更新/删除/标记）——lint 与评审共同把守（spec §2 红线）。
  type ProbeFunc func(ctx context.Context, p provider.Provider, opts Options) error

  // Options 沿用现有 smoke 入口的 owner/repo/target 参数。
  type Options struct{ Owner, Repo, URL string }

  // Probes 是六家族注册表——家族名即 workflow families input 的取值域。
  var Probes = map[string]ProbeFunc{
  	"repo":       probeRepo,       // 现有 GetRepo+ListRepos 逻辑原样迁入
  	"cr":         probeCR,         // GetCR+ListCRs 解析/状态词表/SourceBranch 过滤
  	"webhook":    probeWebhook,    // ListWebhooks 解析（不创建不删除）
  	"pagination": probePagination, // 同 endpoint 2 页 walk：页参生效、二页可空、无死循环
  	"errors":     probeErrors,     // 不存在资源→ErrNotFound；无权→401/403 哨兵
  	"connection": probeConnection, // TestConnection + 连通性
  }

  // ParseFamilies 解析 GIT_PLATFORM_SMOKE_FAMILIES：空→["repo"]（现状），
  // "all"→全六，逗号分隔按序去空白。
  func ParseFamilies(s string) []string {
  	if strings.TrimSpace(s) == "" { return []string{"repo"} }
  	if strings.TrimSpace(s) == "all" { return []string{"repo", "cr", "webhook", "pagination", "errors", "connection"} }
  	var out []string
  	for _, f := range strings.Split(s, ",") {
  		if f = strings.TrimSpace(f); f != "" { out = append(out, f) }
  	}
  	return out
  }
  ```
  六个 probe 函数体：**逐个从现有 `smoke_test.go` 的断言风格出发写**（现场核对其 helper）；每个函数首行可加 `// 只读：…` 注释列明调用的方法名。`probeCR` 等的断言细节按 spec §3.2 表（状态词表、页字段、错误哨兵用 `errors.Is`）。

- [ ] **Step 4: 接线现有入口**
  - `smoke_test.go` 主流程改为：`fams := ParseFamilies(os.Getenv("GIT_PLATFORM_SMOKE_FAMILIES"))` → 逐 family `t.Run(f, …)` 调 `Probes[f]`；原 GetRepo/ListRepos 代码迁入 `probeRepo`（行为逐字保留——现有断言是回归锚）。
  - 跑 `go test ./smoke/`：无 env 时行为与改前一致（只跑 repo 家族的等价面）。

- [ ] **Step 5: workflow matrix**
  - `smoke.yml`：加 input `families`（default `repo`——**缺省保持现状零行为变化**）；job 改 `strategy.matrix.platform: [github, gitlab, gitea, forgejo, gitee, gitcode, tencentcode]`，`if` 收窄：当 `inputs.platform != '' && inputs.platform != 'all'` 时用 fromJSON 过滤单平台（简单实现：两个 job 或 matrix include——现场选更短的写法，报告记录）；env 加 `GIT_PLATFORM_SMOKE_FAMILIES: ${{ github.event.inputs.families }}`。
  - **secrets 注释块**更新：写明 7 个 secret 名 + 未配置 = skip（spec §5 清单原文贴进 workflow 注释）。
  - YAML 校验：`python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/smoke.yml'))"`（或 actionlint 若本机有）。

- [ ] **Step 6: 绿门 + Commit**
  ```bash
  GOWORK=off go build ./... && GOWORK=off go test ./smoke/ && make check-mcp && golangci-lint run ./smoke/
  git add smoke/probes.go smoke/probes_test.go smoke/smoke_test.go .github/workflows/smoke.yml
  git commit -m "test(smoke): 巡检六家族 probe 注册表 + workflow 七平台 matrix（3期 T2）"
  ```
  报告注明：真平台手跑待用户 secrets（spec §5），本 task 以 httptest/现有 env 面证明接线。

---

### Task 3: C2——faultsuite 异常注入三形态

**Files:**
- Create: `contracttest/fault.go`
- Modify: `contracttest/contracttest.go`（Run 挂一行）、`contracttest/coverage_check_test.go`（mountedSuites 列表 + `FaultSuite` 需否 coveredFamilies 行——**非 manager 家族**，按 `DivergenceSuite` 先例：只进 mountedSuites，Direction2 用 `suite != "FaultSuite"` 豁免或直接不带 `Suite` 后缀命名规避——**现场按 coverage_check_test.go 现行规则选：优先改名 `FaultInjection` 子测挂进既有 Platform 级列表**，报告记录裁定）
- Test: fault.go 内子测

**Interfaces:**
- Produces（T5 消费）: `FaultSuite`（或裁定名）三子测绿挂进 `Run`。

- [ ] **Step 1: 现场核三个语义的实现真源**
  - 截断 JSON：`grep -rn "json.NewDecoder\|json.Unmarshal" transport/ provider/ | head`——断言「返回 err 而非静默空集」要按**实际**行为：若某路径会静默空集 → 那是发现的缺陷，**报告 + 另立单，不改生产**（spec 红线），测试按现实钉并注明。
  - 伪翻页：读 `runtime/pageiter.go` 的无进展检测（`DeepEqual(prev)` 预算语义）——stub 要构造「忽略 page 参数恒返同页」。
  - 故障 body：`transport` 对 5xx body 的消费面（是否进错误文本——`provider.Wrap`/`ProviderError` 的 body 处理）。

- [ ] **Step 2: 写三子测（自带 httptest，七后端全跑）**

  `contracttest/fault.go` 骨架（`testFaultSuite(t, h)` + `Run` 挂接同 ErrorClassSuite 形态；无 capability drift——七后端无条件跑）：
  ```go
  // 1) 故障响应体：5xx + 三种 body 形态（JSON 对象/纯文本/空体）
  //    → 每种断言 errors.Is(err, provider.ErrServerSide) 且 err 文本不含 body 内容（防泄漏）
  t.Run("Fault_5xxBodyForms", …)   // stub 按 header 或路径分发三种 body，逐形态驱动 ListRepos 或 ListIssues（现场选七后端都有的读方法——以 ErrorClassSuite 用的 ListIssues 为准）

  // 2) 截断 JSON：200 + 半截数组 / 半截对象 / 非法 UTF-8 尾
  //    → 断言 err != nil（或按 Step1 现场 ruling 钉现实）；绝不接受 panic
  t.Run("TruncatedJSON", …)

  // 3) 伪翻页：第 1、2 页恒返同一非空数组（忽略 page 参数）
  //    → 用 provider.PageFetch/Each 或直接 List* 两页驱动，
  //      断言在预算内以「无进展」语义停止：err 非 nil（ErrPageBudgetExceeded 类哨兵现场核名）
  //      或返回值不重复计入——按 pageiter 现实现场钉，ruling 进报告
  t.Run("FakePagination_Stops", …)
  ```
  每子测自带 `httptest.NewServer` + `h.NewProvider(t, baseCfg(h, srv.URL))`（既有模式）。

- [ ] **Step 3: 挂接 + coverage mirror 一致**
  - `Run` 加 `t.Run("FaultInjection", …)`（若裁定改名）；按 `coverage_check_test.go` 的 Direction1/2 规则同步（mounted 列表 + 是否需 family 行——**现场读该文件现行断言后执行，保持三个 coverage 测试全绿**）。

- [ ] **Step 4: 绿门 + Commit**
  ```bash
  GOWORK=off go test -race -count=1 ./contracttest/ ./backends/github/ ./backends/gitlab/ ./backends/gitea/ ./backends/forgejo/ ./backends/gitee/ ./backends/gitcode/ ./backends/tencentcode/
  git add contracttest/fault.go contracttest/contracttest.go contracttest/coverage_check_test.go
  git commit -m "test(contracttest): FaultInjection 三形态——5xx body/截断 JSON/伪翻页止损（3期 T3）"
  ```

---

### Task 4: C1——e2e 沙箱写 roundtrip（gitea/forgejo）

**Files:**
- Create: `e2e/doc.go`（`//go:build e2e` 说明）、`e2e/harness_test.go`（`//go:build e2e`）、`e2e/roundtrip_test.go`（`//go:build e2e`）、`.github/workflows/e2e.yml`
- Modify: `Makefile`（`e2e` target）、`.gitignore`（若需忽略 e2e 临时物）

**Interfaces:**
- Produces（T5 消费）: `make e2e`；`e2e.yml`；env 契约 `E2E_GITEA_URL`/`E2E_FORGEJO_URL`/`E2E_GITEA_TOKEN`/`E2E_FORGEJO_TOKEN`（harness 读；本地 harness 自建时可不设）。

- [ ] **Step 1: 镜像版本现场钉（先实测再钉——仓规）**
  - `grep 'gitea.dev/sdk\|forgejo-sdk' go.mod` → SDK 版本；查两 SDK 对应的服务端大版本（SDK changelog/目录内 version 常量/`grep -r "1\." $(go env GOMODCACHE)/gitea.dev/sdk@v1.5.1/*.go | grep -i version` 等现场手段）。
  - 若本机 docker 可用：`docker run --rm gitea/gitea:<候选>:latest /version`（或拉起后打 `/api/v1/version`）对 SDK 编译面冒烟——**写进 Makefile 变量 `GITEA_IMAGE`/`FORGEJO_IMAGE`，报告记录实测证据**；本机无 docker 则按 SDK 文档钉候选并在报告标注「待 CI 首跑验证」。

- [ ] **Step 2: harness（先失败测试形态——e2e 本身即测试）**

  `e2e/harness_test.go`（全部文件头 `//go:build e2e`）：
  ```go
  package e2e

  // startPlatform：本地模式（无 E2E_<X>_URL）用 docker run 起镜像、
  // t.Cleanup docker rm -f；CI 模式（env 已给 URL+TOKEN）直接复用。
  // 就绪：轮询 GET <url>/api/v1/version 至 200（120s 超时，1s 间隔）。
  // 初始化：创建 admin token（本地模式从容器日志/默认账号 env 取——
  //   gitea: GITEA__security__INSTALL_LOCK + 启动参数注入 admin，
  //   forgejo 同族；现场按镜像文档实现，报告贴实际命令）、建测试仓库。
  func startPlatform(t *testing.T, kind string) (baseURL, token string)

  // newProvider：provider.NewProvider(provider.Config{Platform: …, BaseURL, Token})
  // 断言实现 provider.IssueManager 等被测接口。
  ```

- [ ] **Step 3: roundtrip 测试**

  `e2e/roundtrip_test.go`（`//go:build e2e`），每平台 `t.Run("gitea",…)/t.Run("forgejo",…)`，写族（spec §3.4）：
  1. Issue：Create→Get（字段回读一致）→UpdateTitle→Close→Reopen→Delete→Get 必须 `ErrNotFound`；
  2. Label：Create→List 含→AddIssueLabels→ListIssueLabels 含→RemoveIssueLabel→不含→Delete；
  3. CR：Create→Get→UpdateTitle→Close（gitea/forgejo 支持面内，现场核状态动词）；
  4. Ensure 族：`runtime.EnsureIssueState` 两段（首跑 Created/Updated、二跑 Unchanged）+ `EnsureIssueLabels` 同构——**真 wire 上的零写断言**（二跑后请求计数为 0 可用 provider Hooks 计数，现场选实现）；
  5. 每步失败 `t.Logf("docker logs 尾部…")`（本地模式）。
  **只在容器内写**；跳过条件：无 `-tags=e2e` 不编译；env URL 缺失且无 docker → `t.Skip`（带原因）。

- [ ] **Step 4: Makefile + workflow**
  ```makefile
  GITEA_IMAGE  ?= gitea/gitea:<Step1钉定>
  FORGEJO_IMAGE ?= forgejo/forgejo:<Step1钉定>
  e2e:
  	go test -tags=e2e -count=1 -timeout 15m ./e2e/...
  ```
  `.github/workflows/e2e.yml`：
  ```yaml
  name: e2e
  on:
    push:
      branches: [main]
      paths: ['adapter/**','transport/**','contracttest/**','runtime/**','e2e/**','Makefile','.github/workflows/e2e.yml']
    pull_request:
      paths: ['adapter/**','transport/**','contracttest/**','runtime/**','e2e/**','Makefile','.github/workflows/e2e.yml']
  permissions: { contents: read }
  jobs:
    e2e:
      runs-on: ubuntu-latest
      timeout-minutes: 20
      services:
        gitea:
          image: gitea/gitea:<same pin>
          ports: ['3000:3000']
          options: >-
            --health-cmd "curl -fsS http://localhost:3000/api/v1/version || exit 1"
            --health-interval 5s --health-timeout 5s --health-retries 24
        forgejo:
          image: forgejo/forgejo:<same pin>
          ports: ['3001:3001']
          options: >- (同上，端口换 3001/版本端点按现场)
      steps:
        - uses: actions/checkout@v7
        - uses: actions/setup-go@v7
          with: { go-version-file: go.mod, cache: true }
        - name: Init admin + repo（现场按镜像写：curl API 建 token/仓库）
          run: …
        - run: go test -tags=e2e -count=1 -timeout 15m ./e2e/...
          env:
            E2E_GITEA_URL: http://localhost:3000
            E2E_GITEA_TOKEN: <init 步骤产出>
            E2E_FORGEJO_URL: http://localhost:3001
            E2E_FORGEJO_TOKEN: <…>
  ```
  services 健康检查与 init 步骤是本 task 最大不确定面——**现场实现、报告贴真实 YAML 与首跑日志**；本地 `make e2e` 必须先绿（本地是 harness 的第一验证场合）。

- [ ] **Step 5: 绿门 + Commit**
  ```bash
  make check            # 确认默认面不含 e2e（无 -tags 时 ./e2e/ 编译跳过）
  make e2e              # 本机有 docker：两平台 roundtrip 绿；无 docker：t.Skip 面报告注明
  git add e2e/ Makefile .github/workflows/e2e.yml .gitignore
  git commit -m "test(e2e): gitea/forgejo 沙箱写 roundtrip——本地 make e2e + CI services 双场合（3期 T4）"
  ```
  push 后盯 `e2e.yml` 首跑（本 task 完成定义含 CI 首绿；红则修复循环）。

---

### Task 5: T5 收口——全量门 + 批次评审 + 发版暂停点

**Files:**
- Modify: `CHANGELOG.md`（Unreleased）

- [ ] **Step 1: CHANGELOG Unreleased**
  `### Changed（3 期：兼容性与测试工程化）` 一段（中文括注风格同邻段）：四域覆盖门进 make check（webhook 55.6→80+）/ 巡检六家族 matrix / FaultInjection 三形态 / e2e 沙箱双场合；注明「零 API 变化，纯测试与 CI面」+ secrets 清单指引（spec §5）。

- [ ] **Step 2: 全量收尾门（仓规逐字）**
  ```bash
  make check && make check-mcp
  golangci-lint run ./...          # 根含 examples
  cd mcp && golangci-lint run ./...
  go tool cover -func=coverage.out | tail -1   # ≥45% 地板
  GOTOOLCHAIN=go1.27.4 govulncheck ./...
  cd mcp && GOTOOLCHAIN=go1.27.4 govulncheck ./...
  ```
  任一失败 → 修复小提交后重跑该腿。

- [ ] **Step 3: 验收对照（spec §6 六条逐条打勾进报告）**
  含：`git diff <base>..HEAD --name-only` 不触生产文件核验（只加测试/脚本/workflow/Makefile/CHANGELOG）。

- [ ] **Step 4: Commit + 批次评审**
  ```bash
  git add CHANGELOG.md && git commit -m "docs: 3 期 CHANGELOG 未发布段（3期 T5）"
  ```
  随后 controller 派全分支批次评审（base=本 task 起点、head=HEAD）→ findings 修复 → **发版暂停点**（`ls-remote` 实查 v0.142.0 候选、切段、tag、push——等用户确认）。

---

## Self-Review 记录

- **Spec 覆盖**：§1.1 基线→T1 Step1/3；§1.2 巡检→T2；§1.3 docker 现状+SDK 版本→T4 Step1；§3.1→T1；§3.2 六家族表→T2 Step3（probe 函数按表逐个实现）；§3.3 三形态→T3（T3 修订：§3.3 形态1 的 body 断言被现实推翻——body 进
  错误文本是 Wrap 设计，见 contracttest/fault.go canary 在场钉与另立单）；
  §3.4 双场合/写族清单/env 契约→T4；§4 分期表→T1-T5 一一对应；§5 secrets→T2 Step5 注释+T2 报告；§6 验收→各 task 绿门+T5 Step3；§7 不做→Global Constraints。**无缺口。**
- **裁定落位**：①不刷已达标域（T1 Step1 只打 webhook 0% 行+transport 顺手、gitbackend/pageiter 明令不动）②不加 cron（T2 无 cron 项）③push/PR 门禁（T4 Step4 paths）④异常注入归 contracttest（T3 独立、T4 不含）。
- **占位符扫描**：镜像版本/健康检查细节/init 步骤标「现场钉+报告贴证据」——这是执行期实测项（同 2B 现场核对先例），非 TBD 懒惰；三处 ruling 点（截断 JSON 现实、伪翻页停止语义、coverage mirror 命名）都给了现场裁定路径与报告要求。无 "TODO/implement later/similar to Task N"。
- **类型一致**：`smoke.ProbeFunc`/`Probes`/`ParseFamilies` 在 T2 Step2 测试与 Step3 实现一致；`make e2e`、`E2E_*_URL/TOKEN` 在 T4 Step2/4/5 一致；`FaultInjection` 挂接名在 T3 Step2/3 一致（若现场改名须同步两处——Step3 已写明）。
- **顺序依赖**：T1 门先立（T5 依赖 cover-domains 绿）；T3 不依赖 T4；T4 push 后 CI 首跑归 T4 完成定义、T5 收口时复核。
