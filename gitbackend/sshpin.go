package gitbackend

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// 本文件让 native 路径的 SSH 指纹钉扎（HostKeyFingerprint）真正生效。
//
// 历史：sshHostKeyArgs 对指纹钉扎只退化为 StrictHostKeyChecking=yes +
// 已有 known_hosts，指纹本身从未被比对——而 gogit 路径（gogit.go
// hostKeyCallbackWithConfig）严格匹配。同一配置两个后端语义不一致。
//
// 现在：跑 git 前先解析本次命令要连的主机（argv 里的 URL，或经
// `git remote get-url` 解析 remote），ssh-keyscan 抓公钥、在 Go 里比对
// SHA256 指纹，匹配的行写入 0600 临时 known_hosts，git 以
// StrictHostKeyChecking 强制校验。任一主机无匹配指纹即整体失败
// （fail-closed：钉了扎就必须验过才连）。密钥本身经网络获得并立即与
// 钉扎比对，MITM 的假密钥指纹不会命中 pin，等同 TOFU-then-pin。

const keyscanTimeout = 10 * time.Second

// resolveSSHPin 为带 HostKeyFingerprint 的 SSH 认证物化临时 known_hosts。
// 返回的 AuthConfig 已把指纹转写成 KnownHostsPath（sshHostKeyArgs 相应
// 走 known_hosts 分支）；cleanup 删除临时文件。不适用钉扎时原样返回。
func (b *NativeGitBackend) resolveSSHPin(ctx context.Context, repoPath string, args []string, auth AuthConfig) (AuthConfig, func(), error) {
	noop := func() {}
	if auth.Type != AuthSSH || auth.InsecureSkipTLS || strings.TrimSpace(auth.HostKeyFingerprint) == "" {
		return auth, noop, nil
	}

	targets, applies := b.gitSSHTargets(repoPath, args)
	if !applies {
		// 非联网子命令（status/commit 等）不会碰 SSH，钉扎不参与
		return auth, noop, nil
	}
	if len(targets) == 0 {
		return auth, noop, fmt.Errorf(
			"HostKeyFingerprint 已设置，但无法从命令解析出远端主机（native 钉扎依赖 ssh-keyscan 预校验）。" +
				"请使用 ssh:// URL、可解析的 remote 名称，或改设 KnownHostsPath")
	}

	pin := normalizePin(auth.HostKeyFingerprint)
	var lines []string
	seen := map[string]bool{}
	for _, t := range targets {
		if seen[t.host] {
			continue
		}
		seen[t.host] = true
		matched, err := pinnedKnownHostsLines(ctx, t.host, t.port, pin)
		if err != nil {
			return auth, noop, err
		}
		lines = append(lines, matched...)
	}

	tmp, err := os.CreateTemp("", "git_ssh_known_hosts_*")
	if err != nil {
		return auth, noop, fmt.Errorf("ssh pin: create temp known_hosts: %w", err)
	}
	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return auth, noop, fmt.Errorf("ssh pin: write known_hosts: %w", err)
	}
	_ = tmp.Close()
	_ = os.Chmod(tmp.Name(), 0o600)

	b.logger.Info("native ssh pin verified via ssh-keyscan",
		"hosts", len(seen), "matched_keys", len(lines))

	out := auth
	out.KnownHostsPath = tmp.Name()
	out.HostKeyFingerprint = "" // 已由 pin 校验背书，走 known_hosts 分支
	return out, func() { _ = os.Remove(tmp.Name()) }, nil
}

// sshTarget 是一个待校验的主机。
type sshTarget struct {
	host string
	port int
}

