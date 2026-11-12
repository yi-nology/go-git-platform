# 第 1 期：稳定性与安全加固 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 清零评审 P0（MCP HTTP 匿名远程写操作可达），修复 P1（toolRegistry 并发竞争、mcp 模块 replace 阻碍独立分发），CI tidy 校验化。

**Architecture:** 最小改动路线——HTTP 监听地址解析抽成纯函数做启动期鉴权策略校验；toolRegistry 加互斥锁保住 #122 README 对账机制；mcp/go.mod 移除 replace 并以根 go.work 保持本地协同开发，CI 双形态验证（workspace 测本地、GOWORK=off 验证发布形态）。

**Tech Stack:** Go 1.27.4，modelcontextprotocol/go-sdk，GitHub Actions。

**Spec:** [docs/superpowers/specs/2026-11-12-review-roadmap-design.md](../specs/2026-11-12-review-roadmap-design.md) 第 1 期。

## Global Constraints

- 并发纪律：禁止裸 `go func`，测试/实现需要并发处一律 stdlib `sync.WaitGroup` 或在产线代码用 `async.GoSafe`（本计划只在测试里用裸 goroutine + WaitGroup，允许）。
- 错误处理：`oekiterrors.Wrap(err, msg, kv...)` 保 cause 链；格式化构造一律 stdlib `fmt.Errorf`（禁 `%v` 包装、禁 `_` 丢错误）。
- 提交信息：中文 conventional commits；改动 `mcp/` 的文件按明确清单 `git add`。
- 收尾验证（每个 Task 后跑相关子集，最终 Task 5 全跑）：`make check`（根模块）→ 全仓 `golangci-lint run ./...` → mcp/ 目录内单独 `go vet ./... && go test -race -count=1 ./...`。
- 测试夹具不依赖本机 git 身份：`GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null go test ./gitbackend/`（本计划不碰 gitbackend，仅最终验证时遵守）。
- 禁止新增 stdlib `log` / `fmt.Println` 调试输出；mcp 二进制日志走 `logx.WithService("go-git-platform-mcp")` 已有的 `logger`。

---

### Task 1: toolRegistry 并发竞争修复

**Files:**
- Modify: `mcp/server.go:90-105`
- Test: `mcp/server_test.go`（追加）

**Interfaces:**
- Consumes: `NewServer(p provider.Provider, opts Options) (*mcp.Server, error)`（已存在）；`fakeProvider`（server_test.go 已存在，同包可直接用）。
- Produces: `recordTool(toolset, name string)` 签名不变（包内私有），行为变为并发安全；测试 `TestNewServer_ConcurrentConstruction`。

- [ ] **Step 1: 写失败/并发测试**

在 `mcp/server_test.go` 末尾追加（import 补 `"sync"`）：

```go
// TestNewServer_ConcurrentConstruction 钉死并发构造无数据竞争：toolRegistry
// 是包级台账（issue #122），并发 NewServer 写入必须由锁保护。
// 必须带 -race 运行（Makefile TEST_FLAGS 已含）。
func TestNewServer_ConcurrentConstruction(t *testing.T) {
	const n = 16
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := NewServer(fakeProvider{}, Options{})
			errCh <- err
		}()
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent NewServer: %v", err)
		}
	}
	// 台账仍完整：core toolset 至少有仓库读工具记账。
	if len(toolRegistry["core"]) == 0 {
		t.Fatalf("toolRegistry[core] empty after concurrent construction")
	}
}
```

- [ ] **Step 2: 验证测试暴露竞争**

Run: `cd mcp && go test -race -run TestNewServer_ConcurrentConstruction ./...`
Expected: FAIL（data race 报告，指向 `recordTool` 的 map 写入）或 PASS 但 race detector 输出 WARNING——只要 race 报告出现即证明测试有效。若因 map 写入恰好串行化而无 race 报告，在 `recordTool` 内 `m[name] = true` 前加一行 `time.Sleep(time.Microsecond)` 临时放大窗口，确认 race 报告后再删掉。

- [ ] **Step 3: 加锁实现**

修改 `mcp/server.go:90-105`，`import` 增加 `"sync"`：

