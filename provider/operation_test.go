package provider

import (
	"errors"
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
