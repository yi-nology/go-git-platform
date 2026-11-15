package credential

import (
	"encoding/base64"
	"os"
	"testing"
)

func TestCryptoManager_EncryptDecrypt(t *testing.T) {
	mgr := NewCryptoManagerFromKey("test-key-32-bytes-long-for-aes!!")

	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"simple text", "hello world"},
		{"special chars", "!@#$%^&*()_+{}|:<>?"},
		{"unicode", "你好世界"},
		{"long text", "this is a longer plaintext message that should still encrypt and decrypt correctly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := mgr.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			if tt.plaintext == "" {
				if encrypted != "" {
					t.Fatalf("expected empty string for empty input, got %q", encrypted)
				}
				return
			}

			if encrypted == "" {
				t.Fatal("expected non-empty encrypted output")
			}

			if encrypted == tt.plaintext {
				t.Fatal("encrypted output should differ from plaintext")
			}

			decrypted, err := mgr.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Fatalf("expected %q, got %q", tt.plaintext, decrypted)
			}
		})
	}
}

func TestCryptoManager_ShortKey_Derived(t *testing.T) {
	// A short passphrase is stretched to a 32-byte AES key per ciphertext via
	// argon2id with a random salt.
	mgr := NewCryptoManagerFromKey("short")

	encrypted, err := mgr.Encrypt("hello")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := mgr.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != "hello" {
		t.Fatalf("expected %q, got %q", "hello", decrypted)
	}
}

func TestCryptoManager_LongKey_Derived(t *testing.T) {
	// A long passphrase is also stretched via argon2id.
	longKey := "this-key-is-much-longer-than-32-bytes-and-should-be-hashed"
	mgr := NewCryptoManagerFromKey(longKey)

	encrypted, err := mgr.Encrypt("world")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := mgr.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != "world" {
		t.Fatalf("expected %q, got %q", "world", decrypted)
	}
}

func TestCryptoManager_RotateKey(t *testing.T) {
	mgr := NewCryptoManagerFromKey("old-key-exactly-thirty-two-byte!")

	encrypted, err := mgr.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Rotate to new key
	mgr.RotateKey("new-key-exactly-thirty-two-byte!")

	// Decrypting old ciphertext with new key should fail
	_, err = mgr.Decrypt(encrypted)
	if err == nil {
		t.Fatal("expected error when decrypting with rotated key")
	}

	// Encrypting with new key should work
	encrypted2, err := mgr.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt after rotation failed: %v", err)
	}

	decrypted, err := mgr.Decrypt(encrypted2)
	if err != nil {
		t.Fatalf("Decrypt after rotation failed: %v", err)
	}

	if decrypted != "secret" {
		t.Fatalf("expected %q, got %q", "secret", decrypted)
	}
}

func TestCryptoManager_DifferentKeys_CantDecrypt(t *testing.T) {
	mgr1 := NewCryptoManagerFromKey("key-one-exactly-thirty-two-byte!")
	mgr2 := NewCryptoManagerFromKey("key-two-exactly-thirty-two-byte!")

	encrypted, err := mgr1.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = mgr2.Decrypt(encrypted)
	if err == nil {
		t.Fatal("expected error when decrypting with different key")
	}
}

func TestCryptoManager_InvalidCiphertext(t *testing.T) {
	mgr := NewCryptoManagerFromKey("test-key-32-bytes-long-for-aes!!")

	_, err := mgr.Decrypt("invalid-base64-data!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64 data")
	}
}

func TestCryptoManager_TruncatedCiphertext(t *testing.T) {
	mgr := NewCryptoManagerFromKey("test-key-32-bytes-long-for-aes!!")

	encrypted, err := mgr.Encrypt("test message")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if len(encrypted) > 10 {
		_, err = mgr.Decrypt(encrypted[:5])
		if err == nil {
			t.Fatal("expected error for truncated ciphertext")
		}
	}
}

