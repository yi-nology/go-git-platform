# gongfeng（收编的腾讯工蜂 REST SDK）

本目录是 [zy84338719/gongfeng-sdk-go](https://github.com/zy84338719/gongfeng-sdk-go)
main 分支的收编副本（Apache License 2.0，见 [LICENSE](LICENSE)），替代原外部依赖
`github.com/studyzy/gongfeng-sdk-go v0.6.0`。

收编原因：studyzy 上游删除了 v0.6.0 tag，外部引用不再可靠；该 fork 在 v0.6.0
基础上做了一批“对齐官方文档”的修正（Release 改 ID 寻址、commit status /
merge request 路径修正、移除官方 API 不存在的 SearchProjects/DeleteIssue、
补齐 review_events/token 等字段），并带有基于 `GONGFENG_TOKEN` 的真实环境
集成测试验证。

目录内容：

- `*.go` — SDK 源码与单元测试（`integration_test.go` 需要环境变量
  `GONGFENG_TOKEN`，未设置时自动跳过，不会在 CI 触网）。
- `docs/api/` — 官方 API 文档镜像（https://code.tencent.com/help/api ），
  是本 SDK wire shape 的权威依据；上游删 tag 的教训同样适用于文档，一并留存。
- 模块自身的 `go.mod` / `go.sum` / Makefile / lint 配置未收编：本目录并入
  go-git-platform 主模块，第三方依赖仅剩 `github.com/google/go-querystring`
  （主模块原本就有的间接依赖，收编后转为直接依赖）。

## 本仓适配层（升级 fork 时需重新套用）

收编基线之上叠加了少量 oekit 化适配，与上游 fork 存在以下**有意偏离**，
整体覆盖升级后需对照重新套用：

1. **错误面走 oekit/pkg errors**：`fmt.Errorf` 的带格式构造（含 `%w`）改为
   `oekiterrors.Errorf`（`setBaseURL` / `parseID` / `Time.UnmarshalJSON`），
   静态错误改为 `oekiterrors.New`（`NewClient` 的 token 校验）。
2. **日志注入**：`Client` 增加可选 `provider.Logger`（kv 风格，默认 noop），
   经 `WithLogger` 注入；`Do` 记录请求 debug 行（method/path/status/耗时）
   与失败 error 行。这是本仓"SDK 包面向 provider.Logger 注入"约定的落地，
   oekit 生态消费方经 `provider.NewLogxLogger("gongfeng")` 接入。
3. **并发约定**：SDK 保持无 goroutine 的同步设计——有界并发由 provider 层
   的批量引擎（`provider.GetFileContents` 等，oekit/concurrency
   syncx.FanInLimitCtx 驱动）承担，SDK 内不引入 async。

升级方式：同步 fork 上游提交后整体覆盖本目录，重新套用上述适配层，并保持
本 README 与 LICENSE。
