package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
)

func TestHMACSHA256Validator(t *testing.T) {
	const secret = "super-secret"
	body := []byte(`{"hello":"world"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// A signature forged with an empty key. Without the empty-secret guard
	// this verifies successfully, which lets an attacker spoof predictable
	// payloads (e.g. push events on public repos).
	emptyMac := hmac.New(sha256.New, []byte(""))
	emptyMac.Write(body)
	emptyKeySig := "sha256=" + hex.EncodeToString(emptyMac.Sum(nil))

	tests := []struct {
		name    string
		header  string
		secret  string
		body    []byte
		wantErr bool
	}{
		{"valid", sig, secret, body, false},
		{"valid no prefix", hex.EncodeToString(mac.Sum(nil)), secret, body, false},
		{"missing header", "", secret, body, true},
		{"wrong secret", sig, "other", body, true},
		{"tampered body", sig, secret, []byte(`{"hello":"WORLD"}`), true},
		{"malformed sig", "sha256=not-hex", secret, body, true},
		{"empty secret", sig, "", body, true},
		{"empty secret forgery", emptyKeySig, "", body, true},
	}
	v := HMACSHA256Validator{Header: "X-Hub-Signature-256"}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodPost, "/hook", nil)
			if tc.header != "" {
				r.Header.Set("X-Hub-Signature-256", tc.header)
			}
			err := v.Validate(r, tc.body, tc.secret)
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestStaticTokenValidator(t *testing.T) {
	v := StaticTokenValidator{Header: "X-Gitlab-Token"}
	tests := []struct {
		name    string
		header  string
		secret  string
		wantErr bool
	}{
		{"match", "the-token", "the-token", false},
		{"missing", "", "the-token", true},
		{"mismatch", "nope", "the-token", true},
		{"empty secret", "the-token", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodPost, "/hook", nil)
			if tc.header != "" {
				r.Header.Set("X-Gitlab-Token", tc.header)
			}
			err := v.Validate(r, nil, tc.secret)
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDefaultWebhookRegistry(t *testing.T) {
	// All known platforms should have a default validator registered.
	for _, p := range []model.Platform{
		model.PlatformGitHub, model.PlatformGitea, model.PlatformForgejo, model.PlatformGitee,
		model.PlatformGitCode, model.PlatformTencentCode, model.PlatformGitLab,
	} {
		if v := DefaultWebhookRegistry().Get(p); v == nil {
			t.Errorf("expected default validator for %s", p)
		}
	}
	if v := DefaultWebhookRegistry().Get(model.Platform("unknown")); v != nil {
		t.Error("expected no validator for unknown platform")
	}
}

func TestValidatorFunc_Adaptation(t *testing.T) {
	called := 0
	v := ValidatorFunc{
		N: "test",
		Fn: func(r *http.Request, body []byte, secret string) error {
			called++
			return nil
		},
	}
	r, _ := http.NewRequest(http.MethodPost, "/hook", nil)
	if err := v.Validate(r, nil, ""); err != nil {
		t.Fatal(err)
	}
	if v.Name() != "test" {
		t.Errorf("expected name 'test', got %q", v.Name())
	}
	if called != 1 {
		t.Errorf("expected 1 call, got %d", called)
	}
}

func TestWebhookValidatorRegistry_RegisterAndGet(t *testing.T) {
	r := NewWebhookValidatorRegistry()
	v := HMACSHA256Validator{Header: "X-Foo"}
	r.Register(model.Platform("custom"), v)
	got := r.Get(model.Platform("custom"))
	if got == nil {
		t.Fatal("expected validator after Register")
	}
	if got.Name() != "hmac-sha256" {
		t.Errorf("expected hmac-sha256, got %q", got.Name())
	}
	// Register with nil should be a no-op
	r.Register(model.Platform("custom"), nil)
	if r.Get(model.Platform("custom")) == nil {
		t.Error("nil register should not unregister")
	}
}

func TestReadAndRestoreBody(t *testing.T) {
	r, _ := http.NewRequest(http.MethodPost, "/hook", strings.NewReader(`{"a":1}`))
	body, err := ReadAndRestoreBody(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"a":1}` {
		t.Errorf("expected body to be captured, got %q", body)
	}
	// Body should still be readable after restore
	buf := make([]byte, 8)
	n, _ := r.Body.Read(buf)
	if n == 0 {
		t.Error("expected body to be readable after restore")
	}
}
