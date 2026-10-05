package gitbackend

import "testing"

// TestNewTokenAuth verifies the token auth mapping: a non-empty token
// travels as the HTTP Basic password under the fixed "token" username
// placeholder, and an empty token collapses to AuthNone.
func TestNewTokenAuth(t *testing.T) {
	if got := NewTokenAuth(""); got.Type != AuthNone {
		t.Errorf("empty token: expected AuthNone, got %+v", got)
	}
	got := NewTokenAuth("secret")
	if got.Type != AuthHTTPBasic || got.Username != "token" || got.Password != "secret" {
		t.Errorf("unexpected config: %+v", got)
	}
}

// TestNewHTTPBasicAuth verifies both credentials must be empty for the
// AuthNone collapse; a lone username or password stays HTTP Basic.
func TestNewHTTPBasicAuth(t *testing.T) {
	if got := NewHTTPBasicAuth("", ""); got.Type != AuthNone {
		t.Errorf("empty credentials: expected AuthNone, got %+v", got)
	}
	got := NewHTTPBasicAuth("user", "pass")
	if got.Type != AuthHTTPBasic || got.Username != "user" || got.Password != "pass" {
		t.Errorf("unexpected config: %+v", got)
	}
	if got := NewHTTPBasicAuth("user", ""); got.Type != AuthHTTPBasic {
		t.Errorf("username-only: expected AuthHTTPBasic, got %+v", got)
	}
}

// TestNewSSHKeyFileAuth verifies the empty-path collapse and the SSH
// mapping of path plus passphrase.
func TestNewSSHKeyFileAuth(t *testing.T) {
	if got := NewSSHKeyFileAuth("", "pw"); got.Type != AuthNone {
		t.Errorf("empty path: expected AuthNone, got %+v", got)
	}
	got := NewSSHKeyFileAuth("/home/me/.ssh/id_ed25519", "pw")
	if got.Type != AuthSSH || got.SSHKey != "/home/me/.ssh/id_ed25519" || got.Passphrase != "pw" {
		t.Errorf("unexpected config: %+v", got)
	}
}

// TestNewSSHKeyContentAuth verifies the empty-content collapse and the
// in-memory key mapping.
func TestNewSSHKeyContentAuth(t *testing.T) {
	if got := NewSSHKeyContentAuth("", ""); got.Type != AuthNone {
		t.Errorf("empty content: expected AuthNone, got %+v", got)
	}
	got := NewSSHKeyContentAuth("-----BEGIN OPENSSH PRIVATE KEY-----", "pw")
	if got.Type != AuthSSH || got.SSHKeyContent != "-----BEGIN OPENSSH PRIVATE KEY-----" || got.Passphrase != "pw" {
		t.Errorf("unexpected config: %+v", got)
	}
}