func TestNewCryptoManager_NoEnvVar(t *testing.T) {
	// Save and restore
	oldVal, hadVal := os.LookupEnv("ENCRYPTION_KEY")
	os.Unsetenv("ENCRYPTION_KEY")
	defer func() {
		if hadVal {
			os.Setenv("ENCRYPTION_KEY", oldVal)
		}
	}()

	_, err := NewCryptoManager()
	if err == nil {
		t.Fatal("expected error when ENCRYPTION_KEY is not set")
	}
}

func TestNewCryptoManager_FromEnv(t *testing.T) {
	os.Setenv("ENCRYPTION_KEY", "test-key-32-bytes-long-for-aes!!")
	defer os.Unsetenv("ENCRYPTION_KEY")

	mgr, err := NewCryptoManager()
	if err != nil {
		t.Fatalf("NewCryptoManager failed: %v", err)
	}

	encrypted, err := mgr.Encrypt("hello")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := mgr.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != "hello" {
		t.Fatalf("expected %q, got %q", "hello", decrypted)
	}
}

func TestDeriveKey_OutputLength(t *testing.T) {
	salt := make([]byte, saltLen)
	result := deriveKey([]byte("any-passphrase"), salt)
	if len(result) != argonKeyLen {
		t.Fatalf("expected %d bytes, got %d", argonKeyLen, len(result))
	}
}

func TestDeriveKey_DeterministicSameSalt(t *testing.T) {
	salt := make([]byte, saltLen)
	salt[0] = 0x42
	r1 := deriveKey([]byte("short"), salt)
	r2 := deriveKey([]byte("short"), salt)
	if len(r1) != len(r2) {
		t.Fatalf("length mismatch: %d vs %d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i] != r2[i] {
			t.Fatal("deriveKey should be deterministic for the same (material, salt)")
		}
	}
}

func TestDeriveKey_DifferentSaltProducesDifferentKey(t *testing.T) {
	saltA := make([]byte, saltLen)
	saltB := make([]byte, saltLen)
	saltB[0] = 0x01
	r1 := deriveKey([]byte("same-passphrase"), saltA)
	r2 := deriveKey([]byte("same-passphrase"), saltB)
	same := true
	for i := range r1 {
		if r1[i] != r2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different salts should produce different keys for the same passphrase")
	}
}

func TestDeriveKey_DifferentInputs(t *testing.T) {
	salt := make([]byte, saltLen)
	r1 := deriveKey([]byte("aaa"), salt)
	r2 := deriveKey([]byte("bbb"), salt)
	same := true
	for i := range r1 {
		if r1[i] != r2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different inputs should produce different keys")
	}
}

// TestCryptoManager_DifferentCiphertextsHaveDifferentSalts verifies that
// encrypting the same plaintext twice yields two ciphertexts that carry
// distinct salts (and therefore distinct ciphertexts).
func TestCryptoManager_DifferentCiphertextsHaveDifferentSalts(t *testing.T) {
	mgr := NewCryptoManagerFromKey("passphrase")

	c1, err := mgr.Encrypt("same")
	if err != nil {
		t.Fatalf("Encrypt #1 failed: %v", err)
	}
	c2, err := mgr.Encrypt("same")
	if err != nil {
		t.Fatalf("Encrypt #2 failed: %v", err)
	}
	if c1 == c2 {
		t.Fatal("encrypting the same plaintext twice must yield different ciphertexts")
	}
}

// TestCryptoManager_RejectsWrongVersion verifies the version guard: a
// ciphertext whose leading version byte is not the current one is rejected
// with a clear error rather than being fed to the cipher.
func TestCryptoManager_RejectsWrongVersion(t *testing.T) {
	mgr := NewCryptoManagerFromKey("passphrase")

	ct, err := mgr.Encrypt("hello")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	raw, err := base64.URLEncoding.DecodeString(ct)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	b := raw
	b[0] = 0xFF // corrupt the version byte
	tampered := base64.URLEncoding.EncodeToString(b)

	_, err = mgr.Decrypt(tampered)
	if err == nil {
		t.Fatal("expected error for unsupported ciphertext version")
	}
}
