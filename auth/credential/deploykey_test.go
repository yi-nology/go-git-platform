package credential

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestGenerateEd25519DeployKey(t *testing.T) {
	k, err := GenerateEd25519DeployKey("gitferry-mirror")
	require.NoError(t, err)
	require.NotNil(t, k)

	assert.Contains(t, k.PrivateKeyPEM, "BEGIN PRIVATE KEY")
	assert.True(t, strings.HasPrefix(k.PublicKeyAuthorized, "ssh-ed25519 "))
	assert.Contains(t, k.PublicKeyAuthorized, "gitferry-mirror")
	assert.True(t, strings.HasPrefix(k.Fingerprint, "SHA256:"))

	// 公钥能被 ssh 解析
	_, _, _, _, err = ssh.ParseAuthorizedKey([]byte(k.PublicKeyAuthorized))
	require.NoError(t, err)

	// 私钥能解析
	_, err = ssh.ParsePrivateKey([]byte(k.PrivateKeyPEM))
	require.NoError(t, err)
}

func TestGenerateEd25519DeployKey_Unique(t *testing.T) {
	a, err := GenerateEd25519DeployKey("a")
	require.NoError(t, err)
	b, err := GenerateEd25519DeployKey("b")
	require.NoError(t, err)
	assert.NotEqual(t, a.PrivateKeyPEM, b.PrivateKeyPEM)
	assert.NotEqual(t, a.Fingerprint, b.Fingerprint)
}

func TestMatchFingerprint(t *testing.T) {
	k, err := GenerateEd25519DeployKey("")
	require.NoError(t, err)

	assert.True(t, MatchFingerprint(k.PublicKeyAuthorized, k.Fingerprint))
	// 裸 base64 也认
	assert.True(t, MatchFingerprint(k.PublicKeyAuthorized, strings.TrimPrefix(k.Fingerprint, "SHA256:")))
	assert.False(t, MatchFingerprint(k.PublicKeyAuthorized, "SHA256:not-the-right-one"))
	assert.False(t, MatchFingerprint(k.PublicKeyAuthorized, ""))
}
