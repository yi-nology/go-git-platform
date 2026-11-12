# oekit 收编优化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按 Tier1+Tier2+卫生清单把仓内手写等价物收编进 oekit 三模块（白名单不变），并同步存档。

**Architecture:** 机械替换先行（encoding/shellx/slicex/stringx），词表与注册表容器化居中，并发敏感改造（wait/ttlcache）带测试门与回滚门，评估类任务（cachetest/SlogLogger）允许「已评估不引入」结论，最后文档存档与全量验证。

**Tech Stack:** Go 1.27.4；oekit pkg v0.46.0 / concurrency v0.34.0 / logx v0.10.0（签名均已 `go doc` 核实）。

**Spec:** [docs/superpowers/specs/2026-11-12-oekit-adoption-design.md](../specs/2026-11-12-oekit-adoption-design.md)

## Global Constraints

- 依赖白名单不变：只许 `github.com/oekit/pkg` / `concurrency` / `logx`（含子包）；禁止引入其他 oekit 模块。
- 错误处理：格式化构造 stdlib `fmt.Errorf`（`%w`，禁 `%v`）；保 cause 链用 `oekiterrors.Wrap(err, msg, kv...)`；禁止 `_` 静默丢错误（本计划 T11 专项处理；确属 best-effort 的丢弃必须带显式注释）。
- 并发：禁止裸 `go func`；生产并发原语优先 oekit。
- 提交信息：中文 conventional commits；按明确文件清单 `git add`；`mcp/` 是独立模块（go.work 已入库，测试在 `mcp/` 内跑）。
- 收尾验证（T12 全跑）：`make check` → `make check-mcp` → 双模块 `golangci-lint run ./...` → 覆盖率 ≥45% → 双模块 `GOTOOLCHAIN=go1.27.4 govulncheck ./...`。
- gitbackend 测试夹具：`GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null go test ./gitbackend/`。
- **行为变更必须记 CHANGELOG**：T4 词表合并改变 strip/redact 覆盖面（安全正向）；其余任务行为不变。
- 每个 Task 结束跑其覆盖测试；并发敏感 Task（T7/T8）必须带 `-race`。

---

### Task 1: encoding 收编（HashToken + FingerprintSHA256）

**Files:**
- Modify: `provider/manager.go:89-95`
- Modify: `pkg/credential/deploykey.go:58-69`
- Test: `provider`（既有测试）、`pkg/credential/deploykey_test.go`（既有）

**Interfaces:**
- Produces: 行为不变的 `HashToken(token string) string`（前 16 hex）、`FingerprintSHA256(authorizedKey string) string`（`SHA256:<base64>` 无填充）。

- [ ] **Step 1: 改 HashToken**

```go
func HashToken(token string) string {
	if token == "" {
		return ""
	}
	return encoding.Sha256(token)[:16]
}
```

import 增 `github.com/oekit/pkg/encoding`；若 `crypto/sha256`、`encoding/hex` 在 manager.go 无其他使用则删 import（grep 确认）。

- [ ] **Step 2: 改 FingerprintSHA256 的 base64 两端**

```go
func FingerprintSHA256(authorizedKey string) string {
	fields := strings.Fields(strings.TrimSpace(authorizedKey))
	if len(fields) < 2 {
		return ""
	}
	raw, err := encoding.Base64DecodeBytes(fields[1])
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "SHA256:" + encoding.Base64RawEncodeBytes(sum[:])
}
```

说明：`encoding` 无 raw-sha256 字节原语（`Sha256` 只回 hex 字符串），`sha256.Sum256` 保留——诚实部分收编，勿硬凑。先 `go doc github.com/oekit/pkg/encoding Base64DecodeBytes` 确认与 `base64.StdEncoding` 同语义（标准字母表+填充）。

- [ ] **Step 3: 跑测试**

```bash
go test -race -count=1 ./provider/ ./pkg/credential/
```
Expected: PASS（deploykey 指纹向量、manager 缓存键测试钉住行为不变）。

- [ ] **Step 4: Commit**

```bash
git add provider/manager.go pkg/credential/deploykey.go
git commit -m "refactor: HashToken/FingerprintSHA256 收编 oekit encoding（Sha256/Base64 两端）"
```

---

### Task 2: shellx.Quote 收编 GIT_SSH_COMMAND ×5 + 补测试

**Files:**
- Modify: `pkg/credential/manager.go:53,63`、`pkg/credential/sshkey.go:97,104`、`gitbackend/native.go:287-291`
- Test: `pkg/credential/manager_test.go`、`pkg/credential/sshkey_test.go`（新增用例）

**Interfaces:**
- Produces: 同名 `BuildSSHCommand`/`BuildSSHCommandInsecure`（输出引号形态从 Go `%q` 双引号变为 shellx 单引号——**输出字符串变化**，无既有测试钉旧形态，已 grep 确认）。

- [ ] **Step 1: 写失败测试（先钉新语义）**

在 `pkg/credential/manager_test.go` 追加：

