package provider

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"

	oekiterrors "github.com/oekit/pkg/errors"
)

// --- fakes ---
//
// opFake 嵌入 nil Provider：只覆写 Divergences()。门接口（NotificationManager/
// WebhookConfigManager）用嵌入 nil 接口字段按需「实现」——派生链只做类型
// 断言、从不调用方法，方法集来自嵌入接口即可满足断言。

type opFake struct {
	Provider
	divs []Divergence
}

func (f *opFake) Divergences() []Divergence { return f.divs }

// opNotifFake 实现 NotificationManager（门接口在场）。
type opNotifFake struct {
	opFake
	NotificationManager
}

// opWebhookFake 实现 WebhookConfigManager（门接口在场）。
type opWebhookFake struct {
	opFake
	WebhookConfigManager
}

// opFullFake 两个门接口都在场。
type opFullFake struct {
	opFake
	NotificationManager
	WebhookConfigManager
}

// opCapableFake 走 OperationCapable 覆盖路径：不实现任何门接口、台账无桩，
// 支持态完全由 support 表声明。
type opCapableFake struct {
	opFake
	support map[Operation]OperationSupport
}

func (f *opCapableFake) OperationSupport(op Operation) OperationSupport {
	if s, ok := f.support[op]; ok {
		return s
	}
	return Supported
}

var (
	_ Provider             = (*opFake)(nil)
	_ NotificationManager  = (*opNotifFake)(nil)
	_ WebhookConfigManager = (*opWebhookFake)(nil)
	_ OperationCapable     = (*opCapableFake)(nil)
)

// allOps 是封闭枚举的五个种子操作（顺序无关，遍历用）。
var allOps = []Operation{
	OpListRepoNotifications,
	OpListGlobalNotifications,
	OpUpdateGlobalInbox,
	OpGetSingleWebhook,
	OpUpdateSingleWebhook,
}

// TestIsStub：Capability+Method 双匹配才 true。桩数据里造两条同 Method 异
// Capability 的 stub（外加一条同 Cap+Method 但 Kind=ignore 的），钉住
// Capability 维度不被 Method 单键短路。
func TestIsStub(t *testing.T) {
	divs := []Divergence{
		{Capability: "NotificationManager", Method: "GetWebhook", Kind: DivergenceStub},
		{Capability: "WebhookConfigManager", Method: "GetWebhook", Kind: DivergenceStub},
		{Capability: "NotificationManager", Method: "ListNotifications", Kind: DivergenceIgnore},
	}

	t.Run("cap+method both match", func(t *testing.T) {
		if !IsStub(divs, "NotificationManager", "GetWebhook") {
			t.Error("IsStub(NotificationManager, GetWebhook) = false, want true")
		}
		if !IsStub(divs, "WebhookConfigManager", "GetWebhook") {
			t.Error("IsStub(WebhookConfigManager, GetWebhook) = false, want true")
		}
	})

	t.Run("same method different capability not fooled", func(t *testing.T) {
		if IsStub(divs, "IssueManager", "GetWebhook") {
			t.Error("IsStub(IssueManager, GetWebhook) = true, want false (capability mismatch)")
		}
	})

	t.Run("kind must be stub", func(t *testing.T) {
		if IsStub(divs, "NotificationManager", "ListNotifications") {
			t.Error("IsStub(NotificationManager, ListNotifications) = true, want false (kind=ignore)")
		}
	})

	t.Run("empty ledger", func(t *testing.T) {
		if IsStub(nil, "NotificationManager", "GetWebhook") {
			t.Error("IsStub(nil, ...) = true, want false")
		}
	})
}

