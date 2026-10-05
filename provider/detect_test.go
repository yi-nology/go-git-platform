package provider

import (
	"testing"
)

func TestDetectPlatform_SSH_GitHub(t *testing.T) {
	r, err := DetectPlatform("git@github.com:owner/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Platform != PlatformGitHub {
		t.Errorf("expected GitHub, got %s", r.Platform)
	}
	if r.Owner != "owner" {
		t.Errorf("expected owner, got %s", r.Owner)
	}
	if r.Repo != "repo" {
		t.Errorf("expected repo, got %s", r.Repo)
	}
	if r.BaseURL != "https://api.github.com" {
		t.Errorf("unexpected BaseURL: %s", r.BaseURL)
	}
}

func TestDetectPlatform_SSH_GitLab(t *testing.T) {
	r, err := DetectPlatform("git@gitlab.com:org/project.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Platform != PlatformGitLab {
		t.Errorf("expected GitLab, got %s", r.Platform)
	}
	if r.Owner != "org" || r.Repo != "project" {
		t.Errorf("owner=%s repo=%s", r.Owner, r.Repo)
	}
}

func TestDetectPlatform_SSH_Gitea(t *testing.T) {
	r, err := DetectPlatform("git@gitea.com:user/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Platform != PlatformGitea {
		t.Errorf("expected Gitea, got %s", r.Platform)
	}
}

func TestDetectPlatform_SSH_StripGitSuffix(t *testing.T) {
	r, err := DetectPlatform("git@github.com:owner/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo != "repo" {
		t.Errorf("expected .git stripped, got %s", r.Repo)
	}
}

func TestDetectPlatform_SSH_NoGitSuffix(t *testing.T) {
	r, err := DetectPlatform("git@github.com:owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo != "repo" {
		t.Errorf("expected repo, got %s", r.Repo)
	}
}

func TestDetectPlatform_HTTPS_GitHub(t *testing.T) {
	r, err := DetectPlatform("https://github.com/owner/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Platform != PlatformGitHub {
		t.Errorf("expected GitHub, got %s", r.Platform)
	}
	if r.Owner != "owner" || r.Repo != "repo" {
		t.Errorf("owner=%s repo=%s", r.Owner, r.Repo)
	}
}

func TestDetectPlatform_HTTPS_GitLab(t *testing.T) {
	r, err := DetectPlatform("https://gitlab.com/org/project.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Platform != PlatformGitLab {
		t.Errorf("expected GitLab, got %s", r.Platform)
	}
}

func TestDetectPlatform_HTTPS_TrailingSlash(t *testing.T) {
	tests := []string{
		"https://github.com/owner/repo/",
		"https://github.com/owner/repo.git/",
		"http://github.com/owner/repo/",
	}
	for _, url := range tests {
		r, err := DetectPlatform(url)
		if err != nil {
			t.Fatalf("%s: %v", url, err)
		}
		if r.Owner != "owner" || r.Repo != "repo" {
			t.Errorf("%s: owner=%q repo=%q, want owner/repo (no trailing \"/\")", url, r.Owner, r.Repo)
		}
	}
}

func TestDetectPlatform_SSHProtocol_TrailingSlash(t *testing.T) {
	r, err := DetectPlatform("ssh://git@github.com:22/owner/repo/")
	if err != nil {
		t.Fatal(err)
	}
	if r.Owner != "owner" || r.Repo != "repo" {
		t.Errorf("owner=%q repo=%q, want owner/repo", r.Owner, r.Repo)
	}
}

func TestDetectPlatform_GitLabSubgroup_OwnerRepoSplit(t *testing.T) {
	// Documented semantics: first path segment -> Owner, remainder -> Repo
	// (Repo may itself contain slashes for GitLab subgroups).
	r, err := DetectPlatform("https://gitlab.com/group/sub/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Owner != "group" {
		t.Errorf("owner = %q, want %q", r.Owner, "group")
	}
	if r.Repo != "sub/repo" {
		t.Errorf("repo = %q, want %q", r.Repo, "sub/repo")
	}
}

