// Package layering 以机器方式执行 spec §4.1 的 import 边表（文件排布重构 S6，
// spec §4.2）。守卫扫根模块全部 .go 文件：抽取 github.com/yi-nology/go-git-platform/...
// 前缀的仓内 import，逐边对照下表；违例打印 `from → to` 并 fail。该测试挂进
// `go test ./...`（= make check），杜绝「文档-only 的分层规则」。
//
// 表内容 = spec §4.1 全表逐字（specTableLines）、补充边逐字（specSupplementLines）、
// §4.2 边界口径逐字（specBoundaryLines）；机器编码（baseEdges / specTestOnlyCells /
// specConditionalCells / documentedTestOnlyDeviations）与逐字表的逐格一致性由
// TestSpecTableMatchesEncoding 双向钉死——改一边不改另一边即红。
package layering

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// modulePrefix 是本守卫管辖的模块路径（go.mod module 行）。
const modulePrefix = "github.com/yi-nology/go-git-platform"

// specTableLines 逐字抄录 spec §4.1 边表（docs/superpowers/specs/
// 2026-11-14-restructure-layout-design.md，行 89–100）。每行一个字符串元素：
// 含反引号的行用双引号串（反引号是 markdown 行内码，Go 原始串放不下），
// 表头行的 `\` 在双引号串里写 `\\`。与 spec 原文的逐字比对在 S6 报告里做，
// 本表之后只允许随 spec 修订同步改动。
var specTableLines = []string{
	"| from \\ to | model | provider | runtime | adapter/* | contracttest | auth | filter | transport | gitbackend |",
	"|---|---|---|---|---|---|---|---|---|---|",
	"| **model/**（含 projection） | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |",
	"| **provider/** | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |",
	"| **runtime/** | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |",
	"| **adapter/*** | ✓ | ✓ | ✓ | ✓（仅 `adapter/internal/*`、`adapter/all` 的平台空导入、各平台 `_test.go` 夹具） | ✓（仅 `_test.go`） | ✓（仅 `_test.go`） | ✗ | ✓ | ✗ |",
	"| **contracttest/** | ✓ | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |",
	"| **auth/githubapp** | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✓ | ✗ |",
	"| **auth/credential** | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |",
	"| **filter/branchfilter** | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |",
	"| **gitbackend/** | ✗ | ✗ | ✗ | ✗ | ✗ | ✓ | ✗ | ✗ | ✗ |",
	"| **transport/** | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |",
}

// specSupplementLines 逐字抄录 spec §4.1 补充边段（spec 行 102–106）。
var specSupplementLines = []string{
	"补充边（消费方杂项，同样进守卫）：`examples/*|smoke|internal/tools/genledger →",
	"{model, provider, runtime, adapter/*, contracttest, auth, filter, gitbackend, transport}`；",
	"`internal/version` 可被任何仓内包引用（`internal/` 本身不受限，守卫跳过",
	"`internal/` 与 `mcp/` 前缀）。mcp 是独立 go module，不进本守卫（其 go.mod 依赖面",
	"由 tidy/install CI 守）。",
}

// specBoundaryLines 逐字抄录 spec §4.2 机器守卫段的边界口径（spec 行 114–120）。
var specBoundaryLines = []string{
	"新增 `internal/layering/layering_test.go`：`go/parser` 遍历根模块全部 `.go` 文件，",
	"抽取 `github.com/yi-nology/go-git-platform/...` 前缀 import，按 §4.1 边表断言；",
	"违例打印 `from → to` 并 fail。边界口径：`mcp/` 下文件不检（独立模块）；`internal/`",
	"下文件作为**来源**不检（genledger 等工具自由），`internal/*` 作为**目标**对全部包",
	"放行；`_test.go` 的边用同一张表的 test-only 列校验（非测试文件出现 test-only 边即",
	"违例）。该测试挂进 `go test ./...`，即 `make check` 自动覆盖，杜绝「文档-only 的",
	"分层规则」（与规则二 divergence 机器登记同一精神）。",
}

// columnHeaders 是边表列头（第 5 列起的 9 个目标列，顺序即下表 columns 顺序）。
var columnHeaders = []string{
	"model", "provider", "runtime", "adapter/*", "contracttest", "auth", "filter", "transport", "gitbackend",
}

// columns 是机器编码用的目标列键（adapter/* → adapter；其余与列头同名）。
var columns = []string{
	"model", "provider", "runtime", "adapter", "contracttest", "auth", "filter", "transport", "gitbackend",
}

