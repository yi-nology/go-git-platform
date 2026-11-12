package gitbackend

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	oekiterrors "github.com/oekit/pkg/errors"
	"github.com/oekit/pkg/shellx"
)

type NativeGitBackend struct {
	gitPath string
	logger  Logger
}

func NewNativeGitBackend(opts Options) (*NativeGitBackend, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return nil, oekiterrors.Wrap(ErrGitNotFound, "look up git binary", "err", err)
	}
	logger := opts.Logger
	if logger == nil {
		logger = NewNoopLogger()
	}
	return &NativeGitBackend{gitPath: path, logger: logger}, nil
}

// --- Internal helpers ---

// gitSubcommands is the closed set of git subcommands this backend issues.
// args[0] must be one of them: that whitelists the command verb before any
// caller-controlled value reaches exec.
var gitSubcommands = map[string]bool{
	"add": true, "branch": true, "checkout": true, "cherry-pick": true,
	"clone": true, "commit": true, "config": true, "diff": true,
	"fetch": true, "for-each-ref": true, "init": true, "log": true, "ls-remote": true,
	"ls-tree": true, "merge": true, "merge-base": true, "pull": true,
	"push": true, "rebase": true, "remote": true, "rev-list": true,
	"rev-parse": true, "show": true, "stash": true, "status": true,
	"tag": true,
}

// gitExecFlags are git options that hand git an executable or a config
// override. A caller-controlled positional (URL, ref, pathspec) beginning
// with one of these would otherwise be parsed as a flag by git's
// interspersed option parser and turn a routine clone/fetch/push into
// arbitrary command execution or config injection.
var gitExecFlags = []string{
	"-c", "--exec", "--upload-pack", "--receive-pack", "--config-env",
}

// safeConfigOverrides matches the `-c key=value` pairs this backend issues
// itself (CommitWithIdentity identity overrides). git config keys that
// carry executables — core.sshCommand, filter.*.command, protocol.*.command,
// core.pager — are outside this set, so a smuggled `-c` can never turn into
// command execution.
var safeConfigOverrides = regexp.MustCompile(
	`^user\.(name|email)=[^=\x00-\x1f]*$|^commit\.gpgsign=(true|false)$`)

// checkGitArg validates one non-leading argument: no control characters,
// no ext:: transport URLs, none of the exec/config primitives.
func checkGitArg(arg string) error {
	for _, r := range arg {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			return oekiterrors.Wrap(ErrInvalidGitArg, "control character in argument")
		}
	}
	if strings.HasPrefix(arg, "ext::") {
		return oekiterrors.Wrap(ErrInvalidGitArg, "ext:: transport URLs are not allowed")
	}
	for _, flag := range gitExecFlags {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return oekiterrors.Wrap(ErrInvalidGitArg, "exec/config flag is not accepted from callers", "arg", arg)
		}
	}
	return nil
}

// checkNameArg rejects a caller-supplied name or rev that begins with "-".
// Git's option parser is interspersed, so a positional starting with "-" is
// read as a flag and can flip the command's semantics: CreateBranch("-D", x)
// deletes a branch, a tag "-d v1" deletes a tag, and a rev like
// --output=file in the log/diff/show family writes files. Dash-prefixed refs
// are legal per check-ref-format but ambiguous in argv, so they are refused
// explicitly (ErrInvalidGitArg) instead of being silently reinterpreted;
// where the subcommand supports it, argv also carries a `--` separator as a
// second line of defense. Empty args are allowed (empty ref = HEAD on the
// Create* APIs).
func checkNameArg(op, arg string) error {
	if strings.HasPrefix(arg, "-") {
		return oekiterrors.Wrap(ErrInvalidGitArg, "positional argument would be parsed as a git flag; names/revs must not begin with '-'", "op", op, "arg", arg)
	}
	return nil
}

// checkNameArgs runs checkNameArg over a list of caller-supplied positionals.
func checkNameArgs(op string, args ...string) error {
	for _, arg := range args {
		if err := checkNameArg(op, arg); err != nil {
			return err
		}
	}
	return nil
}