// TestDeriveOperationSupport：派生链按 spec 2.2 的优先级逐级钉——台账桩 >
// 门接口缺失 > Supported，OperationCapable 覆盖优先于整条链，未知 op 保守拒绝。
func TestDeriveOperationSupport(t *testing.T) {
	stubbed := []Divergence{
		{Capability: "NotificationManager", Method: "ListRepoNotifications", Kind: DivergenceStub,
			Reason: "test fixture"},
	}

	t.Run("registered stub makes that op unsupported", func(t *testing.T) {
		p := &opNotifFake{opFake: opFake{divs: stubbed}}
		if got := DeriveOperationSupport(p, OpListRepoNotifications); got != Unsupported {
			t.Errorf("DeriveOperationSupport(stubbed op) = %v, want Unsupported", got)
		}
		// 方法级粒度：同族未登记桩的方法仍是 Supported。
		if got := DeriveOperationSupport(p, OpListGlobalNotifications); got != Supported {
			t.Errorf("DeriveOperationSupport(unstubbed op) = %v, want Supported", got)
		}
	})

	t.Run("stub is capability scoped", func(t *testing.T) {
		// 同 Method 名、异 Capability 的桩不能误伤 NotificationManager 族。
		divs := []Divergence{
			{Capability: "WebhookConfigManager", Method: "ListNotifications", Kind: DivergenceStub},
		}
		p := &opNotifFake{opFake: opFake{divs: divs}}
		if got := DeriveOperationSupport(p, OpListGlobalNotifications); got != Supported {
			t.Errorf("DeriveOperationSupport = %v, want Supported (cross-capability stub ignored)", got)
		}
		// 对照：Stubs 只按 Method，会误报——IsStub 的 Capability 维度即为此立。
		if !Stubs(divs, "ListNotifications") {
			t.Fatal("Stubs(ListNotifications) = false, fixture should register the method")
		}
		if IsStub(divs, "NotificationManager", "ListNotifications") {
			t.Error("IsStub = true, want false (different capability)")
		}
	})

	t.Run("missing gate iface makes whole family unsupported", func(t *testing.T) {
		p := &opFake{} // 两个门接口都不在场
		for _, op := range allOps {
			if got := DeriveOperationSupport(p, op); got != Unsupported {
				t.Errorf("DeriveOperationSupport(%s) = %v, want Unsupported (iface missing)", op, got)
			}
		}
	})

	t.Run("iface present and no stub is supported", func(t *testing.T) {
		p := &opFullFake{}
		for _, op := range allOps {
			if got := DeriveOperationSupport(p, op); got != Supported {
				t.Errorf("DeriveOperationSupport(%s) = %v, want Supported", op, got)
			}
		}
	})

	t.Run("partial iface only lifts its own family", func(t *testing.T) {
		p := &opNotifFake{}
		want := map[Operation]OperationSupport{
			OpListRepoNotifications:   Supported,
			OpListGlobalNotifications: Supported,
			OpUpdateGlobalInbox:       Supported,
			OpGetSingleWebhook:        Unsupported,
			OpUpdateSingleWebhook:     Unsupported,
		}
		for op, w := range want {
			if got := DeriveOperationSupport(p, op); got != w {
				t.Errorf("DeriveOperationSupport(%s) = %v, want %v", op, got, w)
			}
		}
	})

	t.Run("unknown op is conservatively unsupported", func(t *testing.T) {
		p := &opFullFake{}
		if got := DeriveOperationSupport(p, Operation("bogus_op")); got != Unsupported {
			t.Errorf("DeriveOperationSupport(unknown) = %v, want Unsupported", got)
		}
	})

	t.Run("OperationCapable override wins without stub", func(t *testing.T) {
		// 台账无桩、门接口也不在场——覆盖声明仍是唯一真源。
		p := &opCapableFake{support: map[Operation]OperationSupport{
			OpListRepoNotifications: CredentialDenied,
		}}
		if got := DeriveOperationSupport(p, OpListRepoNotifications); got != CredentialDenied {
			t.Errorf("DeriveOperationSupport = %v, want CredentialDenied (override)", got)
		}
		// 未显式声明的 op 走缺省 Supported。
		if got := DeriveOperationSupport(p, OpGetSingleWebhook); got != Supported {
			t.Errorf("DeriveOperationSupport(undeclared) = %v, want Supported", got)
		}
	})

	t.Run("OperationCapable override beats registered stub", func(t *testing.T) {
		p := &opCapableFake{
			opFake:  opFake{divs: stubbed},
			support: map[Operation]OperationSupport{OpListRepoNotifications: Supported},
		}
		if got := DeriveOperationSupport(p, OpListRepoNotifications); got != Supported {
			t.Errorf("DeriveOperationSupport = %v, want Supported (override beats stub)", got)
		}
	})
}

