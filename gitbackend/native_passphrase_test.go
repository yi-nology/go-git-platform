package gitbackend

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	xssh "golang.org/x/crypto/ssh"
)

// 测试夹具：同一密钥对（ed25519, comment y4-fixture）的口令保护版与无口令版。
// passphrase 是公开测试常量，无凭据价值——只用于钉住 native 后端对
// AuthConfig.Passphrase 的消费（issue #142）。
//
//go:embed testdata/ssh_passphrase
var sshPassphraseFS embed.FS

func fixtureKey(t *testing.T, name string) string {
	t.Helper()
	data, err := sshPassphraseFS.ReadFile("testdata/ssh_passphrase/" + name)
	require.NoError(t, err)
	return string(data)
}

// 解析断言的公共件：用无口令 ParsePrivateKey 验证解析成功并返回与夹具
// 公钥一致的指纹——证明落盘的是明文 PEM（口令保护版必须先被解开才能过）。
func parsePlainAndMatch(t *testing.T, keyContent string) {
	t.Helper()
	signer, err := xssh.ParsePrivateKey([]byte(keyContent))
	require.NoError(t, err, "key must be parseable WITHOUT a passphrase after resolveAuth")
	pubFp := xssh.FingerprintSHA256(signer.PublicKey())

	pubPEM := fixtureKey(t, "key_enc.pub")
	wantFp := ""
	if pubKey, _, _, _, err := xssh.ParseAuthorizedKey([]byte(strings.TrimSpace(pubPEM))); err == nil {
		wantFp = xssh.FingerprintSHA256(pubKey)
	}
	require.Equal(t, wantFp, pubFp, "resolved key must be the same keypair as the fixture")
}

// TestNative_ResolveAuth_DecryptsPassphraseKey 回归（issue #142）：native 后端
// 旧实现静默丢弃 AuthConfig.Passphrase——带口令私钥原样落盘后交给
// `-o BatchMode=yes` 的 ssh，口令无门可问，密钥被静默跳过、认证必然失败，
// 而同一份 AuthConfig 在 gogit 后端（xssh.NewPublicKeys）正常。现在
// resolveAuth 用 pkg/credential 的 ProcessPrivateKey 在进程内解密成明文 PEM
// 再落盘；口令错误时 fail-fast 报错而不是静默丢弃。
func TestNative_ResolveAuth_DecryptsPassphraseKey(t *testing.T) {
	b := &NativeGitBackend{gitPath: "git"}
	encKey := fixtureKey(t, "key_enc.pem")

	t.Run("SSHKeyContent with passphrase yields plaintext temp key", func(t *testing.T) {
		auth := AuthConfig{Type: AuthSSH, SSHKeyContent: encKey, Passphrase: "test-pass-123"}
		resolved, cleanup, err := b.resolveAuth(auth)
		require.NoError(t, err)
		defer cleanup()
		require.NotEmpty(t, resolved.SSHKey, "SSHKey must point at the temp key file")
		require.NotEqual(t, resolved.SSHKey, "", "temp file expected")

		content, err := os.ReadFile(resolved.SSHKey)
		require.NoError(t, err)
		// ProcessPrivateKey 把 openssh 加密块重编码为无口令 PKCS8
		require.Contains(t, string(content), "-----BEGIN PRIVATE KEY-----")
		require.NotContains(t, string(content), "OPENSSH PRIVATE KEY")
		parsePlainAndMatch(t, string(content))
	})

	t.Run("SSHKey file path with passphrase decrypts into temp copy", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "id_enc")
		require.NoError(t, os.WriteFile(src, []byte(encKey), 0o600))

		auth := AuthConfig{Type: AuthSSH, SSHKey: src, Passphrase: "test-pass-123"}
		resolved, cleanup, err := b.resolveAuth(auth)
		require.NoError(t, err)
		defer cleanup()
		require.NotEqual(t, src, resolved.SSHKey, "must not rewrite the user's key file")
		if _, err := os.Stat(src); err != nil {
			t.Fatalf("original key file must stay intact: %v", err)
		}
		content, err := os.ReadFile(resolved.SSHKey)
		require.NoError(t, err)
		require.Contains(t, string(content), "-----BEGIN PRIVATE KEY-----")
		parsePlainAndMatch(t, string(content))
	})

	t.Run("wrong passphrase fails fast", func(t *testing.T) {
		auth := AuthConfig{Type: AuthSSH, SSHKeyContent: encKey, Passphrase: "wrong-pass"}
		_, cleanup, err := b.resolveAuth(auth)
		defer cleanup()
		require.Error(t, err, "wrong passphrase must surface an error, not a silently unusable key")
	})

	t.Run("unencrypted key with passphrase errors like gogit", func(t *testing.T) {
		// 无口令 key 误填 passphrase：x/crypto 的
		// ParseRawPrivateKeyWithPassphrase 直接报 "key is not password
		// protected"——gogit 后端（xssh.NewPublicKeys 同原语）同输入同报错。
		// native 保持一致 fail-fast，不做"静默忽略口令"的另一种分叉。
		plain := fixtureKey(t, "key_plain.pem")
		auth := AuthConfig{Type: AuthSSH, SSHKeyContent: plain, Passphrase: "needless-pass"}
		_, cleanup, err := b.resolveAuth(auth)
		defer cleanup()
		require.Error(t, err)
	})

	t.Run("no passphrase keeps legacy passthrough", func(t *testing.T) {
		auth := AuthConfig{Type: AuthSSH, SSHKeyContent: fixtureKey(t, "key_plain.pem")}
		resolved, cleanup, err := b.resolveAuth(auth)
		require.NoError(t, err)
		defer cleanup()
		content, err := os.ReadFile(resolved.SSHKey)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(string(content), "-----BEGIN OPENSSH PRIVATE KEY-----"))
	})
}