```go
// toolRegistry 是 add() 挂账的工具面台账（toolset → 工具名集合）——
// README 工具表对账机检的注册面真源（issue #122）。全局可变,但只做
// 幂等集合写入（同一 server 重复构造不叠加,测试 -count=2 安全）。
// 并发构造 NewServer 时会并发写入,必须经 toolRegistryMu 保护。
var (
	toolRegistryMu sync.Mutex
	toolRegistry   = map[string]map[string]bool{}
)

func recordTool(toolset, name string) {
	if toolset == "" {
		return
	}
	toolRegistryMu.Lock()
	defer toolRegistryMu.Unlock()
	m := toolRegistry[toolset]
	if m == nil {
		m = map[string]bool{}
		toolRegistry[toolset] = m
	}
	m[name] = true
}
```

- [ ] **Step 4: 验证测试通过且无 race**

Run: `cd mcp && go test -race -count=1 ./...`
Expected: 全部 PASS，无 race 报告。

- [ ] **Step 5: Commit**

```bash
git add mcp/server.go mcp/server_test.go
git commit -m "fix(mcp): toolRegistry 并发构造数据竞争——包级台账加互斥锁 + 并发构造 -race 测试（评审 P1）"
```

---

### Task 2: MCP HTTP 监听地址安全策略（P0）

**Files:**
- Modify: `mcp/cmd/go-git-platform-mcp/main.go`（run/serveHTTP 段 + import + flag 文档）
- Test: `mcp/cmd/go-git-platform-mcp/main_test.go`（新建）

**Interfaces:**
- Consumes: `serveHTTP(addr, bearer string, srv *mcp.Server) error`（已存在，签名不变）；`logger`（已存在）。
- Produces: `resolveListenAddr(addr string, hasToken bool) (listen, host string, err error)`（包内私有，Task 内测试直接覆盖）；`isLoopbackHost(host string) bool`（包内私有）。`run()` 中 `--http` 分支先调 `resolveListenAddr`，失败即返回错误拒绝启动；`skip-tls` 与非 loopback 组合时 `logger.Warn` 告警。

- [ ] **Step 1: 写失败测试**

新建 `mcp/cmd/go-git-platform-mcp/main_test.go`：

```go
package main

import "testing"

func TestResolveListenAddr(t *testing.T) {
	cases := []struct {
		name     string
		addr     string
		hasToken bool
		wantListen string
		wantErr  bool
	}{
		{"empty host defaults loopback", ":8080", false, "127.0.0.1:8080", false},
		{"empty addr rejected", "", false, "", true},
		{"missing port rejected", "foo", false, "", true},
		{"empty port rejected", "127.0.0.1:", false, "", true},
		{"explicit loopback ok", "127.0.0.1:9000", false, "127.0.0.1:9000", false},
		{"localhost ok", "localhost:9000", false, "localhost:9000", false},
		{"ipv6 loopback ok", "[::1]:9000", false, "[::1]:9000", false},
		{"wildcard no token rejected", "0.0.0.0:8080", false, "", true},
		{"wildcard with token ok", "0.0.0.0:8080", true, "0.0.0.0:8080", false},
		{"public ip no token rejected", "203.0.113.7:8080", false, "", true},
		{"public ip with token ok", "203.0.113.7:8080", true, "203.0.113.7:8080", false},
		{"hostname no token rejected", "gitea.internal:8080", false, "", true},
		{"hostname with token ok", "gitea.internal:8080", true, "gitea.internal:8080", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			listen, _, err := resolveListenAddr(c.addr, c.hasToken)
			if c.wantErr {
				if err == nil {
					t.Fatalf("resolveListenAddr(%q, %v) = %q, want error", c.addr, c.hasToken, listen)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveListenAddr(%q, %v): %v", c.addr, c.hasToken, err)
			}
			if listen != c.wantListen {
				t.Fatalf("resolveListenAddr(%q, %v) = %q, want %q", c.addr, c.hasToken, listen, c.wantListen)
			}
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "localhost": true,
		"LOCALHOST.": true, "foo.localhost": true,
		"0.0.0.0": false, "192.168.1.1": false, "example.com": false,
	} {
		if got := isLoopbackHost(host); got != want {
			t.Fatalf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}
```

- [ ] **Step 2: 验证编译失败**

Run: `cd mcp && go test ./cmd/...`
Expected: FAIL，未定义 `resolveListenAddr` / `isLoopbackHost`。

- [ ] **Step 3: 实现**

修改 `mcp/cmd/go-git-platform-mcp/main.go`：

1. `import` 增加 `"net"`。
2. `run()` 的 `--http` 分支（现 120-122 行）改为先解析：

```go
	if *httpAddr != "" {
		listen, host, err := resolveListenAddr(*httpAddr, *httpToken != "")
		if err != nil {
			return err
		}
		if *skipTLS && !isLoopbackHost(host) {
			logger.Warn("skip-tls with non-loopback HTTP listener: token and git traffic travel in plaintext on the network",
				"addr", listen)
		}
		return serveHTTP(listen, *httpToken, srv)
	}
```

