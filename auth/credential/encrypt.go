package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Ciphertext layout produced by Encrypt (after base64url encoding):
//
//	version(1) || salt(16) || nonce(12) || aes-gcm-ciphertext
//
// The version byte allows future format changes. Version 1 derives the AES-256
// key from the passphrase and the per-ciphertext salt using argon2id.
const (
	cipherVersion = 1
	saltLen       = 16
)

// argon2id parameters. These follow the OWASP "sensitive" recommendation
// (t=3, m=64MiB, p=2). A single Encrypt/Decrypt costs tens of milliseconds,
// which is acceptable for credential-at-rest storage (not a hot path) while
// making offline brute force of low-entropy passphrases expensive.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB -> 64 MiB
	argonThreads = 2
	argonKeyLen  = 32
)

// CryptoManager provides AES-256-GCM encryption/decryption with an
// injectable key source. Use NewCryptoManager or NewCryptoManagerFromKey
// to create an instance. The zero-value CryptoManager is not usable.
//
// The key material (passphrase) is stored in memory and a fresh argon2id
// key derivation is performed per encryption using a random salt, so
// identical plaintexts encrypt to different ciphertexts.
type CryptoManager struct {
	mu       sync.RWMutex
	material []byte
}

// NewCryptoManager creates a CryptoManager that reads the encryption key from
// the ENCRYPTION_KEY environment variable. The key may be a passphrase of any
// length; it is stretched to a 32-byte AES-256 key per ciphertext via argon2id
// with a random salt.
//
// Returns an error if ENCRYPTION_KEY is unset or empty.
func NewCryptoManager() (*CryptoManager, error) {
	k := os.Getenv("ENCRYPTION_KEY")
	if k == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY environment variable is required for credential encryption")
	}
	return NewCryptoManagerFromKey(k), nil
}

// NewCryptoManagerFromKey creates a CryptoManager with an explicit key string.
// The key may be a passphrase of any length (including short, low-entropy
// passphrases); argon2id makes brute-forcing such passphrases expensive.
func NewCryptoManagerFromKey(key string) *CryptoManager {
	// Copy so callers cannot mutate the stored material via the original slice.
	m := make([]byte, len(key))
	copy(m, key)
	return &CryptoManager{material: m}
}

// deriveKey derives a 32-byte AES-256 key from material and a per-ciphertext
// salt using argon2id. Deterministic for the same (material, salt) pair;
// different salts yield independent keys even for the same passphrase.
func deriveKey(material, salt []byte) []byte {
	return argon2.IDKey(material, salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

// Encrypt encrypts plaintext using AES-256-GCM. Returns empty string for
// empty input. The output is base64url(version || salt || nonce || ciphertext).
func (cm *CryptoManager) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	cm.mu.RLock()
	material := cm.material
	cm.mu.RUnlock()
	if len(material) == 0 {
		return "", errors.New("crypto manager: key material is empty")
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}

	block, err := aes.NewCipher(deriveKey(material, salt))
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Prepend version + salt + nonce; Seal appends the GCM ciphertext.
	prefix := make([]byte, 0, 1+saltLen+len(nonce)+len(plaintext)+aesGCM.Overhead())
	prefix = append(prefix, cipherVersion)
	prefix = append(prefix, salt...)
	prefix = append(prefix, nonce...)
	sealed := aesGCM.Seal(prefix, nonce, []byte(plaintext), nil)
	return base64.URLEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts ciphertext produced by Encrypt. Returns empty string for
// empty input. Only the current ciphertext version is supported; ciphertexts
// encrypted by older builds (single-iteration SHA-256 KDF) will fail to
// decrypt and must be re-encrypted with the current key.
func (cm *CryptoManager) Decrypt(cryptoText string) (string, error) {
	if cryptoText == "" {
		return "", nil
	}

	cm.mu.RLock()
	material := cm.material
	cm.mu.RUnlock()
	if len(material) == 0 {
		return "", errors.New("crypto manager: key material is empty")
	}

	data, err := base64.URLEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}

	if len(data) < 1 {
		return "", errors.New("ciphertext too short")
	}
	if data[0] != cipherVersion {
		return "", fmt.Errorf("unsupported ciphertext version %d (supported %d); re-encrypt the data with the current key", data[0], cipherVersion)
	}
	data = data[1:]

	if len(data) < saltLen {
		return "", errors.New("ciphertext too short")
	}
	salt := data[:saltLen]
	data = data[saltLen:]

	block, err := aes.NewCipher(deriveKey(material, salt))
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// RotateKey replaces the current key material. Subsequent Encrypt calls use
// the new key; Decrypt calls for data encrypted with the old key will fail.
func (cm *CryptoManager) RotateKey(newKey string) {
	m := make([]byte, len(newKey))
	copy(m, newKey)
	cm.mu.Lock()
	cm.material = m
	cm.mu.Unlock()
}
