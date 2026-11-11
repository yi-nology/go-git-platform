package mcpserver

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/yi-nology/go-git-platform/backends/tencentcode"
	"github.com/yi-nology/go-git-platform/provider"
)

// fakeAllProvider 类型断言全通过(嵌入全部工具面涉及的能力接口,类型集满足
// 即可——对账测试只注册工具、从不调用 handler,嵌入的 nil 接口不会被触碰)。
type fakeAllProvider struct {
	provider.Provider
	provider.IssueManager
	provider.NoteManager
	provider.CommitStatusManager
	provider.SearchManager
	provider.BranchProtectionManager
	provider.CollaboratorManager
	provider.WebhookConfigManager
	provider.CILogManager
	provider.DeploymentManager
	provider.TemplateRepoManager
	provider.ForkSyncManager
	tencentcode.TencentCodeExtras
}

func (f fakeAllProvider) Platform() provider.Platform { return provider.PlatformGitHub }
func (f fakeAllProvider) Capabilities() provider.CapabilitySet {
	return provider.CapabilitySet{}
}

// TestReadmeToolTableMatchesRegistry 对账 README 工具表与 add() 注册面
// (issue #122 的防再漂网):delete_issue_comment 上线时两份手维护表都没跟,
// 本测试让「注册了没写进表」/「表里列了没注册」都红。注册面真源是
// toolRegistry(add() 挂账),表是 mcp/README.md 的工具面表。
func TestReadmeToolTableMatchesRegistry(t *testing.T) {
	if _, err := NewServer(fakeAllProvider{}, Options{}); err != nil {
		t.Fatalf("NewServer over all-capability fake: %v", err)
	}
	if len(toolRegistry) == 0 {
		t.Fatal("toolRegistry is empty — registration recording drifted")
	}

	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read mcp/README.md: %v", err)
	}
	table := parseToolTable(t, string(raw))
	if len(table) == 0 {
		t.Fatal("no tool table rows found in mcp/README.md — parser drifted")
	}

	seenToolsets := map[string]bool{}
	for toolset, docTools := range table {
		seenToolsets[toolset] = true
		reg, ok := toolRegistry[toolset]
		if !ok {
			t.Errorf("README lists toolset %q but the registry has no such toolset (renamed/removed?)", toolset)
			continue
		}
		for name := range docTools {
			if !reg[name] {
				t.Errorf("README lists tool %q under %q but no such tool is registered — the table drifted ahead of the code", name, toolset)
			}
		}
		for name := range reg {
			if !docTools[name] {
				t.Errorf("tool %q is registered under %q but missing from the README tool table (issue #122 class of drift)", name, toolset)
			}
		}
	}
	for toolset := range toolRegistry {
		if !seenToolsets[toolset] {
			t.Errorf("toolset %q is registered but has no README tool-table row", toolset)
		}
	}
}

// parseToolTable extracts the toolset → tool-name rows from the mcp README
// tool table: rows of the form "| `core`(必有) | `get_repo` `list_repos` … |".
// 工具名取反引号span;`(写)`/`(需 X)` 等标注在 span 外,自然剥离。
func parseToolTable(t *testing.T, readme string) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	backtick := regexp.MustCompile("`([^`]+)`")
	for _, ln := range strings.Split(readme, "\n") {
		ln = strings.TrimSpace(ln)
		if !strings.HasPrefix(ln, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(ln, "|"), "|")
		if len(cells) != 2 {
			continue // 表头行(Toolset|工具)与分隔行都在两列之外或被跳过
		}
		spans := backtick.FindAllStringSubmatch(cells[0], -1)
		if len(spans) == 0 {
			continue
		}
		toolset := spans[0][1]
		if !knownToolsets[toolset] {
			continue // 非工具面表行(如表头)
		}
		names := map[string]bool{}
		for _, m := range backtick.FindAllStringSubmatch(cells[1], -1) {
			names[m[1]] = true
		}
		if len(names) > 0 {
			out[toolset] = names
		}
	}
	return out
}