// TestCheckOperation：三态 → 三类错误的映射（nil / ErrNotImplemented /
// ErrPermissionDenied），未知 op 按 Unsupported 处理。
func TestCheckOperation(t *testing.T) {
	t.Run("supported returns nil", func(t *testing.T) {
		p := &opFullFake{}
		for _, op := range allOps {
			if err := CheckOperation(p, op); err != nil {
				t.Errorf("CheckOperation(%s) = %v, want nil", op, err)
			}
		}
	})

	t.Run("unsupported maps to ErrNotImplemented", func(t *testing.T) {
		p := &opFake{} // 门接口缺失
		for _, op := range allOps {
			err := CheckOperation(p, op)
			if !errors.Is(err, ErrNotImplemented) {
				t.Errorf("CheckOperation(%s) = %v, want ErrNotImplemented", op, err)
			}
		}
	})

	t.Run("stubbed op maps to ErrNotImplemented", func(t *testing.T) {
		p := &opNotifFake{opFake: opFake{divs: []Divergence{
			{Capability: "NotificationManager", Method: "ListRepoNotifications", Kind: DivergenceStub},
		}}}
		err := CheckOperation(p, OpListRepoNotifications)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("CheckOperation(stubbed) = %v, want ErrNotImplemented", err)
		}
	})

	t.Run("credential denied maps to ErrPermissionDenied", func(t *testing.T) {
		p := &opCapableFake{support: map[Operation]OperationSupport{
			OpUpdateSingleWebhook: CredentialDenied,
		}}
		err := CheckOperation(p, OpUpdateSingleWebhook)
		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("CheckOperation(CredentialDenied) = %v, want ErrPermissionDenied", err)
		}
		// 凭据轴分界：不得与服务端 403 的 ErrForbidden 混淆。
		if errors.Is(err, ErrForbidden) {
			t.Errorf("CheckOperation(CredentialDenied) also matches ErrForbidden, want no")
		}
	})

	t.Run("unknown op maps to ErrNotImplemented", func(t *testing.T) {
		err := CheckOperation(&opFullFake{}, Operation("bogus_op"))
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("CheckOperation(unknown) = %v, want ErrNotImplemented", err)
		}
	})
}

// TestErrPermissionDeniedIsMatchable：哨兵经 Wrap 后 errors.Is 可判（规则二
// 分类判定只走 errors.Is）。
func TestErrPermissionDeniedIsMatchable(t *testing.T) {
	t.Run("oekiterrors.Wrap keeps Is", func(t *testing.T) {
		err := oekiterrors.Wrap(ErrPermissionDenied, "operation denied for this credential",
			"op", string(OpUpdateSingleWebhook))
		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("errors.Is(wrapped, ErrPermissionDenied) = false, want true")
		}
	})

	t.Run("provider.Wrap keeps Is", func(t *testing.T) {
		err := Wrap(PlatformGitHub, "UpdateWebhook", ErrPermissionDenied)
		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("errors.Is(ProviderError, ErrPermissionDenied) = false, want true")
		}
	})

	t.Run("sentinel is distinct from siblings", func(t *testing.T) {
		if errors.Is(ErrPermissionDenied, ErrNotImplemented) {
			t.Error("ErrPermissionDenied matches ErrNotImplemented, want distinct")
		}
		if errors.Is(ErrPermissionDenied, ErrForbidden) {
			t.Error("ErrPermissionDenied matches ErrForbidden, want distinct")
		}
		if errors.Is(ErrPermissionDenied, ErrAuthentication) {
			t.Error("ErrPermissionDenied matches ErrAuthentication, want distinct")
		}
	})
}

// TestOpRequirementsCoverEveryConst 钉封闭枚举接线完备性：operation.go 的
// 每个 Operation 常量都必须有 opRequirements 条目，且每个表项都对应真实
// 常量（双向）。常量块是唯一真源、运行时不可枚举——用 go/ast 读源码对表，
// 新增 Operation 忘接 opRequirements（或表里残留死项）即红。
func TestOpRequirementsCoverEveryConst(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "operation.go", nil, 0)
	if err != nil {
		t.Fatalf("parse operation.go: %v", err)
	}
	var consts []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// 不得按 Values 个数跳过：grouped 多值形（OpA, OpB Operation =
			// "a", "b"）正是要抓的声明形态，个数不齐交给 helper Fatal 响亮失败。
			vals, isOp := operationConstSpec(t, vs)
			if !isOp {
				continue
			}
			consts = append(consts, vals...)
		}
	}
	if len(consts) == 0 {
		t.Fatal("no Operation constants parsed from operation.go")
	}
	for _, c := range consts {
		if _, ok := opRequirements[Operation(c)]; !ok {
			t.Errorf("Operation %q has no opRequirements entry", c)
		}
	}
	for op := range opRequirements {
		if !slices.Contains(consts, string(op)) {
			t.Errorf("opRequirements entry %q has no Operation const", op)
		}
	}
}