```go
// BuildSSHCommand 必须产出 POSIX shell 单词：key 路径含 $ / 反引号 / 空格
// 时不得被 GIT_SSH_COMMAND 的 sh -c 展开（Go %q 不转义 $ 与反引号——
// 旧实现的保护声明不实，shellx.Quote 单引号化才是真保护）。
func TestBuildSSHCommand_ShellMetacharPath(t *testing.T) {
	m := &Manager{}
	got := m.BuildSSHCommand("/tmp/$HOME `id` key")
	if !strings.Contains(got, `'/tmp/$HOME `+"`id`"+` key'`) {
		t.Fatalf("key path not single-quoted: %s", got)
	}
	if strings.Contains(got, `"/tmp/$HOME`) {
		t.Fatalf("Go %%q double-quote form leaked: %s", got)
	}
}
```

`sshkey_test.go` 同款（对 `SSHKeyHelper.BuildSSHCommand`）。Expected: FAIL（当前产出 `"/tmp/$HOME..."` 双引号形态）。

- [ ] **Step 2: 实现替换（5 处）**

`manager.go:53`：

```go
	return fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=yes -o UserKnownHostsFile=~/.ssh/known_hosts", shellx.Quote(sshKeyPath))
```

`manager.go:63`：

```go
	return fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", shellx.Quote(sshKeyPath))
```

`sshkey.go:97`：

```go
	return fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=~/.ssh/known_hosts", shellx.Quote(keyPath))
```

`sshkey.go:104`：

```go
	return fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", shellx.Quote(keyPath))
```

`native.go:287-291`——**注释必须改写**（旧注释称 %q 保护 `$`/反引号，不实）：

```go
			// shellx.Quote 单引号化 key 路径：GIT_SSH_COMMAND 由 git 交给
			// sh -c 执行，$ / 反引号 / 空格在双引号里仍会被 shell 解释
			// （Go %q 不转义它们）——单引号内一切字面。
			sshCmd := fmt.Sprintf("ssh -i %s -o BatchMode=yes", shellx.Quote(auth.SSHKey))
			sshCmd += " " + sshHostKeyArgs(auth)
			cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
```

各文件 import `github.com/oekit/pkg/shellx`；`sshHostKeyArgs` 返回值若含未引号化的用户输入，一并核对其是否已 Quote（读函数体，若手拼则同收编）。

- [ ] **Step 3: 跑测试**

```bash
go test -race -count=1 ./pkg/credential/ && GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null go test -race -count=1 ./gitbackend/
```
Expected: PASS（新元字符用例 + gitbackend 全量——SSH 相关集成测试）。

- [ ] **Step 4: Commit**

```bash
git add pkg/credential/manager.go pkg/credential/sshkey.go pkg/credential/manager_test.go pkg/credential/sshkey_test.go gitbackend/native.go
git commit -m "fix(ssh): GIT_SSH_COMMAND 五处 %q 改 shellx.Quote——sh -c 下 \$ 与反引号不再被展开（收编 + 元字符测试）"
```

---

### Task 3: stringx/slicex 收编（branchfilter + divergence + ensure）

**Files:**
- Modify: `pkg/branchfilter/filter.go:60-68`（New 的拆分样板）、`:133-137`（Apply 收集循环）
- Modify: `provider/divergence.go:44-51`（FindByMethod）
- Modify: `provider/ensure.go:80-86`（matches 收集）
- Test: 既有 `pkg/branchfilter` 测试、`provider/divergence_test.go`、`provider` ensure 测试

**Interfaces:**
- Produces: `BranchFilter.New` / `BranchFilter.Apply`（签名不变）、`FindByMethod`（签名不变）、ensure 内部 matches（不变）。

- [ ] **Step 1: branchfilter New 换 SplitCSVNonEmpty**

```go
func New(filterStr string) (*BranchFilter, error) {
	if filterStr == "" {
		return &BranchFilter{patterns: nil}, nil
	}
	patterns := stringx.SplitCSVNonEmpty(filterStr)
	return newPatterns(patterns)
}
```

注意语义差：`SplitCSVNonEmpty` 带 RFC4180 引号语义（含 `"` 的 pattern 解析会变）——branch filter 无引号用法；若既有测试有含引号用例且断言变化，按测试意图判定并记录（预期无）。

- [ ] **Step 2: branchfilter Apply 收集循环换 slicex.Filter**

```go
	result := make([]string, 0, len(branches))
	return slicex.Filter(branches, f.Match)
```

（若现场是先判 `len(f.patterns)==0` 再收集，保留早退分支，只换收集段；`f.Match` 方法值签名 `func(string) bool` 与 `slicex.Filter[string]` 对齐——先 `grep -n "func.*Match" pkg/branchfilter/filter.go` 确认。）

- [ ] **Step 3: divergence.FindByMethod 换 slicex.Filter**

```go
// FindByMethod returns the ledger entries registered for method.
func FindByMethod(divs []Divergence, method string) []Divergence {
	return slicex.Filter(divs, func(d Divergence) bool { return d.Method == method })
}
```

