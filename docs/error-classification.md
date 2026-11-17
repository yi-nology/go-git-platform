# 错误分类矩阵（provider）

以 `provider/errors.go` 的 `classifyStatusCode` 为准——**改分类先改这里**，
本表是它的逐行转写（锚定 `provider/errors.go` 的**符号名**而非行号——行号锚曾于 2026-11-14 标注、因后续演进漂移已弃用）。分类
判定一律 `errors.Is` 哨兵（规则二），禁止匹配错误消息文本；分类单源除
`classifyStatusCode` 外还有 `Wrap`/`classifyProviderStatus` 的限流改道
（见下「403 节流改道」）。

## 状态码 → 哨兵

| 状态码 | 哨兵 | 含义 | 源码 |
|---|---|---|---|
| 400 / 422 | `ErrInvalidInput` | 请求载荷被拒（issue #64：两种最常见的校验拒绝形态，此前落文本缺省致 `IsInvalidInput` 恒 false） | classifyStatusCode 400/422 分支 |
| 401 | `ErrAuthentication` | 认证失败 | classifyStatusCode 401 分支 |
| 403 | `ErrForbidden` | 服务端授权拒绝（例外：节流改道见下） | classifyStatusCode 403 分支 |
| 404 | `ErrNotFound` | 资源不存在 | classifyStatusCode 404 分支 |
| 409 | `ErrConflict` | 并发/状态冲突 | classifyStatusCode 409 分支 |
| 410 | `ErrGone` | 资源曾存在但永久消失（区别于「从未存在」的 404） | classifyStatusCode 410 分支 |
| 429 | `ErrRateLimited` | 限流 | classifyStatusCode 429 分支 |
| 500–599 | `ErrServerSide` | 服务端故障——退避后重试有意义 | classifyStatusCode 5xx 分支 |
| 其余 | 无哨兵：裸 `fmt.Errorf("HTTP %d")` | 专用哨兵缺席是设计（文本只读）——用 `ProviderError.IsStatus` 匹配，**不用** `errors.Is` | classifyStatusCode default 分支 |

函数 doc 见 `ClassifyStatus`（`classifyStatusCode` 全分支）：完整映射即上表；上表
没有专用哨兵的状态一律文本形态。

### 403 节流改道（唯一的状态码例外）

裸状态分类 403 → `ErrForbidden`，但 GitHub/Gitea 把最常见的节流信号发成
403 + `X-RateLimit-Remaining: 0` / `Retry-After`（issue #42/#50）。两条改道
路都在状态分类之上：

1. **transport 标记**：cause 实现 `rateLimitedMarker`（`IsRateLimited()
   bool`，`rateLimitedMarker` 接口）且为真 ⇒ `classifyProviderStatus` 改判
   `ErrRateLimited`（`classifyProviderStatus`），复用 429 的哨兵；
2. **平台 SDK 分类器**：`Wrap` 在状态提取后、裸状态分类前查平台注册的
   `ThrottleClassifier`（`ThrottleClassifier`），权威节流判定直接归
   `ErrRateLimited` 并复制恢复窗口（`RetryAfter`/`ResetAt`）。

钉测 `provider/errors_ratelimit403_test.go`：

- `TestWrap403WithThrottleHeadersClassifiesRateLimited`——403 + 节流头 →
  `ErrRateLimited`；
- `TestWrap403WithoutThrottleHeadersStaysForbidden`——裸 403 保持
  `ErrForbidden`（重试永不成功，不放大负载）；
- `TestWrapSDKThrottleClassifierMechanism`——SDK 分类器判节流 →
  `ErrRateLimited`、非节流 → `ErrForbidden`；
- `TestWrapWithoutThrottleClassifierKeepsStatusClassification`——未注册
  分类器的平台维持裸状态分类（403 → `ErrForbidden`）。

## 三分界（2B）

| 分界 | 哨兵 | 判定面 |
|---|---|---|
| 平台不支持 | `ErrNotImplemented` | `DivergenceStub` 台账（`IsStub` = Capability+Method+Kind==DivergenceStub 三谓词，provider/divergence.go）+ `DivergenceSuite` 锁定；`CheckOperation` 同哨兵 |
| 凭据无权限 | `ErrPermissionDenied` | 仅 `CheckOperation` 的声明面（`OperationCapable` 声明 `CredentialDenied`）；**不做 wire 猜测**——服务端 403 分不出资源策略与凭据轴 |
| 服务端拒绝 | 状态码分类面（`ErrForbidden` 等） | 上表 + 403 改道；`errors.Is` 判定 |

三者互斥由 `TestErrPermissionDeniedIsMatchable` 钉住（distinct 于
`ErrNotImplemented`/`ErrForbidden`/`ErrAuthentication`）。

## 既有行为契约清单

- **404**：status 驱动——`contracttest testIsNotFound`
  （contracttest/contracttest.go:210，httptest 404 → `IsNotFound`），
  每个后端的主套件跑。
- **400/422**：**pre-wire 校验拒绝**断言——`families_contract`
  （`assertMalformedTimestampRejected`：`ErrInvalidInput` + 0 GETs，拒绝
  发生在 wire 之前，contracttest/families_contract.go:218-243）与
  `notifications`（malformed `Since`/`LastReadAt` → `IsInvalidInput`，
  contracttest/notifications.go:99-111 起）。这些是**后端本地
  校验契约、不是 mock 状态码**：断言的是「同一畸形值每后端同拒、绝不静默
  丢过滤器」，不经过 `classifyStatusCode` 的 400/422 分支；wire 路的
  400/422 → `ErrInvalidInput` 由上表与 `provider` 单测钉住，本版不扩套件。
- **401/403/409/429/5xx**：`ErrorClassSuite`（本版新增，
  contracttest/errorclass.go——自带 httptest（`stubHandler` 不能改
  状态码），五状态经各后端真实错误链 `errors.Is` 钉到哨兵；套件前置：
  `baseCfg` 不设 RetryConfig ⇒ 429/5xx 单次尝试不被重试）。
- **操作层**：`OperationSuite`（本版新增，contracttest/operation.go
  ——台账/接口测试侧独立直查期望 → `CheckOperation` 哨兵 → stub 派生项
  行为，三层一致；测试侧期望不调 `DeriveOperationSupport`，防同义反复；
  418 Teapot 兜底任何真 wire 调用）。

## 交叉引用

- 分歧台账：[`docs/divergence-ledger.md`](divergence-ledger.md)（`DivergenceStub`
  登记与 `DivergenceSuite` 锁定面；生成物，`go generate ./...` 再生成）。
- 操作级能力：[`provider/operation.go`](../provider/operation.go)
  （`Operation` 闭集 / `OperationSupport` 三态 / `OperationCapable` /
  `DeriveOperationSupport` / `CheckOperation`）。
- 分类单源：[`provider/errors.go`](../provider/errors.go)（`ClassifyStatus` /
  `classifyStatusCode` / `classifyProviderStatus` / `Wrap`）。
