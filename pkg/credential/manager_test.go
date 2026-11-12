package credential

import (
	"strings"
	"testing"
)

func TestBuildAuthURL_HTTPSToken(t *testing.T) {
	m := NewManager()
	result := m.BuildAuthURL("https://github.com/owner/repo.git", "http_token", "", "mytoken")
	expected := "https://mytoken@github.com/owner/repo.git"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestBuildAuthURL_HTTPSBasic(t *testing.T) {
	m := NewManager()
	result := m.BuildAuthURL("https://github.com/owner/repo.git", "http_basic", "user", "pass")
	expected := "https://user:pass@github.com/owner/repo.git"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestBuildAuthURL_HTTPBasicNoUser(t *testing.T) {
	m := NewManager()
	result := m.BuildAuthURL("https://github.com/owner/repo.git", "http_basic", "", "pass")
	expected := "https://pass@github.com/owner/repo.git"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestBuildAuthURL_HTTP(t *testing.T) {
	m := NewManager()
	result := m.BuildAuthURL("http://gitea.local/owner/repo.git", "http_token", "", "mytoken")
	expected := "http://mytoken@gitea.local/owner/repo.git"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestBuildAuthURL_SSH(t *testing.T) {
	m := NewManager()
	result := m.BuildAuthURL("git@github.com:owner/repo.git", "ssh", "", "")
	expected := "git@github.com:owner/repo.git"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestBuildAuthURL_UnknownType(t *testing.T) {
	m := NewManager()
	result := m.BuildAuthURL("https://github.com/owner/repo.git", "unknown", "", "")
	expected := "https://github.com/owner/repo.git"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

// TestBuildAuthURL_EscapesSpecialChars guards M6: characters that are special
// in the userinfo component (@, :, /) must be percent-encoded so the URL stays
// unambiguous and the secret does not leak into another component.
func TestBuildAuthURL_EscapesSpecialChars(t *testing.T) {
	m := NewManager()
	// A token containing '@' previously produced "https://tok@en@host/...",
	// shifting the userinfo boundary. It must now be percent-encoded.
	result := m.BuildAuthURL("https://github.com/owner/repo.git", "http_token", "", "tok@en")
	if !strings.Contains(result, "tok%40en@github.com") {
		t.Fatalf("expected '@' escaped in userinfo, got %q", result)
	}

	// http_basic with a username containing ':' must encode the colon.
	result = m.BuildAuthURL("https://github.com/owner/repo.git", "http_basic", "user:name", "pass")
	// userinfo should render as user%3Aname:pass@github.com
	if !strings.Contains(result, "user%3Aname:pass@github.com") {
		t.Fatalf("expected ':' in username escaped, got %q", result)
	}
}

func TestBuildAuthURL_NonHTTPSPassthrough(t *testing.T) {
	m := NewManager()
	// Non-http(s) schemes fall through unchanged even for token auth.
	in := "git://github.com/owner/repo.git"
	if got := m.BuildAuthURL(in, "http_token", "", "tok"); got != in {
		t.Fatalf("expected passthrough for non-http(s), got %q", got)
	}
}

// BuildSSHCommand 必须产出 POSIX shell 单词：key 路径含 $ / 反引号 / 空格
// 时不得被 GIT_SSH_COMMAND 的 sh -c 展开（Go %q 不转义 $ 与反引号——
// 旧实现的保护声明不实，shellx.Quote 单引号化才是真保护）。
func TestBuildSSHCommand_ShellMetacharPath(t *testing.T) {
	m := &Manager{}
	got := m.BuildSSHCommand("/tmp/$HOME `id` key")
	if !strings.Contains(got, `'/tmp/$HOME `+"`id`"+` key'`) {
		t.Fatalf("key path not single-quoted: %s", got)
	}
	if strings.Contains(got, `"/tmp/$HOME`) {
		t.Fatalf("Go %%q double-quote form leaked: %s", got)
	}
}
