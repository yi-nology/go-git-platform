package gitbackend

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oekit/pkg/shellx"
)

func TestNewCredSession_WritesSecureTokenFile(t *testing.T) {
	s, err := newCredSession("user", "s3cret-token")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	tokenPath := filepath.Join(s.dir, "token")
	st, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode = %v, want 0600", st.Mode().Perm())
	}
	data, _ := os.ReadFile(tokenPath) //nolint:gosec // test
	if string(data) != "s3cret-token" {
		t.Fatalf("token content = %q", data)
	}
	// helper 可执行且包含 username
	helper, _ := os.ReadFile(s.helperPath) //nolint:gosec // test
	if !strings.Contains(string(helper), "echo username=") || !strings.Contains(string(helper), "user") {
		t.Fatalf("helper missing username: %s", helper)
	}
	if !strings.Contains(string(helper), "cat") {
		t.Fatal("helper should read token file")
	}
}

func TestNewCredSession_EmptySecret(t *testing.T) {
	if _, err := newCredSession("u", ""); err == nil {
		t.Fatal("want error for empty secret")
	}
}

func TestCredSession_EnvHasNoSecret(t *testing.T) {
	s, err := newCredSession("token", "super-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	for _, e := range s.env() {
		if strings.Contains(e, "super-secret-value") {
			t.Fatalf("secret leaked into env: %s", e)
		}
	}
	if !strings.Contains(strings.Join(s.env(), "\n"), "credential.helper") {
		t.Fatal("want credential.helper in env")
	}
}

func TestConfigureAuth_TokenNotInArgvOrEnv(t *testing.T) {
	b := &NativeGitBackend{gitPath: "git"}
	cmd := exec.Command("git", "ls-remote", "https://example.com/o/r.git")
	secret := "ghp_do_not_leak_this_token_xyz"
	cleanup, cerr := b.configureAuth(cmd, AuthConfig{Type: AuthHTTPToken, Token: secret})
	require.NoError(t, cerr)
	defer cleanup()

	// argv 无令牌
	for _, a := range cmd.Args {
		if strings.Contains(a, secret) {
			t.Fatalf("secret in argv: %s", a)
		}
	}
	// environ 无令牌明文
	for _, e := range cmd.Env {
		if strings.Contains(e, secret) {
			t.Fatalf("secret in env: %s", e)
		}
	}
	// 有 credential.helper 指向临时 helper
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, "credential.helper") {
		t.Fatalf("want credential.helper env, got:\n%s", joined)
	}
	if !strings.Contains(joined, "GIT_ASKPASS=") {
		t.Fatal("want GIT_ASKPASS env")
	}
	// cleanup 后目录消失
	if b2, err := newCredSession("t", secret); err == nil {
		dir := b2.dir
		b2.close()
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("cred dir not cleaned: %v", err)
		}
	}
}

// 转义语义由上游 shellx.Quote 负责（其 round-trip 由上游测试兜底），下游不再
// 抄一份转义实现。这里钉的是**可观测**的那条性质：生成的 credential helper
// 脚本经真实 sh 执行后，用户名仍是恰好一个参数——含空格与单引号的用户名既不会
// 被拆成多个参数，也不会被 shell 解释掉。
func TestCredHelperScriptPassesUsernameAsOneArg(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 无 sh：helper 脚本由 msys 执行，不在本测试的断言范围内")
	}
	const user = "user name'with-quote"
	s, err := newCredSession(user, "s3cret-token")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	out, err := exec.Command("sh", s.helperPath, "get").Output()
	if err != nil {
		t.Fatalf("执行 helper 脚本失败: %v", err)
	}
	want := "username=" + user
	if !strings.Contains(string(out), want) {
		t.Fatalf("helper 脚本未还原出完整用户名 %q，实际输出: %q", want, string(out))
	}
}

func TestConfigureAuth_TokenFallbackOnCredSessionError(t *testing.T) {
	// 空 token 不建会话，也不注入 extraheader
	b := &NativeGitBackend{gitPath: "git"}
	cmd := exec.Command("git", "status")
	cleanup, cerr := b.configureAuth(cmd, AuthConfig{Type: AuthHTTPToken, Token: ""})
	require.NoError(t, cerr)
	defer cleanup()
	joined := strings.Join(cmd.Env, "\n")
	if strings.Contains(joined, "http.extraheader") {
		t.Fatal("empty token should not set extraheader")
	}
}

