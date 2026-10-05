package gitbackend

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	xhttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	xssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type GoGitBackend struct {
	logger Logger
}

func NewGoGitBackend(opts Options) *GoGitBackend {
	logger := opts.Logger
	if logger == nil {
		logger = NewNoopLogger()
	}
	return &GoGitBackend{logger: logger}
}

// TransportAuth maps AuthConfig onto a go-git transport.AuthMethod for callers
// that drive go-git directly (e.g. a mirror pipeline that needs to list remote
// tags or fetch by refspec) instead of going through a GitBackend. It is
// package-level and stateless: the mapping depends only on the AuthConfig.
//
// AuthNone (and the empty Type) maps to a nil AuthMethod — anonymous access —
// which is what go-git expects for "no credentials".
func TransportAuth(auth AuthConfig) (transport.AuthMethod, error) {
	return buildTransportAuth(auth)
}

// buildTransportAuth maps AuthConfig onto a go-git AuthMethod. SSH key parse
// failures and host-key setup problems are returned as errors wrapping
// ErrAuthFailed — silently falling back to anonymous auth used to surface as
// a misleading "authentication required" instead of the real cause.
func buildTransportAuth(auth AuthConfig) (transport.AuthMethod, error) {
	switch auth.Type {
	case AuthHTTPBasic:
		return &xhttp.BasicAuth{
			Username: auth.Username,
			Password: auth.Password,
		}, nil
	case AuthHTTPToken:
		return &xhttp.TokenAuth{
			Token: auth.Token,
		}, nil
	case AuthSSH:
		authFail := func(err error) (transport.AuthMethod, error) {
			return nil, fmt.Errorf("%w: %v", ErrAuthFailed, err)
		}
		hk, err := hostKeyCallbackWithConfig(auth)
		if err != nil {
			return authFail(err)
		}
		// Try SSHKeyContent first (for DB-stored keys)
		if auth.SSHKeyContent != "" {
			signer, err := xssh.NewPublicKeys("git", []byte(auth.SSHKeyContent), auth.Passphrase)
			if err != nil {
				return authFail(fmt.Errorf("parse SSH key from SSHKeyContent: %w", err))
			}
			signer.HostKeyCallback = hk
			return signer, nil
		}
		// Fall back to SSHKey file path
		if auth.SSHKey != "" {
			if _, err := os.Stat(auth.SSHKey); err == nil {
				signer, err := xssh.NewPublicKeysFromFile("git", auth.SSHKey, auth.Passphrase)
				if err != nil {
					return authFail(fmt.Errorf("parse SSH key %s: %w", auth.SSHKey, err))
				}
				signer.HostKeyCallback = hk
				return signer, nil
			}
			return authFail(fmt.Errorf("SSH key file %s not found", auth.SSHKey))
		}
	}
	return nil, nil
}

// hostKeyCallback returns the SSH host-key verification strategy:
//
//   - InsecureSkipTLS: accept any host key (explicit opt-out);
//   - otherwise: verify against ~/.ssh/known_hosts. A missing known_hosts
//     file is an error with an actionable message — first-use MITM is the
//     exact threat host-key verification exists for, so unknown hosts are
//     never silently trusted.
//
// hostKeyCallback 返回主机密钥校验策略(兼容旧签名)。
func hostKeyCallback(insecure bool) (ssh.HostKeyCallback, error) {
	return hostKeyCallbackWithConfig(AuthConfig{InsecureSkipTLS: insecure})
}

// hostKeyCallbackWithConfig 按 AuthConfig 选择校验策略:
//  1. InsecureSkipTLS: 跳过校验(显式不安全)
//  2. HostKeyFingerprint: 指纹钉扎,必须匹配
//  3. 默认: known_hosts(可用 KnownHostsPath 指定路径)
func hostKeyCallbackWithConfig(auth AuthConfig) (ssh.HostKeyCallback, error) {
	if auth.InsecureSkipTLS {
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		}, nil
	}

	// 指纹钉扎:最强约束,不依赖 known_hosts 文件
	if fp := strings.TrimSpace(auth.HostKeyFingerprint); fp != "" {
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			got := ssh.FingerprintSHA256(key)
			want := fp
			if !strings.HasPrefix(want, "SHA256:") {
				want = "SHA256:" + want
			}
			if got != want {
				return fmt.Errorf("ssh host key fingerprint mismatch for %s: got %s want %s (possible MITM)", hostname, got, want)
			}
			return nil
		}, nil
	}

	kh := strings.TrimSpace(auth.KnownHostsPath)
	if kh == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir for known_hosts: %w", err)
		}
		kh = filepath.Join(home, ".ssh", "known_hosts")
	}
	if _, err := os.Stat(kh); err != nil {
		return nil, fmt.Errorf("ssh host key verification: %s not found; add the host key (ssh-keyscan host >> %s), set HostKeyFingerprint, or set InsecureSkipTLS", kh, kh)
	}
	cb, err := knownhosts.New(kh)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", kh, err)
	}
	return cb, nil
}

// buildFetchRefSpecs builds the refspecs for a fetch operation.
func buildFetchRefSpecs(opts FetchOptions) []config.RefSpec {
	if len(opts.Branches) == 0 {
		return []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/*:refs/remotes/%s/*", opts.Remote)),
		}
	}
	specs := make([]config.RefSpec, 0, len(opts.Branches))
	for _, branch := range opts.Branches {
		if isCommitSHA(branch) {
			continue
		}
		if strings.HasPrefix(branch, "refs/") {
			branchName := strings.TrimPrefix(branch, "refs/heads/")
			specs = append(specs, config.RefSpec(fmt.Sprintf("+%s:refs/remotes/%s/%s", branch, opts.Remote, branchName)))
		} else {
			specs = append(specs, config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", branch, opts.Remote, branch)))
		}
	}
	if len(specs) == 0 {
		return []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/*:refs/remotes/%s/*", opts.Remote)),
		}
	}
	return specs
}

// openRepo opens the repository at repoPath, mapping go-git's not-found error
// to the package's ErrRepoNotFound GitError tagged with op. Every gogit
// backend method funnels its repository open through here, so the wrap
// semantics live in exactly one place.
func openRepo(op, repoPath string) (*git.Repository, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, newGitError(op, repoPath, "", fmt.Errorf("%w: %v", ErrRepoNotFound, err))
	}
	return repo, nil
}