`Ignores`/`Stubs` **保留手写循环**——早退谓词无 slicex 对应原语（无 Any/FindFunc），Filter 会白分配；在两函数注释补一行「slicex 无早退谓词原语，保留循环」。

- [ ] **Step 4: ensure matches 换 slicex.Filter**

```go
	matches := slicex.Filter(existing, func(h *PlatformWebhook) bool {
		return h != nil && h.URL == opts.URL
	})
```

（替换 `var matches []*PlatformWebhook` + for 循环；类型名以现场为准。）

- [ ] **Step 5: 跑测试**

```bash
go test -race -count=1 ./pkg/branchfilter/ ./provider/
```
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add pkg/branchfilter/filter.go provider/divergence.go provider/ensure.go
git commit -m "refactor: stringx.SplitCSVNonEmpty + slicex.Filter 收编（branchfilter 拆分样板与三处收集循环；早退谓词类保留并注明）"
```

---

### Task 4: 敏感参数词表合并为单一真源（行为变更，记 CHANGELOG）

**Files:**
- Modify: `transport/client.go:540-560`（词表）、`:899-910`（redactURL）、`:943-951`（redactPath）
- Test: `transport/redirect_credential_test.go`、`transport/client_test.go`

**Interfaces:**
- Produces: 包级 `sensitiveQueryParams`（15 项超集）——strip（`StripSensitiveQueryParams`）与 redact 共用；函数签名全部不变。

- [ ] **Step 1: 合并词表**

```go
// sensitiveQueryParams 是本仓认定「凭据类」的 query 参数单一真源，
// 同时服务两处消费：
//   - StripSensitiveQueryParams：跨 host 重定向剥离（防令牌随 query
//     发给第三方主机）；
//   - redactURL/redactPath：日志与错误信息里的 query 掩码。
//
// 合并前两份词表互有出入（strip 11 项独有 auth/jwt/session/signature/sig；
// redact 9 项独有 password/passwd/secret/key）——任何一份缺席都是漏洞面：
// 口令参数会随重定向外泄、jwt 会明文进日志。合并为超集后双向补齐。
var sensitiveQueryParams = []string{
	"access_token", "private_token", "token", "api_key", "apikey",
	"auth", "authorization", "jwt", "session", "signature", "sig",
	"password", "passwd", "secret", "key",
}
```

- [ ] **Step 2: redact 两函数改读共享词表**

`redactURL:902-905` 与 `redactPath:944-947` 的内联 `[]string{...}` 均替换为 `sensitiveQueryParams`：

```go
	for _, k := range sensitiveQueryParams {
		if q.Get(k) != "" {
			q.Set(k, "***")
			changed = true
		}
	}
```

- [ ] **Step 3: 更新测试**

- 跑 `go test ./transport/` 找失败断言：redirect_credential_test 对 strip 面的既有断言（11 项子集）应仍绿；若 redact 断言里有「非词表参数不掩码」用例用的恰是 password/key 等新并入项，按新语义更新并注释词表合并背景。
- **新增用例**：`redactURL` 掩码 `jwt`/`session`（原 strip 独有项）；`StripSensitiveQueryParams` 剥离 `password`/`secret`（原 redact 独有项）——双向补齐各钉一例。

- [ ] **Step 4: 跑测试**

```bash
go test -race -count=1 ./transport/
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add transport/client.go transport/redirect_credential_test.go transport/client_test.go
git commit -m "fix(transport): 敏感 query 词表合并 15 项超集单一真源——strip 补口令类、redact 补 jwt/session 类（行为变更，CHANGELOG 记 Changed）"
```

---

### Task 5: mcp toolRegistry → container.SafeMap（第 6 处注册表迁移）

**Files:**
- Modify: `mcp/server.go:90-113`（toolRegistry 段）
- Test: `mcp/readme_tools_test.go:44,60,76`、`mcp/server_test.go:331`

**Interfaces:**
- Produces: `recordTool(toolset, name string)`（签名不变）；新增包内 `recordedToolsOf(toolset string) []string`、`recordedToolsets() []string`（测试读取面）；`toolRegistry` 变为 `*container.SafeMap[string, bool]`（key = `toolset + "\x00" + name`）。

- [ ] **Step 1: 重写注册表段（mcp/server.go）**

```go
// toolRegistry 是 add() 挂账的工具面台账（key = toolset + "\x00" + 工具名）
// —— README 工具表对账机检的注册面真源（issue #122）。oekit container.
// SafeMap 收编（本仓第 6 处注册表迁移，前五处见 AGENTS.md 规则一）：
// 并发构造 NewServer 的写入由容器层互斥承接，Phase 1 的手写
// toolRegistryMu 随之删除。
var toolRegistry = container.NewSafeMap[string, bool]()

const toolsetSep = "\x00"

func toolsetKey(toolset, name string) string { return toolset + toolsetSep + name }

func recordTool(toolset, name string) {
	if toolset == "" {
		return
	}
	toolRegistry.Set(toolsetKey(toolset, name), true)
}

