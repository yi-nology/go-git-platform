package gitbackend

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oekit/pkg/shellx"
)

func TestNewCredSession_WritesSecureTokenFile(t *testing.T) {
	s, err := newCredSession("user", "s3cret-token", []string{"example.com"})
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
	if !strings.Contains(string(helper), "printf 'username=") || !strings.Contains(string(helper), "user") {
		t.Fatalf("helper missing username: %s", helper)
	}
	if !strings.Contains(string(helper), "cat") {
		t.Fatal("helper should read token file")
	}
}

func TestNewCredSession_EmptySecret(t *testing.T) {
	if _, err := newCredSession("u", "", nil); err == nil {
		t.Fatal("want error for empty secret")
	}
}

func TestCredSession_EnvHasNoSecret(t *testing.T) {
	s, err := newCredSession("token", "super-secret-value", []string{"example.com"})
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
	cleanup, cerr := b.configureAuth(cmd, AuthConfig{Type: AuthHTTPToken, Token: secret}, []string{"example.com"})
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
	if b2, err := newCredSession("t", secret, []string{"example.com"}); err == nil {
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
	s, err := newCredSession(user, "s3cret-token", []string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	out, err := shHelperGet(t, s.helperPath, "example.com").Output()
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
	cleanup, cerr := b.configureAuth(cmd, AuthConfig{Type: AuthHTTPToken, Token: ""}, nil)
	require.NoError(t, cerr)
	defer cleanup()
	joined := strings.Join(cmd.Env, "\n")
	if strings.Contains(joined, "http.extraheader") {
		t.Fatal("empty token should not set extraheader")
	}
}

func TestCredSession_EnvResetsHostCredentialHelpers(t *testing.T) {
	s, err := newCredSession("user", "s3cret-token", []string{"example.com"})
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
	s, err := newCredSession("alice", "topsecret", []string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	out, err := shHelperGet(t, s.helperPath, "example.com").CombinedOutput()
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

// shHelperGet 用 sh 执行 helper 的 get 动作，并把 host= 喂进 stdin
// （credential 协议的 get 请求体）。
func shHelperGet(t *testing.T, script, host string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", script, "get")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n\n")
	return cmd
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

	s, err := newCredSession("me", "mytoken", []string{"example.com"})
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

// TestCredSession_PasswordSurvivesShellExpansion 回归（issue #105）：
// 旧的 helper 模板 `echo password=$(cat %q)` 未加引号——$(…) 的输出会经历
// IFS 分词 + glob 展开，dash 的 echo 还会把 \t/\n/\c 当转义处理，含特殊
// 字符的密码被静默改写（`*` 甚至会把 token 目录的文件名拼进密码）。改
// printf '%s\n' "$(cat …)" 后，密码按字节原样输出。
func TestCredSession_PasswordSurvivesShellExpansion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts require a POSIX sh")
	}
	cases := []struct {
		name string
		pw   string
	}{
		{"space", "a b c"},
		{"glob star", "a*b"},
		{"glob question", "a?b"},
		{"dollar", "pa$$word"},
		{"backslash", `a\b\c`},
		{"backslash-t like escape", `a\tb`},
		{"single quote", "it's"},
		{"double quote", `say "hi"`},
		{"percent", "100%s%d"},
		{"leading/trailing spaces", "  padded  "},
		{"tab char", "a\tb"},
		{"tilde", "~user"},
		{"braces", "${HOME}"},
		{"combined hostile", `a b*c$d\e'f"g%h${i}~j`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := newCredSession("tok", tc.pw, []string{"example.com"})
			if err != nil {
				t.Fatal(err)
			}
			defer s.close()
			out, err := shHelperGet(t, s.helperPath, "example.com").Output()
			if err != nil {
				t.Fatalf("helper get: %v\n%s", err, out)
			}
			want := "password=" + tc.pw
			found := false
			for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
				if line == want {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("password 被改写：want line %q，实际输出 %q", want, string(out))
			}
		})
	}
}

// TestCredSession_HelperRefusesForeignHost 回归（issue #106）：helper 只对
// 声明过的目标主机应答——为 A 主机准备的令牌不再发给 B 主机。
func TestCredSession_HelperRefusesForeignHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts require a POSIX sh")
	}
	s, err := newCredSession("tok", "s3cret", []string{"internal.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	t.Run("matching host answers", func(t *testing.T) {
		out, err := shHelperGet(t, s.helperPath, "internal.example.com").Output()
		if err != nil {
			t.Fatalf("helper get: %v", err)
		}
		if !strings.Contains(string(out), "password=s3cret") {
			t.Fatalf("declared host must receive the credential, got %q", out)
		}
	})
	t.Run("foreign host gets empty response", func(t *testing.T) {
		out, err := shHelperGet(t, s.helperPath, "evil.example.com").Output()
		if err != nil {
			t.Fatalf("helper get: %v", err)
		}
		if strings.TrimSpace(string(out)) != "" {
			t.Fatalf("foreign host must get an empty response, got %q", out)
		}
	})
	t.Run("case-insensitive host match", func(t *testing.T) {
		out, err := shHelperGet(t, s.helperPath, "INTERNAL.Example.COM").Output()
		if err != nil {
			t.Fatalf("helper get: %v", err)
		}
		if !strings.Contains(string(out), "password=s3cret") {
			t.Fatalf("host match should be case-insensitive, got %q", out)
		}
	})
	t.Run("no host line gets empty response", func(t *testing.T) {
		cmd := exec.Command("sh", s.helperPath, "get")
		cmd.Stdin = strings.NewReader("protocol=https\n\n")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("helper get: %v", err)
		}
		if strings.TrimSpace(string(out)) != "" {
			t.Fatalf("request without host must get an empty response, got %q", out)
		}
	})
	t.Run("no declared hosts refuses everything", func(t *testing.T) {
		s2, err := newCredSession("tok", "s3cret", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer s2.close()
		out, err := shHelperGet(t, s2.helperPath, "internal.example.com").Output()
		if err != nil {
			t.Fatalf("helper get: %v", err)
		}
		if strings.TrimSpace(string(out)) != "" {
			t.Fatalf("session without declared hosts must answer nothing, got %q", out)
		}
	})
}

