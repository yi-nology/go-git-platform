package credential

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"

	oekiterrors "github.com/oekit/pkg/errors"

	"golang.org/x/crypto/ssh"
)

// DeployKey 镜像/备份专用部署密钥对。
// 私钥存 DB/密钥库,公钥交给目标平台做 deploy key(只读或只写)。
type DeployKey struct {
	// PrivateKeyPEM PKCS#8 PEM 私钥内容
	PrivateKeyPEM string
	// PublicKeyAuthorized authorized_keys 一行公钥(可粘贴到目标平台)
	PublicKeyAuthorized string
	// Fingerprint SHA256 公钥指纹(openssh 格式),便于核对
	Fingerprint string
}

// GenerateEd25519DeployKey 生成一对 Ed25519 部署密钥。
// 采用 Ed25519:短、快、无弱参数,是当前 SSH 部署密钥推荐算法。
func GenerateEd25519DeployKey(comment string) (*DeployKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, oekiterrors.Wrap(err, "generate ed25519 key")
	}

	// 转 OpenSSH/authorized_keys 公钥
	sshPub, err := sshPublicKeyFromEd25519(pub, comment)
	if err != nil {
		return nil, err
	}

	// PKCS#8 PEM 私钥
	der, err := marshalEd25519Private(priv)
	if err != nil {
		return nil, err
	}
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	return &DeployKey{
		PrivateKeyPEM:       privPEM,
		PublicKeyAuthorized: sshPub,
		Fingerprint:         FingerprintSHA256(sshPub),
	}, nil
}

// FingerprintSHA256 计算 authorized_keys 公钥的 SHA256 指纹。
// 返回 "SHA256:<base64>"(openssh -lf 格式,无填充)。
func FingerprintSHA256(authorizedKey string) string {
	fields := strings.Fields(strings.TrimSpace(authorizedKey))
	if len(fields) < 2 {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// MatchFingerprint 判断 authorized_keys 公钥是否匹配期望指纹。
// 期望值支持 "SHA256:xxx" 或裸 base64(自动补前缀)。
func MatchFingerprint(authorizedKey, want string) bool {
	got := FingerprintSHA256(authorizedKey)
	if got == "" || want == "" {
		return false
	}
	want = strings.TrimSpace(want)
	if !strings.HasPrefix(want, "SHA256:") {
		want = "SHA256:" + want
	}
	return got == want
}

// sshPublicKeyFromEd25519 把 ed25519 公钥编码为 authorized_keys 行。
func sshPublicKeyFromEd25519(pub ed25519.PublicKey, comment string) (string, error) {
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", oekiterrors.Wrap(err, "wrap ed25519 public key")
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if comment != "" && !strings.Contains(line, " "+comment) {
		line += " " + comment
	}
	return line, nil
}

func marshalEd25519Private(priv ed25519.PrivateKey) ([]byte, error) {
	return x509.MarshalPKCS8PrivateKey(priv)
}
