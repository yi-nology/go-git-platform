package gitbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oekit/pkg/shellx"
)

// credSession 短生命周期的 git 凭证会话。
//
// 安全模型（对标 gickup credential helper 改造）：
//   - 令牌写入 0600 临时文件，**不进 argv、不进 git 进程 environ**
//   - git 仅通过 credential.helper / GIT_ASKPASS 读取
//   - helper/askpass 只对 hosts 白名单内的目标主机应答（issue #106）：
//     令牌只发给本次调用声明过的主机，多 remote 跨信任域时不再互相渗透
//   - 会话结束后整目录删除（RAII）
//
// 替代原先的 http.extraheader（令牌以 Base64 出现在 GIT_CONFIG_VALUE_* 环境变量）。
type credSession struct {
	dir        string
	helperPath string
	askPath    string
	username   string
	hosts      []string
}

// close 清理临时目录。幂等。
func (s *credSession) close() {
	if s == nil || s.dir == "" {
		return
	}
	_ = os.RemoveAll(s.dir)
	s.dir = ""
}

// newCredSession 为一次 git 调用建立凭证会话。
// username 为空时默认 "token"（GitHub x-access-token / GitLab oauth2 均可）。
// hosts 是本次调用声明过的目标主机（来自 credentialHostsFor）；为空时生成的
// helper/askpass 对一切主机静默拒绝——fail-closed，令牌不外泄。
func newCredSession(username, secret string, hosts []string) (*credSession, error) {
	if secret == "" {
		return nil, fmt.Errorf("cred session: empty secret")
	}
	if username == "" {
		username = "token"
	}
	dir, err := os.MkdirTemp("", "gitcred-*")
	if err != nil {
		return nil, err
	}
	s := &credSession{dir: dir, username: username, hosts: normalizeHosts(hosts)}

	// 1) 令牌文件 0600
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte(secret), 0o600); err != nil {
		s.close()
		return nil, err
	}

	s.helperPath, err = s.writeHelperScript(filepath.ToSlash(tokenPath))
	if err != nil {
		s.close()
		return nil, err
	}
	s.askPath, err = s.writeAskpassScript(filepath.ToSlash(tokenPath))
	if err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