// sanitizeGitArgs enforces the exec boundary of runGit:
//
//  1. any leading `-c` flags are accepted only as pairs whose value
//     matches safeConfigOverrides (the identity overrides this backend
//     issues itself);
//  2. args[i] after those pairs must be a whitelisted git subcommand
//     (gitSubcommands);
//  3. every later argument passes checkGitArg.
//
// Library-issued flags such as --all, --tags or --abort pass through;
// withInsecureArgs prepends the library's own `-c http.sslVerify=false`
// AFTER this check.
func sanitizeGitArgs(args []string) error {
	if len(args) == 0 {
		return oekiterrors.Wrap(ErrInvalidGitArg, "empty git command")
	}
	i := 0
	for i < len(args) && args[i] == "-c" {
		value := ""
		if i+1 < len(args) {
			value = args[i+1]
		}
		if !safeConfigOverrides.MatchString(value) {
			return oekiterrors.Wrap(ErrInvalidGitArg, "-c is only accepted with a known-safe config override")
		}
		i += 2
	}
	if i >= len(args) || !gitSubcommands[args[i]] {
		return oekiterrors.Wrap(ErrInvalidGitArg, "not an allowed git subcommand", "arg", args[i])
	}
	for ; i < len(args); i++ {
		if err := checkGitArg(args[i]); err != nil {
			return err
		}
	}
	return nil
}

func (b *NativeGitBackend) runGit(ctx context.Context, repoPath string, args []string, auth AuthConfig) (string, string, error) {
	return b.runGitEnv(ctx, repoPath, args, auth, nil)
}