func TestCredSession_EnvResetsHostCredentialHelpers(t *testing.T) {
	s, err := newCredSession("user", "s3cret-token")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	vals := map[string]string{}
	for _, e := range s.env() {
		k, v, _ := strings.Cut(e, "=")
		vals[k] = v
	}
	// 槽位 0 必须是空串 helper：清空主机已配置的 credential.helper 列表
	if vals["GIT_CONFIG_KEY_0"] != "credential.helper" || vals["GIT_CONFIG_VALUE_0"] != "" {
		t.Fatalf("slot 0 must be the empty helper reset, got %q=%q",
			vals["GIT_CONFIG_KEY_0"], vals["GIT_CONFIG_VALUE_0"])
	}
	// 槽位 1 是我们的一次性 helper，且必须晚于重置
	if vals["GIT_CONFIG_VALUE_1"] != filepath.ToSlash(s.helperPath) {
		t.Fatalf("slot 1 must point at our helper, got %q", vals["GIT_CONFIG_VALUE_1"])
	}
	if strings.Contains(strings.Join(s.env(), "\n"), "useHttpPath") {
		t.Fatal("useHttpPath should not be set")
	}
}

func TestCredSession_HelperScriptsExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts require a POSIX sh")
	}
	s, err := newCredSession("alice", "topsecret")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	out, err := exec.Command("sh", s.helperPath, "get").CombinedOutput()
	if err != nil {
		t.Fatalf("helper get: %v\n%s", err, out)
	}
	if got := string(out); !strings.Contains(got, "username=alice") || !strings.Contains(got, "password=topsecret") {
		t.Fatalf("helper get output wrong: %q", got)
	}
	// store/erase 必须是无输出 no-op——令牌绝不落盘
	out, err = exec.Command("sh", s.helperPath, "store").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("helper store must be a silent no-op: %v %q", err, out)
	}
	// askpass：Password 提示回令牌，Username 提示回用户名
	out, err = exec.Command("sh", s.askPath, "Password for 'https://example.com':").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "topsecret" {
		t.Fatalf("askpass password prompt: %v %q", err, out)
	}
	out, err = exec.Command("sh", s.askPath, "Username for 'https://example.com':").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "alice" {
		t.Fatalf("askpass username prompt: %v %q", err, out)
	}
}

// TestCredSession_GitIntegration_HostHelperNotConsulted 用真实 git 钉死两个回归：
//  1. 主机全局 credential helper（osxkeychain 等）不得遮蔽本次令牌（get）；
//  2. 认证成功后 store 不得扩散到主机 helper（一次性令牌不落用户凭证库）。
func TestCredSession_GitIntegration_HostHelperNotConsulted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts require a POSIX sh")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	// 模拟主机全局配置里的个人凭证 helper（osxkeychain 角色）
	dir := t.TempDir()
	hostHelper := filepath.Join(dir, "hosthelper.sh")
	hostLog := filepath.Join(dir, "host.log")
	script := "#!/bin/sh\n" +
		"echo \"$1\" >> " + shellx.Quote(filepath.ToSlash(hostLog)) + "\n" +
		"[ \"$1\" = get ] && { echo username=hostuser; echo password=hostpass; }\n"
	if err := os.WriteFile(hostHelper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	globalCfg := filepath.Join(dir, "gitconfig")
	cfg := fmt.Sprintf("[credential]\n\thelper = %s\n", filepath.ToSlash(hostHelper))
	if err := os.WriteFile(globalCfg, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := newCredSession("me", "mytoken")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	env := append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL_FILE="+globalCfg,
	)
	env = append(env, s.env()...)
	input := "protocol=https\nhost=example.com\n"

	// get：必须返回我们的令牌，且主机 helper 根本不被查询
	var out bytes.Buffer
	cmd := exec.Command("git", "credential", "fill")
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("credential fill: %v\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, "password=mytoken") || strings.Contains(got, "hostpass") {
		t.Fatalf("host helper shadowed our credential:\n%s", got)
	}
	if data, _ := os.ReadFile(hostLog); len(data) != 0 {
		t.Fatalf("host helper must not be consulted at all, log: %q", data)
	}

	// approve：store 不得扩散到主机 helper
	cmd = exec.Command("git", "credential", "approve")
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input + "username=me\npassword=mytoken\n")
	if err := cmd.Run(); err != nil {
		t.Fatalf("credential approve: %v", err)
	}
	if data, _ := os.ReadFile(hostLog); strings.Contains(string(data), "store") {
		t.Fatalf("host helper received store — token would persist to the user's credential store: %q", data)
	}
}
