package contracttest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// OperationSuite 钉操作级三层一致（2B spec §3.2）：台账/接口（测试侧
// 独立直查）→ CheckOperation 哨兵 → stub 派生项的行为。
// 测试侧期望值不调 DeriveOperationSupport（否则同义反复）——直接用
// provider.IsStub + 接口断言计算。
//
// 自有 httptest：418 Teapot 兜底任何真 wire 调用（stub 派生项必须在
// wire 前被 ErrNotImplemented 拦下，落网即套件红）。与 ErrorClassSuite
// 同模式（stubHandler 不能改状态码）。
func testOperationSuite(t *testing.T, h Harness) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // 任何真调用都该在 wire 前被 stub 拦下
	}))
	defer srv.Close()
	p := h.NewProvider(t, baseCfg(h, srv.URL))

	// 种子 op 的测试侧期望（与 provider/operation.go 的 opRequirements 表
	// 同构但独立书写——两份表不一致即套件红，正是要抓的漂移）。
	type expectation struct {
		op         provider.Operation
		capability string
		method     string
		hasIface   func(provider.Provider) bool
		invoke     func(provider.Provider) error // 仅 stub 派生项用于行为钉
	}
	expectations := []expectation{
		{
			op:         provider.OpListRepoNotifications,
			capability: "NotificationManager",
			method:     "ListRepoNotifications",
			hasIface: func(p provider.Provider) bool {
				_, ok := p.(provider.NotificationManager)
				return ok
			},
			invoke: func(p provider.Provider) error {
				nm, ok := p.(provider.NotificationManager)
				if !ok {
					// 无真实方法可调——层 1 已断言 CheckOperation 给
					// ErrNotImplemented，行为钉按未实现语义收口。
					return provider.ErrNotImplemented
				}
				_, err := nm.ListRepoNotifications(context.Background(), "owner", "repo", provider.ListNotificationsOptions{})
				return err
			},
		},
		{
			op:         provider.OpListGlobalNotifications,
			capability: "NotificationManager",
			method:     "ListNotifications",
			hasIface: func(p provider.Provider) bool {
				_, ok := p.(provider.NotificationManager)
				return ok
			},
			invoke: func(p provider.Provider) error {
				nm, ok := p.(provider.NotificationManager)
				if !ok {
					return provider.ErrNotImplemented
				}
				_, err := nm.ListNotifications(context.Background(), provider.ListNotificationsOptions{})
				return err
			},
		},
		{
			op:         provider.OpUpdateGlobalInbox,
			capability: "NotificationManager",
			method:     "MarkNotificationsRead",
			hasIface: func(p provider.Provider) bool {
				_, ok := p.(provider.NotificationManager)
				return ok
			},
			invoke: func(p provider.Provider) error {
				nm, ok := p.(provider.NotificationManager)
				if !ok {
					return provider.ErrNotImplemented
				}
				return nm.MarkNotificationsRead(context.Background(), provider.MarkNotificationsOptions{})
			},
		},
		{
			op:         provider.OpGetSingleWebhook,
			capability: "WebhookConfigManager",
			method:     "GetWebhook",
			hasIface: func(p provider.Provider) bool {
				_, ok := p.(provider.WebhookConfigManager)
				return ok
			},
			invoke: func(p provider.Provider) error {
				wcm, ok := p.(provider.WebhookConfigManager)
				if !ok {
					return provider.ErrNotImplemented
				}
				_, err := wcm.GetWebhook(context.Background(), "owner", "repo", 1)
				return err
			},
		},
		{
			op:         provider.OpUpdateSingleWebhook,
			capability: "WebhookConfigManager",
			method:     "UpdateWebhook",
			hasIface: func(p provider.Provider) bool {
				_, ok := p.(provider.WebhookConfigManager)
				return ok
			},
			invoke: func(p provider.Provider) error {
				wcm, ok := p.(provider.WebhookConfigManager)
				if !ok {
					return provider.ErrNotImplemented
				}
				_, err := wcm.UpdateWebhook(context.Background(), "owner", "repo", 1, provider.UpdateWebhookOptions{})
				return err
			},
		},
	}
	for _, e := range expectations {
		t.Run(string(e.op), func(t *testing.T) {
			stubbed := provider.IsStub(p.Divergences(), e.capability, e.method)
			ifaceOK := e.hasIface(p)
			err := provider.CheckOperation(p, e.op)
			switch {
			case stubbed || !ifaceOK:
				if !errors.Is(err, provider.ErrNotImplemented) {
					t.Fatalf("expected ErrNotImplemented (stub=%v iface=%v), got %v", stubbed, ifaceOK, err)
				}
				if stubbed && e.invoke != nil {
					// 行为钉：真实方法调用也必须是 ErrNotImplemented（wire 前拦截）。
					if ierr := e.invoke(p); !errors.Is(ierr, provider.ErrNotImplemented) {
						t.Fatalf("stubbed method call: want ErrNotImplemented, got %v", ierr)
					}
				}
			default:
				if err != nil {
					t.Fatalf("expected Supported (nil), got %v", err)
				}
			}
		})
	}
}
