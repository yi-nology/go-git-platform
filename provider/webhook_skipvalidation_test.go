package provider

import (
	"net/http"
	"strings"
	"testing"
)

// newWebhookReq 构造一个带 body 的 POST 请求，用于 envelope 解析测试。
func newWebhookReq(body string, headers map[string]string) *http.Request {
	req, err := http.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	if err != nil {
		panic(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

// TestEnvelopeRead_SkipValidationSecret 锁定哨兵语义：显式传入
// SkipValidationSecret 时 Read 跳过签名校验，只做 body 还原与 header 读取。
func TestEnvelopeRead_SkipValidationSecret(t *testing.T) {
	env := WebhookEnvelope{
		Platform:     PlatformGitLab,
		EventHeaders: []string{"X-Gitlab-Event"},
	}
	body := `{"object_kind":"merge_request"}`
	req := newWebhookReq(body, map[string]string{"X-Gitlab-Event": "Merge Request Hook"})

	// 无哨兵：gitlab 校验器要求 X-Gitlab-Token 头与 secret，缺失即失败。
	if _, _, err := env.Read(req, ""); err == nil {
		t.Fatal("expected validation error without sentinel")
	}

	// 带哨兵：跳过校验，正常返回事件类型与原始 body。
	eventType, got, err := env.Read(req, SkipValidationSecret)
	if err != nil {
		t.Fatalf("sentinel read: %v", err)
	}
	if eventType != "Merge Request Hook" {
		t.Errorf("eventType = %q, want %q", eventType, "Merge Request Hook")
	}
	if string(got) != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

// TestEnvelopeRead_EmptySecretStillRejected 回归护栏：哨兵是唯一豁免路径，
// 普通空 secret 仍必须被校验器拒绝（防止有人图省事把空 secret 改成放行）。
func TestEnvelopeRead_EmptySecretStillRejected(t *testing.T) {
	env := WebhookEnvelope{Platform: PlatformGitLab}
	req := newWebhookReq(`{"a":1}`, map[string]string{"X-Gitlab-Token": "some-token"})

	if _, _, err := env.Read(req, ""); err == nil {
		t.Fatal("expected empty secret to be rejected")
	}
}

// TestValidateWebhookWithRegistry_SentinelDoesNotBypass 回归护栏：哨兵只在
// envelope.Read 生效，公开的校验入口 ValidateWebhookWithRegistry 必须照常
// 跑真校验器——否则哨兵会变成全局签名绕过开关。
func TestValidateWebhookWithRegistry_SentinelDoesNotBypass(t *testing.T) {
	req := newWebhookReq(`{"a":1}`, map[string]string{"X-Gitlab-Token": "some-token"})

	if err := ValidateWebhookWithRegistry(PlatformGitLab, req, SkipValidationSecret); err == nil {
		t.Fatal("sentinel must not bypass ValidateWebhookWithRegistry")
	}
}