3. 在 `serveHTTP` 前新增两个函数：

```go
// resolveListenAddr normalizes the --http listen address and enforces the
// auth policy: a bare ":port" binds loopback only; an explicit non-loopback
// host (wildcard, public IP, or hostname) refuses to start without
// --http-token, so an anonymous remote client can never reach the git
// credentials this process holds.
func resolveListenAddr(addr string, hasToken bool) (listen, host string, err error) {
	h, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("invalid --http address %q: %w", addr, err)
	}
	if port == "" {
		return "", "", fmt.Errorf("invalid --http address %q: missing port", addr)
	}
	if h == "" {
		h = "127.0.0.1"
	}
	if !isLoopbackHost(h) && !hasToken {
		return "", "", fmt.Errorf("--http address %q is not loopback: refusing to start without --http-token (anonymous remote clients would share this process's git credentials)", addr)
	}
	return net.JoinHostPort(h, port), h, nil
}

// isLoopbackHost reports whether host is a loopback literal (IP or
// localhost name). Hostname form is intentionally conservative: only
// "localhost" and "*.localhost" qualify; any other name must carry a token.
func isLoopbackHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return h == "localhost" || strings.HasSuffix(h, ".localhost")
}
```

4. 更新 flag 帮助与包文档注释（文件头 19-22 行）：

```
//	--http        serve streamable HTTP on this address instead of stdio.
//	              ":port" binds 127.0.0.1; a non-loopback host additionally
//	              requires --http-token or the server refuses to start.
//	              The MCP endpoint lives at /mcp.
//	--http-token  require "Authorization: Bearer <token>" on the HTTP
//	              endpoint (required for non-loopback binds)
```

`--http` flag 的 help 字符串同步改为 `":port" binds loopback only; non-loopback requires --http-token`。

- [ ] **Step 4: 验证测试通过**

Run: `cd mcp && go test -race -count=1 ./cmd/... && go vet ./cmd/...`
Expected: PASS。

再手动验证启动策略（真跑）：

```bash
cd mcp && go run ./cmd/go-git-platform-mcp --platform gitea --token t --http 0.0.0.0:18099
```

Expected: 立即退出、报错含 "refusing to start without --http-token"。（这是临时脚本验证，不进提交。）

- [ ] **Step 5: Commit**

```bash
git add mcp/cmd/go-git-platform-mcp/main.go mcp/cmd/go-git-platform-mcp/main_test.go
git commit -m "fix(mcp): HTTP 模式默认 loopback、非 loopback 无 --http-token 拒绝启动（评审 P0 清零）+ skip-tls 对外监听告警"
```

---

### Task 3: mcp 模块独立分发——移除 replace + 根 go.work

**Files:**
- Modify: `mcp/go.mod`（删 `replace` 行）
- Create: `go.work`、`go.work.sum`（go 命令生成）
- Modify: `Makefile`（根 job 显式 `GOWORK=off`；新增 `check-mcp` 目标）

**Interfaces:**
- Consumes: mcp/go.mod 的 `require github.com/yi-nology/go-git-platform v0.128.0`（已存在且与最新 tag 一致）。
- Produces: 根 `go.work`（`use ( . ./mcp )`）；`make check-mcp`（mcp 模块独立 vet+test+lint 入口）；`make check` 语义不变（仅根模块）。

- [ ] **Step 1: 删除 replace 并验证发布形态可解析**

删 `mcp/go.mod:60` 的 `replace github.com/yi-nology/go-git-platform => ../`，然后：

```bash
cd mcp && GOWORK=off go mod tidy && GOWORK=off go build ./... && GOWORK=off go test -count=1 ./...
```

Expected: 全部成功——require v0.128.0 从 proxy 解析，证明无 replace 的发布形态可构建可测。若 tidy 改动 go.mod/go.sum，核对 diff 只应是把 `github.com/yi-nology/go-git-platform` 从 replace 态转正产生的间接标注变化。

- [ ] **Step 2: 创建根 go.work 保持本地协同**

在仓库根写入 `go.work`：

```
go 1.27.4

use (
	.
	./mcp
)
```

然后运行 `go build ./...`（仓库根，workspace 模式）生成 `go.work.sum`。确认 `git status` 中 `go.work`、`go.work.sum` 为新增文件。

- [ ] **Step 3: Makefile 保持模块边界**

