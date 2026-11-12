package credential

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"crypto/x509"
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

// BuildSSHCommand 必须产出 POSIX shell 单词：key 路径含 $ / 反引号 / 空格
// 时不得被 GIT_SSH_COMMAND 的 sh -c 展开（Go %q 不转义 $ 与反引号——
// 旧实现的保护声明不实，shellx.Quote 单引号化才是真保护）。
func TestSSHKeyHelper_BuildSSHCommand_ShellMetacharPath(t *testing.T) {
	h := &SSHKeyHelper{}
	got := h.BuildSSHCommand("/tmp/$HOME `id` key")
	if !strings.Contains(got, `'/tmp/$HOME `+"`id`"+` key'`) {
		t.Fatalf("key path not single-quoted: %s", got)
	}
	if strings.Contains(got, `"/tmp/$HOME`) {
		t.Fatalf("Go %%q double-quote form leaked: %s", got)
	}
}