// normalizeHosts 统一主机口径：trim + 小写（credential 协议的 host 是小写）。
func normalizeHosts(hosts []string) []string {
	var out []string
	seen := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// writeHelperScript 生成 credential.helper 脚本。
//
// 协议实现（`git credential-helper get`）：
//   - 先读 stdin 的 credential 协议（key=value 行，空行/EOF 结束），取出 host；
//   - host 不在白名单时静默退出（空应答 = 不提供凭据，git 会以无凭据继续/失败
//     ——绝不把令牌发给未声明的主机）；
//   - 命中白名单才输出 username/password。
//
// 输出用 printf '%s\n' 而不是 echo：
//   - password 值 "$(cat file)" 放在双引号里，不做 IFS 分词/glob 展开，
//     含空格、*、$、反斜杠的密码按字节原样输出（issue #105：旧的
//     `echo password=$(cat …)` 未加引号，shell 会改写密码）；
//   - printf 的 %s 不解释反斜杠——dash 的 echo 会把 \t/\n/\c 当转义处理。
//
// 已知边界：command substitution 会剥掉值末尾的换行，且 credential 协议本身
// 是行式的——含换行的密码在 git 协议层不可表达（askpass 路径同样受限）。
func (s *credSession) writeHelperScript(tokenPath string) (string, error) {
	helperPath := filepath.Join(s.dir, "helper")
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# gitferry ephemeral credential helper (auto-generated)\n")
	b.WriteString("if [ \"$1\" != \"get\" ]; then exit 0; fi\n")
	if len(s.hosts) == 0 {
		// 无声明主机：一律空应答（fail-closed）
		b.WriteString("exit 0\n")
	} else {
		b.WriteString("host=\n")
		b.WriteString("while IFS= read -r line; do\n")
		b.WriteString("\tcase \"$line\" in\n")
		b.WriteString("\t\thost=*) host=$(printf '%s' \"${line#host=}\" | tr '[:upper:]' '[:lower:]') ;;\n")
		b.WriteString("\t\t'') break ;;\n")
		b.WriteString("\tesac\n")
		b.WriteString("done\n")
		b.WriteString("case \"$host\" in\n")
		patterns := make([]string, 0, len(s.hosts))
		for _, h := range s.hosts {
			// case 模式里的单引号串是字面量——主机名不参与 glob/展开
			patterns = append(patterns, shellx.Quote(h))
		}
		b.WriteString("\t" + strings.Join(patterns, "|") + ")\n")
		b.WriteString("\t\tprintf 'username=%s\\n' " + shellx.Quote(s.username) + "\n")
		b.WriteString("\t\tprintf 'password=%s\\n' \"$(cat " + shellx.Quote(tokenPath) + ")\"\n")
		b.WriteString("\t\t;;\n")
		b.WriteString("esac\n")
	}
	//nolint:gosec // G306: git 需要以 0700 可执行方式调用 helper
	if err := os.WriteFile(helperPath, []byte(b.String()), 0o700); err != nil {
		return "", err
	}
	return helperPath, nil
}

// writeAskpassScript 生成 GIT_ASKPASS / GIT_SSH_ASKPASS 兜底脚本。
//
// 从提示文本（"Username for 'https://example.com': " 形态）里提取目标主机，
// 只在白名单内才应答；未知提示一律静默退出——旧实现对任意提示回退输出
// username，同样构成跨域泄漏面（issue #106）。
func (s *credSession) writeAskpassScript(tokenPath string) (string, error) {
	askPath := filepath.Join(s.dir, "askpass")
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# gitferry ephemeral askpass (auto-generated)\n")
	b.WriteString("# 只应答白名单主机的 Username/Password 提示，其余静默退出。\n")
	if len(s.hosts) == 0 {
		b.WriteString("exit 0\n")
		//nolint:gosec // G306: git 需要以 0700 可执行方式调用 askpass
		if err := os.WriteFile(askPath, []byte(b.String()), 0o700); err != nil {
			return "", err
		}
		return askPath, nil
	}
	// "Username for 'https://example.com': " → 剥前缀到 ` for '`，再剥到引号
	b.WriteString("u=\n")
	b.WriteString("case \"$1\" in\n")
	b.WriteString("\t*\" for '\"*) u=${1#*\" for '\"} ;;\n")
	b.WriteString("esac\n")
	b.WriteString("u=${u%%\\'*}\n")
	b.WriteString("u=${u#*://}\n")
	b.WriteString("u=${u%%/*}\n")
	b.WriteString("u=${u#*@}\n")
	b.WriteString("host=${u%%:*}\n")
	b.WriteString("host=$(printf '%s' \"$host\" | tr '[:upper:]' '[:lower:]')\n")
	b.WriteString("case \"$host\" in\n")
	patterns := make([]string, 0, len(s.hosts))
	for _, h := range s.hosts {
		// case 模式里的单引号串是字面量——主机名不参与 glob/展开
		patterns = append(patterns, shellx.Quote(h))
	}
	b.WriteString("\t" + strings.Join(patterns, "|") + ")\n")
	b.WriteString("\t\tcase \"$1\" in\n")
	b.WriteString("\t\t\t[Uu]sername*) printf '%s\\n' " + shellx.Quote(s.username) + " ;;\n")
	b.WriteString("\t\t\t[Pp]assword*) cat " + shellx.Quote(tokenPath) + " ;;\n")
	b.WriteString("\t\tesac\n")
	b.WriteString("\t\t;;\n")
	b.WriteString("esac\n")
	//nolint:gosec // G306: git 需要以 0700 可执行方式调用 askpass
	if err := os.WriteFile(askPath, []byte(b.String()), 0o700); err != nil {
		return "", err
	}
	return askPath, nil
}

// env 返回注入 git 进程的环境变量（无令牌内容）。
//
// KEY_0 是空串 helper：清空 system/global/local 已累积的 credential.helper
// 列表（osxkeychain、store、cache 等）。不清空会有两个后果（实测确认）：
//   - get 时主机 helper 先被查询，首个给出完整凭证者胜出——本次令牌会被
//     用户个人凭证（如钥匙串里的另一账号）遮蔽；
//   - 认证成功后 git 对列表中**全部** helper 执行 store，一次性令牌会被
//     持久化进用户钥匙串 / ~/.git-credentials 明文文件。
//
// GIT_TERMINAL_PROMPT=0 由 configureAuth 统一注入，这里不重复。
func (s *credSession) env() []string {
	if s == nil {
		return nil
	}
	// ToSlash：Windows 上 helper 由 msys sh 执行，反斜杠路径不可用。
	return []string{
		"GIT_ASKPASS=" + filepath.ToSlash(s.askPath),
		"GIT_SSH_ASKPASS=" + filepath.ToSlash(s.askPath),
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=credential.helper",
		"GIT_CONFIG_VALUE_1=" + filepath.ToSlash(s.helperPath),
	}
}
