package all_test

// README 能力矩阵 ↔ CapabilitySet 对账机检（issue #120 的防再漂网）。
//
// README 的能力矩阵宣称「与 Capabilities() 声明一一对应」,但它是手维护
// 面:2026-11-03 的 GitCode 能力摘除轮漏改了 Reactions×GitCode 格
// (✅ 与脚注¹及 wire 测试矛盾,issue #120),v0.125.0 落地的 TeamReviews
// 则三处失明没进矩阵(issue #121)。本测试按七个平台真实构造 provider、
// 取 DeriveCapabilities 结果,与 README 矩阵逐格对账:
//
//	矩阵格非 "—"  ⇔  该平台声明该能力
//
// ⚠️²（GitCode 通知仅仓库域）与 ❌¹（能力摘除）等脚注格按「非 —」/「—」
// 归一判定——它们表达的是"声明了但部分受限"或"未声明",声明位真假由
// backends/contracttest/capabilities.go 的双向断言守着,这里只守矩阵面。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	// 注册全部七平台后端(空导入触发各 init 的注册面)。
	_ "github.com/yi-nology/go-git-platform/backends/all"
	"github.com/yi-nology/go-git-platform/provider"
)

// matrixPlatformCols 是 README 矩阵的列顺序（列头→provider.Platform）。
var matrixPlatformCols = []struct {
	header string
	plat   provider.Platform
}{
	{"GH", provider.PlatformGitHub},
	{"GitLab", provider.PlatformGitLab},
	{"Gitea", provider.PlatformGitea},
	{"Forgejo", provider.PlatformForgejo},
	{"Gitee", provider.PlatformGitee},
	{"GitCode", provider.PlatformGitCode},
	{"Tencent", provider.PlatformTencentCode},
}

// TestReadmeMatrixMatchesCapabilities parses the README capability matrix
// and reconciles every cell against the platform's real Capabilities().
func TestReadmeMatrixMatchesCapabilities(t *testing.T) {
	// 仓库根的 README.md（测试 CWD 是本包目录,上跳两级）。
	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	rows := parseMatrixRows(t, string(raw))
	if len(rows) == 0 {
		t.Fatal("no capability matrix rows found in README — parser drifted")
	}

	capsType := reflect.TypeOf(provider.CapabilitySet{})
	seen := map[string]bool{}
	for _, row := range rows {
		field, cells := row[0], row[1:]
		if _, ok := capsType.FieldByName(field); !ok {
			t.Errorf("README matrix row %q is not a CapabilitySet field — matrix and declarations have drifted", field)
			continue
		}
		// 行尾多一列 Interface 名(反引号接口名),平台格在中间。
		if len(cells) != len(matrixPlatformCols)+1 {
			t.Errorf("matrix row %s has %d cells, want %d platforms + interface name (columns drifted?)", field, len(cells), len(matrixPlatformCols))
			continue
		}
		cells = cells[:len(matrixPlatformCols)]
		seen[field] = true
		for i, cell := range cells {
			declared := capBool(t, field, matrixPlatformCols[i].plat)
			// "—" = 未声明;❌¹ = 能力已摘除(同样未声明);✅/⚠️²/✅⁵ =
			// 声明(脚注只表达受限面,声明位真假由 contracttest capabilities
			// 的双向断言守着)。
			cell := strings.TrimSpace(cell)
			cellDeclared := cell != "—" && !strings.HasPrefix(cell, "❌")
			if cellDeclared != declared {
				t.Errorf("README matrix says %s×%s = %q, but Capabilities().%s = %v — matrix and declarations have drifted",
					field, matrixPlatformCols[i].header, cell, field, declared)
			}
		}
	}
	// 反向:CapabilitySet 的每个字段都要有矩阵行——「一一对应」承诺的双向面。
	for i := 0; i < capsType.NumField(); i++ {
		name := capsType.Field(i).Name
		if !seen[name] {
			t.Errorf("README capability matrix has no row for %s — the matrix claims one-to-one correspondence with Capabilities()", name)
		}
	}
}

// matrixStubSrv serves the construction-time endpoints some platform SDKs
// hit on NewClient (gitea/forgejo 的 GET /api/v1/version 版本门);能力推导
// 只看类型集,此后无网络流量。
var matrixStubSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"version":"1.26.0"}`))
}))

// capBool builds the platform's provider against the stub server and reads
// the CapabilitySet field via reflection.
func capBool(t *testing.T, field string, plat provider.Platform) bool {
	t.Helper()
	p, err := provider.NewProvider(provider.Config{Platform: plat, BaseURL: matrixStubSrv.URL, Token: "matrix-check"})
	if err != nil {
		t.Fatalf("new provider for %s: %v", plat, err)
	}
	caps := p.Capabilities()
	v := reflect.ValueOf(caps).FieldByName(field)
	if !v.IsValid() {
		t.Fatalf("CapabilitySet has no field %q", field)
	}
	return v.Bool()
}

// parseMatrixRows extracts the rows between the matrix header line
// ("| Capability | GH |") and the first non-table line after it.
func parseMatrixRows(t *testing.T, readme string) [][]string {
	t.Helper()
	lines := strings.Split(readme, "\n")
	start := -1
	for i, ln := range lines {
		if strings.HasPrefix(ln, "| Capability |") && strings.Contains(ln, "GH") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatal("matrix header row not found in README")
	}
	var rows [][]string
	for i := start; i < len(lines); i++ {
		ln := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(ln, "|") {
			break
		}
		cells := strings.Split(strings.Trim(ln, "|"), "|")
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		if strings.HasPrefix(cells[0], ":") || strings.Trim(cells[0], ":-") == "" {
			continue // separator row
		}
		rows = append(rows, cells)
	}
	return rows
}
