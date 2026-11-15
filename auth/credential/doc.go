// Package credential manages the long-lived secrets a git automation
// service keeps on behalf of its users: platform access tokens, SSH key
// pairs, and per-repository deploy keys.
//
// Three cooperating pieces:
//
//   - Manager (manager.go) stores encrypted credentials (AES-GCM) and
//     hands back plaintext for a single operation. Keys are derived from
//     a master secret; stored ciphertext never contains the raw token.
//   - SSHKey (sshkey.go) builds git-ready SSH authentication from a
//     private key, and assembles the ssh command line for the native git
//     backend (including optional host-key fingerprint pinning).
//   - DeployKey (deploykey.go) generates ed25519 deploy-key pairs whose
//     public half is registered on the target platform via
//     provider.DeploymentKeyManager; the private half stays in the
//     manager's encrypted store.
//
// The package deliberately contains no platform API calls: it is the
// storage and key-material layer beneath the provider and gitbackend
// packages.
package credential
