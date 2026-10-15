package gitbackend

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"

	oekiterrors "github.com/oekit/pkg/errors"
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
	// cleanup removes the temp token/script dir.
	cleanupCred := b.configureAuth(cmd, pinAuth)
	defer cleanupKey()
	defer cleanupPin()
	defer cleanupCred()

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
// 返回 cleanup：调用方在 git 命令结束后删除临时凭证目录。
func (b *NativeGitBackend) configureAuth(cmd *exec.Cmd, auth AuthConfig) (cleanup func()) {
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
			return cleanup
		}
		sess, err := newCredSession(username, token)
		if err != nil {
			// 回退：无法建会话时仍用 extraheader（与旧行为一致），令牌仍不进 argv
			b.logger.Warn("cred session unavailable, falling back to http.extraheader", "error", err)
			cred := base64.StdEncoding.EncodeToString([]byte(username + ":" + token))
			cmd.Env = append(cmd.Env,
				"GIT_CONFIG_COUNT=1",
				"GIT_CONFIG_KEY_0=http.extraheader",
				fmt.Sprintf("GIT_CONFIG_VALUE_0=Authorization: Basic %s", cred),
			)
			return cleanup
		}
		cmd.Env = append(cmd.Env, sess.env()...)
		return sess.close
	case AuthSSH:
		if auth.SSHKey != "" {
			// %q keeps a caller-supplied key path from being interpreted by
			// the shell GIT_SSH_COMMAND runs under (spaces, $, backticks).
			sshCmd := fmt.Sprintf("ssh -i %q -o BatchMode=yes", auth.SSHKey)
			sshCmd += " " + sshHostKeyArgs(auth)
			cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
		}
	}
	return cleanup
}

// --- Output parsers ---

func parsePushRefs(output string) []string {
	var refs []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "To ") || strings.Contains(line, "..") {
			if strings.Contains(line, " ") {
				for _, p := range strings.Fields(line) {
					if strings.Contains(p, ":") || strings.Contains(p, "..") {
						refs = append(refs, p)
					}
				}
			}
		}
	}
	return refs
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
			return fmt.Sprintf("-o StrictHostKeyChecking=yes -o UserKnownHostsFile=%q", auth.KnownHostsPath)
		}
		return "-o StrictHostKeyChecking=yes"
	}
	if auth.KnownHostsPath != "" {
		return fmt.Sprintf("-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=%q", auth.KnownHostsPath)
	}
	// Secure default: verify against the user's known_hosts and
	// auto-accept new hosts on first use (MITM on a known host
	// still fails hard).
	return "-o StrictHostKeyChecking=accept-new"
}
