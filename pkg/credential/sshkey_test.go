package credential

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"crypto/x509"
	"embed"
	"encoding/pem"
	"errors"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// TestSSHKeyHelper_HostKeysConcurrent guards M5: the host-key store must be
// safe for concurrent access. Under -race this fails instantly if the
// hostKeys map is mutated without synchronization.
func TestSSHKeyHelper_HostKeysConcurrent(t *testing.T) {
	h := NewSSHKeyHelper()
	cb := h.GetHostKeyCallback()

	// A few distinct test public keys to insert/verify concurrently.
	keys := make([]ssh.PublicKey, 4)
	for i := range keys {
		pub, _, err := ed25519.GenerateKey(crand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		pk, err := ssh.NewPublicKey(pub)
		if err != nil {
			t.Fatalf("new public key: %v", err)
		}
		keys[i] = pk
	}

	const goroutines = 60
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			known := "known.example.com"
			tofu := "tofu.example.com"
			k := keys[i%len(keys)]
			h.AddHostKey(known, k)
			_ = cb(known, nil, k) // verify against stored key
			_ = cb(tofu, nil, k)  // trust-on-first-use insert
		}(i)
	}
	wg.Wait()
}

// TestSSHKeyHelper_HostKeyMismatch ensures a returning host with a different
// key is rejected (TOFU verification actually compares keys).
func TestSSHKeyHelper_HostKeyMismatch(t *testing.T) {
	h := NewSSHKeyHelper()

	pub1, _, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key1: %v", err)
	}
	pub2, _, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key2: %v", err)
	}
	pk1, _ := ssh.NewPublicKey(pub1)
	pk2, _ := ssh.NewPublicKey(pub2)

	cb := h.GetHostKeyCallback()
	if err := cb("host.example.com", nil, pk1); err != nil {
		t.Fatalf("first TOFU accept failed: %v", err)
	}
	if err := cb("host.example.com", nil, pk2); err == nil {
		t.Fatal("expected mismatch error when a known host presents a different key")
	}
	// Same key must still verify cleanly.
	if err := cb("host.example.com", nil, pk1); err != nil {
		t.Fatalf("expected clean verify for matching key, got %v", err)
	}
}

// TestSSHKeyHelper_ProcessPrivateKey_PreservesCauseChain pins the rule-1 fix
// (issue #68): wrapping a parse failure with oekiterrors.Wrap must keep the
// cause reachable via errors.Is/Unwrap — the pre-fix `%v` formatting erased
// the chain, hiding typed causes like ssh.PassphraseMissingError from
// callers.
func TestSSHKeyHelper_ProcessPrivateKey_PreservesCauseChain(t *testing.T) {
	// A valid, unencrypted key: parsing it WITH a passphrase is a
	// deterministic rejection whose typed cause comes straight from the ssh
	// package, so the assertion does not depend on message text.
	_, priv, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	_, wantCause := ssh.ParseRawPrivateKeyWithPassphrase([]byte(pemKey), []byte("secret"))
	if wantCause == nil {
		t.Fatal("expected the ssh package to reject an unencrypted key with a passphrase")
	}

	h := NewSSHKeyHelper()
	_, err = h.ProcessPrivateKey(pemKey, "secret")
	if err == nil {
		t.Fatal("expected ProcessPrivateKey to fail")
	}
	// The ssh package builds this cause inline, so identity-based errors.Is
	// cannot match across calls; chain reachability plus cause-message
	// equality is the exact property the pre-fix `%v` formatting destroyed.
	gotCause := errors.Unwrap(err)
	if gotCause == nil {
		t.Fatal("Unwrap(err) = nil: percent-v-style wrapping erased the cause chain")
	}
	if gotCause.Error() != wantCause.Error() {
		t.Fatalf("wrong cause on chain: got %q, want %q", gotCause, wantCause)
	}
	if !strings.Contains(err.Error(), "failed to parse encrypted private key") {
		t.Errorf("friendly context missing from message: %q", err.Error())
	}
}

//go:embed testdata
var keyFixtures embed.FS

// TestSSHKeyHelper_ProcessPrivateKey_DecryptRoundTrip 钉解密成功路径：带口令
// 的 openssh ed25519 私钥经 ProcessPrivateKey 解密重编码为无口令 PKCS8 后，
// 必须能用无口令 ParsePrivateKey 解析且与夹具公钥同一密钥对。回归背景
// （随 gitbackend #142 接线发现）：openssh 的 ed25519 解出 *ed25519.PrivateKey，
// x509.MarshalPKCS8PrivateKey 不认指针类型——不解引用则最常见的现代密钥类型
// 走到 "unknown key type while marshaling PKCS#8"。
func TestSSHKeyHelper_ProcessPrivateKey_DecryptRoundTrip(t *testing.T) {
	encPEM, err := keyFixtures.ReadFile("testdata/enc_ed25519.pem")
	if err != nil {
		t.Fatal(err)
	}
	pubPEM, err := keyFixtures.ReadFile("testdata/enc_ed25519.pub")
	if err != nil {
		t.Fatal(err)
	}

	h := NewSSHKeyHelper()
	plain, err := h.ProcessPrivateKey(string(encPEM), "pkgcred-pass")
	if err != nil {
		t.Fatalf("decrypt with correct passphrase: %v", err)
	}
	signer, err := ssh.ParsePrivateKey([]byte(plain))
	if err != nil {
		t.Fatalf("re-encoded key must parse WITHOUT passphrase: %v\nkey:\n%s", err, plain)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubPEM)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ssh.FingerprintSHA256(signer.PublicKey()), ssh.FingerprintSHA256(pubKey); got != want {
		t.Fatalf("round-trip produced a different keypair: got %s, want %s", got, want)
	}

	// CRLF 归一面：Windows 行尾的加密 key 同样可解
	crlf := strings.ReplaceAll(string(encPEM), "\n", "\r\n")
	if plain2, err := h.ProcessPrivateKey(crlf, "pkgcred-pass"); err != nil || !strings.HasSuffix(plain2, "\n") {
		t.Fatalf("CRLF key: plain=%q err=%v", plain2, err)
	}
}
