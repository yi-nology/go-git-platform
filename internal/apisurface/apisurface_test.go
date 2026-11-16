// Package apisurface guards the v1 frozen API surface: the live extraction
// of the 22 spec §3.1 (+T2 评审勘误) packages must equal the committed golden file
// docs/api-surface-v1.txt byte for byte. Any added, removed or re-rendered
// exported symbol (增/删/改签名同权) turns `go test ./...` red until the
// golden is explicitly regenerated (go run ./internal/tools/gensurface) and
// the change is logged in CHANGELOG.md — the spec §3.1 golden discipline.
//
// Single-source extraction: this test does not implement extraction. It
// invokes the gensurface tool itself (`go run ./internal/tools/gensurface
// -stdout`), so the tool that regenerates the golden and the guard that
// compares against it always run the exact same code path
// (internal/tools/gensurface/surface.go). A second, test-side extraction
// implementation would drift and silently weaken the guard.
package apisurface

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// goldenRelPath is the committed golden file, relative to the module root.
const goldenRelPath = "docs/api-surface-v1.txt"

// frozenImports is the v1 frozen package list per spec §3.1 plus its T2
// review errata — 22 packages: the original §3.1 list plus the four public
// packages transport, transport/metrics, gitbackend and smoke added by the
// T2 review (errata recorded in spec §3.1; a v1 freeze missing public
// packages would be hollow), mirrored here on purpose — NOT as a second
// extraction implementation (there is none), but as an independent
// cross-check of the tool's package list: if frozenPackages in
// internal/tools/gensurface ever drifts from spec §3.1 (a package dropped or
// added), regenerating the golden would move the section set and this test
// fails even though the byte comparison in TestGoldenSurfaceMatches would
// stay green against the regenerated file.
var frozenImports = []string{
	"github.com/yi-nology/go-git-platform/provider",
	"github.com/yi-nology/go-git-platform/model",
	"github.com/yi-nology/go-git-platform/runtime",
	"github.com/yi-nology/go-git-platform/adapter/github",
	"github.com/yi-nology/go-git-platform/adapter/gitlab",
	"github.com/yi-nology/go-git-platform/adapter/gitea",
	"github.com/yi-nology/go-git-platform/adapter/forgejo",
	"github.com/yi-nology/go-git-platform/adapter/gitee",
	"github.com/yi-nology/go-git-platform/adapter/gitcode",
	"github.com/yi-nology/go-git-platform/adapter/tencentcode",
	"github.com/yi-nology/go-git-platform/adapter/internal/giteafamily",
	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil",
	"github.com/yi-nology/go-git-platform/adapter/all",
	"github.com/yi-nology/go-git-platform/contracttest",
	"github.com/yi-nology/go-git-platform/auth/githubapp",
	"github.com/yi-nology/go-git-platform/auth/credential",
	"github.com/yi-nology/go-git-platform/filter/branchfilter",
	"github.com/yi-nology/go-git-platform/model/projection",
	"github.com/yi-nology/go-git-platform/transport",
	"github.com/yi-nology/go-git-platform/transport/metrics",
	"github.com/yi-nology/go-git-platform/gitbackend",
	"github.com/yi-nology/go-git-platform/smoke",
}

// TestGoldenSurfaceMatches compares the live extracted surface against the
// committed golden, byte for byte.
func TestGoldenSurfaceMatches(t *testing.T) {
	root := moduleRoot(t)
	goldenPath := filepath.Join(root, goldenRelPath)
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("v1 金样 %s 不可读: %v\n（首次生成或更新金样：go run ./internal/tools/gensurface；v1 期 API 变化须同时登记 CHANGELOG.md）", goldenPath, err)
	}
	current := runExtraction(t, root)
	if string(golden) == current {
		return
	}
	t.Fatalf("v1 金样面漂移：当前抽取 != %s\n%s\n（修复：重生成 go run ./internal/tools/gensurface；有意 API 变化则重生成 + CHANGELOG 登记）", goldenPath, surfaceDiff(string(golden), current))
}