// operationConstSpec 判定一个 const spec 是否声明 Operation 常量，并提取
// 其字符串值。两种声明形都算：显式 `OpX Operation = "x"` 与 type-less
// 转换形 `OpX = Operation("x")`——后者正是旧卫语句 `vs.Type == nil →
// continue` 的 fail-open（漏检会静默放过未接 opRequirements 的新 op）；
// 其余 type-less 常量（iota 风格等）返回 ok=false 照旧跳过。形态写错
// （个数不齐 / 非字符串字面量 / unquote 失败）响亮 t.Fatalf——字符串
// 枚举无 iota，静默跳过会让漏检悄悄失效。
func operationConstSpec(t *testing.T, vs *ast.ValueSpec) (values []string, ok bool) {
	t.Helper()
	if vs.Type != nil {
		ident, isIdent := vs.Type.(*ast.Ident)
		if !isIdent || ident.Name != "Operation" {
			return nil, false
		}
	} else if !declaresOperationConversion(vs) {
		return nil, false // 非 Operation 的 type-less 常量（iota 风格等）
	}
	if len(vs.Names) != len(vs.Values) {
		t.Fatalf("Operation const %v: want one string literal per name", vs.Names)
	}
	values = make([]string, 0, len(vs.Values))
	for i, name := range vs.Names {
		values = append(values, operationConstValue(t, name.Name, vs.Values[i]))
	}
	return values, true
}

// declaresOperationConversion 报告 type-less spec 的任一值写成了 Operation
// 转换形（call 的函数 ident 是 Operation）——命中即视为 Operation 声明。
func declaresOperationConversion(vs *ast.ValueSpec) bool {
	for _, val := range vs.Values {
		if call, ok := val.(*ast.CallExpr); ok && isOperationIdent(call.Fun) && len(call.Args) == 1 {
			return true
		}
	}
	return false
}

// operationConstValue 提取单个值的字符串：转换形 `Operation("x")` 取实参，
// 其余按字面量解。非字面量/非字符串/unquote 失败响亮 Fatalf。
func operationConstValue(t *testing.T, name string, e ast.Expr) string {
	t.Helper()
	if call, ok := e.(*ast.CallExpr); ok && isOperationIdent(call.Fun) && len(call.Args) == 1 {
		e = call.Args[0]
	}
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		t.Fatalf("Operation const %s: want string literal value", name)
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Fatalf("Operation const %s: unquote %s: %v", name, lit.Value, err)
	}
	return v
}

// isOperationIdent 报告表达式是裸 Operation 类型标识（转换形的函数侧）。
func isOperationIdent(e ast.Expr) bool {
	ident, ok := e.(*ast.Ident)
	return ok && ident.Name == "Operation"
}

// TestOperationConstSpecTypeLessConversion 钉 type-less 转换形不再被静默
// 跳过（M1 修复的聚焦证明）：`OpConv = Operation("conv")` 必须被收集进
// 覆盖检查的 consts（缺失 opRequirements 条目的 Errorf 由主测试对 consts
// 的循环触发）；iota 风格的非 Operation type-less 常量照旧跳过。
func TestOperationConstSpecTypeLessConversion(t *testing.T) {
	src := `package provider
const (
	OpTyped Operation = "typed"
	OpConv           = Operation("conv")
	OpIota           = iota
	OpPlain          = 7
)
`
	f, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", src, 0)
	if err != nil {
		t.Fatalf("parse synthetic source: %v", err)
	}
	var got []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if vals, isOp := operationConstSpec(t, vs); isOp {
				got = append(got, vals...)
			}
		}
	}
	if want := []string{"typed", "conv"}; !slices.Equal(got, want) {
		t.Errorf("collected = %v, want %v — type-less Operation conversion must not be skipped, iota-style consts must stay skipped", got, want)
	}
}

func TestAllOperations(t *testing.T) {
	// 期望全集显式列出：新增种子时本测试与 AST 完备性测试双红，
	// 强制有意识地更新（勿在本列表上做「自动跟随」——它是消费方
	// 硬编码清单问题的镜像钉子）。
	want := []Operation{
		OpGetSingleWebhook,
		OpListGlobalNotifications,
		OpListRepoNotifications,
		OpUpdateGlobalInbox,
		OpUpdateSingleWebhook,
	}
	got := AllOperations()
	if !slices.Equal(got, want) {
		t.Fatalf("AllOperations() = %v, want %v", got, want)
	}
	if !slices.IsSorted(got) {
		t.Fatalf("AllOperations() not sorted: %v", got)
	}

	// 新切片：改返回值不影响下次调用（导出面不得泄露内部 map 的迭代态）。
	got[0] = Operation("tampered")
	again := AllOperations()
	if !slices.Equal(again, want) {
		t.Fatalf("AllOperations() not a fresh copy: %v", again)
	}

	// 全集与单一真源互为子集：每个 op 都有 requirement。
	for _, op := range again {
		if _, ok := opRequirements[op]; !ok {
			t.Errorf("AllOperations returned %q missing from opRequirements", op)
		}
	}
}
