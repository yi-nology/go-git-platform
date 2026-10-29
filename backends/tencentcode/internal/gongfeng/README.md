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

升级方式：同步 fork 上游提交后整体覆盖本目录，并保持本 README 与 LICENSE。