// runGitEnv 与 runGit 相同,但允许附加环境变量(如 GIT_AUTHOR_NAME 覆盖全局身份)。
// env 为空时沿用当前进程环境。
func (b *NativeGitBackend) runGitEnv(ctx context.Context, repoPath string, args []string, auth AuthConfig, extraEnv []string) (string, string, error) {
	if err := sanitizeGitArgs(args); err != nil {
		return "", "", err
	}
	args = withInsecureArgs(auth, args)

	//nolint:gosec // G204: args are intentionally dynamic — this wraps arbitrary git commands.
	cmd := exec.CommandContext(ctx, b.gitPath, args...)
	if repoPath != "" {
		cmd.Dir = repoPath
	}

	// resolveAuth may create a temp key file for SSHKeyContent; cleanup after run.
	resolvedAuth, cleanupKey := b.resolveAuth(auth)
	// SSH 指纹钉扎（native 路径 v0.70.0 起真正校验）：keyscan 预校验 +
	// 临时 known_hosts；fail-closed——指纹不匹配/无法校验直接报错。
	pinAuth, cleanupPin, pinErr := b.resolveSSHPin(ctx, repoPath, args, resolvedAuth)
	if pinErr != nil {
		cleanupKey()
		return "", "", pinErr
	}
	// configureAuth creates an ephemeral credential helper session for tokens;
	// cleanup removes the temp token/script dir. The helper only answers
	// credential requests whose host was declared by this invocation
	// (credentialHostsFor), so a token prepared for one host can never be
	// handed to another.
	cleanupCred, cerr := b.configureAuth(cmd, pinAuth, credentialHostsFor(args, repoPath))
	defer cleanupKey()
	defer cleanupPin()
	defer cleanupCred()
	if cerr != nil {
		return "", "", cerr
	}

	if len(extraEnv) > 0 {
		// 追加而非替换：保留 configureAuth 注入的凭证/SSH 环境
		cmd.Env = append(cmd.Env, extraEnv...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// withInsecureArgs prepends `git -c http.sslVerify=false` to args when the
// auth requests skipping TLS verification. This keeps skip-SSL handling in one
// place so every command (fetch/push/clone/pull/ls-remote/...) honors it.
func withInsecureArgs(auth AuthConfig, args []string) []string {
	if auth.InsecureSkipTLS {
		return append([]string{"-c", "http.sslVerify=false"}, args...)
	}
	return args
}

// resolveAuth handles SSHKeyContent by writing it to a temp file so that
// the native git command can use it via GIT_SSH_COMMAND. Returns the resolved
// AuthConfig (with SSHKey pointing to the temp file) and a cleanup function.
func (b *NativeGitBackend) resolveAuth(auth AuthConfig) (AuthConfig, func()) {
	cleanup := func() {}

	if auth.Type == AuthSSH && auth.SSHKeyContent != "" && auth.SSHKey == "" {
		tmpFile, err := os.CreateTemp("", "git_ssh_key_*")
		if err != nil {
			return auth, cleanup
		}
		keyContent := auth.SSHKeyContent
		if !strings.HasSuffix(keyContent, "\n") {
			keyContent += "\n"
		}
		if _, err := tmpFile.WriteString(keyContent); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
			return auth, cleanup
		}
		_ = tmpFile.Close()
		_ = os.Chmod(tmpFile.Name(), 0o600)

		auth.SSHKey = tmpFile.Name()
		cleanup = func() { _ = os.Remove(tmpFile.Name()) }
	}

	return auth, cleanup
}

// configureAuth 为 git 命令注入认证环境。
// HTTPS 令牌走**临时 credential helper + ASKPASS**（gickup 模式）：
// 令牌写入 0600 文件，git 进程只拿到 helper 路径，**argv/environ 均无令牌**。
// hosts 是本次调用声明过的目标主机（见 credentialHostsFor）；helper/askpass
// 只对白名单内的主机应答，令牌绝不发给未声明的主机。
// 返回 cleanup：调用方在 git 命令结束后删除临时凭证目录。
func (b *NativeGitBackend) configureAuth(cmd *exec.Cmd, auth AuthConfig, hosts []string) (cleanup func(), err error) {
	cleanup = func() {}
	if cmd.Env == nil {
		cmd.Env = cmd.Environ()
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")

	switch auth.Type {
	case AuthHTTPBasic, AuthHTTPToken:
		token := auth.Token
		if token == "" {
			token = auth.Password
		}
		username := auth.Username
		if username == "" {
			username = "token"
		}
		if token == "" {
			return cleanup, nil
		}
		sess, err := newCredSession(username, token, hosts)
		if err != nil {
			// **不再回退到 extraheader**:那条路把
			// `GIT_CONFIG_VALUE_0=Authorization: Basic <base64>` 放进 **git 进程的
			// environ** ⇒ 同用户的 `ps eww` / `/proc/<pid>/environ` 直接可见,
			// base64 原样可还原 ⇒ 与本文件自述的安全模型("argv/environ 均无令牌")
			// 直接矛盾,而且这正是被它替换掉的旧做法。
			//
			// 建不出凭证会话(临时目录/文件建不了)本就不该继续跑 git ——
			// 宁可直接报错,也不要带着明文凭据去执行命令。
			b.logger.Error("cred session unavailable, refusing to continue", "error", err)
			return cleanup, newGitError("configureAuth", cmd.Dir, "", err)
		}
		cmd.Env = append(cmd.Env, sess.env()...)
		return sess.close, nil
	case AuthSSH:
		if auth.SSHKey != "" {
			// shellx.Quote 单引号化 key 路径：GIT_SSH_COMMAND 由 git 交给
			// sh -c 执行，$ / 反引号 / 空格在双引号里仍会被 shell 解释
			// （Go %q 不转义它们）——单引号内一切字面。
			sshCmd := fmt.Sprintf("ssh -i %s -o BatchMode=yes", shellx.Quote(auth.SSHKey))
			sshCmd += " " + sshHostKeyArgs(auth)
			cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
		}
	}
	return cleanup, nil
}

// --- Output parsers ---

// parsePushRefs 解析 `git push` 的输出，返回被远端成功更新的 ref 名。
//
// git push 的报告行形如（实测）：
//
//	To github.com:foo/bar.git            ← URL 行（可能内嵌凭据），忽略
//	 * [new branch]      main -> main    ← 新建：dst 是「->」后的名字
//	   abc123..def456    main -> main    ← 快进更新：同上
//	 + def456...abc123   main -> main    ← 强推：同上
//	 * [new tag]         v1 -> v1        ← 新标签：同上
//	 - [deleted]         other           ← 删除：无「->」，ref 是末字段
//	 ! [rejected]        main -> main (…)/[remote rejected] ← 失败：排除
//
// 旧实现把含 ":"/".." 的字段全收——URL（含内嵌凭据）与 old..new 区间都进了
// 结果，而 `* [new branch]`/`- [deleted]` 行完全丢失（issue #107）。
func parsePushRefs(output string) []string {
	var refs []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "! "):
			// rejected/remote rejected/remote failure——未推送成功
			continue
		case strings.Contains(line, "->"):
			fields := strings.Fields(line)
			// dst 是最后一个字段（"src -> dst" 之后无后缀；rejected 行已在上面排除）
			refs = append(refs, fields[len(fields)-1])
		case strings.Contains(line, "[deleted]"):
			fields := strings.Fields(line)
			if len(fields) > 0 {
				refs = append(refs, fields[len(fields)-1])
			}
		}
	}
	return refs
}