func TestDetectPlatform_SSH_TrailingSlash(t *testing.T) {
	r, err := DetectPlatform("git@github.com:owner/repo/")
	if err != nil {
		t.Fatal(err)
	}
	if r.Owner != "owner" || r.Repo != "repo" {
		t.Errorf("owner=%q repo=%q, want owner/repo", r.Owner, r.Repo)
	}
}

func TestDetectPlatform_HTTP_SelfHosted_Unrecognized(t *testing.T) {
	_, err := DetectPlatform("http://gitlab.local/group/repo.git")
	if err == nil {
		t.Error("expected error for unrecognized self-hosted instance")
	}
	if !IsPlatformNotSupported(err) {
		t.Errorf("expected ErrPlatformNotSupported, got %v", err)
	}
}

func TestDetectPlatform_SSHProtocol(t *testing.T) {
	r, err := DetectPlatform("ssh://git@github.com:22/owner/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Platform != PlatformGitHub {
		t.Errorf("expected GitHub, got %s", r.Platform)
	}
	if r.Owner != "owner" || r.Repo != "repo" {
		t.Errorf("owner=%s repo=%s", r.Owner, r.Repo)
	}
}

func TestDetectPlatform_EmptyURL(t *testing.T) {
	_, err := DetectPlatform("")
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestDetectPlatform_UnsupportedFormat(t *testing.T) {
	_, err := DetectPlatform("ftp://example.com/repo.git")
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestDetectPlatform_InvalidSSHFormat(t *testing.T) {
	_, err := DetectPlatform("git@github.com")
	if err == nil {
		t.Error("expected error for invalid SSH URL (no path)")
	}
}

func TestDetectPlatform_InvalidHTTPPath(t *testing.T) {
	_, err := DetectPlatform("https://github.com/owner")
	if err == nil {
		t.Error("expected error for HTTP URL with no repo part")
	}
}

func TestClassifyHost_Unrecognized(t *testing.T) {
	_, _, err := classifyHost("git.mycompany.com")
	if err == nil {
		t.Error("expected error for unrecognized host")
	}
	if !IsPlatformNotSupported(err) {
		t.Errorf("expected ErrPlatformNotSupported, got %v", err)
	}
}

func TestClassifyHost_SelfHostedDomainsNotMisrouted(t *testing.T) {
	// These hostnames merely *contain* a known public domain; substring
	// matching used to route them to the public-cloud BaseURL.
	tests := []struct {
		host   string
		needle string // the public domain it used to trip on
	}{
		{"gitlab.company.com", "gitlab.com"},
		{"gitlab.corp.example.com", "gitlab.com"},
		{"gitee.com.cn", "gitee.com"},
		{"mygithub.com", "github.com"},
		{"github.com.evil.io", "github.com"},
		{"xcode.tencent.com", "code.tencent.com"},
		{"gitea.com.ru", "gitea.com"},
		{"gitcode.company.cn", "gitcode.com"},
	}
	for _, tc := range tests {
		platform, baseURL, err := classifyHost(tc.host)
		if err == nil {
			t.Errorf("%s: expected error, got platform=%s baseURL=%s", tc.host, platform, baseURL)
			continue
		}
		if !IsPlatformNotSupported(err) {
			t.Errorf("%s: expected ErrPlatformNotSupported, got %v", tc.host, err)
		}
	}
}

func TestClassifyHost_KnownHostsExactAndSubdomain(t *testing.T) {
	tests := []struct {
		host     string
		platform Platform
		baseURL  string
	}{
		{"github.com", PlatformGitHub, "https://api.github.com"},
		{"www.github.com", PlatformGitHub, "https://api.github.com"},
		{"gitlab.com", PlatformGitLab, "https://gitlab.com/api/v4"},
		{"code.tencent.com", PlatformTencentCode, "https://git.code.tencent.com/api/v3"},
		{"git.code.tencent.com", PlatformTencentCode, "https://git.code.tencent.com/api/v3"},
		{"codeberg.org", PlatformForgejo, "https://codeberg.org"},
		{"gitea.com", PlatformGitea, "https://gitea.com/api/v1"},
		{"gitee.com", PlatformGitee, "https://gitee.com/api/v5"},
		{"gitcode.com", PlatformGitCode, "https://api.gitcode.com/api/v5"},
	}
	for _, tc := range tests {
		platform, baseURL, err := classifyHost(tc.host)
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.host, err)
			continue
		}
		if platform != tc.platform || baseURL != tc.baseURL {
			t.Errorf("%s: got (%s, %s), want (%s, %s)", tc.host, platform, baseURL, tc.platform, tc.baseURL)
		}
	}
}

