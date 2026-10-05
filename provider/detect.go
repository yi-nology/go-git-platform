package provider

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
)

// DetectResult holds the platform, owner, repo, and base API URL extracted
// from a git remote URL by DetectPlatform.
//
// Owner/Repo are split as "first path segment = Owner, remainder = Repo".
// For GitLab repositories nested in subgroups the full subgroup path stays
// part of Repo: "https://gitlab.com/group/sub/repo" yields Owner="group"
// and Repo="sub/repo". Callers must therefore treat Repo as a path (it may
// contain "/"), not a bare repository name.
type DetectResult struct {
	Platform Platform
	Owner    string
	Repo     string
	BaseURL  string
}

// DetectPlatform parses a git remote URL (HTTPS, SSH, or ssh://) and returns
// the detected platform, owner, repo name, and base API URL. Returns
// ErrPlatformNotSupported for unrecognized hosts; use NewProvider with
// explicit Config for self-hosted instances not in the known-host list.
func DetectPlatform(remoteURL string) (*DetectResult, error) {
	if remoteURL == "" {
		return nil, fmt.Errorf("%w: empty remote URL", ErrInvalidInput)
	}

	if strings.HasPrefix(remoteURL, "git@") {
		return detectSSH(remoteURL)
	}
	if strings.HasPrefix(remoteURL, "https://") || strings.HasPrefix(remoteURL, "http://") {
		return detectHTTP(remoteURL)
	}
	if strings.HasPrefix(remoteURL, "ssh://") {
		return detectSSHProtocol(remoteURL)
	}
	return nil, fmt.Errorf("%w: unsupported URL format: %s", ErrInvalidInput, remoteURL)
}

func detectSSH(raw string) (*DetectResult, error) {
	rest := raw[4:]
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid SSH URL: %s", raw)
	}
	host := parts[0]
	path := cleanRepoPath(parts[1])
	pathParts := strings.SplitN(path, "/", 2)
	if len(pathParts) != 2 {
		return nil, fmt.Errorf("invalid SSH path: %s", path)
	}
	platform, baseURL, err := classifyHost(host)
	if err != nil {
		return nil, err
	}
	return &DetectResult{
		Platform: platform,
		Owner:    pathParts[0],
		Repo:     pathParts[1],
		BaseURL:  baseURL,
	}, nil
}

func detectSSHProtocol(raw string) (*DetectResult, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host := u.Host
	path := cleanRepoPath(u.Path)
	pathParts := strings.SplitN(path, "/", 2)
	if len(pathParts) != 2 {
		return nil, fmt.Errorf("invalid SSH path: %s", path)
	}
	platform, baseURL, err := classifyHost(host)
	if err != nil {
		return nil, err
	}
	return &DetectResult{
		Platform: platform,
		Owner:    pathParts[0],
		Repo:     pathParts[1],
		BaseURL:  baseURL,
	}, nil
}

func detectHTTP(raw string) (*DetectResult, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host := u.Host
	path := cleanRepoPath(u.Path)
	pathParts := strings.SplitN(path, "/", 2)
	if len(pathParts) != 2 {
		return nil, fmt.Errorf("invalid HTTP path: %s", path)
	}
	platform, baseURL, err := classifyHost(host)
	if err != nil {
		return nil, err
	}
	return &DetectResult{
		Platform: platform,
		Owner:    pathParts[0],
		Repo:     pathParts[1],
		BaseURL:  baseURL,
	}, nil
}

// cleanRepoPath normalizes the path portion of a clone URL: surrounding
// slashes are removed first so a trailing slash (".../owner/repo/") does not
// leak a "/" into Repo, then the ".git" suffix is stripped.
func cleanRepoPath(p string) string {
	return strings.TrimSuffix(strings.Trim(p, "/"), ".git")
}

// classifyHost maps a clone-URL host to a platform and its public API base
// URL. The port is stripped first, then matching is exact host or subdomain
// ("host == x" or strings.HasSuffix(host, ".x")). Self-hosted instances whose
// hostname merely *contains* a known public domain (e.g. "gitlab.company.com"
// contains "gitlab.com", "gitee.com.cn" contains "gitee.com") are NOT routed
// to the public-cloud BaseURL; they fall through to ErrPlatformNotSupported.
// knownHostPlatforms maps the public forge hosts (and their subdomains) to
// the platform they identify and its public API base URL.
var knownHostPlatforms = []struct {
	host     string
	platform Platform
	baseURL  string
}{
	{"github.com", PlatformGitHub, "https://api.github.com"},
	{"code.tencent.com", PlatformTencentCode, "https://git.code.tencent.com/api/v3"},
	{"codeberg.org", PlatformForgejo, "https://codeberg.org"},
	{"gitlab.com", PlatformGitLab, "https://gitlab.com/api/v4"},
	{"gitea.com", PlatformGitea, "https://gitea.com/api/v1"},
	{"gitee.com", PlatformGitee, "https://gitee.com/api/v5"},
	{"gitcode.com", PlatformGitCode, "https://api.gitcode.com/api/v5"},
}

// hostAliases holds hosts registered at init time via RegisterHostAlias, so
// a self-hosted alias (or a private deployment on a known domain shape) can
// be classified without editing this package. Registered aliases are matched
// before the built-in table.
var hostMu sync.RWMutex
var hostAliases []hostEntry

type hostEntry struct {
	host     string
	platform Platform
	baseURL  string
}

// RegisterHostAlias teaches DetectPlatform that host (and its subdomains)
// identifies platform with the given base API URL — the registry extension
// matching Register: adding a self-hosted alias no longer requires editing
// the provider core. Panics on an empty host, an empty platform, or a
// duplicate host (same fail-fast convention as Register).
func RegisterHostAlias(host string, platform Platform, baseURL string) {
	if host == "" {
		panic("provider: RegisterHostAlias with empty host")
	}
	if platform == "" {
		panic("provider: RegisterHostAlias with empty platform")
	}
	hostMu.Lock()
	defer hostMu.Unlock()
	for _, e := range hostAliases {
		if strings.EqualFold(e.host, host) {
			panic("provider: duplicate host alias registration for " + host)
		}
	}
	hostAliases = append(hostAliases, hostEntry{host: strings.ToLower(host), platform: platform, baseURL: baseURL})
}

func classifyHost(host string) (Platform, string, error) {
	lower := strings.ToLower(host)
	// Strip a trailing port ("github.com:8443"); SplitHostPort also handles
	// bracketed IPv6 ("[::1]:22"). Hosts without a port fail the split and
	// are kept as-is.
	if h, _, err := net.SplitHostPort(lower); err == nil {
		lower = h
	}
	hostMu.RLock()
	aliases := hostAliases
	hostMu.RUnlock()
	for _, k := range aliases {
		if lower == k.host || strings.HasSuffix(lower, "."+k.host) {
			return k.platform, k.baseURL, nil
		}
	}
	for _, k := range knownHostPlatforms {
		if lower == k.host || strings.HasSuffix(lower, "."+k.host) {
			return k.platform, k.baseURL, nil
		}
	}
	return "", "", fmt.Errorf("%w: unrecognized host %q; use provider.NewProvider with explicit platform config instead of DetectPlatform", ErrPlatformNotSupported, host)
}
