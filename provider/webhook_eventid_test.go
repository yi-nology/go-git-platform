package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookEnvelope_EventIDNeverEmpty 事件 ID 绝不能为空。
//
// 回归背景(入站同步 P0):IDPrefix 为空时 NewEvent **不写 ID** ⇒ GitHub /
// GitCode 的事件 ID 恒为空串。上层(core)按 event_id 做唯一索引 + 去重 ⇒
// 第一条空 ID 事件入库后,**第 2 条起的所有事件**都被它挡掉
// (不建事件、不匹配规则、不触发同步)⇒ GitHub 入站同步从第 2 条起完全失效。
//
// 这条用例是**反向验证**:把 NewEvent 改回"前缀为空就不写 ID",本用例变红。
func TestWebhookEnvelope_EventIDNeverEmpty(t *testing.T) {
	for _, p := range []Platform{PlatformGitHub, PlatformGitCode, PlatformGitea, PlatformForgejo, PlatformGitee, PlatformGitLab, PlatformTencentCode} {
		env := WebhookEnvelope{Platform: p}
		ev := env.NewEvent(0, []byte(`{}`))
		assert.NotEmpty(t, ev.ID, "platform=%s 的事件 ID 不能为空", p)
		assert.Contains(t, ev.ID, string(p), "ID 应带平台前缀(platform=%s)", p)
	}
}

// TestWebhookEnvelope_ExplicitPrefixKept 显式 IDPrefix 的格式不变(向后兼容)。
func TestWebhookEnvelope_ExplicitPrefixKept(t *testing.T) {
	env := WebhookEnvelope{Platform: PlatformGitLab, IDPrefix: "gl"}
	ev := env.NewEvent(42, []byte(`{}`))
	assert.Contains(t, ev.ID, "gl-")
	assert.Contains(t, ev.ID, "-42")
}

// TestWebhookEnvelope_NewEventWithID_UsesDeliveryID 平台投递 ID 是真幂等键。
//
// 平台重投同一事件时投递 ID 不变 ⇒ 上层按事件 ID 去重能真正生效。
func TestWebhookEnvelope_NewEventWithID_UsesDeliveryID(t *testing.T) {
	env := WebhookEnvelope{Platform: PlatformGitHub, IDHeaders: []string{"X-GitHub-Delivery"}}

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-GitHub-Delivery", "11111111-2222-3333-4444-555555555555")
	ev := env.NewEventWithID(r, 0, []byte(`{}`))
	assert.Contains(t, ev.ID, "11111111-2222-3333-4444-555555555555",
		"有投递 ID 时必须用它作事件 ID")

	// 同一投递重投 ⇒ 事件 ID 不变(上层据此去重)
	ev2 := env.NewEventWithID(r, 0, []byte(`{}`))
	assert.Equal(t, ev.ID, ev2.ID, "同一投递的事件 ID 必须稳定")

	// 没有投递头 ⇒ 回落到本地合成的 ID(仍非空)
	r2 := httptest.NewRequest(http.MethodPost, "/", nil)
	ev3 := env.NewEventWithID(r2, 0, []byte(`{}`))
	require.NotEmpty(t, ev3.ID)
	assert.NotContains(t, ev3.ID, "11111111")
}

// TestWebhookEnvelope_NewEventWithID_MultipleHeaders 多个候选头按顺序取第一个非空。
func TestWebhookEnvelope_NewEventWithID_MultipleHeaders(t *testing.T) {
	env := WebhookEnvelope{Platform: PlatformGitCode, IDHeaders: []string{"X-GitHub-Delivery", "X-GitCode-Delivery"}}

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-GitCode-Delivery", "gc-delivery-1")
	ev := env.NewEventWithID(r, 0, []byte(`{}`))
	assert.Contains(t, ev.ID, "gc-delivery-1")

	r2 := httptest.NewRequest(http.MethodPost, "/", nil)
	r2.Header.Set("X-GitHub-Delivery", "gh-delivery-1")
	r2.Header.Set("X-GitCode-Delivery", "gc-delivery-1")
	ev2 := env.NewEventWithID(r2, 0, []byte(`{}`))
	assert.Contains(t, ev2.ID, "gh-delivery-1", "按声明顺序取第一个非空")
}
