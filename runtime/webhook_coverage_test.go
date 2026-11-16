package runtime

// 3期 T1：webhook 验签域零覆盖清单补测（分支层，spec §1.1）。
// 覆盖 0% 的 Name 方法族、Forgejo/GitCode/Generic-HMAC 验证器 Validate、
// IsWebhookDeletionSHA，以及 ValidateWebhookWithRegistry / ReadAndRestoreBody
// 的 miss/读体失败分支。错误判定只走 errors.Is 哨兵（AGENTS.md 规则二）。
// HMAC 夹具手算 stdlib——不走 oekit SignHMAC（AGENTS.md「测试夹具手写
// HMAC 保留独立对照」：验签委托链需要跨实现真源）。

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// hmacHexSum 手算 HMAC-hex（stdlib 独立对照，见文件头注释）。
func hmacHexSum(newHash func() hash.Hash, key, payload string) string {
	m := hmac.New(newHash, []byte(key))
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

// errWebhookBodyRead / webhookErrBody 驱动读体失败分支：
// ReadAndRestoreBody 与 envelope.Read 的错误路径必须把 cause 链原样上抛。
var errWebhookBodyRead = errors.New("webhook body read boom")

type webhookErrBody struct{}

func (webhookErrBody) Read([]byte) (int, error) { return 0, errWebhookBodyRead }
func (webhookErrBody) Close() error             { return nil }

// TestWebhookValidator_Names 打掉 0% 的 Name 方法族零覆盖清单项
// （Forgejo/GitCode/StaticToken/Gitee/Generic-HMAC），并钉住
// HmacValidator.Name 对 Algorithm 原样拼接（不折叠大小写）的现实行为。
func TestWebhookValidator_Names(t *testing.T) {
	tests := []struct {
		name string
		v    WebhookValidator
		want string
	}{
		{"forgejo", ForgejoWebhookValidator{}, "forgejo-hmac-sha256"},
		{"gitcode", GitCodeWebhookValidator{}, "gitcode"},
		{"static-token", StaticTokenValidator{Header: "X-Gitlab-Token"}, "static-token"},
		{"gitee", GiteeWebhookValidator{}, "gitee-sign"},
		{"hmac-sha1", &HmacValidator{Algorithm: "sha1"}, "hmac-sha1"},
		{"hmac name passthrough keeps algorithm case", &HmacValidator{Algorithm: "SHA512"}, "hmac-SHA512"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.Name(); got != tc.want {
				t.Errorf("Name() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestForgejoWebhookValidator_Validate 覆盖 0% 的 Validate：双 header 回退、
// 裸 hex（真 wire）与 sha256= 前缀（契约夹具）双形态、空密钥与错签的哨兵链。
func TestForgejoWebhookValidator_Validate(t *testing.T) {
	const secret = "forgejo-secret"
	body := `{"zen":"keep it simple"}`
	bare := hmacHexSum(sha256.New, secret, body)
	wrong := hmacHexSum(sha256.New, "other-secret", body)
	forgedEmptyKey := hmacHexSum(sha256.New, "", body)

	tests := []struct {
		name    string
		headers map[string]string
		secret  string
		wantErr bool
	}{
		{"valid bare hex (真 wire 形态)", map[string]string{"X-Forgejo-Signature": bare}, secret, false},
		{"valid sha256= prefix (契约夹具形态)", map[string]string{"X-Forgejo-Signature": "sha256=" + bare}, secret, false},
		{"fallback header X-Gitea-Signature (老分叉)", map[string]string{"X-Gitea-Signature": bare}, secret, false},
		{"missing both signature headers", nil, secret, true},
		{"empty secret", map[string]string{"X-Forgejo-Signature": bare}, "", true},
		{"wrong signature", map[string]string{"X-Forgejo-Signature": wrong}, secret, true},
		{"empty-key forgery rejected", map[string]string{"X-Forgejo-Signature": forgedEmptyKey}, secret, true},
	}
	v := ForgejoWebhookValidator{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newWebhookReq(body, tc.headers)
			err := v.Validate(r, []byte(body), tc.secret)
			if tc.wantErr {
				if !errors.Is(err, provider.ErrWebhookValidation) {
					t.Errorf("err = %v, want errors.Is(provider.ErrWebhookValidation)", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestGitCodeWebhookValidator_Validate 覆盖 0% 的 Validate：签名（X-GitCode-
// Signature / X-Gitea-Signature 回退）与静态令牌（X-GitCode-Token）双模、
// 签名在场错签硬失败不降级（防同 secret 交叉认证）。
func TestGitCodeWebhookValidator_Validate(t *testing.T) {
	const secret = "gitcode-secret"
	body := `{"action":"open"}`
	sig := hmacHexSum(sha256.New, secret, body)
	wrong := hmacHexSum(sha256.New, "other", body)

	tests := []struct {
		name    string
		headers map[string]string
		body    string
		secret  string
		wantErr bool
	}{
		{"valid signature X-GitCode-Signature", map[string]string{"X-GitCode-Signature": sig}, body, secret, false},
		{"fallback X-Gitea-Signature (老分叉)", map[string]string{"X-Gitea-Signature": sig}, body, secret, false},
		{"static token mode X-GitCode-Token", map[string]string{"X-GitCode-Token": secret}, body, secret, false},
		{"signature present but wrong: hard fail, no token downgrade",
			map[string]string{"X-GitCode-Signature": wrong, "X-GitCode-Token": secret}, body, secret, true},
		{"missing all mode headers", nil, body, secret, true},
		{"empty secret", map[string]string{"X-GitCode-Token": "tok"}, body, "", true},
		{"static token mismatch", map[string]string{"X-GitCode-Token": "nope"}, body, secret, true},
		{"tampered body", map[string]string{"X-GitCode-Signature": sig}, body + "x", secret, true},
	}
	v := GitCodeWebhookValidator{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newWebhookReq(tc.body, tc.headers)
			err := v.Validate(r, []byte(tc.body), tc.secret)
			if tc.wantErr {
				if !errors.Is(err, provider.ErrWebhookValidation) {
					t.Errorf("err = %v, want errors.Is(provider.ErrWebhookValidation)", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestHmacValidator_Validate 覆盖 0% 的泛型 HmacValidator.Validate：
// 算法白名单（sha1/sha512/空/未知回落 sha256）、可配前缀剥离与错前缀拒绝、
// 缺头/空密钥/错签的哨兵链。
func TestHmacValidator_Validate(t *testing.T) {
	const secret = "generic-secret"
	body := `{"n":1}`
	sha1Sig := hmacHexSum(sha1.New, secret, body)
	sha256Sig := hmacHexSum(sha256.New, secret, body)
	sha512Sig := hmacHexSum(sha512.New, secret, body)
	wrong := hmacHexSum(sha256.New, "other", body)

	tests := []struct {
		name    string
		h       *HmacValidator
		got     string
		secret  string
		wantErr bool
	}{
		{"sha1 with configured prefix", &HmacValidator{Header: "X-Custom-Sig", Algorithm: "sha1", Prefix: "sha1="}, "sha1=" + sha1Sig, secret, false},
		{"prefix configured but bare hex accepted", &HmacValidator{Header: "X-Custom-Sig", Algorithm: "sha1", Prefix: "sha1="}, sha1Sig, secret, false},
		{"sha512 bare hex", &HmacValidator{Header: "X-Custom-Sig", Algorithm: "sha512"}, sha512Sig, secret, false},
		{"empty algorithm defaults sha256", &HmacValidator{Header: "X-Custom-Sig"}, sha256Sig, secret, false},
		{"unknown algorithm falls back sha256", &HmacValidator{Header: "X-Custom-Sig", Algorithm: "md5"}, sha256Sig, secret, false},
		{"algorithm matching is case-insensitive", &HmacValidator{Header: "X-Custom-Sig", Algorithm: "SHA1", Prefix: "sha1="}, "sha1=" + sha1Sig, secret, false},
		{"wrong prefix is not stripped: decode fails", &HmacValidator{Header: "X-Custom-Sig", Algorithm: "sha256", Prefix: "sha256="}, "md5=" + sha256Sig, secret, true},
		{"missing signature header", &HmacValidator{Header: "X-Custom-Sig"}, "", secret, true},
		{"empty secret", &HmacValidator{Header: "X-Custom-Sig"}, sha256Sig, "", true},
		{"wrong signature", &HmacValidator{Header: "X-Custom-Sig"}, wrong, secret, true},
		{"algorithm mismatch: sha1 declared, sha256 sent", &HmacValidator{Header: "X-Custom-Sig", Algorithm: "sha1"}, sha256Sig, secret, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newWebhookReq(body, nil)
			if tc.got != "" {
				r.Header.Set("X-Custom-Sig", tc.got)
			}
			err := tc.h.Validate(r, []byte(body), tc.secret)
			if tc.wantErr {
				if !errors.Is(err, provider.ErrWebhookValidation) {
					t.Errorf("err = %v, want errors.Is(provider.ErrWebhookValidation)", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestIsWebhookDeletionSHA 覆盖 0% 的删除标记判定：全零（含常量、大小写、
// 短位数变体）为真，任何带非零字符的 SHA 为假。
func TestIsWebhookDeletionSHA(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{WebhookDeletionSHA, true},
		{strings.ToUpper(WebhookDeletionSHA), true},
		{"", true},                      // 空串与全零同形（TrimRight 后同为空）
		{strings.Repeat("0", 39), true}, // 位数不足 40 的全零同样成立
		{"0000000000000000000000000000000000000001", false}, // 尾部非零
		{"000000000000000000000000000000000000000a", false}, // 尾部十六进制字母
		{"00000000000000000000000000000000000000z", false},  // 非十六进制尾
		{"1111111111111111111111111111111111111111", false}, // 普通 SHA
		{"0x0", false}, // 中段非零
	}
	for _, tc := range tests {
		if got := IsWebhookDeletionSHA(tc.in); got != tc.want {
			t.Errorf("IsWebhookDeletionSHA(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestWebhookValidatorRegistry_ReRegisterOverwritesAndDelegates 钉「多注册」
// 语义：同平台二次注册覆盖前者，Validate 委托给后注册者（不是先注册者）。
func TestWebhookValidatorRegistry_ReRegisterOverwritesAndDelegates(t *testing.T) {
	r := NewWebhookValidatorRegistry()
	p := model.Platform("re-register-platform")
	errFirst := errors.New("first validator ran")
	errSecond := errors.New("second validator ran")
	r.Register(p, ValidatorFunc{N: "first", Fn: func(*http.Request, []byte, string) error { return errFirst }})
	r.Register(p, ValidatorFunc{N: "second", Fn: func(*http.Request, []byte, string) error { return errSecond }})

	got := r.Get(p)
	if got == nil {
		t.Fatal("Get() = nil after re-registration")
	}
	if got.Name() != "second" {
		t.Errorf("Get().Name() = %q, want %q (later registration must win)", got.Name(), "second")
	}
	req := newWebhookReq(`{}`, nil)
	err := r.Validate(p, req, []byte(`{}`), "s")
	if !errors.Is(err, errSecond) {
		t.Errorf("Validate() = %v, want chain to errSecond (later registration)", err)
	}
	if errors.Is(err, errFirst) {
		t.Errorf("Validate() reached the overwritten first registration: %v", err)
	}
}

// TestValidateWebhookWithRegistry_Branches 补 miss/多注册/读体失败分支
// （spec §1.1 点名的单位分支层缺口）。
func TestValidateWebhookWithRegistry_Branches(t *testing.T) {
	const secret = "gh-webhook-secret"
	body := `{"action":"opened"}`
	sig := "sha256=" + hmacHexSum(sha256.New, secret, body)

	t.Run("unknown platform is ErrNotImplemented (registry miss)", func(t *testing.T) {
		req := newWebhookReq(body, nil)
		err := ValidateWebhookWithRegistry(model.Platform("no-such-platform"), req, secret)
		if !errors.Is(err, provider.ErrNotImplemented) {
			t.Errorf("err = %v, want errors.Is(provider.ErrNotImplemented)", err)
		}
	})

	t.Run("valid signature passes and body stays readable", func(t *testing.T) {
		req := newWebhookReq(body, map[string]string{"X-Hub-Signature-256": sig})
		if err := ValidateWebhookWithRegistry(model.PlatformGitHub, req, secret); err != nil {
			t.Fatalf("ValidateWebhookWithRegistry() = %v, want nil", err)
		}
		got, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("reading restored body: %v", err)
		}
		if string(got) != body {
			t.Errorf("restored body = %q, want %q", got, body)
		}
	})

	t.Run("wrong signature is ErrWebhookValidation", func(t *testing.T) {
		req := newWebhookReq(body, map[string]string{"X-Hub-Signature-256": "sha256=" + hmacHexSum(sha256.New, "other", body)})
		err := ValidateWebhookWithRegistry(model.PlatformGitHub, req, secret)
		if !errors.Is(err, provider.ErrWebhookValidation) {
			t.Errorf("err = %v, want errors.Is(provider.ErrWebhookValidation)", err)
		}
	})

	t.Run("empty secret rejected", func(t *testing.T) {
		req := newWebhookReq(body, map[string]string{"X-Hub-Signature-256": sig})
		err := ValidateWebhookWithRegistry(model.PlatformGitHub, req, "")
		if !errors.Is(err, provider.ErrWebhookValidation) {
			t.Errorf("err = %v, want errors.Is(provider.ErrWebhookValidation)", err)
		}
	})

	t.Run("body read failure is ErrWebhookValidation", func(t *testing.T) {
		req := newWebhookReq(body, map[string]string{"X-Hub-Signature-256": sig})
		req.Body = webhookErrBody{}
		err := ValidateWebhookWithRegistry(model.PlatformGitHub, req, secret)
		// 读体失败经 oekiterrors.Wrap 的 kv 面上抛：分类只认哨兵
		// （底层读错误是 kv 属性、不进 cause 链，故不断言消息文本）。
		if !errors.Is(err, provider.ErrWebhookValidation) {
			t.Errorf("err = %v, want errors.Is(provider.ErrWebhookValidation)", err)
		}
	})
}

// TestReadAndRestoreBody_Branches 补 nil body 与读体失败分支（正常/二次读
// 已由 TestReadAndRestoreBody 钉住）。
func TestReadAndRestoreBody_Branches(t *testing.T) {
	t.Run("nil body yields nil slice and nil error", func(t *testing.T) {
		got, err := ReadAndRestoreBody(&http.Request{})
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
		if got != nil {
			t.Errorf("body = %v, want nil", got)
		}
	})

	t.Run("read failure surfaces with cause chain", func(t *testing.T) {
		req := newWebhookReq("x", nil)
		req.Body = webhookErrBody{}
		got, err := ReadAndRestoreBody(req)
		if !errors.Is(err, errWebhookBodyRead) {
			t.Errorf("err = %v, want errors.Is(errWebhookBodyRead)", err)
		}
		if got != nil {
			t.Errorf("body = %v, want nil on read failure", got)
		}
	})

	t.Run("second read after restore returns the same bytes", func(t *testing.T) {
		req := newWebhookReq(`{"a":1}`, nil)
		first, err := ReadAndRestoreBody(req)
		if err != nil {
			t.Fatalf("first read: %v", err)
		}
		second, err := ReadAndRestoreBody(req)
		if err != nil {
			t.Fatalf("second read: %v", err)
		}
		if string(first) != `{"a":1}` || string(second) != string(first) {
			t.Errorf("reads = %q/%q, want both %q", first, second, `{"a":1}`)
		}
	})
}

// TestEnvelopeRead_BodyReadFailure 钉 envelope.Read 的读体失败分支走
// ValidateWebhookSignature op 且 cause 链可达。
func TestEnvelopeRead_BodyReadFailure(t *testing.T) {
	env := WebhookEnvelope{
		Platform:     model.PlatformGitHub,
		EventHeaders: []string{"X-GitHub-Event"},
	}
	req := newWebhookReq(`{}`, nil)
	req.Body = webhookErrBody{}
	_, _, err := env.Read(req, "any-secret")
	if !errors.Is(err, errWebhookBodyRead) {
		t.Errorf("err = %v, want cause chain to errWebhookBodyRead", err)
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want provider.ProviderError", err)
	}
	if pe.Op != "ValidateWebhookSignature" {
		t.Errorf("Op = %q, want %q", pe.Op, "ValidateWebhookSignature")
	}
}