// pushedRefsFromRefSpecs 把请求的 refspecs 归一成与 parsePushRefs 相同的口径
// （git push 报告的 dst 短名），供 gogit 后端（拿不到逐 ref 报告）与 native
// 的解析失败回填共用。refspec 形态："[+]src[:dst]"——无 dst 时推到同名 ref；
// 通配 refspec（含 *）无法归一，保留 dst 原样。
func pushedRefsFromRefSpecs(specs []string) []string {
	refs := make([]string, 0, len(specs))
	for _, spec := range specs {
		spec = strings.TrimPrefix(spec, "+")
		dst := spec
		if src, after, ok := strings.Cut(spec, ":"); ok {
			if after == "" {
				// "src:"（dst 空）＝推到同名 ref
				dst = src
			} else {
				dst = after
			}
		}
		switch {
		case strings.Contains(dst, "*"):
			// 通配 refspec 归一不出短名，保留全名
		default:
			dst = strings.TrimPrefix(dst, "refs/heads/")
			dst = strings.TrimPrefix(dst, "refs/tags/")
		}
		refs = append(refs, dst)
	}
	return refs
}

// --- Credential host pinning ---

// credentialHostsFor 推导一次 git 调用可能合法认证的目标主机集合：
//   - argv 里 URL 形态的参数（clone <url> / fetch <url> / ls-remote <url>…）；
//   - argv 里第一个非 flag 位置参数按 remote 名在本仓 config 中解析出的
//     URL（`git fetch origin` 这类调用只带名字，主机只存在于 .git/config）；
//   - argv 完全不含位置参数（如裸 `git push` 走上游追踪）时，回退为全部
//     已配置 remote 的主机——它们都在本仓 config 里声明过。
//
// 推导是 best-effort：结果为空时凭证会话对一切主机静默拒绝（fail-closed，
// 宁可认证失败也不把令牌发给未知主机）。
func credentialHostsFor(args []string, repoPath string) []string {
	var hosts []string
	seen := make(map[string]bool)
	add := func(h string) {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" && !seen[h] {
			seen[h] = true
			hosts = append(hosts, h)
		}
	}

	remoteURLs := repoRemoteURLs(repoPath)

	candidate := ""
	sawPositional := false
	for i, arg := range args {
		if i == 0 {
			continue // args[0] 是子命令
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// 任何非 flag 位置参数（含 URL 本身）都算「已声明目标」，
		// 不再触发全部 remote 的回退
		sawPositional = true
		if h, ok := hostOfURL(arg); ok {
			add(h)
			continue
		}
		if candidate == "" {
			candidate = arg
		}
	}

	if sawPositional {
		if urls, ok := remoteURLs[candidate]; ok {
			for _, u := range urls {
				if h, ok := hostOfURL(u); ok {
					add(h)
				}
			}
		}
	} else {
		for _, urls := range remoteURLs {
			for _, u := range urls {
				if h, ok := hostOfURL(u); ok {
					add(h)
				}
			}
		}
	}
	return hosts
}

// repoRemoteURLs 从仓库 config 读取 remote 名 → URL 列表。只做尽力而为的
// 文本解析（config 是唯一存放 remote 定义的地方，子命令执行会造成
// runGit→configureAuth→runGit 的递归面）；读不到时返回空——凭证会话按
// fail-closed 处理。远程定义存于 common dir（worktree/submodule 的
// <gitdir>/commondir 指回主仓）。
func repoRemoteURLs(repoPath string) map[string][]string {
	cfgPath, ok := gitConfigPath(repoPath)
	if !ok {
		return nil
	}
	data, err := os.ReadFile(cfgPath) //nolint:gosec // 路径由仓库布局推导，非调用方输入
	if err != nil {
		return nil
	}
	return parseRemoteURLs(string(data))
}

