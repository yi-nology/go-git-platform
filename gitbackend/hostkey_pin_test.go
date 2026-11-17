package gitbackend

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os/exec"
	"runtime"
	"testing"

	"github.com/oekit/pkg/shellx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func fakeHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return sshPub
}

func TestHostKeyCallback_FingerprintPin(t *testing.T) {
	key := fakeHostKey(t)
	want := ssh.FingerprintSHA256(key)

	cb, err := hostKeyCallbackWithConfig(AuthConfig{HostKeyFingerprint: want})
	require.NoError(t, err)

	addr := &net.TCPAddr{}
	require.NoError(t, cb("git.example.com", addr, key))

	// 换一把 key 应拒绝
	other := fakeHostKey(t)
	require.Error(t, cb("git.example.com", addr, other))
}

func TestHostKeyCallback_Insecure(t *testing.T) {
	cb, err := hostKeyCallbackWithConfig(AuthConfig{InsecureSkipTLS: true})
	require.NoError(t, err)
	require.NoError(t, cb("any.host", &net.TCPAddr{}, fakeHostKey(t)))
}

func TestHostKeyCallback_MissingKnownHosts(t *testing.T) {
	_, err := hostKeyCallbackWithConfig(AuthConfig{KnownHostsPath: "/nonexistent/known_hosts"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// sshHostKeyArgs 专属元字符测试：KnownHostsPath 内嵌 GIT_SSH_COMMAND（git 交给
// sh -c 执行），路径含 shell 元字符时既不能提前结束该词（引号逃逸）也不能被
// 展开（$ “ ` “、分号等）。形态钉住 shellx.Quote 单引号化，round-trip 用真
// sh 验证词切分逐字节还原；退回裸拼接 `%s`（旧 `%q`/raw 形态）任一即红。
func TestSSHHostKeyArgs_ShellMetacharQuoting(t *testing.T) {
	cases := []string{
		"/tmp/kh with space",
		"/tmp/kh'quote",
		`/tmp/kh"dq`,
		"/tmp/$(touch pwned)",
		"/tmp/`touch pwned`",
		"/tmp/kh;rm -rf x",
		"$HOME/pwned",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			got := sshHostKeyArgs(AuthConfig{KnownHostsPath: path})
			want := "-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=" + shellx.Quote(path)
			assert.Equal(t, want, got, "metachar path must be shellx.Quote 单引号化")

			// 真 sh round-trip：把 sshHostKeyArgs 产出的引号词喂给 sh -c 解析，
			// 词切分后必须逐字节还原原路径（引号逃逸或变量展开都会破坏还原）。
			if runtime.GOOS == "windows" {
				t.Skip("no /bin/sh on windows")
			}
			script := "printf %s " + shellx.Quote(path)
			out, err := exec.Command("/bin/sh", "-c", script).Output()
			require.NoError(t, err)
			assert.Equal(t, path, string(out), "sh 词切分必须逐字节还原原路径")
		})
	}
}
