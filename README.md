# go-git-platform

A unified Go SDK for 7 Git hosting platforms — plus local git operations, CI-failure diagnostics, and an MCP server — through a single interface.

[English](#english) | [中文](#简介)

[![Go Reference](https://pkg.go.dev/badge/github.com/yi-nology/go-git-platform.svg)](https://pkg.go.dev/github.com/yi-nology/go-git-platform)
[![CI](https://github.com/yi-nology/go-git-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/yi-nology/go-git-platform/actions/workflows/ci.yml)
[![Release](https://github.com/yi-nology/go-git-platform/actions/workflows/release.yml/badge.svg)](https://github.com/yi-nology/go-git-platform/actions/workflows/release.yml)
[![Latest Release](https://img.shields.io/github/v/release/yi-nology/go-git-platform?include_prereleases)](https://github.com/yi-nology/go-git-platform/releases)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)

## English

### Overview

`go-git-platform` gives you one Go interface for **GitHub, GitLab, Gitea, Forgejo, Gitee, GitCode, and Tencent Code (工蜂)** — repositories, change requests, webhooks, files, releases, and more — plus a local git backend (`gitbackend`) for clone/fetch/push/merge workflows, agent-oriented primitives (wait-for-CI, batched reads, field projection), and a ready-to-use [MCP server](mcp/README.md).

**Highlights**

- **Unified transport layer** — one auth/retry/hooks/rate-limit/logging pipeline for every platform; third-party SDKs (go-github, gitlab client-go, …) plug in via `http.RoundTripper`
- **Capability-gated optional APIs** — 17 optional capability interfaces declared via `Capabilities()`; absence is expressed by not declaring, never by stub methods
- **CI failure diagnostics** — `CILogManager` (GitLab) returns failed jobs with the tail of their execution logs, sized for LLM consumption
- **Agent/automation primitives** — `WaitForCommitStatus` (CI gates), bounded-concurrency batch file reads, page-walking iterators (`Each`/`Collect`), field `projection` to shrink payloads before feeding LLMs
- **Rotating credentials** — `Config.TokenSource` supplies the access token per request, so expiring credentials (GitHub App installation tokens, OAuth) rotate without rebuilding the provider; the `githubapp` package mints the RS256 JWT and its caching `InstallationTokenSource` exchanges it once per hour
- **Conditional requests** — opt-in `Config.ConditionalRequests` sends `If-None-Match` and replays cached 304s as 200s; on GitHub, 304s don't count against the rate-limit budget
- **Idempotent ensure-helpers** — `EnsureWebhook` / `EnsureBranchProtection` / `EnsureDeployKey` converge desired state (created/updated/unchanged), safe to re-run
- **Webhook event corpus** — per-backend golden fixtures pin the normalized event vocabulary (`cr./push/tag./branch./issue./comment.`) across all seven platforms
- **Secure by default**
  - HTTPS tokens reach git via an **ephemeral credential helper** — never in argv or the child process environment, never persisted to the host credential store
  - SSH **host-key fingerprint pinning** enforced on both git backends (keyscan pre-verification, fail-closed)
  - Constant-time webhook signature comparison; SHA-256 token hashing in cache keys
- **Contract tests + divergence ledger** — a cross-platform suite keeps behavior consistent; every deliberate departure is machine-readably registered and rendered to [docs/divergence-ledger.md](docs/divergence-ledger.md)

### Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    _ "github.com/yi-nology/go-git-platform/backends/all" // registers all platforms
    "github.com/yi-nology/go-git-platform/provider"
)

func main() {
    p, err := provider.NewProvider(provider.Config{
        Platform: provider.PlatformGitHub,
        Token:    "ghp_...",
    })
    if err != nil {
        log.Fatal(err)
    }

    repos, err := p.ListRepos(context.Background(), provider.ListRepoOptions{
        Owner: "my-org",
    })
    if err != nil {
        log.Fatal(err)
    }
    for _, r := range repos {
        fmt.Println(r.FullName, r.Stars, r.Archived) // repo metadata for import filtering
    }
}
```

### Installation

```bash
go get github.com/yi-nology/go-git-platform
```

### Capability matrix

`Provider` composes 8 core sub-interfaces (repos / change requests / webhooks / branches / diffs / commits / files / releases). Optional capabilities are declared via `Capabilities()` and consumed via type assertion:

```go
if caps := p.Capabilities(); caps.Reviews {
    rm := p.(provider.ReviewManager)
    _ = rm
}
```

| Capability | GH | GitLab | Gitea | Forgejo | Gitee | GitCode | Tencent | Interface |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|---|
| Issues | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `IssueManager` |
| Search | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — | `SearchManager` |
| Labels | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `LabelManager` |
| Milestones | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `MilestoneManager` |
| Reviews | ✅ | ✅ | ✅ | ✅ | — | ✅ | ✅ | `ReviewManager` |
| CommitStatuses | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `CommitStatusManager` |
| Notifications | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — | `NotificationManager` |
| Reactions | ✅ | ✅ | ✅ | ✅ | — | ✅ | — | `ReactionManager` |
| BranchProtections | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — | `BranchProtectionManager` |
| Collaborators | ✅ | — | ✅ | ✅ | ✅ | ✅ | — | `CollaboratorManager` |
| DeployKeys | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — | `DeploymentKeyManager` |
| RepoStats | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `RepoStatsManager` |
| Users | — | ✅ | — | — | — | — | ✅ | `UserManager` |
| Gists | ✅ | — | — | — | — | — | — | `GistManager` |
| Starred | ✅ | — | — | — | — | — | — | `StarredManager` |
| Migrations | ✅ | — | — | — | — | — | — | `MigrationManager` |
| ReleaseAssets | ✅ | — | — | — | — | — | — | `ReleaseAssetManager` |
| CI failure logs | — | ✅ | — | — | — | — | — | `CILogManager` (type-assert only) |

The matrix stays honest: `examples/capabilities` probes every declared capability with read-only calls (`PLATFORM=gitea PLATFORM_TOKEN=xxx OWNER=o REPO=r go run ./examples/capabilities`).

### Local git operations

`gitbackend` wraps local git behind two interchangeable backends — `native` (shells out to `git`, fullest feature set incl. rebase/stash) and `gogit` (pure Go via go-git/v5, no git binary needed). See the [中文 · Git 后端操作](#git-后端操作) section for the auth security model (token credential helper, SSH fingerprint pinning) and partial-clone options.

### Documentation

- [docs/divergence-ledger.md](docs/divergence-ledger.md) — every registered cross-platform divergence
- [docs/v1.0-readiness.md](docs/v1.0-readiness.md) — roadmap toward 1.0
- [mcp/README.md](mcp/README.md) — the MCP server module
- The 中文 section below is the complete reference (platform detection, provider manager, webhook validation, known limitations, project layout, development)

---

## 简介

`go-git-platform` 用一套统一 Go 接口操作 **GitHub / GitLab / Gitea / Forgejo / Gitee / GitCode / 腾讯工蜂** 七个 Git 托管平台,并提供本地 Git 操作双后端(`gitbackend`)、CI 失败诊断、面向 Agent 的原语(门禁等待/批量读取/字段投影)与开箱即用的 MCP server。写一次,跑在任何平台。

### 架构亮点

- **统一传输层** (`transport/`): 所有平台共享 auth/retry/hooks/rate-limit/logger 管道, 第三方 SDK (go-github, gitlab client-go 等) 通过 `http.RoundTripper` 包装接入
- **按平台拆包** (`backends/<platform>/`): 每个平台独立包, 按职责拆文件 (repos/crs/webhooks/branches/commits/files/diffs/releases)
- **契约测试** (`backends/contracttest/`): 跨平台统一测试套件, 确保接口行为一致
- **分歧台账** (`Divergence`): 每个后端把与统一语义的偏离 (stub/ignore/mapping/detour) 机器可读地登记, 由 `Provider.Divergences()` 与 `provider.Ignores` 等谓词暴露; 渲染文档在 [docs/divergence-ledger.md](docs/divergence-ledger.md), 编辑后用 `go generate ./...` 再生成。契约套件会在台账与实际行为漂移时失败
- **错误归一** (`provider.ProviderError`): 自动从 4 种来源 (StatusCode 方法/字段, `*http.Response` 字段, 错误字符串) 提取 HTTP 状态码
- **主动限流** (`transport.RateLimiter`): 跟踪 `X-RateLimit-*` 响应头, 在撞限前自适应节流 (含并发预约定, 避免惊群); 限流类错误携带 `Retry-After`/`ResetAt` 恢复窗口 (`provider.RateLimitRecovery`)
- **可刷新凭证** (`Config.TokenSource`): transport 层每请求取 token, GitHub App installation token / OAuth 轮换类凭证下一次请求即生效; 顶层 `githubapp` 包提供 JWT 铸造 + installation token 换取, 可直接作为 TokenSource 接入
- **ETag 条件请求** (`Config.ConditionalRequests`, 默认关): GET 自动 `If-None-Match`, 304 透明回放缓存的 200 —— 轮询型负载在 GitHub 上不消耗限流配额
- **期望态助手**: `EnsureWebhook` / `EnsureBranchProtection` / `EnsureDeployKey` 幂等收敛 (created/updated/unchanged)
- **泛型分页迭代器**: `provider.Each` / `Collect`(带 Bounded 变体)逐页惰性遍历, 空页终止 + 页预算硬错误, 不再手写 page++ 循环
- **版本可观测**: `provider.Version()` + 默认 `User-Agent: ...go-git-platform/<ver>` 产品标识; `transport/metrics` 零依赖观测接口 (Recorder/ResponseHook/ClassifyPath)
- **安全默认**:
  - HTTPS 令牌经**临时 credential helper** 注入 git —— 不进 argv、不进子进程环境变量、绝不落盘到主机凭证库 (钥匙串/~/.git-credentials)
  - SSH **主机密钥指纹钉扎** 在两个后端都真正生效 (native 走 keyscan 预校验 + 临时 known_hosts, fail-closed; gogit 内存中直接比对)
  - Webhook 签名常数时间比较; 缓存键使用 `SHA256(token)[:16]` 不泄露原 token

## 安装

```bash
go get github.com/yi-nology/go-git-platform
```

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "log"

    _ "github.com/yi-nology/go-git-platform/backends/all" // 注册所有平台
    "github.com/yi-nology/go-git-platform/provider"
)

func main() {
    ctx := context.Background()

    // 方式 1: 自动检测平台
    result, err := provider.DetectPlatform("https://github.com/owner/repo.git")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("检测到平台: %s\n", result.Platform)

    // 方式 2: 手动指定平台
    p, err := provider.NewProvider(provider.Config{
        Platform: provider.PlatformGitHub,
        Token:    "your-token",
    })
    if err != nil {
        log.Fatal(err)
    }

    repo, err := p.GetRepo(ctx, "owner", "repo")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("仓库: %s (stars=%d, fork=%v, archived=%v, 语言=%s)\n",
        repo.FullName, repo.Stars, repo.Fork, repo.Archived, repo.Language)
}
```

> **重要**: 必须空白导入 `backends/all` 才能注册所有平台后端。
> 只需要特定平台时可单独导入, 例如 `_ "github.com/yi-nology/go-git-platform/backends/github"`。

## 平台检测

SDK 支持自动检测远程 URL 对应的平台:

```go
// HTTPS URL
result, _ := provider.DetectPlatform("https://github.com/owner/repo.git")
// result.Platform == provider.PlatformGitHub

// SSH URL
result, _ = provider.DetectPlatform("git@gitlab.com:owner/repo.git")
// result.Platform == provider.PlatformGitLab

// 自托管实例
result, _ = provider.DetectPlatform("https://my-gitea.example.com/owner/repo.git")
// result.Platform == provider.PlatformGitea (默认)
```

## API 使用

### Provider Manager(带缓存 + 统计)

```go
import "github.com/yi-nology/go-git-platform/provider"

// 缓存 30 分钟过期, 最多 100 个 provider
mgr := provider.NewManager(30*time.Minute, provider.WithMaxSize(100))

// 后台 janitor 每 5 分钟清理过期条目
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
mgr.StartJanitor(ctx, 5*time.Minute)
defer mgr.Stop()

// 通过 URL 自动检测平台并获取 Provider
p, err := mgr.GetByURL("https://github.com/owner/repo.git", "your-token")

// 缓存统计与手动管理
stats := mgr.Stats()  // Hits/Misses/Size
mgr.Purge()           // 清空
mgr.Cleanup()         // 清除过期条目
```

**缓存键安全性**: Manager 使用 `SHA256(token)[:16]` 作为缓存键的一部分, 不会在内存或日志中泄露原始 token。

### Provider 接口

Provider 接口由 8 个子接口组合而成, 消费者可以只依赖需要的子接口:

```go
type Provider interface {
    Platform() Platform
    TestConnection(ctx context.Context) (*TestConnectionResult, error)
    Capabilities() CapabilitySet

    RepoManager          // ListRepos, GetRepo, DeleteRepo, UpdateRepo, ForkRepo
    ChangeRequestManager // CreateCR, GetCR, ListCRs, MergeCR, CloseCR, ReopenCR, UpdateCR, ...
    WebhookManager       // CreateWebhook, DeleteWebhook, ListWebhooks, ParseWebhookEvent, ...
    BranchManager        // ListBranches, CreateBranch, DeleteBranch
    DiffManager          // GetCRDiff, GetCRFiles, CreateNote, DeleteNote, CreateDiscussion
    CommitManager        // GetCommit, ListCommits, CompareCommits
    FileManager          // GetFileContent, CreateFile, UpdateFile, DeleteFile
    ReleaseManager       // ListTags, ListReleases, CreateRelease, GetReleaseByTag, UpdateRelease, DeleteRelease, GetArchive
}
```

### 可选能力(17 项 + CI 日志)

可选能力不进入 `Provider` 组合, 调用方通过 `Capabilities()` 声明式判断(或直接类型断言):

```go
caps := p.Capabilities()

if caps.Reviews {
    rm := p.(provider.ReviewManager)
    reviews, err := rm.ListReviews(ctx, "o", "r", "1")
    _ = reviews
    _ = err
}

// CI 失败日志(GitLab): 拿到最近一次 pipeline 的失败作业 + 日志尾部
if cl, ok := p.(provider.CILogManager); ok {
    logs, _ := cl.ListFailedJobLogs(ctx, "o", "r", "commit-sha")
    for _, j := range logs {
        fmt.Println(j.Stage, j.Name, "truncated:", j.Truncat)
    }
}
```

完整的能力×平台矩阵见 [英文 Capability matrix](#capability-matrix)(与 `Capabilities()` 声明一一对应, `examples/capabilities` 是其可运行巡检版本)。要点:

- **CILogManager 只在 GitLab 实现**: GitLab 是唯一公开暴露逐作业日志的平台; GitHub check-runs 只有摘要/注解, 缺失以"不实现该接口"表达而非桩方法
- **Gitee 不声明 Reviews / Reactions / Users**; **GitHub 不声明 Users**(走独立 REST 即可); **腾讯工蜂不声明 Search / Notifications / Reactions / BranchProtections**(分支保护在工蜂专属接口中, 见下)

### 自动化与 Agent 场景(等待/批量/投影/MCP)

面向 CI 门禁与 LLM/agent 流水线的原语, 全部平台无关:

```go
// CI 门禁: 轮询等待提交状态到终态(错误优先级同 GitHub combined status)
state, err := provider.WaitForCommitStatus(ctx, p, "o", "r", "sha",
    provider.WaitOptions{Timeout: 10 * time.Minute, Contexts: []string{"ci/test"}})

// 批量: 有界并发读取多个文件, 单项失败不影响整批
results := provider.GetFileContents(ctx, p, "o", "r", "main", []string{
    "README.md", "go.mod", "Makefile",
}, provider.BatchOptions{Concurrency: 4})

// 投影: 把统一模型裁剪成只含选定字段, 喂给 LLM 前省上下文
doc, _ := projection.Project(cr, "number", "title", "head.ref", "state")

// 分页: 逐页惰性遍历全量列表(空页终止, 平台忽略 page 参数时页预算兜底报错)
err := provider.Each(ctx, func(ctx context.Context, page int) ([]*provider.PlatformRepo, error) {
    return p.ListRepos(ctx, provider.ListRepoOptions{Owner: "org", Page: page, PerPage: 100})
}, func(r *provider.PlatformRepo) error {
    fmt.Println(r.FullName)
    return nil // provider.ErrStopIteration 可提前止步
})

// 期望态: 幂等创建或修复 webhook, 重跑安全(created/updated/unchanged)
action, hook, err := provider.EnsureWebhook(ctx, p, provider.CreateWebhookOptions{
    Owner: "o", Repo: "r", URL: "https://ci.example.com/hook",
    Events: []string{"push", "pull_request"},
})
```

`mcp/` 子模块(独立 go module)提供开箱即用的 MCP server: 七平台一套工具面, 六个 toolset(core/crs/issues/status/search/releases)按能力接口断言门控、`--read-only` 注册期丢弃写工具、列表工具支持 `fields` 投影与 `per_page` 分页参数、`--http` 可切换 streamable HTTP 远程部署(`/mcp` 端点 + `--http-token` Bearer 门禁)。详见 [mcp/README.md](mcp/README.md)。

### 仓库元数据与部分克隆

- `PlatformRepo` 携带 `Archived` / `Fork` / `Stars` / `Language`, 供上层做导入过滤(排除归档/fork、按 star/语言裁剪)
- `gitbackend.CloneOptions` 支持 `Filter: "blob:none"`(部分克隆)与 `Submodules: true`(递归子模块), 大仓拉取显著省时省流量

### 统一 Webhook 验证

SDK 内置多种 Webhook 签名验证策略(HMAC-SHA256 / SHA1 / GitLab token / Gitee / GitCode / 工蜂), 通过注册表统一管理, 签名比较为常数时间:

```go
// 默认注册表(init 时自动注册所有平台)
err := provider.DefaultWebhookRegistry().Validate(
    provider.PlatformGitHub, r, body, secret,
)

// 自定义验证器
registry := provider.NewWebhookValidatorRegistry()
registry.Register(provider.Platform("custom"), provider.HMACSHA256Validator{Header: "X-Custom-Sig"})
```

### 配置

```go
p, err := provider.NewProvider(provider.Config{
    Platform: provider.PlatformGitHub,
    BaseURL:  "https://github.example.com/api/v3", // 可选, 用于自托管
    Token:    "your-token",
    SkipTLS:  true,                                 // 可选, 跳过 TLS 验证
    Logger:   myLogger,                             // 可选, 注入日志
    RetryConfig: &provider.RetryConfig{             // 可选, 自动重试
        MaxRetries: 3,
        BaseDelay:  500 * time.Millisecond,
    },
    Hooks: &provider.Hooks{                         // 可选, 请求/响应 Hook
        Response: []provider.ResponseHook{
            func(ctx context.Context, req *http.Request, resp *http.Response, d time.Duration, err error) {
                log.Printf("%s %s %d %v", req.Method, req.URL.Path, resp.StatusCode, d)
            },
        },
    },
})
```

**Retry/Hooks/Logger 对所有平台生效** (包括使用第三方 SDK 的 GitHub/GitLab/Gitea/Forgejo), 因为它们都通过 `transport.RoundTripper` 包装。

`Config` 还携带两个 v0.72 引入的开关:

```go
p, err := provider.NewProvider(provider.Config{
    Platform: provider.PlatformGitHub,
    // TokenSource 优先于 Token: 每请求取 token, 过期凭证轮换下一请求即生效
    TokenSource: rotatingSource,
    // ConditionalRequests: GET 携带 If-None-Match, 304 透明回放缓存的 200
    // (GitHub 上 304 不占限流配额, 轮询型负载受益)
    ConditionalRequests: true,
})
```

GitHub App 凭证两步曲已下沉为顶层 `githubapp` 包: `MintJWT`(RS256 签名) + `FetchInstallationToken`(换短期 installation token), 配合 `Config.TokenSource` 即为完整的每小时轮换链路。

## Git 后端操作

`gitbackend` 提供本地 Git 仓库的底层操作 (Fetch/Push/Clone/状态/Diff/分支/标签/文件/Stash/Rebase…), 双后端实现, 工厂自动选择:

| 后端 | Type | 说明 |
|------|------|------|
| 原生 git | `"native"` | 调用本地 `git` 命令, 功能最全 (支持 Rebase/Stash/CherryPick/RunRaw) |
| go-git | `"gogit"` | 纯 Go 实现 (基于 go-git/v5), 无需 git 二进制, 部分高级操作返回 `ErrNotSupported` |

直接使用 go-git 的调用方可通过 `gitbackend.TransportAuth(auth)` 把同一套 `AuthConfig`(含 SSH 指纹钉扎)映射为 `transport.AuthMethod`, 与本包后端共享认证语义。

```go
import "github.com/yi-nology/go-git-platform/gitbackend"

// 显式指定后端; 留空自动选择 (优先 native, 回退 gogit)
backend, _ := gitbackend.NewGitBackend(gitbackend.Options{Type: "native"})
```

### 认证方式 (SSH / HTTPS / 跳过 SSL)

```go
// 1) HTTPS Token
auth := gitbackend.NewTokenAuth("your-access-token")

// 2) HTTP Basic
auth := gitbackend.NewHTTPBasicAuth("user", "pass")

// 3) SSH 私钥文件
auth := gitbackend.NewSSHKeyFileAuth("/home/user/.ssh/id_ed25519", "passphrase")

// 4) SSH 私钥内容 (适合 DB 存储的 key)
auth := gitbackend.NewSSHKeyContentAuth(pemContent, "passphrase")

// 跳过 TLS (显式不安全)
auth.InsecureSkipTLS = true
```

### 认证安全模型 (native 后端)

- **HTTPS 令牌不进 argv、不进环境变量明文**: 令牌写入 `0600` 临时文件, git 经一次性 credential helper + `GIT_ASKPASS` 读取, 会话结束整目录删除(RAII)。注入前先以空串 `credential.helper=` 清空主机已有 helper 列表(osxkeychain/store 等)——否则个人凭证会遮蔽本次令牌, 且认证成功后 store 会把一次性令牌**持久化进用户钥匙串**(两者均实测钉死为集成测试)
- **SSH 指纹钉扎 fail-closed**: 设置 `auth.HostKeyFingerprint = "SHA256:..."` 后, 跑 git 前先用 `ssh-keyscan` 抓公钥、Go 内比对指纹, 匹配才生成临时 known_hosts 放行; 解析不出主机 / keyscan 缺失 / 指纹零匹配(可能 MITM)一律直接报错。不设指纹时默认 `accept-new`(首次信任, 已知主机 MITM 必失败)
- **输出解析加固**: git 输出用 NUL/TAB 分隔解析, 提交标题/标签消息含 `|` 等任意字符不错位

### Repository 封装与部分克隆

```go
// 常规克隆 (bare=true 克成裸仓)
repo, err := gitbackend.CloneRepository(ctx, backend,
    "https://git.example.com/owner/repo.git", "/path/to/repo", auth, false)
defer repo.Close()

repo.Fetch(ctx, "main")
repo.RevParse(ctx, "HEAD")
repo.Diff(ctx, baseSHA, headSHA)

// 部分克隆 + 子模块 (大仓场景)
backend.Clone(ctx, gitbackend.CloneOptions{
    URL:         "https://git.example.com/owner/big-monorepo.git",
    Path:        "/path/to/clone",
    Auth:        auth,
    Filter:      "blob:none",   // blob 按需拉取
    Submodules:  true,          // 递归初始化子模块
})
```

## 腾讯工蜂专属能力

腾讯工蜂 backend 额外实现了 `TencentCodeExtras` 接口, 暴露工蜂独有的功能:

```go
import "github.com/yi-nology/go-git-platform/backends/tencentcode"

p, _ := provider.NewProvider(provider.Config{
    Platform: provider.PlatformTencentCode,
    Token:    "your-token",
})

if tc, ok := p.(*tencentcode.Provider); ok {
    // 原生代码评审
    review, _ := tc.CreateCodeReview(ctx, owner, repo, tencentcode.CreateCodeReviewOptions{
        Title: "code review", SourceBranch: "feature", TargetBranch: "main",
    })
    // MR 评审流程
    _ = tc.SubmitMRReview(ctx, owner, repo, 42, tencentcode.SubmitReviewOptions{
        Event: tencentcode.ReviewEventApprove, Summary: "LGTM",
    })
    // 分支保护
    _ = tc.ProtectBranch(ctx, owner, repo, "main", tencentcode.ProtectBranchOptions{})
}
```

## 已知限制

- **Gitee `ChangeRequest.Draft` 恒为 `false`**: 线上 PR 载荷有原生 `draft` 布尔字段，但 go-gitee SDK 的 `PullRequest` 模型缺该字段（上游 swagger 遗漏）， SDK 补齐前无法如实返回。
- **GitLab Reviews 是 approvals 汇总映射**（已登记， spec §4.6）: GitLab 没有 逐条 review 对象， `ListReviews`/`GetReview` 走 MR 审批状态并按审批人合成 `approved` 汇总条目（ID 均为 MR IID）； `RequestReviewers` 为登记忽略（`reviewer_ids` 需 username→ID 解析， SDK 无此面）。
- **GitLab 行内评论 diff 感知定位**: `CreateReview` 的行内评论按 MR diff hunk 映射行号, 行不在 hunk 时降级 file 级 position, 文件不在 diff 时跳过并 `Warn` 留痕(不再静默丢弃)。
- **Gitea / Forgejo 的 `REQUEST_CHANGES`/`COMMENT` 评审需要 body 或行内评论**: 两平台 SDK 的客户端校验拒绝空 body 且无评论的非 APPROVE 评审（APPROVE 豁免）。
- **Milestone 寻址语义随平台不同**: `MilestoneRef.Number` / `Milestone.Number` 在 GitHub 上是 milestone number，在 GitLab/Gitea/Forgejo/GitCode/Tencent Code 上是 milestone ID，在 Gitee 上是里程碑序号（载荷 `number` 字段）；跨平台传递 ID 不可移植。
- **CR/评审寻址已全面 string 化（迁移提示）**: `ChangeRequestManager` 与 `DiffManager` 的 number 参数、`ChangeRequest.Number` 及 `ReviewManager` 各方法均以 string 寻址，与 Issues/Milestones/Search 同一规则；数字平台内部解析，非法输入返回包裹的 `invalid pull request number` 错误。旧代码中传 `1` 的调用点改为传 `"1"`。
- **Gitee 企业版 issue 状态原样透传**: `Issue.State` 是开放字符串词表而非封闭枚举；Gitee 企业空间在 open/closed 之外还有 progressing/rejected 等工作流状态，按平台返回值原样出现在 `Issue.State` 中（已登记）。
- **Search 的 `Sort`/`Order` 走各平台自身词表**: 取值随平台不同（如 GitHub 的 stars/forks/updated + asc/desc）；Gitea/Forgejo 对未知值返回 HTTP 422，GitLab 搜索 API 无 sort/order 参数（登记忽略）。请按目标平台文档取值。
- **Tencent Code 工蜂 `Label.ID` 恒为 0**: 工蜂标签按名寻址（更新/删除经 options 携带当前名，无需 name→ID 解析扫描），gongfeng Label 模型无 id 字段，`Label.ID` 在该平台恒为 0（标签端到端按名操作）。
- **Tencent Code 工蜂 issue 三处登记限制**: `CreateIssue`/`UpdateIssue` 的 `Assignees` 经 Users API（`GET /users/{username}`）解析为 `assignee_ids`（数值用户 ID csv）后生效,每 provider 带 TTL 缓存,未知用户名报 `provider.IsNotFound`;`ListIssuesOptions.Assignee` 过滤不携带——工蜂 issue 列表端点无 assignee 过滤参数（登记忽略）；移除 issue 的最后一个标签是 no-op——空 label csv 因 `omitempty` 上不了 PUT 体，标签保留；`Issue.WebURL` 恒为空、`Issue.ClosedAt` 恒为 nil——gongfeng issue 模型无这两个字段。
- **Tencent Code 工蜂 Reviews 是 MR notes 映射（已登记）**: 工蜂原生评审以携带 `reviewer_state` verdict 的 MR note 表达，与普通 MR 评论共用同一集合——普通评论会以 `commented` 评审混入 `ListReviews`（工蜂 system 记账 note 已过滤）；读侧无 verdict 字段，`Review.State` 恒为 `commented`；`CreateReview` 的 `Comments`/`CommitID` 不映射（一条 note 至多携带一个行内位置，且无 commit 概念）；`RequestReviewers` 为登记忽略（工蜂 MR 更新面不收评审人，原生邀请端点按数值 user ID 寻址，username→ID 不可达）；`DismissReview` 为登记桩返回 `provider.ErrNotImplemented`（工蜂无评审撤销面）。

## 项目结构

```
go-git-platform/
├── provider/                    # 公共 API (类型 + 接口 + 工厂 + Manager)
│   ├── provider.go              # Provider 接口 + CapabilitySet (13 项能力声明)
│   ├── options.go               # 所有 Options/Result 类型 (集中定义)
│   ├── errors.go                # ProviderError + Wrap/New 助手 + 状态码反射
│   ├── webhook.go               # WebhookValidator 接口 + 注册表 + 策略
│   ├── manager.go               # TTL 缓存 Manager (SHA256 键 + Stats + Janitor)
│   ├── detect.go                # 平台自动检测
│   ├── factory.go / registry.go # 平台注册 + NewProvider
│   ├── pagination.go            # NormalizePageOpts + X-Total-Count 解析
│   ├── pageiter.go              # Each/Collect 泛型分页迭代器 (空页终止+页预算)
│   ├── ensure.go                # EnsureWebhook/BranchProtection/DeployKey 期望态收敛
│   ├── tokensource.go           # 可刷新 TokenSource + StaticTokenSource
│   ├── version.go               # provider.Version() (buildinfo)
│   ├── diffutil.go / stateutil.go / convertutil.go
│   ├── wait.go / batch.go       # Agent 原语: CI 门禁等待 + 有界并发批量
│   ├── commitstatus.go          # 提交状态工具
│   ├── divergence.go            # 分歧台账类型 + 谓词
│   ├── iface_ci.go              # CILogManager (可选, GitLab)
│   └── iface_*.go               # 其余 20+ 子接口 (按领域拆分)
│
├── transport/                   # 统一 HTTP 传输层
│   ├── client.go                # Client + Do/DoJSON/DoRaw + RoundTripper + AuthStrategy
│   ├── ratelimit.go             # 主动限流 (X-RateLimit-* 自适应)
│   ├── retry.go                 # 指数退避 + jitter + Retry-After + 幂等感知 + 限流 403 判定
│   ├── etag.go                  # ETag 条件请求 (If-None-Match / 304 回放)
│   ├── tokensource.go           # 每请求取 token 的 AuthStrategy
│   ├── useragent.go             # 默认 User-Agent 产品标识
│   ├── metrics/                 # 零依赖观测接口 (Recorder/ResponseHook/ClassifyPath)
│   ├── hooks.go / errors.go / logger.go
│
├── githubapp/                   # GitHub App 凭证: MintJWT + FetchInstallationToken

├── backends/                    # 平台实现 (每个独立包)
│   ├── github/                  # GitHub (go-github SDK + transport 包装)
│   ├── gitlab/                  # GitLab (client-go SDK + transport 包装, 含 CI 日志/行内评论)
│   ├── gitea/  forgejo/         # Gitea / Forgejo (各自 SDK + transport 包装)
│   ├── gitcode/                 # GitCode (go-gitcode SDK)
│   ├── gitee/                   # Gitee (go-gitee SDK + transport 包装)
│   ├── tencentcode/             # 腾讯工蜂 (transport.Client + Extras 专属能力)
│   ├── all/                     # 一行 blank import 注册所有平台
│   └── contracttest/            # 跨平台契约套件 + webhook 事件语料库 (testdata/webhooks)
│
├── gitbackend/                  # 本地 Git 操作 (native + gogit 双后端)
│   ├── native*.go               # 原生 git 后端 (argguard 白名单 + 冲突检测)
│   ├── gogit*.go                # go-git 后端
│   ├── credhelper.go            # 令牌临时 credential helper (不进 argv/env)
│   ├── sshpincache.go           # SSH 指纹钉扎 (keyscan 预校验, fail-closed)
│   └── repository.go / backend.go / iface.go
│
├── pkg/
│   ├── branchfilter/            # 分支过滤
│   ├── credential/              # 凭证管理 + AES-GCM 加密 + SSH 命令构建
│   └── projection/              # 字段投影 (LLM/agent 上下文经济)
│
├── mcp/                         # MCP server 独立模块 (独立 go.mod)
│   ├── server.go / tools_*.go   # toolsets + 能力门控 + 读写分离
│   └── cmd/go-git-platform-mcp/ # stdio 入口
│
├── docs/
│   ├── divergence-ledger.md     # 渲染后的分歧台账 (go generate 再生成)
│   └── v1.0-readiness.md        # 1.0 路线
├── examples/                    # automation/capabilities/credential/gitbackend/notification/provider/reaction/webhook
├── Makefile                     # test/lint/fmt/cover 等命令
├── .golangci.yml                # lint 配置
└── go.mod
```

## 开发

### 常用命令

```bash
make test       # 运行所有测试 (race + coverage)
make lint       # golangci-lint
make fmt        # gofmt + goimports
make vet        # go vet
make check      # CI 门禁 (vet + lint + test)
make cover      # 打印覆盖率摘要
```

测试夹具全量 hermetic: 不依赖本机 git 全局身份/配置, CI runner 裸机即可复现(本地可用 `GIT_CONFIG_GLOBAL=/dev/null go test ./...` 等价模拟)。

### 添加新平台

1. 创建 `backends/<platform>/` 目录
2. 实现 `provider.Provider` 接口 (参考 `backends/gitee/` 作为模板)
3. 添加 `init.go` 注册到 `provider.Register`
4. 在 `backends/all/all.go` 添加 blank import
5. 创建 `contract_test.go` 调用 `contracttest.Run` 验证契约
6. 登记 `divergence.go` 分歧台账并 `go generate ./...` 再渲染

### CI/CD

本项目通过 GitHub Actions 实现自动化测试与发布, 配置位于 `.github/workflows/`:

- **CI** (push/PR 到 main): golangci-lint + govulncheck + ubuntu/macos 双矩阵测试 (race + 覆盖率地板 45%)
- **Release** (推送 `v*` tag): 测试门禁 → 编译 → 自动识别预发布 → 创建 GitHub Release

```bash
git tag v0.73.0
git push origin v0.73.0
```

## 相关项目

- [go-gitcode](https://github.com/yi-nology/go-gitcode) - GitCode 专用 API 客户端

## 许可证

MIT

## 贡献

欢迎提交 Issue 和 Pull Request！