// TestGoldenCoversExactlyFrozenPackages pins the golden's section set to the
// spec §3.1 list: every frozen package must have a section, and no section
// may exist for anything outside the list.
func TestGoldenCoversExactlyFrozenPackages(t *testing.T) {
	root := moduleRoot(t)
	golden, err := os.ReadFile(filepath.Join(root, goldenRelPath))
	if err != nil {
		t.Fatalf("v1 金样不可读: %v", err)
	}
	sections := parseSections(string(golden))
	want := make(map[string]bool, len(frozenImports))
	for _, imp := range frozenImports {
		want[imp] = true
	}
	var problems []string
	for _, imp := range frozenImports {
		if _, ok := sections[imp]; !ok {
			problems = append(problems, fmt.Sprintf("金样缺 spec §3.1 包 section: %s", imp))
		}
	}
	for imp := range sections {
		if !want[imp] {
			problems = append(problems, fmt.Sprintf("金样含 spec §3.1 之外的包 section: %s", imp))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("金样包清单漂移（22 包冻结面，spec §3.1 + T2 评审勘误）:\n%s", strings.Join(problems, "\n"))
	}
}

// runExtraction runs the gensurface tool in stdout mode from the module root.
// The test and the golden generator share this exact binary path — there is
// no test-side extraction logic to drift.
func runExtraction(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("go", "run", "./internal/tools/gensurface", "-stdout")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("抽取工具运行失败（go run ./internal/tools/gensurface -stdout）: %v\n%s", err, stderr)
	}
	return string(out)
}

// section is one golden package section: the import path and its symbol lines.
type section struct {
	importPath string
	lines      []string
}

// parseSections splits golden text into per-package sections. Comment and
// marker lines (# …) outside sections and blank lines are ignored.
func parseSections(text string) map[string]section {
	out := make(map[string]section)
	cur := ""
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "# package "); ok {
			cur = rest
			out[cur] = section{importPath: rest}
			continue
		}
		if cur == "" || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s := out[cur]
		s.lines = append(s.lines, line)
		out[cur] = s
	}
	return out
}

// shortPkg renders the package-qualified context of a diff line, e.g.
// "provider" or "adapter/github", so a guard failure names the package and
// symbol (漂移定位).
func shortPkg(importPath string) string {
	idx := strings.LastIndex(importPath, "/")
	if idx < 0 {
		return importPath
	}
	return importPath[idx+1:]
}

// surfaceDiff renders the multiset line difference between the golden and the
// live extraction, grouped per package and prefixed with the package short
// name so the failure directly names the drifted symbol (e.g.
// "provider: func ApisurfaceProbe()").
func surfaceDiff(golden, current string) string {
	gs := parseSections(golden)
	cs := parseSections(current)
	var b strings.Builder
	names := make(map[string]bool, len(gs)+len(cs))
	for n := range gs {
		names[n] = true
	}
	for n := range cs {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	for _, n := range sorted {
		g, inG := gs[n]
		c, inC := cs[n]
		switch {
		case !inG:
			fmt.Fprintf(&b, "+ %s: 整包新增（金样无此 section）\n", n)
		case !inC:
			fmt.Fprintf(&b, "- %s: 整包从当前抽取面消失\n", n)
		default:
			gi, ci := 0, 0
			for gi < len(g.lines) || ci < len(c.lines) {
				switch {
				case ci >= len(c.lines) || (gi < len(g.lines) && g.lines[gi] < c.lines[ci]):
					fmt.Fprintf(&b, "- %s: %s\n", shortPkg(n), g.lines[gi])
					gi++
				case gi >= len(g.lines) || (ci < len(c.lines) && c.lines[ci] < g.lines[gi]):
					fmt.Fprintf(&b, "+ %s: %s\n", shortPkg(n), c.lines[ci])
					ci++
				default:
					gi++
					ci++
				}
			}
		}
	}
	if b.Len() == 0 {
		return "（行集合一致但字节不同——section 布局或头注漂移）"
	}
	return strings.TrimRight(b.String(), "\n")
}

// moduleRoot walks up from this test file's directory to the directory
// holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("向上找不到 go.mod（模块根）")
		}
		dir = parent
	}
}