// gitConfigPath 定位工作树对应的 git config 文件：
//   - repoPath/.git 是目录 → 其下 config；
//   - repoPath/.git 是文件（gitfile：worktree/submodule）→ "gitdir: <path>"
//     指向的 gitdir（存在 commondir 时再跳到 common dir）下的 config。
func gitConfigPath(repoPath string) (string, bool) {
	if repoPath == "" {
		return "", false
	}
	dotGit := filepath.Join(repoPath, ".git")
	st, err := os.Stat(dotGit)
	if err != nil {
		return "", false
	}
	gitDir := ""
	if st.IsDir() {
		gitDir = dotGit
	} else {
		data, err := os.ReadFile(dotGit) //nolint:gosec // 同上
		if err != nil {
			return "", false
		}
		line := strings.TrimSpace(string(data))
		rest, ok := strings.CutPrefix(line, "gitdir:")
		if !ok {
			return "", false
		}
		gitDir = strings.TrimSpace(rest)
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(repoPath, gitDir)
		}
		// linked worktree/submodule：remote 定义在 common dir
		if common, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil { //nolint:gosec // 同上
			c := strings.TrimSpace(string(common))
			if c != "" {
				if !filepath.IsAbs(c) {
					c = filepath.Join(gitDir, c)
				}
				gitDir = c
			}
		}
	}
	//nolint:gosec // G703: 路径由仓库自身布局推导（.git / gitfile gitdir / commondir），非调用方输入
	cfg := filepath.Join(gitDir, "config")  //nolint:gosec // G703: 同上
	if _, err := os.Stat(cfg); err != nil { //nolint:gosec // G703: 同上
		return "", false
	}
	return cfg, true
}

// parseRemoteURLs 从 git config 文本里提取 [remote "<name>"] 节的 url /
// pushurl 值。只识别节头与这两个键；值两侧的引号剥掉。
func parseRemoteURLs(content string) map[string][]string {
	result := make(map[string][]string)
	section := ""
	for line := range strings.SplitSeq(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			header := strings.TrimSpace(line[1 : len(line)-1])
			if name, ok := strings.CutPrefix(header, "remote "); ok {
				name = strings.TrimSpace(name)
				// [remote "origin"] — 剥 name 两侧的引号
				name = strings.Trim(name, `"`)
				section = "remote:" + name
			} else {
				section = ""
			}
			continue
		}
		if section == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key != "url" && key != "pushurl" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)
		name := strings.TrimPrefix(section, "remote:")
		result[name] = append(result[name], value)
	}
	return result
}

// hostOfURL 从 git 远端 URL 提取小写主机名。支持 scheme:// 形态（https/ssh/
// git…）与 scp 形态（git@host:path、host:path）。非 URL 返回 false。
func hostOfURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-") {
		return "", false
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.ToLower(u.Host), true
	}
	// scp 形态：[user@]host:path（首段不含 / 且含 :，且不像盘符 C:\…）
	if before, _, ok := strings.Cut(raw, ":"); ok &&
		!strings.Contains(before, "/") && !strings.HasPrefix(before, ".") &&
		len(before) > 1 && !isWindowsDrive(before) {
		host := before
		if at := strings.LastIndexByte(host, '@'); at >= 0 {
			host = host[at+1:]
		}
		if host != "" {
			return strings.ToLower(host), true
		}
	}
	return "", false
}

func isWindowsDrive(s string) bool {
	return len(s) == 2 && s[1] == ':' && ((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z'))
}

func isText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	// Check for null bytes
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return utf8.Valid(data)
}

// sshHostKeyArgs 按 AuthConfig 生成 ssh 主机密钥校验参数。
// 优先级:InsecureSkipTLS > HostKeyFingerprint(经 resolveSSHPin 物化为
// 临时 known_hosts 后转写到这里) > KnownHostsPath/默认。
// 注:带 HostKeyFingerprint 到达这里的只剩"未经过 pin 预校验"的路径
// (如 resolveSSHPin 判定不适用),维持旧的 strict+known_hosts 行为。
func sshHostKeyArgs(auth AuthConfig) string {
	if auth.InsecureSkipTLS {
		return "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
	}
	// 指纹钉扎:禁用 known_hosts 自动信任,由应用层校验指纹
	// (native git 走 ssh 命令,指纹校验在 gogit 路径完整;这里用
	//  StrictHostKeyChecking=yes + 自定义 known_hosts 作为第二道防线)
	if fp := strings.TrimSpace(auth.HostKeyFingerprint); fp != "" {
		// 生成仅含期望指纹的临时 known_hosts 太重;用 yes + 指定文件,
		// 由调用方把钉扎主机写进 KnownHostsPath。
		if auth.KnownHostsPath != "" {
			return fmt.Sprintf("-o StrictHostKeyChecking=yes -o UserKnownHostsFile=%s", shellx.Quote(auth.KnownHostsPath))
		}
		return "-o StrictHostKeyChecking=yes"
	}
	if auth.KnownHostsPath != "" {
		return fmt.Sprintf("-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=%s", shellx.Quote(auth.KnownHostsPath))
	}
	// Secure default: verify against the user's known_hosts and
	// auto-accept new hosts on first use (MITM on a known host
	// still fails hard).
	return "-o StrictHostKeyChecking=accept-new"
}
