//go:build e2e

// Package e2e 是 gitea/forgejo 沙箱写 roundtrip（3 期 T4，spec §3.4）。全部
// 文件挂 //go:build e2e 构建标签：默认面（go build/test/vet ./...、make check）
// 不编译本包，只有 make e2e 与 CI e2e.yml 显式 -tags=e2e 才进。
//
// 双场合共用同一套测试代码，差异只在容器编排，以 env 契约切换：
//
//   - CI 模式：E2E_GITEA_URL/E2E_FORGEJO_URL + 对应 E2E_*_TOKEN 已注入
//     （e2e.yml 的 services 已由编排层完成就绪与管理员引导）→ 直接复用，
//     不自起容器；
//   - 本地模式：env 未给 → harness 用 docker run 起一次性官方镜像
//     （版本钉见 Makefile GITEA_IMAGE/FORGEJO_IMAGE，可 env 覆盖）、
//     轮询 /api/v1/version 就绪、docker exec 引导 admin、API 换 token，
//     t.Cleanup 兜底 docker rm -f；失败时 t.Logf 容器日志尾部；
//   - 两头都没有（无 env 且无 docker）→ t.Skip（带原因），不红。
//
// 红线（spec §2）：写操作只发生在一次性沙箱容器内；本包绝不指向真实平台。
// 镜像凭据仅存在于进程内/临时容器，仓库零 secrets（spec §5：E2E 不需要
// secrets，本地/CI 一次性容器自建 token）。
//
// 注意事项：本地 docker 模式下 `t.Cleanup` 在 go test 进程被 kill 时无法
// 回收容器（残留 `e2e-*-<rand>` 需手动 `docker rm`；CI services 场合随
// runner 自清）。`e2e.yml` 的触发路径含 `runtime/**`——超出 spec §3.4
// 原文清单（roundtrip 钉 `runtime.Ensure*`，有意扩展）。
package e2e