// TestCredSession_GitCredentialFill_HostGated 用真实 git 钉 issue #106 的
// 端到端回归：`git credential fill` 对声明主机拿到令牌、对异主机拿不到。
func TestCredSession_GitCredentialFill_HostGated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts require a POSIX sh")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	s, err := newCredSession("me", "mytoken", []string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	fill := func(host string) string {
		cmd := exec.Command("git", "credential", "fill")
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_TERMINAL_PROMPT=0",
		)
		cmd.Env = append(cmd.Env, s.env()...)
		cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("credential fill(%s): %v\n%s", host, err, out.String())
		}
		return out.String()
	}

	if got := fill("example.com"); !strings.Contains(got, "password=mytoken") {
		t.Fatalf("declared host must receive the token, got %q", got)
	}
	if got := fill("evil.com"); strings.Contains(got, "mytoken") {
		t.Fatalf("token leaked to a foreign host: %q", got)
	}
}

// TestCredSession_AskpassRefusesForeignHost 回归（issue #106 的 askpass 面）：
// 提示文本里的主机不在白名单时一律静默——旧实现对任意提示回退输出
// username，也是跨域泄漏面。
func TestCredSession_AskpassRefusesForeignHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts require a POSIX sh")
	}
	s, err := newCredSession("tok", "s3cret", []string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	out, err := exec.Command("sh", s.askPath, "Password for 'https://evil.com':").Output()
	if err != nil {
		t.Fatalf("askpass: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("askpass must stay silent for foreign hosts, got %q", out)
	}
	out, err = exec.Command("sh", s.askPath, "Enter passphrase for key '/tmp/key'").Output()
	if err != nil {
		t.Fatalf("askpass: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("askpass must stay silent for non-credential prompts, got %q", out)
	}
	out, err = exec.Command("sh", s.askPath, "Password for 'git@example.com':").Output()
	if err != nil {
		t.Fatalf("askpass: %v", err)
	}
	if strings.TrimSpace(string(out)) != "s3cret" {
		t.Fatalf("askpass must answer declared scp-style host, got %q", out)
	}
}

// TestCredentialHostsFor 钉主机推导的三个来源：argv URL、remote 名解析
// （.git/config）、以及无位置参数时的全部 remote 回退。
func TestCredentialHostsFor(t *testing.T) {
	repo := t.TempDir()
	cfgDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := "[core]\n\trepositoryformatversion = 0\n" +
		"[remote \"origin\"]\n\turl = https://Origin@GitHub.com/foo/bar.git\n" +
		"[remote \"upstream\"]\n\turl = git@internal:host/path.git\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"argv url wins", []string{"ls-remote", "https://target.example.com/x.git"}, []string{"target.example.com"}},
		{"remote name resolved from config", []string{"fetch", "--prune", "--tags", "origin"}, []string{"github.com"}},
		{"ssh url host", []string{"ls-remote", "ssh://git@sshhost:2222/x.git"}, []string{"sshhost:2222"}},
		{"scp-like url", []string{"ls-remote", "git@scphost:x/y.git"}, []string{"scphost"}},
		{"no positional falls back to all remotes", []string{"push"}, []string{"github.com", "internal"}},
		{"flag-only clone", []string{"clone", "https://c1.example.com/a.git", "/tmp/dst"}, []string{"c1.example.com"}},
		{"unknown remote name: no hosts from argv", []string{"fetch", "missing"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := credentialHostsFor(tc.args, repo)
			// 「全部 remote 回退」走 map 遍历，顺序不定，按集合比较
			gotSet := append([]string(nil), got...)
			wantSet := append([]string(nil), tc.want...)
			sort.Strings(gotSet)
			sort.Strings(wantSet)
			if !slices.Equal(gotSet, wantSet) {
				t.Fatalf("credentialHostsFor = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("no repo: empty set", func(t *testing.T) {
		if got := credentialHostsFor([]string{"fetch", "origin"}, t.TempDir()); len(got) != 0 {
			t.Fatalf("credentialHostsFor = %v, want empty", got)
		}
	})
}

// TestParseRemoteURLs：config 文本解析的最小形状（节头/引号/url+pushurl）。
func TestParseRemoteURLs(t *testing.T) {
	cfg := `# comment
[core]
	bare = false
[remote "origin"]
	url = https://github.com/foo/bar.git
	pushurl = git@push.example.com:foo/bar.git
[branch "main"]
	remote = origin
[remote "second"]
	url = "https://quoted.example.com/x.git"
`
	got := parseRemoteURLs(cfg)
	if len(got["origin"]) != 2 || got["origin"][0] != "https://github.com/foo/bar.git" ||
		got["origin"][1] != "git@push.example.com:foo/bar.git" {
		t.Fatalf("origin urls misparsed: %v", got["origin"])
	}
	if len(got["second"]) != 1 || got["second"][0] != "https://quoted.example.com/x.git" {
		t.Fatalf("second urls misparsed: %v", got["second"])
	}
}

// TestGitConfigPath_Gitfile：worktree（.git 为文件）也要能定位 config。
func TestGitConfigPath_Gitfile(t *testing.T) {
	repo := t.TempDir()
	realGitDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(realGitDir, "config"), []byte("[remote \"o\"]\n\turl = https://wt.example.com/x.git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitfile := filepath.Join(repo, ".git")
	if err := os.WriteFile(gitfile, []byte("gitdir: "+realGitDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := gitConfigPath(repo)
	if !ok || got != filepath.Join(realGitDir, "config") {
		t.Fatalf("gitConfigPath = %q (%v), want %q", got, ok, filepath.Join(realGitDir, "config"))
	}
}

// TestCredSession_AskpassKeepsPort_HostFormMatchesHelper 回归（issue #144，
// 安全面）：askpass 兜底脚本旧模板 `host=${u%%:*}` 把端口剥掉——白名单用
// hostOfURL 的完整 host（含端口）口径，两条路径对「这是不是同一个目标」
// 给出不同答案：白名单 ["example.com"] 时 `http://example.com:8080` 的提示
// 也命中，git 跟随重定向/子模块解析到带端口主机时令牌被投给白名单未声明
// 的 origin。helper 路径（比对 credential 协议的 host= 行）在同场景下拒绝
// ——修复后 askpass 与 helper 同判据：host:port 整体比对。
func TestCredSession_AskpassKeepsPort_HostFormMatchesHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts require a POSIX sh")
	}
	t.Run("port-bearing prompt refused when whitelist has bare host", func(t *testing.T) {
		s, err := newCredSession("tok", "s3cret", []string{"example.com"})
		if err != nil {
			t.Fatal(err)
		}
		defer s.close()
		for _, prompt := range []string{
			"Username for 'http://example.com:8080': ",
			"Password for 'http://example.com:8080': ",
			"Password for 'https://example.com:8443': ",
		} {
			out, err := exec.Command("sh", s.askPath, prompt).Output()
			if err != nil {
				t.Fatalf("askpass(%q): %v", prompt, err)
			}
			if strings.TrimSpace(string(out)) != "" {
				t.Fatalf("whitelist [example.com] must NOT answer %q, got %q", prompt, out)
			}
		}
		// 同场景下 helper 一致拒绝（判据对齐的参照实现）
		out, err := shHelperGet(t, s.helperPath, "example.com:8080").Output()
		if err != nil {
			t.Fatalf("helper get: %v", err)
		}
		if strings.TrimSpace(string(out)) != "" {
			t.Fatalf("helper must refuse host=example.com:8080 for whitelist [example.com], got %q", out)
		}
	})

	t.Run("port-bearing whitelist answers matching host:port", func(t *testing.T) {
		s, err := newCredSession("tok", "s3cret", []string{"example.com:8080"})
		if err != nil {
			t.Fatal(err)
		}
		defer s.close()
		out, err := exec.Command("sh", s.askPath, "Password for 'https://example.com:8080':").Output()
		if err != nil || strings.TrimSpace(string(out)) != "s3cret" {
			t.Fatalf("whitelist [example.com:8080] must answer its own host:port, out=%q err=%v", out, err)
		}
		// 带端口白名单对不带端口的提示同样静默——同一主机不同端口是不同源
		out, err = exec.Command("sh", s.askPath, "Password for 'https://example.com':").Output()
		if err != nil {
			t.Fatalf("askpass: %v", err)
		}
		if strings.TrimSpace(string(out)) != "" {
			t.Fatalf("whitelist [example.com:8080] must NOT answer bare example.com, got %q", out)
		}
	})

	t.Run("bare host prompt still answered by bare whitelist", func(t *testing.T) {
		s, err := newCredSession("tok", "s3cret", []string{"example.com"})
		if err != nil {
			t.Fatal(err)
		}
		defer s.close()
		out, err := exec.Command("sh", s.askPath, "Username for 'https://example.com':").Output()
		if err != nil || strings.TrimSpace(string(out)) != "tok" {
			t.Fatalf("no-port prompts keep working, out=%q err=%v", out, err)
		}
		// scp 形态（无 scheme，无端口）
		out, err = exec.Command("sh", s.askPath, "Password for 'git@example.com':").Output()
		if err != nil || strings.TrimSpace(string(out)) != "s3cret" {
			t.Fatalf("scp-style host prompt keeps working, out=%q err=%v", out, err)
		}
	})
}