func TestClassifyHost_PortIsStripped(t *testing.T) {
	tests := []struct {
		host     string
		platform Platform
	}{
		{"github.com:8443", PlatformGitHub},
		{"gitlab.com:8443", PlatformGitLab},
		{"gitee.com:8080", PlatformGitee},
	}
	for _, tc := range tests {
		platform, _, err := classifyHost(tc.host)
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.host, err)
			continue
		}
		if platform != tc.platform {
			t.Errorf("%s: got %s, want %s", tc.host, platform, tc.platform)
		}
	}
}

func TestDetectPlatform_SelfHostedLookalikeDomains(t *testing.T) {
	tests := []string{
		"https://gitlab.company.com/group/repo.git",
		"git@gitlab.company.com:group/repo.git",
		"ssh://git@gitlab.company.com:2224/group/repo.git",
		"https://gitee.com.cn/owner/repo.git",
	}
	for _, url := range tests {
		_, err := DetectPlatform(url)
		if !IsPlatformNotSupported(err) {
			t.Errorf("%s: expected ErrPlatformNotSupported, got %v", url, err)
		}
	}
}

func TestDetectPlatform_HTTPS_WithPort(t *testing.T) {
	r, err := DetectPlatform("https://github.com:8443/owner/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if r.Platform != PlatformGitHub {
		t.Errorf("expected GitHub, got %s", r.Platform)
	}
	if r.Owner != "owner" || r.Repo != "repo" {
		t.Errorf("owner=%s repo=%s", r.Owner, r.Repo)
	}
}

func TestDetectPlatform_SSH_SelfHosted_Unrecognized(t *testing.T) {
	_, err := DetectPlatform("git@git.mycompany.com:team/project.git")
	if err == nil {
		t.Error("expected error for unrecognized self-hosted SSH URL")
	}
	if !IsPlatformNotSupported(err) {
		t.Errorf("expected ErrPlatformNotSupported, got %v", err)
	}
}

// TestRegisterHostAlias verifies the registry extension: a self-hosted alias
// registered at init time classifies like a built-in host, and duplicate
// registration panics (same convention as Register).
func TestRegisterHostAlias(t *testing.T) {
	RegisterHostAlias("git.company.internal", PlatformGitLab, "https://git.company.internal/api/v4")
	defer func() { hostAliases = nil }()

	res, err := DetectPlatform("git@git.company.internal:team/proj.git")
	if err != nil {
		t.Fatalf("alias host should classify: %v", err)
	}
	if res.Platform != PlatformGitLab || res.BaseURL != "https://git.company.internal/api/v4" {
		t.Errorf("unexpected classification: %+v", res)
	}
	// Subdomains of the alias classify too.
	if _, err := DetectPlatform("https://team.git.company.internal/team/proj"); err != nil {
		t.Errorf("alias subdomain should classify: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Error("expected duplicate registration to panic")
		}
	}()
	RegisterHostAlias("git.company.internal", PlatformGitea, "https://elsewhere")
}