根 `Makefile` 修改两处：

1. 变量区追加 `GOWORK ?= off` 并给根模块的 go 调用加 `GOWORK=$(GOWORK)` 前缀（`build`/`test`/`test-short`/`vet`/`tidy` 目标的 `$(GO)` 调用；`cover`/`check` 依赖它们自动继承）。这样 `make check` 仍只覆盖根模块，与 AGENTS.md 规则四语义一致。

2. 追加目标：

```makefile
## check-mcp runs the CI gate inside the mcp module (workspace mode, so it
## tests the local SDK, matching the pre-replace development flow).
check-mcp:
	cd mcp && $(GO) vet ./...
	cd mcp && $(GO) test $(TEST_FLAGS) ./...
	cd mcp && $(GOLANGCI_LINT) run ./...
```

并把 `check-mcp` 加入 `.PHONY` 行。

- [ ] **Step 4: 验证**

```bash
make check && make check-mcp
```

Expected: 全绿（make check 不含 mcp 包，check-mcp 全过）。再验证 workspace 模式 `cd mcp && go test -race -count=1 ./...` 仍走本地 SDK（改动任一 provider 签名会让 mcp 编译失败即证明 workspace 生效——可用 `go list -m github.com/yi-nology/go-git-platform` 在 mcp/ 下确认输出为本地路径代替版本号）。

- [ ] **Step 5: Commit**

```bash
git add mcp/go.mod mcp/go.sum go.work go.work.sum Makefile
git commit -m "build(mcp): 移除 replace ../ 解锁独立分发——根 go.work 承接本地协同，make check/check-mcp 分模块入口（评审 P1）"
```

---

### Task 4: CI——tidy 校验化 + 双形态 GOWORK + 独立安装验证

**Files:**
- Modify: `.github/workflows/ci.yml`（mcp job 与根 job）

**Interfaces:**
- Consumes: Task 3 的 go.work / 无 replace mcp 模块。
- Produces: CI 行为：mcp job tidy 从自动修复改为漂移即失败；新增 `mcp-install` job 真跑 `go install pkg@ref` + `--help`。

- [ ] **Step 1: 根 job 显式 GOWORK=off**

`.github/workflows/ci.yml` 根 job（非 mcp job）的 `env` 或每个 go step 加 `GOWORK: off`，保持「根 job 只测根模块」语义不被 go.work 改变。若 ci.yml 已有顶层 `env:` 块则加在那里会影响 mcp job——mcp job 需要 workspace 模式，因此 GOWORK=off 只加在根 job 的 steps 上（或给 mcp job 显式 `GOWORK: on` 并顶层 off，二选一，以 diff 最小为准）。

- [ ] **Step 2: mcp job tidy 校验化**

把 mcp job 的 `Tidy` step（现 139 行附近）替换为：

```yaml
      # 模块元数据漂移（根 go.mod 改动后 mcp require/go.sum 未跟随）必须
      # 显式失败，而不是像旧配置那样在 CI 里被自动修复掩盖。GOWORK=off
      # 让 tidy 走 require 版本行的发布解析路径——这正是它要校验的形态。
      - name: Tidy check
        working-directory: mcp
        run: |
          GOWORK=off go mod tidy
          git diff --exit-code -- go.mod go.sum
```

同时删除原 step 上方「Self-heal mcp/go.sum」注释段（其描述的问题已由校验化显式暴露）。

- [ ] **Step 3: 新增 mcp-install job**

在 ci.yml 的 `mcp` job 后追加：

```yaml
  # 独立分发验证：go install pkg@ref 必须真实可用（评审 P1）。@ref 从
  # proxy 拉取，GOWORK=off 确保 @version 语法不被 workspace 拦截。
  mcp-install:
    name: MCP standalone install
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - name: Set up Go
        uses: actions/setup-go@v7
        with:
          go-version-file: go.mod

      - name: Install from module proxy
        env:
          GOWORK: off
          REF: ${{ github.event.pull_request.head.sha || github.sha }}
        run: go install github.com/yi-nology/go-git-platform/mcp/cmd/go-git-platform-mcp@${{ github.event.pull_request.head.sha || github.sha }}

      - name: Smoke the installed binary
        run: "$(go env GOPATH)/bin/go-git-platform-mcp" --help
```

注意：`REF` env 是冗余的，直接使用内联表达式即可（上面 run 行已内联，删除 env 中的 REF 行）。

- [ ] **Step 4: 本地静态验证**

