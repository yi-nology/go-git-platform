// Surface extraction core for the v1 golden file (docs/api-surface-v1.txt).
//
// Single source of truth: both the gensurface tool (main.go) and the guard
// test (internal/apisurface/apisurface_test.go, which invokes this tool via
// `go run ./internal/tools/gensurface -stdout`) go through this file — there
// is deliberately no second extraction implementation anywhere in the repo,
// because two implementations drifting apart would silently weaken the
// v1 API-freeze guard.
//
// Extraction form ruling (2026-11-17, dual trial, phase-4 task 2 step 1):
// form 乙 (this file) — `go list -json` for the package's GoFiles, then
// stdlib go/parser + go/printer, one canonical line per exported symbol,
// sorted with sort.Strings. Form 甲 (`go doc -all <pkg>` text protocol)
// lost for three reasons, all observed live on the frozen package list:
//
//  1. `go doc -all` block interiors (const/var groups, struct bodies) carry
//     doc comments — the spec §2 red line requires the golden to contain no
//     comments, so a text-protocol form needs a fragile strip pass over
//     captured text; form 乙 parses with comments disabled (parser mode 0),
//     so comments never enter the pipeline.
//  2. `go doc -all` renders block members without their `const`/`var`/`type`
//     keyword, so flattening to one sorted line per symbol would require
//     hand-parsing the text protocol back into declarations — the exact
//     rendering cost form 乙 avoids by construction (go/printer).
//  3. Prose lines in package docs false-match the `^(func|type|const|var)`
//     line filter (observed: "types and predicates, …" matches a naive
//     `^type` anchor), so the filter needs word-boundary care that form 乙
//     makes moot (AST, not text).
//
// Both forms were byte-identical across two consecutive runs (reproducibility
// criterion), and both cost zero new dependencies (stdlib only — spec §2);
// fidelity decided the ruling.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// modulePath is the root module this SDK ships under (go.mod module line).
const modulePath = "github.com/yi-nology/go-git-platform"

// frozenPackages is the v1 frozen surface: the 22 packages of spec §3.1 plus
// its T2-review errata (docs/superpowers/specs/2026-11-17-phase4-v1readiness-
// design.md §3.1 勘误行) — provider, model, runtime, the seven platform
// adapters, the two adapter internal packages (the v1 ecosystem contract),
// adapter/all, contracttest, the two auth packages, filter/branchfilter,
// model/projection, and the four public packages the review added (transport,
// transport/metrics, gitbackend, smoke — a v1 freeze missing public packages
// would be hollow). Root internal/ (non-public helpers) and mcp/ (separate go
// module) stay out of the frozen list. This list is the freeze contract
// itself: growing or shrinking it is a spec-§3.1 revision, not a routine
// change.
var frozenPackages = []string{
	"provider",
	"model",
	"runtime",
	"adapter/github",
	"adapter/gitlab",
	"adapter/gitea",
	"adapter/forgejo",
	"adapter/gitee",
	"adapter/gitcode",
	"adapter/tencentcode",
	"adapter/internal/giteafamily",
	"adapter/internal/backendutil",
	"adapter/all",
	"contracttest",
	"auth/githubapp",
	"auth/credential",
	"filter/branchfilter",
	"model/projection",
	// T2 评审勘误（spec §3.1 勘误行同步登记）：补四公共包——v1 冻结漏公共包则空心。
	"transport",
	"transport/metrics",
	"gitbackend",
	"smoke",
}

// goldenHeader is the discipline header written at the top of
// docs/api-surface-v1.txt: how the file is generated, the update discipline
// during the v1 freeze, and the rule that go:generate must never regenerate
// this file — updating it is an explicit, human-reviewed action.
const goldenHeader = `# go-git-platform v1 公共 API 金样（docs/api-surface-v1.txt）
#
# 生成命令：go run ./internal/tools/gensurface
#   （工具与守卫测试 internal/apisurface 共用同一抽取核心，无第二份实现。）
# 更新纪律：v1 冻结期内任何 API 变化 = 显式重生成金样（上面这条命令）+ 在
#   CHANGELOG.md 登记；守卫测试把当前抽取面与金样逐行比对，任何未走本纪律的
#   差异（增/删/改签名同权）直接红。
# 不挂 go:generate：金样更新是 v1 期的显式动作，绝不随 go generate 自动重写
#   （spec §3.1 金样纪律）。
#
# 形态：go list -json + go/parser（stdlib，零新依赖）逐包抽取导出符号，
#   go/printer 规范渲染，一行一符号，包内 sort.Strings 排序；类型块渲染导出
#   字段/方法（未导出成员是内部状态非 API，T2 评审裁定剥离——内部重构不出
#   守卫噪音）、不含未导出符号与任何注释（spec §2 红线）。落选形态 go doc
#   -all 文本协议的落选原因见 internal/tools/gensurface/surface.go 头注。
# 包清单（22，spec §3.1 + T2 评审勘误）：provider model runtime
#   adapter/{github,gitlab,gitea,forgejo,gitee,gitcode,tencentcode}
#   adapter/internal/{giteafamily,backendutil} adapter/all contracttest
#   auth/{githubapp,credential} filter/branchfilter model/projection
#   transport transport/metrics gitbackend smoke —— 根 internal/ 与 mcp/
#   不进金样。
`

