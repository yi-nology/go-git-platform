package credential

import (
	"fmt"
	"net/url"

	"github.com/oekit/pkg/shellx"
)

// Manager provides helpers for constructing authenticated git remote URLs
// and SSH commands. It is stateless and safe for concurrent use.
type Manager struct{}

// NewManager creates a new credential Manager.
func NewManager() *Manager {
	return &Manager{}
}

// BuildAuthURL embeds credentials into a git remote URL. Supported authType
// values: "http_basic" (user:pass), "http_token" (token only), "ssh" (passthrough).
//
// Credentials are interpolated via net/url so that characters with special
// meaning in the userinfo component (@, :, /, #, ?, etc.) are percent-encoded.
// Interpolating them raw produced malformed or ambiguous URLs and could leak
// the secret into the wrong URL component.
func (m *Manager) BuildAuthURL(remoteURL, authType, username, secret string) string {
	if authType != "http_basic" && authType != "http_token" {
		return remoteURL
	}
	u, err := url.Parse(remoteURL)
	if err != nil {
		return remoteURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return remoteURL
	}
	switch {
	case authType == "http_basic" && username != "":
		u.User = url.UserPassword(username, secret)
	default:
		// http_token, or http_basic without a username: the secret goes into
		// the userinfo alone.
		u.User = url.User(secret)
	}
	return u.String()
}

// BuildSSHCommand returns a GIT_SSH_COMMAND value that uses the given key file
// with strict host key checking enabled (the secure default).
// Use BuildSSHCommandInsecure for CI environments where known_hosts is unavailable.
func (m *Manager) BuildSSHCommand(sshKeyPath string) string {
	if sshKeyPath == "" {
		return ""
	}
	return fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=yes -o UserKnownHostsFile=~/.ssh/known_hosts", shellx.Quote(sshKeyPath))
}

// BuildSSHCommandInsecure returns a GIT_SSH_COMMAND value with host key
// checking disabled. Only suitable for CI/server environments where
// known_hosts cannot be populated. Do NOT use in production.
func (m *Manager) BuildSSHCommandInsecure(sshKeyPath string) string {
	if sshKeyPath == "" {
		return ""
	}
	return fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", shellx.Quote(sshKeyPath))
}