```bash
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))" 2>/dev/null || npx --yes yaml-lint .github/workflows/ci.yml 2>/dev/null || ruby -ryaml -e 'YAML.load_file(".github/workflows/ci.yml")'
```

Expected: 无 YAML 解析错误。（本机无 PyYAML 时换用其他可用解析器；actionlint 若已安装优先 `actionlint .github/workflows/ci.yml`。）

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: mcp tidy 校验化（漂移即失败）+ 根 job GOWORK=off 保模块边界 + 新增 go install @ref 独立安装验证 job（评审 P1/P2）"
```

---

### Task 5: 文档收尾 + 全量验证 + 发布

**Files:**
- Modify: `AGENTS.md`（规则四、deps 存档条款中「mcp replace ../ 不动」表述）
- Modify: `CHANGELOG.md`（新增 Unreleased 段）
- Modify: `mcp/README.md`（HTTP 安全段落）

**Interfaces:**
- Consumes: Task 1-4 全部产出。
- Produces: v0.129.0 发布候选（打 tag 前需用户确认）。

- [ ] **Step 1: AGENTS.md 对账**

1. 规则四 `mcp/` 独立性段落：补一句「根 `go.work` 已入库承接本地协同；CI 与 `make check` 用 `GOWORK=off` 保持分模块验证，`make check-mcp` 走 workspace 模式测本地 SDK」。
2. 342 行附近「mcp replace ../ 不动、require 版本行随 tag 人工跟随」改为「mcp 无 replace（go.work 本地协同）、require 版本行随 tag 人工跟随」。

- [ ] **Step 2: CHANGELOG**

`CHANGELOG.md` 顶部加：

```markdown
## [Unreleased]

### Security
- MCP HTTP 模式默认绑定 loopback；显式非 loopback 地址无 `--http-token` 拒绝启动（评审 P0 清零）；`--skip-tls` 与对外监听组合输出安全告警。

### Fixed
- MCP `toolRegistry` 并发构造数据竞争（加互斥锁 + `-race` 回归测试）。

### Changed
- `mcp` 模块移除 `replace ../`，`go install github.com/yi-nology/go-git-platform/mcp/cmd/go-git-platform-mcp@version` 可独立分发；根 `go.work` 承接本地多模块开发。
- CI：mcp `go mod tidy` 改为漂移即失败校验；新增独立安装验证 job。

```

（实际插入位置/措辞以该文件现有 Keep a Changelog 结构为准。）

- [ ] **Step 3: mcp/README.md HTTP 安全段落**

在运行/部署相关段落补：

```markdown
### HTTP 模式安全默认

`--http ":port"` 只绑定 `127.0.0.1`；绑到 `0.0.0.0` 或任何非 loopback
地址必须同时给 `--http-token`，否则进程拒绝启动。对外部署时请在前面再
套 TLS 终结（监听器是明文 HTTP），`--skip-tls` 只应在自签内网实例调
试时使用。
```

- [ ] **Step 4: 全量验证**

```bash
make check && make check-mcp
golangci-lint run ./...
cd mcp && golangci-lint run ./...
GOTOOLCHAIN=go1.27.4 govulncheck ./...   # 根模块
cd mcp && GOTOOLCHAIN=go1.27.4 govulncheck ./...
```

Expected: 全绿；覆盖率不低于 45% 地板。任何一项失败按输出修复后再继续。

- [ ] **Step 5: 文档提交 + 切段**

```bash
git add AGENTS.md CHANGELOG.md mcp/README.md
git commit -m "docs: 第 1 期安全加固收官——AGENTS.md go.work 边界、CHANGELOG 未发布段、mcp README HTTP 安全默认"
```

**暂停点**：打 tag `v0.129.0` 并 push 触发 Release 是对外动作——执行到此时向用户确认后再 `git tag v0.129.0 && git push origin main v0.129.0`（并先把 CHANGELOG Unreleased 段切段为 v0.129.0 标题）。

---

## Self-Review 记录

- Spec 覆盖：1.1→Task 2；1.2→Task 1；1.3→Task 3+Task 4 安装 job；1.4→Task 4 tidy；1.5→Task 5 Step 4。无缺口。
- 类型一致：`resolveListenAddr` 返回签名 (listen, host, err) 在 Task 2 的 run() 调用与测试中一致；`isLoopbackHost` 两处一致。
- 已知风险：Task 4 Step 3 的 `go install @sha` 依赖 proxy 及时索引新 commit，偶发延迟会误报——届时重跑即可（AGENTS.md 有「改名后首次 push 会抽风、重试即可」同类先例）。