// RenderGolden produces the full golden file text: discipline header, then
// one section per frozen package with the package's exported surface, one
// canonical line per symbol, sorted within the package.
func RenderGolden() (string, error) {
	surfaces, err := ExtractPackages(frozenPackages)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(goldenHeader)
	b.WriteString("# --- 冻结面开始（包 section 头 + 每行一个导出符号）---\n")
	for _, p := range frozenPackages {
		b.WriteString("\n# package " + importPath(p) + "\n")
		for _, line := range surfaces[p] {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// importPath maps a repo-relative package directory onto its full import path.
func importPath(rel string) string {
	return modulePath + "/" + rel
}

// ExtractPackage returns the exported API surface of one repo-relative
// package (see ExtractPackages for the semantics).
func ExtractPackage(rel string) ([]string, error) {
	surfaces, err := ExtractPackages([]string{rel})
	if err != nil {
		return nil, err
	}
	return surfaces[rel], nil
}

// ExtractPackages returns the exported API surface of the given repo-relative
// packages: for each, every exported package-level
// func/method/type/const/var rendered through go/printer, one symbol per
// line, sorted. Struct and interface bodies are rendered in full
// (field/method additions are signature changes and must be visible to the
// guard); comments are never present (files are parsed with parser mode 0);
// unexported symbols are skipped, as are exported methods on unexported
// receiver types (matching default `go doc -all` semantics).
//
// Package metadata is resolved with a single batched `go list -json` over all
// import paths: 22 sequential spawns measured 4.2s on this repo versus 0.42s
// batched (phase-4 task 2 timing ruling — the guard runs inside
// `go test ./...`, so spawn count is kept at one).
func ExtractPackages(rels []string) (map[string][]string, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	importPaths := make([]string, len(rels))
	for i, rel := range rels {
		importPaths[i] = importPath(rel)
	}
	listed, err := listPackages(root, importPaths)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(rels))
	for _, rel := range rels {
		symbols, err := parsePackage(listed[importPath(rel)])
		if err != nil {
			return nil, fmt.Errorf("gensurface: package %s: %w", rel, err)
		}
		out[rel] = symbols
	}
	return out, nil
}

// listedPkg is the subset of `go list -json` output the extractor needs.
type listedPkg struct {
	ImportPath string
	Dir        string
	GoFiles    []string
}

// listPackages runs one batched `go list -json` over the given import paths
// and decodes the concatenated JSON objects back into a map keyed by import
// path.
func listPackages(dir string, importPaths []string) (map[string]listedPkg, error) {
	args := append([]string{"list", "-json"}, importPaths...)
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("go list -json: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("go list -json: %w", err)
	}
	out := make(map[string]listedPkg)
	dec := json.NewDecoder(stdout)
	for {
		var lp listedPkg
		if err := dec.Decode(&lp); err != nil {
			if !errors.Is(err, io.EOF) {
				// Reap the child before reporting; its exit status can name
				// the real cause of a truncated/failed listing.
				if waitErr := cmd.Wait(); waitErr != nil {
					return nil, fmt.Errorf("go list -json: decode: %w; wait: %w", err, waitErr)
				}
				return nil, fmt.Errorf("go list -json: decode: %w", err)
			}
			break
		}
		out[lp.ImportPath] = lp
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("go list -json: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// parsePackage extracts the exported surface of one listed package: parse
// each GoFile, keep exported declarations, render, sort. This is the only
// per-file extraction path in the repo — both tool and guard reach it via
// ExtractPackages.
func parsePackage(pkg listedPkg) ([]string, error) {
	fset := token.NewFileSet()
	var out []string
	for _, f := range pkg.GoFiles {
		// parser mode 0: comments are not attached to the AST, so nothing
		// below can leak a comment into the golden (spec §2 red line).
		af, err := parser.ParseFile(fset, filepath.Join(pkg.Dir, f), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for _, decl := range af.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() || unexportedReceiver(d) {
					continue
				}
				line, err := renderFunc(fset, d)
				if err != nil {
					return nil, err
				}
				out = append(out, line)
			case *ast.GenDecl:
				groupIsIota := d.Tok == token.CONST && len(d.Specs) > 0 && hasIotaValue(d.Specs[0])
				for i, spec := range d.Specs {
					line, ok, err := renderSpec(fset, d.Tok, spec, i, groupIsIota)
					if err != nil {
						return nil, err
					}
					if ok {
						out = append(out, line)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// unexportedReceiver reports whether the method's receiver type is
// unexported. Such methods are unreachable through the package's nameable
// API and default `go doc -all` hides them too.
func unexportedReceiver(d *ast.FuncDecl) bool {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return false
	}
	t := d.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	id, ok := t.(*ast.Ident)
	return ok && !id.IsExported()
}

// stripUnexportedMembers removes unexported fields (struct) and unexported
// methods (interface) from a rendered exported-type body, in place on the
// freshly parsed AST. Unexported members are internal state, not API —
// keeping them in the golden over-froze internals (T2 review finding F1:
// renaming adapter/github's unexported `client` field tripped the guard on a
// pure internal refactor). Embedded entries (no explicit names) follow their
// type name's export status.
func stripUnexportedMembers(expr ast.Expr) {
	switch body := expr.(type) {
	case *ast.StructType:
		if body.Fields == nil {
			return
		}
		keep := make([]*ast.Field, 0, len(body.Fields.List))
		for _, f := range body.Fields.List {
			if !fieldExported(f) {
				continue
			}
			stripInlineBodies(f.Type)
			keep = append(keep, f)
		}
		body.Fields.List = keep
	case *ast.InterfaceType:
		if body.Methods == nil {
			return
		}
		keep := make([]*ast.Field, 0, len(body.Methods.List))
		for _, m := range body.Methods.List {
			if !fieldExported(m) {
				continue
			}
			stripInlineBodies(m.Type)
			keep = append(keep, m)
		}
		body.Methods.List = keep
	}
}

// fieldExported reports whether one struct field / interface method entry is
// exported. An embedded entry (no explicit names) is exported iff its type
// name is; a named entry is exported if any of its names is (mixed-name
// declarations do not occur in the frozen packages; keeping the line avoids
// under-reporting a genuinely exported name).
func fieldExported(f *ast.Field) bool {
	if len(f.Names) == 0 {
		return embeddedTypeExported(f.Type)
	}
	for _, n := range f.Names {
		if n.IsExported() {
			return true
		}
	}
	return false
}

func embeddedTypeExported(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.IsExported()
	case *ast.SelectorExpr:
		return e.Sel.IsExported()
	case *ast.StarExpr:
		return embeddedTypeExported(e.X)
	default:
		return true
	}
}

// stripInlineBodies recurses into inline struct/interface types nested inside
// kept fields (through pointers, arrays, maps, chans and func signatures) so
// their unexported members are stripped too.
func stripInlineBodies(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.StructType:
		stripUnexportedMembers(e)
	case *ast.InterfaceType:
		stripUnexportedMembers(e)
	case *ast.StarExpr:
		stripInlineBodies(e.X)
	case *ast.ArrayType:
		stripInlineBodies(e.Elt)
	case *ast.MapType:
		stripInlineBodies(e.Key)
		stripInlineBodies(e.Value)
	case *ast.ChanType:
		stripInlineBodies(e.Value)
	case *ast.FuncType:
		for _, list := range []*ast.FieldList{e.Params, e.Results} {
			if list == nil {
				continue
			}
			for _, f := range list.List {
				stripInlineBodies(f.Type)
			}
		}
	}
}

// renderFunc prints a FuncDecl as a bare signature: doc comments are already
// absent (parse mode 0) and the body is dropped before printing, so the
// golden line is exactly the callable signature.
func renderFunc(fset *token.FileSet, d *ast.FuncDecl) (string, error) {
	body := d.Body
	d.Body = nil
	var buf bytes.Buffer
	err := printer.Fprint(&buf, fset, d)
	d.Body = body
	if err != nil {
		return "", fmt.Errorf("render %s: %w", d.Name.Name, err)
	}
	return buf.String(), nil
}

// renderSpec prints one spec of a GenDecl as a standalone declaration line
// (`const X = …` / `var X …` / `type X …`). Grouping (parenthesized blocks)
// is not API and is flattened away; a value spec whose names mix exported and
// unexported identifiers is rejected loudly rather than mis-rendered.
func renderSpec(fset *token.FileSet, tok token.Token, spec ast.Spec, specIndex int, groupIsIota bool) (string, bool, error) {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		if !s.Name.IsExported() {
			return "", false, nil
		}
		stripUnexportedMembers(s.Type)
		var buf bytes.Buffer
		err := printer.Fprint(&buf, fset, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{s}})
		if err != nil {
			return "", false, fmt.Errorf("render type %s: %w", s.Name.Name, err)
		}
		return buf.String(), true, nil
	case *ast.ValueSpec:
		exported := 0
		for _, n := range s.Names {
			if n.IsExported() {
				exported++
			}
		}
		if exported == 0 {
			return "", false, nil
		}
		if exported != len(s.Names) {
			return "", false, fmt.Errorf("value spec mixes exported and unexported names (%v) — unsupported", s.Names)
		}
		elideUnexportedVarInits(s, tok)
		var buf bytes.Buffer
		err := printer.Fprint(&buf, fset, &ast.GenDecl{Tok: tok, Specs: []ast.Spec{s}})
		if err != nil {
			return "", false, fmt.Errorf("render %v: %w", s.Names, err)
		}
		return annotateIotaInherited(buf.String(), tok, len(s.Values), specIndex, groupIsIota), true, nil
	default:
		return "", false, nil
	}
}

// hasIotaValue reports whether the spec's value list is exactly the bare
// identifier iota (the canonical first member of an iota const group).
func hasIotaValue(spec ast.Spec) bool {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok || len(vs.Values) != 1 {
		return false
	}
	id, ok := vs.Values[0].(*ast.Ident)
	return ok && id.Name == "iota"
}

// containsUnexportedRef reports whether expr references any unexported
// identifier (universal/builtin names excluded). Used to elide var
// initializers that embed internal names from the v1 golden.
func containsUnexportedRef(n ast.Node) bool {
	if n == nil {
		return false
	}
	found := false
	ast.Inspect(n, func(x ast.Node) bool {
		id, ok := x.(*ast.Ident)
		if !ok {
			return true
		}
		switch id.Name {
		case "nil", "true", "false", "iota", "len", "cap", "new", "make", "append", "delete":
			return true
		}
		if !id.IsExported() {
			found = true
			return false
		}
		return true
	})
	return found
}

// keysOnlyMapLit rebuilds a map composite literal keeping only the keys,
// values elided ("...") — deterministic and free of unexported refs.
func keysOnlyMapLit(lit *ast.CompositeLit) *ast.CompositeLit {
	out := &ast.CompositeLit{Type: lit.Type}
	for _, e := range lit.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			out.Elts = append(out.Elts, &ast.KeyValueExpr{Key: kv.Key, Value: &ast.Ellipsis{}})
		}
	}
	return out
}

// elideUnexportedVarInits rewrites var-spec init expressions that reference
// unexported identifiers (e.g. smoke.Probes' family registry) into a
// deterministic elided form — map literals keep their keys with "..." values,
// anything else collapses to "..." — so internal renames no longer trip the
// v1 freeze guard while key add/remove (the consumer-visible face) still does.
func elideUnexportedVarInits(s *ast.ValueSpec, tok token.Token) {
	if tok != token.VAR {
		return
	}
	for i, v := range s.Values {
		if !containsUnexportedRef(v) {
			continue
		}
		if lit, ok := v.(*ast.CompositeLit); ok && lit.Type != nil {
			s.Values[i] = keysOnlyMapLit(lit)
		} else {
			s.Values[i] = &ast.Ellipsis{}
		}
	}
}

// annotateIotaInherited renders value-less members of an iota const group
// with an explicit ordinal ("= iota(N)") — member swaps become visible in the
// textual golden (value order is otherwise invisible once lines are sorted).
// Inherited members of non-iota groups render as before (none in the repo).
func annotateIotaInherited(line string, tok token.Token, valueCount, specIndex int, groupIsIota bool) string {
	if tok != token.CONST || valueCount != 0 || !groupIsIota {
		return line
	}
	trimmed := strings.TrimRight(line, "\n")
	if strings.Contains(trimmed, "=") {
		return trimmed
	}
	return fmt.Sprintf("%s = iota(%d)", trimmed, specIndex)
}

// repoRoot walks up from the process working directory to the directory
// holding the root go.mod (same pattern as internal/tools/genledger).
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("gensurface: no go.mod above %s", dir)
		}
		dir = parent
	}
}