// rowKeys 是边表 10 个数据行的机器键（顺序 = specTableLines[2:] 顺序）。
// 注意 examples/smoke 不在表内——它们走补充边，见 baseEdges 注释。
var rowKeys = []string{
	"model", "provider", "runtime", "adapter", "contracttest",
	"auth/githubapp", "auth/credential", "filter/branchfilter", "gitbackend", "transport",
}

// baseEdges 是 §4.1 表中无条件 ✓ 格的机器编码（row → 放行的目标列集合）：
//   - 普通 ✓ 格逐格照抄；
//   - adapter 行的三个条件/仅测试格（adapter×adapter、adapter×contracttest、
//     adapter×auth）不在此，分别见 specConditionalCells / specTestOnlyCells；
//   - examples/smoke 两行是补充边（spec §4.1 补充边段）：目标列为表内全 9 列。
//
// internal/* 目标对全部来源放行（§4.2 边界口径），在解析层直接短路，不进此表。
var baseEdges = map[string][]string{
	"model":               {}, // 叶子：含 projection 在内一律零仓内边
	"provider":            {"model"},
	"runtime":             {"model", "provider"},
	"adapter":             {"model", "provider", "runtime", "transport"},
	"contracttest":        {"model", "provider", "runtime"},
	"auth/githubapp":      {"transport"},
	"auth/credential":     {},
	"filter/branchfilter": {},
	"gitbackend":          {"auth"},
	"transport":           {},
	"examples":            slices.Clone(columns), // 补充边
	"smoke":               slices.Clone(columns), // 补充边
	// e2e 补充边（3 期 T4 spec 2026-11-16 §3.4：沙箱写 roundtrip 只消费——
	// model/provider/runtime + adapter/all 注册器空导入；红线是只写一次性
	// 容器、不改任何公共 API，方向仍只朝下，与 examples/smoke 同类）。
	"e2e": slices.Clone(columns),
}

// specTestOnlyCells 是 §4.1 表里标 `✓（仅 _test.go）` 的格（仅测试文件放行，
// 非测试文件出现即违例——§4.2 边界口径末句）。
var specTestOnlyCells = map[[2]string]bool{
	{"adapter", "contracttest"}: true,
	{"adapter", "auth"}:         true,
}

// specConditionalCells 是 §4.1 表里带括号条件、测试与非测试都可能放行的格；
// 条件形态在 edgeAllowed 内逐条判定（spec 原文三形态）。
var specConditionalCells = map[[2]string]bool{
	{"adapter", "adapter"}: true,
}

// documentedTestOnlyDeviations 是 spec §4.1 字面 ✗、但守卫对 _test.go 放行的
// **存量边例外**（每条点名存量文件；非测试文件出现同样违例）。这三条是实现期
// 盘点真实 import 面时发现的：spec 撰写时未覆盖，属「表外存量」，按包×目标
// 精确放行（不做「任意包的测试都能导」的宽放——保住 model 叶子在测试里也成立）。
// 新增条目必须先在 spec 修订里登记，再回来加行并点名文件，且同步改
// TestSpecTableMatchesEncoding 的 wantDeviations 冻结清单（例外维度的
// 「改一边不改另一边即红」靠它，见该处注释）。
var documentedTestOnlyDeviations = map[[2]string]string{
	// Task-2 carry（progress.md）：provider 的工厂/管理器测试需要平台已注册。
	// 旧 backends/all 形态随包迁移保留为空导入，spec §4.1 未列 provider×adapter。
	{"provider", "adapter"}: "存量：provider/factory_test.go 空导入 adapter/all 注册平台（S2 carry；按包×目标放行 adapter/all 本体）",
	// Task-2 carry（progress.md）：runtime 的 manager 测试同理。
	{"runtime", "adapter"}: "存量：runtime/{manager,manager_cachekey,manager_stats}_test.go 空导入 adapter/all 注册平台（S2 carry；同上）",
	// 实现期盘点补记（写守卫时从真实 import 面发现；progress.md 记其为
	// v0.139.0 基线存量）：provider 的限流/错误分类测试直接消费 transport 导出面。
	{"provider", "transport"}: "存量：provider/{ratelimit,errors_multibranch,errors_ratelimit403}_test.go 使用 transport（spec §4.1 未列，盘点补记）",
}

// moduleRoot 从测试工作目录（internal/layering）上溯到含本模块 go.mod 的目录。
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for dir := wd; ; {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module "+modulePrefix) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("向上未找到 %s 的 go.mod（从 %s 起）", modulePrefix, wd)
		}
		dir = parent
	}
}

