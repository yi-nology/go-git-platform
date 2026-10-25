package gitbackend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/oekit/pkg/shellx"
)

// credSession 短生命周期的 git 凭证会话。
//
// 安全模型（对标 gickup credential helper 改造）：
//   - 令牌写入 0600 临时文件，**不进 argv、不进 git 进程 environ**
//   - git 仅通过 credential.helper / GIT_ASKPASS 读取
//   - 会话结束后整目录删除（RAII）
//
// 替代原先的 http.extraheader（令牌以 Base64 出现在 GIT_CONFIG_VALUE_* 环境变量）。
type credSession struct {
	dir        string
	helperPath string
	askPath    string
	username   string
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
func newCredSession(username, secret string) (*credSession, error) {
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
	s := &credSession{dir: dir, username: username}

	// 1) 令牌文件 0600
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte(secret), 0o600); err != nil {
		s.close()
		return nil, err
	}

	// 2) credential.helper：实现 git credential 协议的 get
	//    username 固定；password 从 token 文件读。
	helperPath := filepath.Join(dir, "helper")
	helper := fmt.Sprintf("#!/bin/sh\n"+
		"# gitferry ephemeral credential helper (auto-generated)\n"+
		"if [ \"$1\" != \"get\" ]; then exit 0; fi\n"+
		"echo username=%s\n"+
		"echo password=$(cat %q)\n",
		shellx.Quote(username), filepath.ToSlash(tokenPath))
	//nolint:gosec // G306: git 需要以 0700 可执行方式调用 helper
	if err := os.WriteFile(helperPath, []byte(helper), 0o700); err != nil {
		s.close()
		return nil, err
	}
	s.helperPath = helperPath

	// 3) ASKPASS 兜底（任何残留交互提示）
	askPath := filepath.Join(dir, "askpass")
	ask := fmt.Sprintf("#!/bin/sh\n"+
		"# gitferry ephemeral askpass (auto-generated)\n"+
		"case \"$1\" in\n"+
		"  Username*|username*) echo %s ;;\n"+
		"  Password*|password*) cat %q ;;\n"+
		"  *) echo %s ;;\n"+
		"esac\n",
		shellx.Quote(username), filepath.ToSlash(tokenPath), shellx.Quote(username))
	//nolint:gosec // G306: git 需要以 0700 可执行方式调用 askpass
	if err := os.WriteFile(askPath, []byte(ask), 0o700); err != nil {
		s.close()
		return nil, err
	}
	s.askPath = askPath
	return s, nil
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
