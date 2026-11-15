package provider

import (
	"slices"

	oekiterrors "github.com/oekit/pkg/errors"
)

// Operation 是操作级能力的封闭枚举（2B spec）：只增不改，值即契约。
// 粒度案例：仓库级 vs 全局通知、单 webhook get/update——CapabilitySet
// 的 manager 级 bool 表达不了的层。
type Operation string

const (
	OpListRepoNotifications   Operation = "list_repo_notifications"
	OpListGlobalNotifications Operation = "list_global_notifications"
	OpUpdateGlobalInbox       Operation = "update_global_inbox"
	OpGetSingleWebhook        Operation = "get_single_webhook"
	OpUpdateSingleWebhook     Operation = "update_single_webhook"
)

// OperationSupport 是操作级三态（2B）：平台轴与凭据轴分开表示。
type OperationSupport uint8

const (
	Supported        OperationSupport = iota // 平台支持且未声明凭据限制
	Unsupported                              // 平台不支持——调用必得 ErrNotImplemented
	CredentialDenied                         // 平台支持但当前凭据无权限
)

// OperationCapable 是可选能力接口（规则二：新面走可选接口）：实现方可
// 覆盖派生（典型：凭据轴声明）。未实现的 provider 走 DeriveOperationSupport。
type OperationCapable interface {
	OperationSupport(op Operation) OperationSupport
}

// opRequirement 把种子操作映射到两个单一真源：门接口（iface 断言）与
// 台账 method（Capability 感知查 stub）。表只增不改。
type opRequirement struct {
	capability string              // Divergence.Capability 字面（如 "NotificationManager"）
	method     string              // Divergence.Method 字面（如 "ListNotifications"）
	iface      func(Provider) bool // 门接口断言
}

var opRequirements = map[Operation]opRequirement{
	OpListRepoNotifications: {capability: "NotificationManager", method: "ListRepoNotifications",
		iface: func(p Provider) bool { _, ok := p.(NotificationManager); return ok }},
	OpListGlobalNotifications: {capability: "NotificationManager", method: "ListNotifications",
		iface: func(p Provider) bool { _, ok := p.(NotificationManager); return ok }},
	OpUpdateGlobalInbox: {capability: "NotificationManager", method: "MarkNotificationsRead",
		iface: func(p Provider) bool { _, ok := p.(NotificationManager); return ok }},
	OpGetSingleWebhook: {capability: "WebhookConfigManager", method: "GetWebhook",
		iface: func(p Provider) bool { _, ok := p.(WebhookConfigManager); return ok }},
	OpUpdateSingleWebhook: {capability: "WebhookConfigManager", method: "UpdateWebhook",
		iface: func(p Provider) bool { _, ok := p.(WebhookConfigManager); return ok }},
}

// AllOperations 返回种子操作全集（按值升序，每次返回新切片）——消费方
// 遍历能力矩阵（下游 TestConnection 能力输出等）的权威入口，
// 单一真源 = opRequirements：SDK 按「只增不改」演进时全集自动跟随，
// 消费方勿再硬编码种子清单。全集遍历后逐 op 走 DeriveOperationSupport /
// CheckOperation。
func AllOperations() []Operation {
	ops := make([]Operation, 0, len(opRequirements))
	for op := range opRequirements {
		ops = append(ops, op)
	}
	slices.Sort(ops)
	return ops
}

// DeriveOperationSupport 从既有单一真源推导操作级支持态（2B spec 2.2）：
// OperationCapable 覆盖 > 台账 stub > 门接口缺失 > Supported。
// 凭据轴静态派生不了——只经 OperationCapable 声明。
// 全集遍历用 AllOperations。
func DeriveOperationSupport(p Provider, op Operation) OperationSupport {
	if oc, ok := p.(OperationCapable); ok {
		return oc.OperationSupport(op)
	}
	req, known := opRequirements[op]
	if !known {
		return Unsupported // 封闭枚举：未知 op 不在契约内，保守拒绝
	}
	if IsStub(p.Divergences(), req.capability, req.method) {
		return Unsupported
	}
	if !req.iface(p) {
		return Unsupported
	}
	return Supported
}

// CheckOperation 返回 nil（可执行）/ ErrNotImplemented（平台不支持）/
// ErrPermissionDenied（凭据无权限）。未知 op 按 Unsupported 处理。
func CheckOperation(p Provider, op Operation) error {
	switch DeriveOperationSupport(p, op) {
	case Supported:
		return nil
	case CredentialDenied:
		return oekiterrors.Wrap(ErrPermissionDenied, "operation denied for this credential", "op", string(op))
	default:
		return oekiterrors.Wrap(ErrNotImplemented, "operation not supported on this platform", "op", string(op))
	}
}