// gitSSHTargets 从 git argv 与仓库 remote 配置推出本次命令将连的 SSH 主机。
// applies 表示子命令是否会联网（clone/fetch/pull/push/ls-remote）——
// 否则钉扎不适用；applies 且零 target 表示联网但主机无法解析，由
// 调用方 fail-closed。
func (b *NativeGitBackend) gitSSHTargets(repoPath string, args []string) ([]sshTarget, bool) {
	sub := gitSubcommandOf(args)

	var urls []string
	switch sub {
	case "clone", "ls-remote":
		urls = b.cloneTargetURLs(repoPath, args, sub)
	case "fetch", "pull", "push":
		urls = b.remoteTargetURLs(repoPath, args, sub)
	default:
		return nil, false
	}

	var targets []sshTarget
	for _, u := range urls {
		if host, port, ok := hostPortFromSSHURL(u); ok {
			targets = append(targets, sshTarget{host: host, port: port})
		}
	}
	return targets, true
}

// cloneTargetURLs：clone/ls-remote 的目标是 argv 里的 ssh:// URL；
// ls-remote 无 URL 时回落到仓库 remote（与 git 行为一致）。
func (b *NativeGitBackend) cloneTargetURLs(repoPath string, args []string, sub string) []string {
	pos := positionalsAfter(args)
	for _, p := range pos {
		if isSSHURL(p) {
			return []string{p}
		}
	}
	if sub == "ls-remote" && repoPath != "" {
		remote := "origin"
		if len(pos) > 0 {
			remote = pos[0]
		}
		return []string{b.remoteGetURL(repoPath, remote)}
	}
	return nil
}

// remoteTargetURLs：fetch/pull/push 的第一个 positional 是 remote 名或
// URL；fetch --all 时枚举全部 remote。
func (b *NativeGitBackend) remoteTargetURLs(repoPath string, args []string, sub string) []string {
	pos := positionalsAfter(args)
	if len(pos) > 0 && isSSHURL(pos[0]) {
		return []string{pos[0]}
	}
	remote := "origin"
	if len(pos) > 0 {
		remote = pos[0]
	}
	if sub == "fetch" && hasFlagArg(args, "--all") {
		var urls []string
		for _, r := range b.remoteList(repoPath) {
			urls = append(urls, b.remoteGetURL(repoPath, r))
		}
		return urls
	}
	return []string{b.remoteGetURL(repoPath, remote)}
}

// gitSubcommandOf 跳过前置 `-c key=value` 对后取子命令（与 sanitizeGitArgs
// 的前缀规则一致；withInsecureArgs 可能注入前置 -c）。
func gitSubcommandOf(args []string) string {
	i := 0
	for i+1 < len(args) && args[i] == "-c" {
		i += 2
	}
	if i < len(args) {
		return args[i]
	}
	return ""
}

// positionalsAfter 返回子命令之后所有不以 `-` 开头的参数（可能混入
// 带值 flag 的值，如 `--filter blob:none` 的 blob:none，由 isSSHURL 的
// 严格性兜底过滤）。
func positionalsAfter(args []string) []string {
	i := 0
	for i+1 < len(args) && args[i] == "-c" {
		i += 2
	}
	if i >= len(args) {
		return nil
	}
	var pos []string
	for _, a := range args[i+1:] {
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
		}
	}
	return pos
}

func hasFlagArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// remoteGetURL 读仓库 remote 的 URL；失败返回空串（由 hostPortFromSSHURL
// 判定不适用）。
func (b *NativeGitBackend) remoteGetURL(repoPath, remote string) string {
	if repoPath == "" {
		return ""
	}
	//nolint:gosec // G204: 与 runGit 同界——repoPath 由库调用方给定，
	// remote 名是经 sanitizeGitArgs 白名单后的 argv positional，仅只读查询
	out, err := exec.Command(b.gitPath, "-C", repoPath, "remote", "get-url", remote).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (b *NativeGitBackend) remoteList(repoPath string) []string {
	if repoPath == "" {
		return nil
	}
	//nolint:gosec // G204: 同 remoteGetURL——只读查询，参数同界
	out, err := exec.Command(b.gitPath, "-C", repoPath, "remote").Output()
	if err != nil {
		return nil
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			names = append(names, l)
		}
	}
	return names
}