// rowKey 把文件所在目录（slash 形式、相对仓库根）映射到边表行键。
// 表外的新顶层包/新子包返回 false——调用方按违例处理，逼迫先更新 §4.1 边表。
func rowKey(dir string) (string, bool) {
	seg := strings.Split(dir, "/")
	switch seg[0] {
	case "model", "provider", "runtime", "adapter", "contracttest",
		"gitbackend", "transport", "examples", "smoke", "e2e":
		return seg[0], true
	case "auth":
		if len(seg) == 2 && (seg[1] == "githubapp" || seg[1] == "credential") {
			return "auth/" + seg[1], true
		}
	case "filter":
		if len(seg) == 2 && seg[1] == "branchfilter" {
			return "filter/branchfilter", true
		}
	}
	return "", false
}

// columnKey 把仓内 import 路径（已去掉模块前缀与首斜杠）映射到边表目标列键。
func columnKey(tgt string) (string, bool) {
	seg := strings.Split(tgt, "/")
	if slices.Contains(columns, seg[0]) {
		return seg[0], true
	}
	return "", false
}

// edgeAllowed 判定一条仓内 import 边是否放行（§4.1 逐格 + §4.2 测试列口径）。
//   - 无条件 ✓ 格（含补充边行全列）：测试/非测试都放行；
//   - adapter×adapter 条件格：spec 原文三形态（adapter/internal/*、adapter/all
//     平台空导入、各平台 _test.go 夹具）；
//   - specTestOnlyCells：仅 _test.go 放行；
//   - documentedTestOnlyDeviations：仅 _test.go 放行的存量例外（包×目标精确）；
//   - 其余一律不放行——非测试文件撞上任何 test-only 边即违例。
func edgeAllowed(row, srcPkg, tgtPkg, col string, isTest bool) bool {
	key := [2]string{row, col}
	// 1) 无条件 ✓ 格
	if slices.Contains(baseEdges[row], col) {
		return true
	}
	// 2) adapter×adapter 条件格（spec 原文三形态）
	if specConditionalCells[key] {
		if strings.HasPrefix(tgtPkg, "adapter/internal/") {
			return true
		}
		if srcPkg == "adapter/all" { // adapter/all 的平台空导入（all.go 及其测试）
			return true
		}
		return isTest // 各平台 _test.go 夹具（外部测试包自导入已在解析层短路）
	}
	if !isTest {
		return false // 非测试文件：test-only 边与一切 ✗ 均为违例（§4.2）
	}
	// 3) spec `✓（仅 _test.go）` 格
	if specTestOnlyCells[key] {
		return true
	}
	// 4) 存量例外（点名文件见 documentedTestOnlyDeviations 注释）
	if _, ok := documentedTestOnlyDeviations[key]; ok {
		if col == "adapter" {
			return tgtPkg == "adapter/all" // 只放行注册器本体，不放行任意 adapter/*
		}
		return true
	}
	return false
}

// splitTableRow 按 | 切分边表行，去掉首尾空段并 TrimSpace，返回 10 个内容单元格
// （表头 = from \ to + 9 列；数据行 = 行标签 + 9 列）。
func splitTableRow(t *testing.T, line string) []string {
	t.Helper()
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		t.Fatalf("边表行不是合法 markdown 表行: %q", line)
	}
	body := parts[1 : len(parts)-1]
	cells := make([]string, len(body))
	for i, c := range body {
		cells[i] = strings.TrimSpace(c)
	}
	return cells
}

