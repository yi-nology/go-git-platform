// Example: encrypt a token at rest with argon2id-derived AES-256-GCM and
// decrypt it later. Reads the passphrase from ENCRYPTION_KEY.
package main

import (
	"fmt"
	"log"

	"github.com/yi-nology/go-git-platform/auth/credential"
)

func main() {
	mgr, err := credential.NewCryptoManager()
	if err != nil {
		log.Fatalf("new crypto manager: %v", err)
	}

	secret := "super-secret-token"
	encrypted, err := mgr.Encrypt(secret)
	if err != nil {
		log.Fatalf("encrypt: %v", err)
	}
	fmt.Println("encrypted:", encrypted)

	decrypted, err := mgr.Decrypt(encrypted)
	if err != nil {
		log.Fatalf("decrypt: %v", err)
	}
	fmt.Println("decrypted:", decrypted)
}