// isSSHURL 判断参数是否像 ssh:// 或 scp 风格（user@host:path）的 git 地址。
// 保守：host 必须含点/为 localhost/为 IPv6 字面量，避免把 `--filter blob:none`
// 这类 flag 值误认成主机名。
func isSSHURL(s string) bool {
	_, _, ok := hostPortFromSSHURL(s)
	return ok
}

// hostPortFromSSHURL 解析 ssh:// 与 scp 风格地址的 host:port。
// 非 SSH 地址（https://、本地路径、不含主机的 scp 形态）返回 ok=false。
func hostPortFromSSHURL(raw string) (string, int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, false
	}
	if strings.HasPrefix(raw, "ssh://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", 0, false
		}
		port := 22
		if p := u.Port(); p != "" {
			if n, err := strconv.Atoi(p); err == nil {
				port = n
			}
		}
		if !plausibleHost(u.Hostname()) {
			return "", 0, false
		}
		return u.Hostname(), port, true
	}
	if strings.Contains(raw, "://") { // 其他 scheme（https/git/file…）不属 SSH 钉扎
		return "", 0, false
	}
	// scp 风格：[user@]host:path（scp 语法无端口）
	at := strings.LastIndex(raw, "@")
	rest := raw
	if at >= 0 {
		rest = raw[at+1:]
	}
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return "", 0, false
	}
	host := rest[:colon]
	if !plausibleHost(host) {
		return "", 0, false
	}
	return host, 22, true
}

// plausibleHost 过滤明显不是主机名的 positional（flag 值、纯单词等）。
// IPv6 字面量含冒号（仅 ssh:// 形态会到这里）。
func plausibleHost(host string) bool {
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasPrefix(host, "[") || strings.Contains(host, ":") {
		return true
	}
	return strings.Contains(host, ".")
}

// pinnedKnownHostsLines 对 host:port 执行 ssh-keyscan，只保留指纹匹配 pin
// 的 known_hosts 行。零匹配 = 指纹不匹配（可能 MITM / pin 过期），报错。
func pinnedKnownHostsLines(ctx context.Context, host string, port int, pin string) ([]string, error) {
	keyscan, err := exec.LookPath("ssh-keyscan")
	if err != nil {
		return nil, fmt.Errorf("HostKeyFingerprint 已设置但找不到 ssh-keyscan（native 钉扎依赖它预校验）: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, keyscanTimeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, keyscan, "-T", "5", "-p", strconv.Itoa(port), host).Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("ssh-keyscan %s:%d: %w", host, port, err)
	}
	return filterPinnedLines(string(out), pin, host, port)
}

// filterPinnedLines 只保留指纹匹配 pin 的 known_hosts 行（keyscan 原始
// 输出中的 host 字段形式原样保留，ssh 查找语义不变）。零匹配报错。
func filterPinnedLines(raw, pin, host string, port int) ([]string, error) {
	var matched []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		blob, err := base64.StdEncoding.DecodeString(fields[2])
		if err != nil {
			continue
		}
		key, err := ssh.ParsePublicKey(blob)
		if err != nil {
			continue
		}
		if ssh.FingerprintSHA256(key) == pin {
			matched = append(matched, line)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf(
			"ssh host key fingerprint mismatch for %s:%d: 扫描到的主机密钥均不匹配钉扎指纹 %s（可能 MITM 或 pin 过期）",
			host, port, pin)
	}
	return matched, nil
}

// normalizePin 统一为 SHA256:xxx 形式（与 gogit 路径的钉扎格式一致）。
func normalizePin(fp string) string {
	fp = strings.TrimSpace(fp)
	if fp == "" || strings.HasPrefix(fp, "SHA256:") {
		return fp
	}
	return "SHA256:" + fp
}