// TestSpecTableMatchesEncoding 双向钉死「逐字表 ⟷ 机器编码」：
// 表 → 编码：每个 ✓/✗/（仅 格与 baseEdges、specTestOnlyCells、
// specConditionalCells、documentedTestOnlyDeviations 的登记完全对应；
// 编码 → 表：每个登记的格在表里必须是预期形态（条件/仅测试格必须写（仅，
// 存量例外必须是纯 ✗，baseEdges 必须是纯 ✓）。另核补充边段与边界口径段在场。
func TestSpecTableMatchesEncoding(t *testing.T) {
	if got, want := len(specTableLines), 12; got != want {
		t.Fatalf("specTableLines 行数 %d ≠ %d（表头+分隔+10 数据行）", got, want)
	}

	// 表头：from \ to + 9 个列头，顺序与 columns 对齐。
	header := splitTableRow(t, specTableLines[0])
	if len(header) != 10 || header[0] != `from \ to` {
		t.Fatalf("表头首格异常: %#v", header)
	}
	for i, h := range columnHeaders {
		if header[i+1] != h {
			t.Errorf("表头第 %d 列 %q ≠ spec 列头 %q", i, header[i+1], h)
		}
	}
	if !strings.HasPrefix(specTableLines[1], "|---") {
		t.Errorf("第 2 行不是 markdown 分隔行: %q", specTableLines[1])
	}

	// 10 个数据行逐格核对。
	for r, line := range specTableLines[2:] {
		row := rowKeys[r]
		if _, ok := baseEdges[row]; !ok {
			t.Fatalf("行键 %q 不在 baseEdges", row)
		}
		cells := splitTableRow(t, line)
		if len(cells) != 10 {
			t.Fatalf("行 %q 单元格数 %d ≠ 10: %q", row, len(cells), line)
		}
		if !strings.Contains(cells[0], row) {
			t.Errorf("行标签 %q 不含行键 %q", cells[0], row)
		}
		for c, col := range columns {
			cell := cells[c+1]
			key := [2]string{row, col}
			inBase := slices.Contains(baseEdges[row], col)
			_, isDev := documentedTestOnlyDeviations[key]
			switch {
			case cell == "✓":
				if !inBase {
					t.Errorf("%s→%s: 表内纯 ✓ 但 baseEdges 未放行", row, col)
				}
				if specTestOnlyCells[key] || specConditionalCells[key] || isDev {
					t.Errorf("%s→%s: 表内纯 ✓ 但被登记为仅测试/条件/存量例外", row, col)
				}
			case cell == "✗":
				if inBase {
					t.Errorf("%s→%s: baseEdges 放行但表内纯 ✗", row, col)
				}
				if specTestOnlyCells[key] || specConditionalCells[key] {
					t.Errorf("%s→%s: 登记为仅测试/条件格但表内是纯 ✗", row, col)
				}
				// 纯 ✗ + 存量例外 = documentedTestOnlyDeviations 的合法形态（此处即验证）
			case strings.HasPrefix(cell, "✓（仅"):
				if inBase {
					t.Errorf("%s→%s: 条件/仅测试格不得同时进 baseEdges", row, col)
				}
				switch {
				case specTestOnlyCells[key]:
					if !strings.Contains(cell, "_test.go") {
						t.Errorf("%s→%s: 登记为仅测试格但表文未写 仅 `_test.go`: %q", row, col, cell)
					}
				case specConditionalCells[key]:
					// 条件格：表文必须带括号条件（adapter 三形态），不强制关键词
				default:
					t.Errorf("%s→%s: 表内条件/仅测试格未登记进 specTestOnlyCells/specConditionalCells", row, col)
				}
			default:
				t.Errorf("%s→%s: 未知单元格符号 %q", row, col, cell)
			}
		}
	}

	// baseEdges 的行键集合 = 10 表行 + 补充边三行（examples/smoke/e2e——
	// e2e 是 3 期 T4 spec 2026-11-16 §3.4 的新消费方包）。
	wantRows := map[string]bool{}
	for _, k := range rowKeys {
		wantRows[k] = true
	}
	for _, k := range []string{"examples", "smoke", "e2e"} {
		wantRows[k] = true
	}
	for k := range baseEdges {
		if !wantRows[k] {
			t.Errorf("baseEdges 多出表外行键 %q", k)
		}
		delete(wantRows, k)
	}
	for k := range wantRows {
		t.Errorf("baseEdges 缺行键 %q", k)
	}
	// 补充边三行必须全列放行。
	for _, k := range []string{"examples", "smoke", "e2e"} {
		if len(baseEdges[k]) != len(columns) {
			t.Errorf("补充边行 %q 未全列放行: %v", k, baseEdges[k])
		}
	}

	// 补充边段与边界口径段在场（逐字内容由报告与 spec 比对；此处防贴错段）。
	supplement := strings.Join(specSupplementLines, "\n")
	for _, needle := range []string{
		"examples/*|smoke|internal/tools/genledger",
		"internal/version",
		"`internal/` 与 `mcp/` 前缀",
	} {
		if !strings.Contains(supplement, needle) {
			t.Errorf("specSupplementLines 缺少 %q", needle)
		}
	}
	boundary := strings.Join(specBoundaryLines, "\n")
	for _, needle := range []string{
		"`mcp/` 下文件不检",
		"作为**来源**不检",
		"作为**目标**对全部包",
		"test-only 列校验",
	} {
		if !strings.Contains(boundary, needle) {
			t.Errorf("specBoundaryLines 缺少 %q", needle)
		}
	}

	// 冻结存量例外键集合：例外维度也必须「改一边不改另一边即红」——逐字表的
	// ✗ 分支不查 isDev，仅靠表↔编码逐格核对拦不住往 map 里静默塞第 4 条
	// （评审变异实测：注入 model→provider 后两测试曾双 PASS）。新增/删除例外
	// 必须同步改这份 wantDeviations——diff 即「守卫放宽/收窄」的可见审计点。
	wantDeviations := [][2]string{
		{"provider", "adapter"},
		{"provider", "transport"},
		{"runtime", "adapter"},
	}
	if len(documentedTestOnlyDeviations) != len(wantDeviations) {
		t.Errorf("documentedTestOnlyDeviations 有 %d 条 ≠ 冻结清单 %d 条——新增例外必须同步改本测试的 wantDeviations",
			len(documentedTestOnlyDeviations), len(wantDeviations))
	}
	for _, key := range wantDeviations {
		if _, ok := documentedTestOnlyDeviations[key]; !ok {
			t.Errorf("冻结的存量例外 %s→%s 缺失（如系有意收窄，同步改 wantDeviations 并点名受影响文件）", key[0], key[1])
		}
	}

	// 存量例外逐条点名（-v 下可见；也证明 reason 值被消费）。
	for key, reason := range documentedTestOnlyDeviations {
		t.Logf("存量 test-only 例外 %s→%s: %s", key[0], key[1], reason)
	}
}