// recordedToolsOf 返回某 toolset 已挂账的工具名（README 对账读取面）。
// Range 走容器快照、顺序不定 ⇒ 输出排序保证确定性。
func recordedToolsOf(toolset string) []string {
	prefix := toolset + toolsetSep
	var out []string
	toolRegistry.Range(func(k string, _ bool) bool {
		if after, ok := strings.CutPrefix(k, prefix); ok {
			out = append(out, after)
		}
		return true
	})
	slices.Sort(out)
	return out
}

// recordedToolsets 返回全部已挂账的 toolset 名（排序）。
func recordedToolsets() []string {
	var out []string
	toolRegistry.Range(func(k string, _ bool) bool {
		if i := strings.IndexByte(k, toolsetSep); i > 0 && !slices.Contains(out, k[:i]) {
			out = append(out, k[:i])
		}
		return true
	})
	slices.Sort(out)
	return out
}
```

注意：删除 `toolRegistryMu` 与 `sync` import（若 server.go 无其他 sync 使用）；import 增 `strings`、`slices`、`github.com/oekit/pkg/container`。

- [ ] **Step 2: 改测试读取点**

- `mcp/server_test.go:331`：`if len(toolRegistry["core"]) == 0 {` → `if len(recordedToolsOf("core")) == 0 {`
- `mcp/readme_tools_test.go:44`：`len(toolRegistry) == 0` → `len(recordedToolsets()) == 0`
- `mcp/readme_tools_test.go:60`：`reg, ok := toolRegistry[toolset]` → `reg := recordedToolsOf(toolset); ok := len(reg) > 0`（`reg` 后续按 map 迭代的，改为 `for _, name := range reg`——读现场适配）
- `mcp/readme_tools_test.go:76`：`for toolset := range toolRegistry {` → `for _, toolset := range recordedToolsets() {`
- `server_test.go:309-313` 注释里「并发写入必须由锁保护」措辞改为「由 SafeMap 容器层承接」。

- [ ] **Step 3: 跑测试**

```bash
cd mcp && go test -race -count=2 ./...
```
Expected: PASS ×2（并发构造测试 + README 对账 + `-count=2` 幂等）。

- [ ] **Step 4: Commit**

```bash
git add mcp/server.go mcp/server_test.go mcp/readme_tools_test.go
git commit -m "refactor(mcp): toolRegistry 迁 container.SafeMap——第 6 处注册表收编，Phase 1 手写互斥删除"
```

---

### Task 6: logtail → bufiox.TailLines 委托（核实 #198 存档背离）

**Files:**
- Modify: `backends/internal/backendutil/logtail.go:15-43`
- Test: `backends/internal/backendutil/logtail_test.go`（既有，语义之 spec）

**Interfaces:**
- Produces: `TailOfReader(r io.Reader, budget int) (string, bool, error)`——签名与 `bufiox.TailLines` 逐字同形（已 `go doc` 核实）。

- [ ] **Step 1: 核实存档背离**

```bash
git log --oneline -8 -- backends/internal/backendutil/logtail.go
git log -S "bufiox" --oneline -- backends/internal/backendutil/logtail.go
```
Expected: 查明 #198 收编是否曾落地后被回退、还是从未落到此文件——结论写进报告（T12 据此修正 AGENTS.md #198 条目措辞）。

- [ ] **Step 2: 委托**

```go
// TailOfReader 读流并保留尾部 budget 字节。
// 委托 bufiox.TailLines（pkg#198 收编）：行粒度、头部按行丢弃、
// 单行超预算整行保留、\r\n 归一——语义单源在上游，此处仅保留
// CI 日志线的领域入口名。
func TailOfReader(r io.Reader, budget int) (string, bool, error) {
	return bufiox.TailLines(r, budget)
}
```

删 `bufio` import，增 `github.com/oekit/pkg/bufiox`。

- [ ] **Step 3: 跑测试并按结果分流**

```bash
go test -race -count=1 ./backends/internal/backendutil/
```
- PASS → Step 4。
- FAIL → 逐条读失败断言：若上游语义（分隔符不计预算、CRLF 归一等——TailLines 文档明载的固化口径）与测试意图一致而本地旧实现只是未同步，**更新测试对齐上游并在提交信息论证**（规则二：改断言须论证旧 shape 为何错）；若上游丢失测试钉住的本地行为，**回滚本 Task、保留本地实现**，并在报告记 ruling「#198 存档不实，本地实现维持」。两种结局都必须在报告里写明。

- [ ] **Step 4: Commit**

```bash
git add backends/internal/backendutil/logtail.go backends/internal/backendutil/logtail_test.go
git commit -m "refactor(ci): TailOfReader 委托 bufiox.TailLines——#198 收编落地核实并补齐"
```

---

### Task 7: wait.go 定时器卫生 + retry.Poll 评估采纳

**Files:**
- Modify: `provider/wait.go:59-109`（WaitForCommitStatus）、`:153-177`（initWait 加返回值）
- Test: `provider/commitstatus_test.go`（25 处引用，语义之 spec）

**Interfaces:**
- Consumes: `retry.Poll(ctx, interval, timeout, cond func() (bool, error)) error`（已 `go doc`：cond 立即首跑、错误中止透传、超时 `retry.ErrTimeout`、ctx 取消透传）；`retry.ErrTimeout`。
- Produces: `WaitForCommitStatus` 签名不变；`initWait` 改为返回 `(interval, budget, timeout, want)`（timeout = 生效的总预算时长，含默认值回填后）。

- [ ] **Step 1: initWait 返回生效 timeout**

```go
func initWait(opts WaitOptions) (interval time.Duration, budget <-chan time.Time, timeout time.Duration, want map[string]bool) {
	interval = opts.Interval
	// ...（interval 钳制段不动）
	switch {
	case opts.Timeout == 0:
		opts.Timeout = DefaultWaitTimeout
		fallthrough
	case opts.Timeout > 0:
		t := time.NewTimer(opts.Timeout)
		budget = t.C
	}
	timeout = opts.Timeout
	want = make(map[string]bool, len(opts.Contexts))
	// ...
	return interval, budget, timeout, want
}
```

`grep -n "initWait(" provider/*.go` 更新全部调用点（含非本函数的 Wait* 变体，若有）。

- [ ] **Step 2: InitialDelay 定时器卫生（必须做，独立于 Poll 评估）**

```go
	start := time.Now()
	if opts.InitialDelay > 0 {
		timer := time.NewTimer(opts.InitialDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-budget:
			timer.Stop()
			return "", ErrWaitTimedOut
		case <-timer.C:
		}
	}
```

- [ ] **Step 3: 主循环换 retry.Poll**

```go
	remaining := timeout - time.Since(start)
	if remaining <= 0 {
		return "", ErrWaitTimedOut
	}

	var last CombinedCommitStatus
	var final CommitStatusState
	err := retry.Poll(ctx, interval, remaining, func() (bool, error) {
		statuses, err := csm.ListCommitStatuses(ctx, owner, repo, sha)
		if err != nil {
			// 确定性未命中立即中止透传（错 SHA/丢权限不该烧完预算
			// 变成误导性的 ErrWaitTimedOut）；瞬时读错继续轮询。
			if oekiterrors.Is(err, ErrNotFound) {
				return false, err
			}
			return false, nil
		}
		snap, done := foldTerminalStates(statuses, want)
		if done {
			final = snap.State
			return true, nil
		}
		last = snap
		return false, nil
	})
	switch {
	case err == nil:
		return final, nil
	case oekiterrors.Is(err, retry.ErrTimeout):
		return "", oekiterrors.Wrap(ErrWaitTimedOut, "wait budget exhausted", "last_state", last.String())
	default:
		// ctx 取消 / ErrNotFound 原样透传
		return "", err
	}
}
```

删 ticker 段（`time.NewTicker`/`defer Stop`/主 for 循环）；import 增 `github.com/oekit/concurrency/retry`。

**语义对照（实现者逐条核）**：首检立即（Poll cond 首跑 ✓）；间隔轮询 ✓；ErrNotFound 立即返回 ✓；瞬时错继续 ✓；预算耗尽带 `last_state` kv ✓；ctx 取消透传 ✓；InitialDelay 期间 budget 竞速 ✓。

- [ ] **Step 4: 跑测试**

```bash
go test -race -count=1 ./provider/
```
Expected: PASS——`commitstatus_test.go` 全绿。
**FAIL 分流**：若失败暴露 Poll 与本地语义的真实出入（如 interval 起点差一拍、timeout 边界），读失败详情——能以 cond/remaining 计算修正则修；不能则**回滚到 Step 2 为止**（保留定时器卫生），报告 ruling「retry.Poll 不采纳 + 具体语义差」。

- [ ] **Step 5: Commit**

```bash
git add provider/wait.go provider/commitstatus_test.go
git commit -m "refactor(wait): WaitForCommitStatus 轮询环收编 retry.Poll + InitialDelay 定时器卫生"
```

（回滚结局则改提交信息为 `fix(wait): InitialDelay time.After 定时器泄漏清理——retry.Poll 评估不采纳（语义差记档）`，只含 wait.go 定时器段。）

---

### Task 8: ttlcache flights → SafeMap + gen → atomic（强回滚门）

**Files:**
- Modify: `provider/ttlcache.go`（结构体、getOrBuild flight 段、remove/purge）
- Test: `provider/ttlcache*_test.go` 全部（含 #135 世代、32 并发 1 构造）

**Interfaces:**
- Consumes: `container.NewSafeMap[K,V]() *SafeMap[K,V]`；`SafeMap.GetOrSet(k, v) (V, bool)`（bool=本已存在）；`SafeMap.Delete(k)`；`sync/atomic.Uint64`（Load/Add）。
- Produces: `ttlCache[V]` 内部结构变化——**对外 API 与行为零变化**（#353：自建 flight 保留，只换容器）。

- [ ] **Step 1: 结构体改造**

```go
type ttlCache[V any] struct {
	inner *cache.LocalCache[V]
	sf    *cache.SingleFlightCache[V]

	// 按 key 收敛的回源 flight（赢家构造、等待者共享）。容器层互斥
	// 取代手写 flightsMu（oekit 收编；自建 flight 语义不变——#353
	// 存档只禁委托 SingleFlightCache，不禁换容器）。
	flights *container.SafeMap[string, *ttlFlight[V]]
	// 失效世代：remove/purge 推进（issue #135）。原子计数取代与
	// flights 共用的互斥段——捕获/比较语义与锁版逐点等价。
	gen atomic.Uint64

	mu       sync.Mutex
	baseline cache.Stats
}
```

`newTTLCache` 构造处增 `flights: container.NewSafeMap[string, *ttlFlight[V]]()`；删 `flightsMu sync.Mutex` 与 `flights map[...]` 字段；import 增 `sync/atomic`、`github.com/oekit/pkg/container`（若未有）。

- [ ] **Step 2: getOrBuild flight 段改造**

查锁段替换为：

```go
	// 按 key 单飞：GetOrSet 原子收敛——不存在则写入新 flight 并当赢家，
	// 已存在则取既有 flight 等 done（与旧「查+存持锁一步」逐点等价）。
	f, loaded := c.flights.GetOrSet(key, &ttlFlight[V]{done: make(chan struct{})})
	if loaded {
		<-f.done
		if f.err != nil {
			var zero V
			return zero, f.err
		}
		return f.v, nil
	}
```

赢家 defer 段：`c.flightsMu.Lock(); delete(c.flights, key); c.flightsMu.Unlock()` → `c.flights.Delete(key)`（close(done) 在 Delete 之前——保持旧顺序：close 后删，窗口内新到者 GetOrSet 拿到已关闭 flight 立即返回，与旧行为一致）。其余 `c.flightsMu.Lock(); gen := c.gen; Unlock` → `gen := c.gen.Load()`；stale 段 `c.flightsMu.Lock(); stale := c.gen != gen; Unlock` → `stale := c.gen.Load() != gen`。

- [ ] **Step 3: remove/purge 改 atomic**

```go
func (c *ttlCache[V]) remove(key string) {
	c.gen.Add(1)
	_ = c.sf.Del(context.Background(), key)
}
```

purge 同款（`c.gen.Add(1)` 替换锁段）。

- [ ] **Step 4: 跑测试（回滚门）**

```bash
go test -race -count=1 ./provider/ && go test -race -count=3 -run "TTL|Flight|Gen|Purge|Remove" ./provider/
```
Expected: 全绿（#135 世代测试、32 并发 1 构造、`-count=3` 抖动下稳定）。
**任一 FAIL → 整体回滚本 Task**（`git checkout -- provider/ttlcache.go`），报告 ruling「flights 容器迁移不采纳 + 失败测试与原因」——存档维持手写互斥，不重试第二轮。

- [ ] **Step 5: Commit**

```bash
git add provider/ttlcache.go
git commit -m "refactor(provider): ttlcache flights 迁 SafeMap.GetOrSet、世代计数迁 atomic——手写互斥删除（自建 flight 保留，#353 不变）"
```

---

### Task 9: cachetest.Contract 认证 ttlcache（评估，允许否决）

**Files:**
- Create: `provider/ttlcache_contract_test.go`（若采纳）
- 无代码修改（纯评估 + 可选新测试文件）

- [ ] **Step 1: 核对套件要求**

```bash
go doc github.com/oekit/pkg/cache/cachetest Run
go doc github.com/oekit/pkg/cache/cachetest RunHitWithClock
```
读 `Contract[T]`（NewValue/Equal——已核）与 `Run` 的完整参数表（套件对被测缓存要求的接口/方法集）。

- [ ] **Step 2: 对照 ttlCache 方法集**

`grep -n "^func (c \*ttlCache" provider/ttlcache.go` 列出方法集，逐项对照套件要求。

- [ ] **Step 3: 采纳或否决**

- **匹配** → 新建 `provider/ttlcache_contract_test.go`：

```go
package provider

import (
	"testing"

	"github.com/oekit/pkg/cache/cachetest"
)

// 用上游 cachetest 套件认证 ttlCache 的读路径语义（Hit/未命中/隔离……
// 以 Step 1 查到的 Run* 变体为准逐个挂上；无时钟注入的变体优先）。
func TestTTLCache_Contract(t *testing.T) {
	// 构造 ttlCache（newTTLCache 现有签名），挂适用的 cachetest.Run* 变体。
}
```

（构造参数与变体清单以 Step 1/2 结果写实——不得留空壳提交。）跑 `go test -race ./provider/` 绿。
- **不匹配**（缺时钟注入/方法集不符/套件要求导出接口）→ 不建文件，报告记 ruling「cachetest 不适用 ttlCache：具体缺口 X」，T12 存档。

- [ ] **Step 4: Commit（仅采纳结局）**

```bash
git add provider/ttlcache_contract_test.go
git commit -m "test(provider): cachetest.Contract 套件认证 ttlcache 读路径（上游契约测试收编）"
```

---

### Task 10: SlogLogger 经 logx 重写评估（预期否决）

**Files:**
- 无（评估任务；若意外采纳则 `transport/logger.go` + `transport/logger_logx_test.go`）

- [ ] **Step 1: 评估组合路径**

现状：`SlogLogger` 值接收者（`func (l SlogLogger)`），方法直转 `*slog.Logger`，`Enabled` 手写 logx.Level→slog.Level 映射。组合方案 = `logx.NewSlogLogger(service, logx.WithHandler(l.Logger.Handler()))` 得 `logx.Logger`（结构上满足 transport.Logger——`logger_logx.go` 的 `var _ Logger = logx.Delegating("")` 已证）。

**预判约束（逐条验证）**：值接收者下每次调用都要重建 logx.Logger（每条日志分配）；存 `lx` 字段需指针接收者 = 对外 API breaking（`SlogLogger{...}` 按值实现 Logger 的既有用法断裂）；`NewSlogLogger` 需要 service 名而本类型无此概念。

- [ ] **Step 2: 落结论**

- 三条约束任一成立（预期全成立）→ 不改代码，报告记 ruling「SlogLogger 维持手写薄适配：值接收者 API 钉死，logx 组合需逐调用构造或 breaking 改指针」。
- 若发现无破绽的组合路径（例如 WithHandler 方案实测无分配/无 service 引入）→ 实现 + `logger_logx_test.go` 补断言 + 绿后提交。

- [ ] **Step 3: Commit（仅采纳结局）**

```bash
git add transport/logger.go transport/logger_logx_test.go
git commit -m "refactor(transport): SlogLogger 组合 logx.NewSlogLogger——手写级别映射删除"
```

---

### Task 11: 6 处非清理类 `_ =` 丢错误（逐处判定）

**Files:**
- Modify: `gitbackend/gogit_file.go:156`、`gitbackend/gogit_commit.go:501-505,524,648,659`、`backends/internal/backendutil/backendutil.go:38`、`transport/ratelimit.go:79`、`provider/ttlcache.go:160,170`、`backends/internal/backendutil/idcache.go:43`

**判定规则（全局）**：所在函数可返回 error → 包装传播（`oekiterrors.Wrap`）；确属 best-effort/有意忽略 → 紧邻一行注释写明**为什么**可以丢（把静默变有意）；禁止无注释裸 `_ =` 存活。

- [ ] **Step 1: 逐处读上下文并处置**

1. `gogit_file.go:156` `_ = tree.Files().ForEach(...)`：读回调是否可能返 error。回调恒 nil → 注释「回调不产生错误，For
Each 的 error 通道无意义」；回调可能返错 → 传播。
2. `gogit_commit.go` rebase 状态写入 ×N：读所在函数签名——可返回 error 则 `if err := ...; err != nil { return oekiterrors.Wrap(err, "write rebase state") }`（逐个 MkdirAll/writeRebaseStateFile）。
3. `backendutil.go:38` 请求钩子：`_ = rhCopy(ctx, req)` 后 `return nil`——钩子错误语义读 `AddRequest` 契约：钩子是 advisory → 注释「钩子失败不阻断请求（契约：advisory）」；契约要求失败即断 → 改 `return rhCopy(ctx, req)`（行为变更，测试门：backendutil 测试 + 报告标注）。
4. `ratelimit.go:79` `_ = rl.WaitContext(context.Background())`：读注释现状——限速器 reset 等待为后台语义 → 补「后台等待，错误无从处理（下一次请求会重试）」类注释。
5. `ttlcache.go:160,170` 与 `idcache.go:43` 缓存回填 `_ = c.sf.Set/Del`：缓存写 best-effort（失败仅少缓存）→ 注释「best-effort 回填：失败只影响本次命中率」。

- [ ] **Step 2: 跑测试**

```bash
go test -race -count=1 ./gitbackend/ ./backends/internal/backendutil/ ./transport/ ./provider/
```
（gitbackend 段带 `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null` 前缀。）Expected: PASS；若第 3 项改为传播钩子错误，确认 backendutil 既有测试仍绿。

- [ ] **Step 3: Commit**

```bash
git add gitbackend/gogit_file.go gitbackend/gogit_commit.go backends/internal/backendutil/backendutil.go transport/ratelimit.go provider/ttlcache.go backends/internal/backendutil/idcache.go
git commit -m "fix: 六处非清理类 _ = 丢错误逐处收口——可传播者传播、best-effort 者显式注释（规则一）"
```

---

### Task 12: 文档存档 + 全量验证 + 发版确认

**Files:**
- Modify: `AGENTS.md`（规则一新增收编条目 + 修正 #198 措辞 + 评估否决记因）
- Modify: `CHANGELOG.md`（Unreleased 段）

- [ ] **Step 1: AGENTS.md 存档**

规则一追加一段（沿既有「增量审计与收编」条目风格）：

```markdown
- **仓内扫描收编轮（2026-11-12，用户指令「尽可能使用 oekit」；扫描方法 =
  10 模式族仓内全量 × 三模块 go doc API 面 × 存档交叉）**。收编七处：
  ① shellx.Quote——GIT_SSH_COMMAND 五处 %q 改单引号化（旧注释对 $/反引号
  的保护声明不实，sh -c 下会展开；credhelper 消费线同款）；② container.
  SafeMap 第六处——mcp toolRegistry 拍平 key 迁移（Phase 1 手写互斥删除）；
  ③ 敏感 query 词表合并 15 项超集单一真源（strip 11 项与 redact 9 项互有
  出入，合并双向补齐：strip 补 password/secret/key、redact 补 jwt/session/
  signature/sig/auth/authorization——行为变更记 CHANGELOG）；④ encoding
  收编——HashToken→Sha256、FingerprintSHA256 两端 Base64（无 raw-sha256
  字节原语，sha256.Sum256 诚实保留）；⑤ stringx.SplitCSVNonEmpty +
  slicex.Filter——branchfilter 拆分样板与三处收集循环（早退谓词类
  slicex 无 Any/FindFunc 原语，保留并注明）；⑥ bufiox.TailLines——
  TailOfReader 委托（**#198 条目修正**：原存档称已收编但现场未落地，
  本轮补齐；背离原因见 git log）；⑦ retry.Poll——WaitForCommitStatus
  轮询环收编 + InitialDelay 定时器卫生（若 Task 7 回滚则改为「已评估
  不采纳 + 语义差」并删本点）。评估否决（勿再评估）：SlogLogger 值
  接收者 API 钉死（logx 组合需逐调用构造或 breaking 改指针）；
  cachetest 套件与 ttlCache 的匹配结论（Task 9 结局）；ttlCache flights
  容器迁移结论（Task 8 结局——采纳则记收编、回滚则记否决因由）。
  Tier 3 复核维持：retry.go 退避/ratelimit 节流/gitee FixedZone/examples
  日志/contracttest 空密钥 HMAC/ptrx——扫描现场与存档一致。
```

（按 Task 7/8/9 实际结局改写对应句——不得提交与代码不符的存档。）

- [ ] **Step 2: CHANGELOG Unreleased**

```markdown
## [Unreleased]

### Security
- fix(transport): 敏感 query 参数词表合并为 15 项超集单一真源——跨 host
  重定向剥离补 password/passwd/secret/key，日志掩码补 auth/authorization/
  jwt/session/signature/sig。

### Fixed
- fix(ssh): GIT_SSH_COMMAND 五处 `%q` 改 `shellx.Quote`——`sh -c` 执行面下
  `$` 与反引号不再被 shell 展开（旧双引号形态对元字符路径不安全）。
- fix: 六处非清理类 `_ =` 丢错误逐处收口（可传播者传播、best-effort 者显式注释）。

### Changed
- refactor: oekit 收编轮——HashToken/FingerprintSHA256 走 encoding、
  branchfilter 走 stringx/slicex、mcp toolRegistry 迁 container.SafeMap、
  TailOfReader 委托 bufiox.TailLines、WaitForCommitStatus 轮询收编
  retry.Poll（以 Task 7 实际结局为准）。
```

（按实际结局裁剪；Keep a Changelog 结构与仓内中文括注风格对齐。）

- [ ] **Step 3: 全量验证**

```bash
make check && make check-mcp
golangci-lint run ./...
cd mcp && golangci-lint run ./...
go tool cover -func=coverage.out | tail -1   # ≥45%
GOTOOLCHAIN=go1.27.4 govulncheck ./...
cd mcp && GOTOOLCHAIN=go1.27.4 govulncheck ./...
```
Expected: 全绿。任何失败按输出修复后再继续。

- [ ] **Step 4: 文档提交**

```bash
git add AGENTS.md CHANGELOG.md
git commit -m "docs: oekit 收编轮存档——规则一新条目（七收编+三评估结论+Tier3 复核）+ CHANGELOG 未发布段"
```

**暂停点**：打 tag `v0.130.0`（行为变更为 minor 口径）+ push 是对外动作——执行到此向用户确认后再切段、tag、push（模式同 v0.129.0）。

---

## Self-Review 记录

- Spec 覆盖：Tier1 A→T5、B→T2、C→T4、D→T1、E→T3、F→T6；Tier2 G→T7、H→T10、I→T8、J→T9；卫生→T11；存档/验证→T12。无缺口。
- 类型一致：`recordedToolsOf/recordedToolsets` 在 T5 定义、测试步引用一致；`retry.Poll` 四参签名与 T7 代码一致（go doc 实证）；`SafeMap.GetOrSet (V,bool)` 与 T8 代码一致。
- 回滚门齐备：T6/T7/T8 三个高风险 task 各有明确 FAIL 分流与 ruling 出口；T9/T10 允许否决结局。
- 已知风险：T4 词表合并可能翻出既有测试的反向断言（Step 3 已列处理）；T7 `initWait` 返回值变更需 grep 全调用点（Step 1 已列）；T2 输出形态变化无既有测试钉住（已 grep 确认，故直接补新测试）。