// TestImportEdges 是主守卫：遍历根模块 .go 文件，逐 import 对照边表。
// 边界口径（spec §4.2 逐字）：mcp/ 不检（独立模块）；internal/ 作来源不检
// （genledger 等工具自由——其补充边全列放行、无判别力，跳过不损失守卫面）；
// internal/* 作目标对全部包放行；_test.go 用 test-only 列，非测试文件出现
// test-only 边即违例。另两条实现口径（非跨层边，不计入）：
//   - 外部测试包自导入（package foo_test → foo）：Go 惯用法，非层间边；
//   - 子包 import 父包（如 transport/metrics → transport）：同层内部引用。
func TestImportEdges(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	var violations []string
	checked := 0

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if name == "mcp" || strings.HasPrefix(name, ".") {
				return fs.SkipDir // mcp 独立模块；.git/.worktrees/.github 等
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			if filepath.ToSlash(rel) == "internal" {
				return fs.SkipDir // §4.2：internal/ 作来源不检
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		relSlash := filepath.ToSlash(rel)

		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return fmt.Errorf("解析 %s 失败: %w", relSlash, perr)
		}

		dir := filepath.ToSlash(filepath.Dir(relSlash))
		if dir == "." {
			violations = append(violations,
				fmt.Sprintf("%s: 仓库根级 .go 文件无边表行（spec §4.1 需补行）", relSlash))
			return nil
		}
		row, ok := rowKey(dir)
		if !ok {
			violations = append(violations,
				fmt.Sprintf("%s: 来源包族 %q 无边表行（spec §4.1 需补行）", relSlash, dir))
			return nil
		}
		srcPkg := dir
		isTest := strings.HasSuffix(relSlash, "_test.go")

		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			if ip != modulePrefix && !strings.HasPrefix(ip, modulePrefix+"/") {
				continue // 非仓内前缀（stdlib/oekit/第三方）不限
			}
			tgt := strings.TrimPrefix(strings.TrimPrefix(ip, modulePrefix), "/")
			checked++

			// §4.2：internal/* 目标对全部包放行（internal/version 可被任何仓内包引用）。
			if tgt == "internal" || strings.HasPrefix(tgt, "internal/") {
				continue
			}
			// mcp 是独立 go module，根模块源码不得 import 它（tidy/install CI 另守）。
			if tgt == "mcp" || strings.HasPrefix(tgt, "mcp/") {
				violations = append(violations,
					fmt.Sprintf("%s → mcp（%s: import %q，mcp 是独立模块不进本守卫）", row, relSlash, ip))
				continue
			}
			// 非跨层边：外部测试包自导入 / 子包 import 父包（见函数注释）。
			if tgt == srcPkg || strings.HasPrefix(srcPkg, tgt+"/") {
				continue
			}
			col, ok := columnKey(tgt)
			if !ok {
				violations = append(violations,
					fmt.Sprintf("%s → %s（%s: import %q，目标包族不在 spec §4.1 边表列）", row, tgt, relSlash, ip))
				continue
			}
			if !edgeAllowed(row, srcPkg, tgt, col, isTest) {
				violations = append(violations,
					fmt.Sprintf("%s → %s（%s: import %q，spec §4.1 ✗）", row, col, relSlash, ip))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历仓库失败: %v", walkErr)
	}

	t.Logf("对照仓内 import 边 %d 条（spec §4.1）", checked)
	if len(violations) > 0 {
		t.Errorf("layering 边表违例 %d 处（spec §4.1）:", len(violations))
		for _, v := range violations {
			t.Errorf("  %s", v)
		}
	}
}
